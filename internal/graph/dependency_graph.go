package graph

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	pb "github.com/ebpf-dependency-tracker/pkg/proto"
)

// ServiceType represents the type of service
type ServiceType uint32

const (
	ServiceTypeUnknown  ServiceType = 0
	ServiceTypeHTTP     ServiceType = 1
	ServiceTypeDatabase ServiceType = 2
	ServiceTypeCache    ServiceType = 3
)

// Service represents a discovered microservice
type Service struct {
	ID           string            `json:"id"`
	IP           string            `json:"ip"`
	Port         uint32            `json:"port"`
	Name         string            `json:"name"`
	ProcessName  string            `json:"process_name"`
	PID          uint32            `json:"pid"`
	Type         ServiceType       `json:"type"`
	FirstSeen    time.Time         `json:"first_seen"`
	LastSeen     time.Time         `json:"last_seen"`
	Requests     uint64            `json:"total_requests"`
	Responses    uint64            `json:"total_responses"`
	Tags         map[string]string `json:"tags,omitempty"`
	Metadata     map[string]string `json:"metadata,omitempty"`
}

// Dependency represents a relationship between two services
type Dependency struct {
	ID                   string        `json:"id"`
	Source               *Service      `json:"source"`
	Dest                 *Service      `json:"dest"`
	FirstSeen            time.Time     `json:"first_seen"`
	LastSeen             time.Time     `json:"last_seen"`
	RequestCount         uint64        `json:"request_count"`
	TotalLatencyNS       uint64        `json:"total_latency_ns"`
	AverageLatencyMS     float64       `json:"average_latency_ms"`
	ErrorCount           uint64        `json:"error_count"`
	ErrorRate            float64       `json:"error_rate"`
	Protocol             string        `json:"protocol"`
	RelationshipStrength uint32        `json:"relationship_strength"`
	HTTPPaths            []string      `json:"http_paths,omitempty"`
	Tags                 []string      `json:"tags,omitempty"`
}

// DependencyGraph represents the complete service dependency graph
type DependencyGraph struct {
	Services        map[string]*Service    `json:"services"`
	Dependencies    map[string]*Dependency `json:"dependencies"`
	LastUpdated     time.Time              `json:"last_updated"`
	TotalServices   uint32                 `json:"total_services"`
	TotalDependencies uint32               `json:"total_dependencies"`
	mutex           sync.RWMutex
}

// GraphManager manages the dependency graph state
type GraphManager struct {
	graph          *DependencyGraph
	updateCallbacks []func(*DependencyGraph)
	mutex          sync.RWMutex
}

// NewGraphManager creates a new graph manager
func NewGraphManager() *GraphManager {
	return &GraphManager{
		graph: &DependencyGraph{
			Services:     make(map[string]*Service),
			Dependencies: make(map[string]*Dependency),
			LastUpdated:  time.Now(),
		},
		updateCallbacks: make([]func(*DependencyGraph), 0),
	}
}

// AddService adds or updates a service in the graph
func (gm *GraphManager) AddService(serviceInfo *pb.ServiceInfo) *Service {
	gm.mutex.Lock()
	defer gm.mutex.Unlock()

	serviceID := fmt.Sprintf("%s:%d", serviceInfo.Ip, serviceInfo.Port)
	
	service, exists := gm.graph.Services[serviceID]
	if !exists {
		service = &Service{
			ID:          serviceID,
			IP:          serviceInfo.Ip,
			Port:        serviceInfo.Port,
			Name:        serviceInfo.ServiceName,
			ProcessName: serviceInfo.ProcessName,
			PID:         serviceInfo.Pid,
			Type:        ServiceType(serviceInfo.ServiceType),
			FirstSeen:   time.Unix(0, int64(serviceInfo.FirstSeen)),
			Tags:        make(map[string]string),
			Metadata:    make(map[string]string),
		}
		gm.graph.Services[serviceID] = service
		gm.graph.TotalServices++
	}

	// Update service information
	service.LastSeen = time.Unix(0, int64(serviceInfo.LastSeen))
	service.Requests = serviceInfo.TotalRequests
	service.Responses = serviceInfo.TotalResponses
	
	// Infer service name if not provided
	if service.Name == "" {
		service.Name = gm.inferServiceName(service)
	}

	// Add metadata
	service.Metadata["process"] = service.ProcessName
	service.Metadata["pid"] = fmt.Sprintf("%d", service.PID)
	service.Metadata["type"] = gm.serviceTypeToString(service.Type)

	gm.graph.LastUpdated = time.Now()
	gm.notifyCallbacks()

	return service
}

