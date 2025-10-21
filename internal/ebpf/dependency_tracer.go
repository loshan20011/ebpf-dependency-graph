package ebpf

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"sync"
	"time"
	"unsafe"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/perf"
	"github.com/cilium/ebpf/rlimit"

	"github.com/ebpf-dependency-tracker/internal/grpc"
	pb "github.com/ebpf-dependency-tracker/pkg/proto"
)

//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -target bpf dependencyTracer dependency_tracer.c -- -I/usr/include -I/usr/include/x86_64-linux-gnu

// Event structures matching the eBPF C code
type FlowKey struct {
	SAddr    uint32
	DAddr    uint32
	SPort    uint16
	DPort    uint16
	Protocol uint8
}

type FlowInfo struct {
	FirstSeen   uint64
	LastSeen    uint64
	BytesSent   uint64
	PacketsSent uint64
	BytesRecv   uint64
	PacketsRecv uint64
	SrcPID      uint32
	DstPID      uint32
	SrcComm     [16]byte
	DstComm     [16]byte
	HTTPMethod  [8]byte
	HTTPPath    [64]byte
	FlowState   uint8
}

type ServiceKey struct {
	IP   uint32
	Port uint16
}

type ServiceInfo struct {
	FirstSeen      uint64
	LastSeen       uint64
	TotalRequests  uint64
	TotalResponses uint64
	ServiceName    [32]byte
	ProcessName    [16]byte
	PID            uint32
	ServiceType    uint8
}

type DependencyKey struct {
	Source ServiceKey
	Dest   ServiceKey
}

type DependencyInfo struct {
	FirstSeen            uint64
	LastSeen             uint64
	RequestCount         uint64
	TotalLatencyNS       uint64
	AvgLatencyNS         uint64
	ErrorCount           uint64
	Protocol             [8]byte
	RelationshipStrength uint8
}

type FlowEventC struct {
	Timestamp uint64
	Key       FlowKey
	Info      FlowInfo
	EventType uint8
}

type DependencyEventC struct {
	Timestamp uint64
	Key       DependencyKey
	Info      DependencyInfo
	EventType uint8
}

// DependencyTracker manages eBPF programs for dependency graph creation
type DependencyTracker struct {
	objs           *dependencyTracerObjects
	links          []link.Link
	flowReader     *perf.Reader
	depReader      *perf.Reader
	services       map[ServiceKey]*pb.ServiceInfo
	dependencies   map[DependencyKey]*pb.DependencyInfo
	servicesMutex  sync.RWMutex
	depMutex       sync.RWMutex
	lastGraphUpdate time.Time
}

// NewDependencyTracker creates a new dependency tracker
func NewDependencyTracker() (*DependencyTracker, error) {
	// Remove memory limit for eBPF
	if err := rlimit.RemoveMemlock(); err != nil {
		return nil, fmt.Errorf("failed to remove memory limit: %w", err)
	}

	return &DependencyTracker{
		services:     make(map[ServiceKey]*pb.ServiceInfo),
		dependencies: make(map[DependencyKey]*pb.DependencyInfo),
	}, nil
}

// Start begins dependency tracking
func (dt *DependencyTracker) Start(ctx context.Context, client *grpc.Client) error {
	// Load eBPF objects
	objs := &dependencyTracerObjects{}
	if err := loadDependencyTracerObjects(objs, nil); err != nil {
		return fmt.Errorf("failed to load eBPF objects: %w", err)
	}
	dt.objs = objs

	log.Println("Dependency tracking eBPF objects loaded successfully")

	// Attach programs
	if err := dt.attachPrograms(); err != nil {
		dt.Close()
		return fmt.Errorf("failed to attach programs: %w", err)
	}

	log.Println("Dependency tracking eBPF programs attached successfully")

	// Set up perf readers
	if err := dt.setupPerfReaders(); err != nil {
		dt.Close()
		return fmt.Errorf("failed to setup perf readers: %w", err)
	}

	log.Println("Dependency tracking perf readers configured")

	// Start event processing goroutines
	errCh := make(chan error, 3)

	// Process flow events
	go func() {
		errCh <- dt.processFlowEvents(ctx, client)
	}()

	// Process dependency events
	go func() {
		errCh <- dt.processDependencyEvents(ctx, client)
	}()

	// Periodically sync graph state
	go func() {
		errCh <- dt.syncGraphState(ctx, client)
	}()

	// Wait for context cancellation or error
	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-errCh:
		return err
	}
}

// Close cleans up all resources
func (dt *DependencyTracker) Close() {
	if dt.flowReader != nil {
		dt.flowReader.Close()
	}
	if dt.depReader != nil {
		dt.depReader.Close()
	}

	for _, l := range dt.links {
		if l != nil {
			l.Close()
		}
	}

	if dt.objs != nil {
		dt.objs.Close()
	}

	log.Println("Dependency tracker closed")
}

// attachPrograms attaches eBPF programs to various kernel hooks
func (dt *DependencyTracker) attachPrograms() error {
	var links []link.Link

	// Try to attach XDP program (may fail on some systems)
	if l, err := dt.attachXDPProgram(); err != nil {
		log.Printf("Warning: Failed to attach XDP program: %v", err)
	} else if l != nil {
		links = append(links, l)
		log.Println("Attached XDP dependency tracker")
	}

	// Try to attach TC program (may require additional setup)
	if l, err := dt.attachTCProgram(); err != nil {
		log.Printf("Warning: Failed to attach TC program: %v", err)
	} else if l != nil {
		links = append(links, l)
		log.Println("Attached TC dependency tracker")
	}

	// Attach tracepoint for enhanced sendto tracking
	l3, err := link.Tracepoint("syscalls", "sys_enter_sendto", dt.objs.TraceSendtoEnhanced, nil)
	if err != nil {
		log.Printf("Warning: Failed to attach sys_enter_sendto tracepoint: %v", err)
	} else {
		links = append(links, l3)
		log.Println("Attached enhanced sendto tracepoint")
	}

	// Try to attach cgroup program (requires cgroup v2)
	if l, err := dt.attachCgroupProgram(); err != nil {
		log.Printf("Warning: Failed to attach cgroup program: %v", err)
	} else if l != nil {
		links = append(links, l)
		log.Println("Attached cgroup dependency tracker")
	}

	dt.links = links

	if len(links) == 0 {
		return fmt.Errorf("failed to attach any eBPF programs")
	}

	return nil
}

// attachXDPProgram attempts to attach XDP program to a network interface
func (dt *DependencyTracker) attachXDPProgram() (link.Link, error) {
	// Try to find a suitable network interface
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	for _, iface := range interfaces {
		// Skip loopback and down interfaces
		if iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagUp == 0 {
			continue
		}

		// Try to attach XDP program
		l, err := link.AttachXDP(link.XDPOptions{
			Program:   dt.objs.XdpDependencyTracker,
			Interface: iface.Index,
		})
		if err != nil {
			log.Printf("Failed to attach XDP to interface %s: %v", iface.Name, err)
			continue
		}

		log.Printf("Attached XDP program to interface %s", iface.Name)
		return l, nil
	}

	return nil, fmt.Errorf("no suitable network interface found for XDP attachment")
}

// attachTCProgram attempts to attach TC program
func (dt *DependencyTracker) attachTCProgram() (link.Link, error) {
	// TC attachment requires more complex setup and root privileges
	// For now, we'll skip it and rely on other attachment points
	return nil, fmt.Errorf("TC attachment not implemented in this demo")
}