// AddDependency adds or updates a dependency relationship
func (gm *GraphManager) AddDependency(depInfo *pb.DependencyInfo) *Dependency {
	gm.mutex.Lock()
	defer gm.mutex.Unlock()

	// Ensure source and destination services exist
	sourceService := gm.addServiceFromInfo(depInfo.Source)
	destService := gm.addServiceFromInfo(depInfo.Dest)

	depID := fmt.Sprintf("%s->%s", sourceService.ID, destService.ID)
	
	dependency, exists := gm.graph.Dependencies[depID]
	if !exists {
		dependency = &Dependency{
			ID:           depID,
			Source:       sourceService,
			Dest:         destService,
			FirstSeen:    time.Unix(0, int64(depInfo.FirstSeen)),
			Protocol:     depInfo.Protocol,
			HTTPPaths:    make([]string, 0),
			Tags:         make([]string, 0),
		}
		gm.graph.Dependencies[depID] = dependency
		gm.graph.TotalDependencies++
	}

	// Update dependency information
	dependency.LastSeen = time.Unix(0, int64(depInfo.LastSeen))
	dependency.RequestCount = depInfo.RequestCount
	dependency.TotalLatencyNS = depInfo.TotalLatencyNs
	dependency.ErrorCount = depInfo.ErrorCount
	dependency.RelationshipStrength = depInfo.RelationshipStrength

	// Calculate derived metrics
	if dependency.RequestCount > 0 {
		dependency.AverageLatencyMS = float64(dependency.TotalLatencyNS) / float64(dependency.RequestCount) / 1e6
		dependency.ErrorRate = float64(dependency.ErrorCount) / float64(dependency.RequestCount) * 100
	}

	// Add tags based on relationship characteristics
	dependency.Tags = gm.generateDependencyTags(dependency)

	gm.graph.LastUpdated = time.Now()
	gm.notifyCallbacks()

	return dependency
}

// ProcessFlowEvent processes a flow event to update the graph
func (gm *GraphManager) ProcessFlowEvent(flowEvent *pb.FlowEvent) {
	gm.mutex.Lock()
	defer gm.mutex.Unlock()

	// Create or update source service
	sourceID := fmt.Sprintf("%s:%d", flowEvent.SourceIp, flowEvent.SourcePort)
	sourceService := gm.getOrCreateService(sourceID, flowEvent.SourceIp, flowEvent.SourcePort, flowEvent.SrcComm)

	// Create or update destination service
	destID := fmt.Sprintf("%s:%d", flowEvent.DestIp, flowEvent.DestPort)
	destService := gm.getOrCreateService(destID, flowEvent.DestIp, flowEvent.DestPort, flowEvent.DstComm)

	// Update services with flow information
	sourceService.LastSeen = time.Unix(0, flowEvent.Timestamp)
	destService.LastSeen = time.Unix(0, flowEvent.Timestamp)

	// Create or update dependency if it's a client->server relationship
	if gm.isClientServerRelationship(sourceService, destService) {
		depID := fmt.Sprintf("%s->%s", sourceID, destID)
		dependency := gm.getOrCreateDependency(depID, sourceService, destService)
		
		// Update dependency with flow information
		dependency.LastSeen = time.Unix(0, flowEvent.Timestamp)
		if flowEvent.HttpMethod != "" {
			gm.addHTTPPath(dependency, flowEvent.HttpPath, flowEvent.HttpMethod)
		}
	}

	gm.graph.LastUpdated = time.Now()
	gm.notifyCallbacks()
}

// GetGraph returns a copy of the current dependency graph
func (gm *GraphManager) GetGraph() *DependencyGraph {
	gm.mutex.RLock()
	defer gm.mutex.RUnlock()

	// Create a deep copy
	graphCopy := &DependencyGraph{
		Services:          make(map[string]*Service),
		Dependencies:      make(map[string]*Dependency),
		LastUpdated:       gm.graph.LastUpdated,
		TotalServices:     gm.graph.TotalServices,
		TotalDependencies: gm.graph.TotalDependencies,
	}

	// Copy services
	for id, service := range gm.graph.Services {
		serviceCopy := *service
		serviceCopy.Tags = make(map[string]string)
		serviceCopy.Metadata = make(map[string]string)
		for k, v := range service.Tags {
			serviceCopy.Tags[k] = v
		}
		for k, v := range service.Metadata {
			serviceCopy.Metadata[k] = v
		}
		graphCopy.Services[id] = &serviceCopy
	}

	// Copy dependencies
	for id, dep := range gm.graph.Dependencies {
		depCopy := *dep
		depCopy.HTTPPaths = make([]string, len(dep.HTTPPaths))
		copy(depCopy.HTTPPaths, dep.HTTPPaths)
		depCopy.Tags = make([]string, len(dep.Tags))
		copy(depCopy.Tags, dep.Tags)
		
		// Reference the copied services
		depCopy.Source = graphCopy.Services[dep.Source.ID]
		depCopy.Dest = graphCopy.Services[dep.Dest.ID]
		
		graphCopy.Dependencies[id] = &depCopy
	}

	return graphCopy
}