// attachCgroupProgram attempts to attach cgroup program
func (dt *DependencyTracker) attachCgroupProgram() (link.Link, error) {
	// Try to find cgroup v2 mount point
	cgroupPath := "/sys/fs/cgroup"
	if _, err := os.Stat(cgroupPath); err != nil {
		return nil, fmt.Errorf("cgroup v2 not found: %w", err)
	}

	// Attach connect4 (CGROUP_SOCK_ADDR)
	l1, err := link.AttachCgroup(link.CgroupOptions{
		Path:    cgroupPath,
		Attach:  ebpf.AttachCGroupInet4Connect,
		Program: dt.objs.CgroupConnectTracker,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to attach cgroup connect4: %w", err)
	}

	// Attach sockops (CGROUP_SOCK_OPS)
	l2, err := link.AttachCgroup(link.CgroupOptions{
		Path:    cgroupPath,
		Attach:  ebpf.AttachCGroupSockOps,
		Program: dt.objs.SockopsTracker,
	})
	if err != nil {
		// if sockops fails, keep connect4
		log.Printf("Warning: failed to attach sockops: %v", err)
		return l1, nil
	}

	// Keep both links; store both
	dt.links = append(dt.links, l1, l2)
	return l1, nil
}

// setupPerfReaders initializes perf event readers
func (dt *DependencyTracker) setupPerfReaders() error {
	// Flow events reader
	flowReader, err := perf.NewReader(dt.objs.FlowEvents, os.Getpagesize()*16)
	if err != nil {
		return fmt.Errorf("failed to create flow events perf reader: %w", err)
	}
	dt.flowReader = flowReader

	// Dependency events reader
	depReader, err := perf.NewReader(dt.objs.DependencyEvents, os.Getpagesize()*16)
	if err != nil {
		return fmt.Errorf("failed to create dependency events perf reader: %w", err)
	}
	dt.depReader = depReader

	return nil
}

// processFlowEvents reads and processes flow events
func (dt *DependencyTracker) processFlowEvents(ctx context.Context, client *grpc.Client) error {
	log.Println("Starting flow events processing")

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		record, err := dt.flowReader.Read()
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			log.Printf("Error reading flow event: %v", err)
			continue
		}

		if record.LostSamples != 0 {
			log.Printf("Lost %d flow samples", record.LostSamples)
			continue
		}

		// Parse the event
		event := (*FlowEventC)(unsafe.Pointer(&record.RawSample[0]))

		// Convert to protobuf and send
		flowEvent := dt.convertFlowEvent(event)
		if err := client.SendFlowEvent(ctx, flowEvent); err != nil {
			log.Printf("Error sending flow event: %v", err)
		}

		// Update local service discovery
		dt.updateServicesFromFlow(event)
	}
}

// processDependencyEvents reads and processes dependency events
func (dt *DependencyTracker) processDependencyEvents(ctx context.Context, client *grpc.Client) error {
	log.Println("Starting dependency events processing")

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		record, err := dt.depReader.Read()
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			log.Printf("Error reading dependency event: %v", err)
			continue
		}

		if record.LostSamples != 0 {
			log.Printf("Lost %d dependency samples", record.LostSamples)
			continue
		}

		// Parse the event
		event := (*DependencyEventC)(unsafe.Pointer(&record.RawSample[0]))

		// Convert to protobuf and send
		depEvent := dt.convertDependencyEvent(event)
		if err := client.SendDependencyEvent(ctx, depEvent); err != nil {
			log.Printf("Error sending dependency event: %v", err)
		}

		// Update local dependency tracking
		dt.updateDependency(event)
	}
}

// syncGraphState periodically syncs the complete graph state
func (dt *DependencyTracker) syncGraphState(ctx context.Context, client *grpc.Client) error {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			// Read current state from eBPF maps and sync if needed
			dt.syncFromEBPFMaps()
		}
	}
}

// Helper functions

func (dt *DependencyTracker) convertFlowEvent(event *FlowEventC) *pb.FlowEvent {
	return &pb.FlowEvent{
		SourceIp:    intToIP(event.Key.SAddr),
		DestIp:      intToIP(event.Key.DAddr),
		SourcePort:  uint32(event.Key.SPort),
		DestPort:    uint32(event.Key.DPort),
		Protocol:    protocolToString(event.Key.Protocol),
		FirstSeen:   event.Info.FirstSeen,
		LastSeen:    event.Info.LastSeen,
		BytesSent:   event.Info.BytesSent,
		PacketsSent: event.Info.PacketsSent,
		BytesRecv:   event.Info.BytesRecv,
		PacketsRecv: event.Info.PacketsRecv,
		SrcPid:      event.Info.SrcPID,
		DstPid:      event.Info.DstPID,
		SrcComm:     cStringToString(event.Info.SrcComm[:]),
		DstComm:     cStringToString(event.Info.DstComm[:]),
		HttpMethod:  cStringToString(event.Info.HTTPMethod[:]),
		HttpPath:    cStringToString(event.Info.HTTPPath[:]),
		FlowState:   uint32(event.Info.FlowState),
		EventType:   uint32(event.EventType),
		Timestamp:   int64(event.Timestamp),
	}
}

func (dt *DependencyTracker) convertDependencyEvent(event *DependencyEventC) *pb.DependencyEvent {
	return &pb.DependencyEvent{
		Dependency: &pb.DependencyInfo{
			Source: &pb.ServiceInfo{
				Ip:   intToIP(event.Key.Source.IP),
				Port: uint32(event.Key.Source.Port),
			},
			Dest: &pb.ServiceInfo{
				Ip:   intToIP(event.Key.Dest.IP),
				Port: uint32(event.Key.Dest.Port),
			},
			FirstSeen:            event.Info.FirstSeen,
			LastSeen:             event.Info.LastSeen,
			RequestCount:         event.Info.RequestCount,
			TotalLatencyNs:       event.Info.TotalLatencyNS,
			AvgLatencyNs:         event.Info.AvgLatencyNS,
			ErrorCount:           event.Info.ErrorCount,
			Protocol:             cStringToString(event.Info.Protocol[:]),
			RelationshipStrength: uint32(event.Info.RelationshipStrength),
		},
		EventType: uint32(event.EventType),
		Timestamp: int64(event.Timestamp),
	}
}

func (dt *DependencyTracker) updateServicesFromFlow(event *FlowEventC) {
	dt.servicesMutex.Lock()
	defer dt.servicesMutex.Unlock()

	// Update source service
	srcKey := ServiceKey{IP: event.Key.SAddr, Port: event.Key.SPort}
	if _, exists := dt.services[srcKey]; !exists {
		dt.services[srcKey] = &pb.ServiceInfo{
			Ip:          intToIP(event.Key.SAddr),
			Port:        uint32(event.Key.SPort),
			ProcessName: cStringToString(event.Info.SrcComm[:]),
			FirstSeen:   event.Info.FirstSeen,
			LastSeen:    event.Info.LastSeen,
		}
	}

	// Update destination service
	dstKey := ServiceKey{IP: event.Key.DAddr, Port: event.Key.DPort}
	if _, exists := dt.services[dstKey]; !exists {
		dt.services[dstKey] = &pb.ServiceInfo{
			Ip:          intToIP(event.Key.DAddr),
			Port:        uint32(event.Key.DPort),
			ProcessName: cStringToString(event.Info.DstComm[:]),
			FirstSeen:   event.Info.FirstSeen,
			LastSeen:    event.Info.LastSeen,
		}
	}
}

func (dt *DependencyTracker) updateDependency(event *DependencyEventC) {
	dt.depMutex.Lock()
	defer dt.depMutex.Unlock()

	depKey := DependencyKey{
		Source: event.Key.Source,
		Dest:   event.Key.Dest,
	}

	dt.dependencies[depKey] = &pb.DependencyInfo{
		Source: &pb.ServiceInfo{
			Ip:   intToIP(event.Key.Source.IP),
			Port: uint32(event.Key.Source.Port),
		},
		Dest: &pb.ServiceInfo{
			Ip:   intToIP(event.Key.Dest.IP),
			Port: uint32(event.Key.Dest.Port),
		},
		FirstSeen:            event.Info.FirstSeen,
		LastSeen:             event.Info.LastSeen,
		RequestCount:         event.Info.RequestCount,
		TotalLatencyNs:       event.Info.TotalLatencyNS,
		AvgLatencyNs:         event.Info.AvgLatencyNS,
		ErrorCount:           event.Info.ErrorCount,
		Protocol:             cStringToString(event.Info.Protocol[:]),
		RelationshipStrength: uint32(event.Info.RelationshipStrength),
	}
}