// GetGraphAsProtobuf returns the graph in protobuf format
func (gm *GraphManager) GetGraphAsProtobuf() *pb.DependencyGraph {
	gm.mutex.RLock()
	defer gm.mutex.RUnlock()

	services := make([]*pb.ServiceInfo, 0, len(gm.graph.Services))
	for _, service := range gm.graph.Services {
		services = append(services, &pb.ServiceInfo{
			Ip:             service.IP,
			Port:           service.Port,
			ServiceName:    service.Name,
			ProcessName:    service.ProcessName,
			Pid:            service.PID,
			FirstSeen:      uint64(service.FirstSeen.UnixNano()),
			LastSeen:       uint64(service.LastSeen.UnixNano()),
			TotalRequests:  service.Requests,
			TotalResponses: service.Responses,
			ServiceType:    uint32(service.Type),
		})
	}

	dependencies := make([]*pb.DependencyInfo, 0, len(gm.graph.Dependencies))
	for _, dep := range gm.graph.Dependencies {
		dependencies = append(dependencies, &pb.DependencyInfo{
			Source: &pb.ServiceInfo{
				Ip:   dep.Source.IP,
				Port: dep.Source.Port,
			},
			Dest: &pb.ServiceInfo{
				Ip:   dep.Dest.IP,
				Port: dep.Dest.Port,
			},
			FirstSeen:            uint64(dep.FirstSeen.UnixNano()),
			LastSeen:             uint64(dep.LastSeen.UnixNano()),
			RequestCount:         dep.RequestCount,
			TotalLatencyNs:       dep.TotalLatencyNS,
			AvgLatencyNs:         uint64(dep.AverageLatencyMS * 1e6),
			ErrorCount:           dep.ErrorCount,
			Protocol:             dep.Protocol,
			RelationshipStrength: dep.RelationshipStrength,
		})
	}

	return &pb.DependencyGraph{
		Services:          services,
		Dependencies:      dependencies,
		LastUpdated:       gm.graph.LastUpdated.UnixNano(),
		TotalServices:     gm.graph.TotalServices,
		TotalDependencies: gm.graph.TotalDependencies,
	}
}

// RegisterUpdateCallback registers a callback for graph updates
func (gm *GraphManager) RegisterUpdateCallback(callback func(*DependencyGraph)) {
	gm.mutex.Lock()
	defer gm.mutex.Unlock()
	gm.updateCallbacks = append(gm.updateCallbacks, callback)
}

// GetServicesByType returns services filtered by type
func (gm *GraphManager) GetServicesByType(serviceType ServiceType) []*Service {
	gm.mutex.RLock()
	defer gm.mutex.RUnlock()

	services := make([]*Service, 0)
	for _, service := range gm.graph.Services {
		if service.Type == serviceType {
			services = append(services, service)
		}
	}
	return services
}

// GetDependenciesForService returns all dependencies for a given service
func (gm *GraphManager) GetDependenciesForService(serviceID string) []*Dependency {
	gm.mutex.RLock()
	defer gm.mutex.RUnlock()

	dependencies := make([]*Dependency, 0)
	for _, dep := range gm.graph.Dependencies {
		if dep.Source.ID == serviceID || dep.Dest.ID == serviceID {
			dependencies = append(dependencies, dep)
		}
	}
	return dependencies
}

// ExportToJSON exports the graph as JSON
func (gm *GraphManager) ExportToJSON() ([]byte, error) {
	graph := gm.GetGraph()
	return json.MarshalIndent(graph, "", "  ")
}

// Helper methods

func (gm *GraphManager) addServiceFromInfo(serviceInfo *pb.ServiceInfo) *Service {
	serviceID := fmt.Sprintf("%s:%d", serviceInfo.Ip, serviceInfo.Port)
	
	service, exists := gm.graph.Services[serviceID]
	if !exists {
		service = &Service{
			ID:          serviceID,
			IP:          serviceInfo.Ip,
			Port:        serviceInfo.Port,
			Name:        serviceInfo.ServiceName,
			ProcessName: serviceInfo.ProcessName,
			PID:         serviceInfo.Pid,
			Type:        ServiceType(serviceInfo.ServiceType),
			FirstSeen:   time.Unix(0, int64(serviceInfo.FirstSeen)),
			Tags:        make(map[string]string),
			Metadata:    make(map[string]string),
		}
		gm.graph.Services[serviceID] = service
		gm.graph.TotalServices++
	}

	return service
}

func (gm *GraphManager) getOrCreateService(serviceID, ip string, port uint32, processName string) *Service {
	service, exists := gm.graph.Services[serviceID]
	if !exists {
		service = &Service{
			ID:          serviceID,
			IP:          ip,
			Port:        port,
			ProcessName: processName,
			FirstSeen:   time.Now(),
			Tags:        make(map[string]string),
			Metadata:    make(map[string]string),
		}
		
		// Infer service type from port
		service.Type = gm.inferServiceType(port)
		service.Name = gm.inferServiceName(service)
		
		gm.graph.Services[serviceID] = service
		gm.graph.TotalServices++
	}

	return service
}

func (gm *GraphManager) getOrCreateDependency(depID string, source, dest *Service) *Dependency {
	dependency, exists := gm.graph.Dependencies[depID]
	if !exists {
		dependency = &Dependency{
			ID:           depID,
			Source:       source,
			Dest:         dest,
			FirstSeen:    time.Now(),
			Protocol:     "TCP",
			HTTPPaths:    make([]string, 0),
			Tags:         make([]string, 0),
		}
		gm.graph.Dependencies[depID] = dependency
		gm.graph.TotalDependencies++
	}

	return dependency
}

func (gm *GraphManager) isClientServerRelationship(source, dest *Service) bool {
	// Heuristic: destination service typically has a well-known port
	return dest.Port >= 80 && dest.Port <= 9999
}

func (gm *GraphManager) inferServiceType(port uint32) ServiceType {
	switch {
	case port == 80 || port == 8080 || port == 443 || (port >= 8000 && port <= 8999):
		return ServiceTypeHTTP
	case port == 3306 || port == 5432 || port == 27017:
		return ServiceTypeDatabase
	case port == 6379 || port == 11211:
		return ServiceTypeCache
	default:
		return ServiceTypeUnknown
	}
}

func (gm *GraphManager) inferServiceName(service *Service) string {
	if service.Name != "" {
		return service.Name
	}
	
	if service.ProcessName != "" {
		return fmt.Sprintf("%s-%d", service.ProcessName, service.Port)
	}
	
	return fmt.Sprintf("service-%s-%d", service.IP, service.Port)
}

func (gm *GraphManager) serviceTypeToString(serviceType ServiceType) string {
	switch serviceType {
	case ServiceTypeHTTP:
		return "http"
	case ServiceTypeDatabase:
		return "database"
	case ServiceTypeCache:
		return "cache"
	default:
		return "unknown"
	}
}

func (gm *GraphManager) generateDependencyTags(dep *Dependency) []string {
	tags := make([]string, 0)
	
	// Add relationship strength tag
	switch {
	case dep.RelationshipStrength >= 8:
		tags = append(tags, "critical")
	case dep.RelationshipStrength >= 5:
		tags = append(tags, "important")
	case dep.RelationshipStrength >= 2:
		tags = append(tags, "moderate")
	default:
		tags = append(tags, "weak")
	}

	// Add error rate tag
	if dep.ErrorRate > 5.0 {
		tags = append(tags, "high-error")
	} else if dep.ErrorRate > 1.0 {
		tags = append(tags, "medium-error")
	}

	// Add latency tag
	if dep.AverageLatencyMS > 1000 {
		tags = append(tags, "slow")
	} else if dep.AverageLatencyMS > 100 {
		tags = append(tags, "moderate-latency")
	} else {
		tags = append(tags, "fast")
	}

	return tags
}

func (gm *GraphManager) addHTTPPath(dep *Dependency, path, method string) {
	if path == "" || path == "/" {
		return
	}
	
	fullPath := fmt.Sprintf("%s %s", method, path)
	
	// Check if path already exists
	for _, existingPath := range dep.HTTPPaths {
		if existingPath == fullPath {
			return
		}
	}
	
	// Add new path (limit to 10 paths to prevent memory bloat)
	if len(dep.HTTPPaths) < 10 {
		dep.HTTPPaths = append(dep.HTTPPaths, fullPath)
	}
}

func (gm *GraphManager) notifyCallbacks() {
	for _, callback := range gm.updateCallbacks {
		go callback(gm.graph)
	}
}