func (dt *DependencyTracker) syncFromEBPFMaps() {
	// Read services from eBPF service_map
	var key ServiceKey
	var value ServiceInfo
	
	entries := dt.objs.ServiceMap.Iterate()
	dt.servicesMutex.Lock()
	for entries.Next(&key, &value) {
		dt.services[key] = &pb.ServiceInfo{
			Ip:             intToIP(key.IP),
			Port:           uint32(key.Port),
			ServiceName:    cStringToString(value.ServiceName[:]),
			ProcessName:    cStringToString(value.ProcessName[:]),
			Pid:            value.PID,
			FirstSeen:      value.FirstSeen,
			LastSeen:       value.LastSeen,
			TotalRequests:  value.TotalRequests,
			TotalResponses: value.TotalResponses,
			ServiceType:    uint32(value.ServiceType),
		}
	}
	dt.servicesMutex.Unlock()

	if err := entries.Err(); err != nil {
		log.Printf("Error iterating service map: %v", err)
	}

	// Read dependencies from eBPF dependency_map
	var depKey DependencyKey
	var depValue DependencyInfo
	
	depEntries := dt.objs.DependencyMap.Iterate()
	dt.depMutex.Lock()
	for depEntries.Next(&depKey, &depValue) {
		dt.dependencies[depKey] = &pb.DependencyInfo{
			Source: &pb.ServiceInfo{
				Ip:   intToIP(depKey.Source.IP),
				Port: uint32(depKey.Source.Port),
			},
			Dest: &pb.ServiceInfo{
				Ip:   intToIP(depKey.Dest.IP),
				Port: uint32(depKey.Dest.Port),
			},
			FirstSeen:            depValue.FirstSeen,
			LastSeen:             depValue.LastSeen,
			RequestCount:         depValue.RequestCount,
			TotalLatencyNs:       depValue.TotalLatencyNS,
			AvgLatencyNs:         depValue.AvgLatencyNS,
			ErrorCount:           depValue.ErrorCount,
			Protocol:             cStringToString(depValue.Protocol[:]),
			RelationshipStrength: uint32(depValue.RelationshipStrength),
		}
	}
	dt.depMutex.Unlock()

	if err := depEntries.Err(); err != nil {
		log.Printf("Error iterating dependency map: %v", err)
	}

	dt.lastGraphUpdate = time.Now()
}

// GetCurrentGraph returns the current dependency graph
func (dt *DependencyTracker) GetCurrentGraph() *pb.DependencyGraph {
	dt.servicesMutex.RLock()
	dt.depMutex.RLock()
	defer dt.servicesMutex.RUnlock()
	defer dt.depMutex.RUnlock()

	services := make([]*pb.ServiceInfo, 0, len(dt.services))
	for _, svc := range dt.services {
		services = append(services, svc)
	}

	dependencies := make([]*pb.DependencyInfo, 0, len(dt.dependencies))
	for _, dep := range dt.dependencies {
		dependencies = append(dependencies, dep)
	}

	return &pb.DependencyGraph{
		Services:          services,
		Dependencies:      dependencies,
		LastUpdated:       dt.lastGraphUpdate.UnixNano(),
		TotalServices:     uint32(len(services)),
		TotalDependencies: uint32(len(dependencies)),
	}
}

// Helper functions
func intToIP(ip uint32) string {
	return net.IPv4(byte(ip), byte(ip>>8), byte(ip>>16), byte(ip>>24)).String()
}

func cStringToString(data []byte) string {
	for i, b := range data {
		if b == 0 {
			return string(data[:i])
		}
	}
	return string(data)
}

func protocolToString(proto uint8) string {
	switch proto {
	case 6:
		return "TCP"
	case 17:
		return "UDP"
	default:
		return fmt.Sprintf("Unknown(%d)", proto)
	}
}
