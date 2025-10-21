package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"google.golang.org/grpc"

	pb "github.com/ebpf-dependency-tracker/pkg/proto"
)

// MetricsServer implements the MetricsStream gRPC service
type MetricsServer struct {
	pb.UnimplementedMetricsStreamServer

	// Agent management
	agentsMutex sync.RWMutex
	agents      map[string]*AgentInfo

	// Data channels for the controller
	latencyMetricsChan chan *pb.LatencyMetric
	connectionEvtsChan chan *pb.ConnectionEvent
	flowEventsChan     chan *pb.FlowEvent
	dependencyEvtsChan chan *pb.DependencyEvent

	// Statistics
	totalLatencyMetrics uint64
	totalConnectionEvts uint64
	
	// Server instance
	grpcServer *grpc.Server
}

// AgentInfo tracks connected eBPF agents
type AgentInfo struct {
	NodeInfo        *pb.NodeInfo
	ConnectedAt     time.Time
	LastHeartbeat   time.Time
	EventsReceived  uint64
	ActiveStreams   int32
	Status          pb.HealthCheckResponse_ServingStatus
}

// NewMetricsServer creates a new gRPC metrics server
func NewMetricsServer() *MetricsServer {
	return &MetricsServer{
		agents:             make(map[string]*AgentInfo),
		latencyMetricsChan: make(chan *pb.LatencyMetric, 10000),
		connectionEvtsChan: make(chan *pb.ConnectionEvent, 10000),
		flowEventsChan:     make(chan *pb.FlowEvent, 10000),
		dependencyEvtsChan: make(chan *pb.DependencyEvent, 10000),
	}
}

// Start starts the gRPC server on the specified address
func (s *MetricsServer) Start(ctx context.Context, addr string) error {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", addr, err)
	}

	// Create gRPC server with options
	opts := []grpc.ServerOption{
		grpc.MaxRecvMsgSize(4 * 1024 * 1024), // 4MB
		grpc.MaxSendMsgSize(4 * 1024 * 1024), // 4MB
	}

	s.grpcServer = grpc.NewServer(opts...)
	pb.RegisterMetricsStreamServer(s.grpcServer, s)

	log.Printf("🔌 gRPC server listening on %s", addr)

	// Start cleanup routine for stale agents
	go s.cleanupRoutine(ctx)

	// Start serving in a goroutine
	errCh := make(chan error, 1)
	go func() {
		if err := s.grpcServer.Serve(lis); err != nil {
			errCh <- fmt.Errorf("gRPC server serve failed: %w", err)
		}
	}()

	// Wait for context cancellation or server error
	select {
	case <-ctx.Done():
		log.Println("🛑 Shutting down gRPC server...")
		s.grpcServer.GracefulStop()
		return ctx.Err()
	case err := <-errCh:
		return err
	}
}

// RegisterAgent handles agent registration (currently unused, agents connect directly via streaming)
func (s *MetricsServer) registerAgent(nodeInfo *pb.NodeInfo) {
	s.agentsMutex.Lock()
	defer s.agentsMutex.Unlock()

	log.Printf("📋 Agent registration: %s (%s) - %s", 
		nodeInfo.NodeName, nodeInfo.NodeIp, nodeInfo.AgentVersion)

	now := time.Now()
	agent := &AgentInfo{
		NodeInfo:      nodeInfo,
		ConnectedAt:   now,
		LastHeartbeat: now,
		Status:        pb.HealthCheckResponse_SERVING,
	}

	s.agents[nodeInfo.NodeName] = agent

	log.Printf("✅ Agent registered: %s (total active: %d)", 
		nodeInfo.NodeName, len(s.agents))
}

// StreamLatencyMetrics handles streaming latency metrics from agents
func (s *MetricsServer) StreamLatencyMetrics(stream pb.MetricsStream_StreamLatencyMetricsServer) error {
	ctx := stream.Context()
	
	// Get peer information (if available)
	peerInfo := "unknown"
	if p, ok := ctx.Value("peer_info").(string); ok {
		peerInfo = p
	}

	log.Printf("📈 Latency metrics stream started from %s", peerInfo)

	// Increment active streams count
	s.updateAgentActiveStreams(peerInfo, 1)
	defer s.updateAgentActiveStreams(peerInfo, -1)

	for {
		select {
		case <-ctx.Done():
			log.Printf("📉 Latency metrics stream ended from %s", peerInfo)
			return ctx.Err()
		default:
		}

		// Receive metric from agent
		metric, err := stream.Recv()
		if err != nil {
			log.Printf("❌ Error receiving latency metric from %s: %v", peerInfo, err)
			return err
		}

		// Update statistics
		s.totalLatencyMetrics++
		s.updateAgentEventCount(peerInfo, 1)

		// Forward to controller via channel
		select {
		case s.latencyMetricsChan <- metric:
			// Successfully forwarded
		default:
			log.Println("⚠️  Latency metrics channel full, dropping metric")
		}

		// Send acknowledgment
		ack := &pb.Ack{
			Success: true,
			Message: "Metric received",
		}

		if err := stream.Send(ack); err != nil {
			log.Printf("❌ Error sending ack to %s: %v", peerInfo, err)
			return err
		}

		// Log high-latency requests
		if metric.LatencyMs > 100.0 {
			log.Printf("🐌 High latency detected: %s -> %s:%d (%.2fms)",
				metric.SourceIp,
				metric.DestIp,
				metric.DestPort,
				metric.LatencyMs)
		}
	}
}

// StreamConnectionEvents handles streaming connection events from agents
func (s *MetricsServer) StreamConnectionEvents(stream pb.MetricsStream_StreamConnectionEventsServer) error {
	ctx := stream.Context()
	
	peerInfo := "unknown"
	if p, ok := ctx.Value("peer_info").(string); ok {
		peerInfo = p
	}

	log.Printf("🔗 Connection events stream started from %s", peerInfo)

	s.updateAgentActiveStreams(peerInfo, 1)
	defer s.updateAgentActiveStreams(peerInfo, -1)

	for {
		select {
		case <-ctx.Done():
			log.Printf("🔗 Connection events stream ended from %s", peerInfo)
			return ctx.Err()
		default:
		}

		// Receive connection event from agent
		connEvent, err := stream.Recv()
		if err != nil {
			log.Printf("❌ Error receiving connection event from %s: %v", peerInfo, err)
			return err
		}

		// Update statistics
		s.totalConnectionEvts++
		s.updateAgentEventCount(peerInfo, 1)

		// Forward to controller
		select {
		case s.connectionEvtsChan <- connEvent:
			// Successfully forwarded
		default:
			log.Println("⚠️  Connection events channel full, dropping event")
		}

		// Send acknowledgment
		ack := &pb.Ack{
			Success: true,
			Message: "Connection event received",
		}

		if err := stream.Send(ack); err != nil {
			log.Printf("❌ Error sending connection ack to %s: %v", peerInfo, err)
			return err
		}

		log.Printf("🔌 New connection: %s:%d -> %s:%d (%s)",
			connEvent.SourceIp, connEvent.SourcePort,
			connEvent.DestIp, connEvent.DestPort,
			connEvent.ProcessName)
	}
}

// StreamFlowEvents handles streaming flow events from agents
func (s *MetricsServer) StreamFlowEvents(stream pb.MetricsStream_StreamFlowEventsServer) error {
	ctx := stream.Context()
	
	peerInfo := "unknown"
	if p, ok := ctx.Value("peer_info").(string); ok {
		peerInfo = p
	}

	log.Printf("🌊 Flow events stream started from %s", peerInfo)

	s.updateAgentActiveStreams(peerInfo, 1)
	defer s.updateAgentActiveStreams(peerInfo, -1)

	for {
		select {
		case <-ctx.Done():
			log.Printf("🌊 Flow events stream ended from %s", peerInfo)
			return ctx.Err()
		default:
		}

		// Receive flow event from agent
		flowEvent, err := stream.Recv()
		if err != nil {
			log.Printf("❌ Error receiving flow event from %s: %v", peerInfo, err)
			return err
		}

		// Forward to controller
		select {
		case s.flowEventsChan <- flowEvent:
			// Successfully forwarded
		default:
			log.Println("⚠️  Flow events channel full, dropping event")
		}

		// Send acknowledgment
		ack := &pb.Ack{
			Success: true,
			Message: "Flow event received",
		}

		if err := stream.Send(ack); err != nil {
			log.Printf("❌ Error sending flow ack to %s: %v", peerInfo, err)
			return err
		}

		log.Printf("🌊 Flow: %s:%d -> %s:%d (%s, %d bytes)",
			flowEvent.SourceIp, flowEvent.SourcePort,
			flowEvent.DestIp, flowEvent.DestPort,
			flowEvent.Protocol, flowEvent.BytesSent)
	}
}

// StreamDependencyEvents handles streaming dependency events from agents
func (s *MetricsServer) StreamDependencyEvents(stream pb.MetricsStream_StreamDependencyEventsServer) error {
	ctx := stream.Context()
	
	peerInfo := "unknown"
	if p, ok := ctx.Value("peer_info").(string); ok {
		peerInfo = p
	}

	log.Printf("🔗 Dependency events stream started from %s", peerInfo)

	s.updateAgentActiveStreams(peerInfo, 1)
	defer s.updateAgentActiveStreams(peerInfo, -1)

	for {
		select {
		case <-ctx.Done():
			log.Printf("🔗 Dependency events stream ended from %s", peerInfo)
			return ctx.Err()
		default:
		}

		// Receive dependency event from agent
		depEvent, err := stream.Recv()
		if err != nil {
			log.Printf("❌ Error receiving dependency event from %s: %v", peerInfo, err)
			return err
		}

		// Forward to controller
		select {
		case s.dependencyEvtsChan <- depEvent:
			// Successfully forwarded
		default:
			log.Println("⚠️  Dependency events channel full, dropping event")
		}

		// Send acknowledgment
		ack := &pb.Ack{
			Success: true,
			Message: "Dependency event received",
		}

		if err := stream.Send(ack); err != nil {
			log.Printf("❌ Error sending dependency ack to %s: %v", peerInfo, err)
			return err
		}

		log.Printf("🔗 Dependency: %s:%d -> %s:%d (strength: %d)",
			depEvent.Dependency.Source.Ip, depEvent.Dependency.Source.Port,
			depEvent.Dependency.Dest.Ip, depEvent.Dependency.Dest.Port,
			depEvent.Dependency.RelationshipStrength)
	}
}

// HealthCheck handles health check requests
func (s *MetricsServer) HealthCheck(ctx context.Context, req *pb.HealthCheckRequest) (*pb.HealthCheckResponse, error) {
	// Update heartbeat for requesting agent if we can identify it
	peerInfo := "unknown"
	if p, ok := ctx.Value("peer_info").(string); ok {
		peerInfo = p
		s.updateAgentHeartbeat(peerInfo)
	}

	return &pb.HealthCheckResponse{
		Status: pb.HealthCheckResponse_SERVING,
	}, nil
}

// GetLatencyMetricsChannel returns the channel for latency metrics
func (s *MetricsServer) GetLatencyMetricsChannel() <-chan *pb.LatencyMetric {
	return s.latencyMetricsChan
}

// GetConnectionEventsChannel returns the channel for connection events
func (s *MetricsServer) GetConnectionEventsChannel() <-chan *pb.ConnectionEvent {
	return s.connectionEvtsChan
}

// GetFlowEventsChannel returns the channel for flow events
func (s *MetricsServer) GetFlowEventsChannel() <-chan *pb.FlowEvent {
	return s.flowEventsChan
}

// GetDependencyEventsChannel returns the channel for dependency events
func (s *MetricsServer) GetDependencyEventsChannel() <-chan *pb.DependencyEvent {
	return s.dependencyEvtsChan
}

// GetActiveAgents returns information about connected agents
func (s *MetricsServer) GetActiveAgents() map[string]*AgentInfo {
	s.agentsMutex.RLock()
	defer s.agentsMutex.RUnlock()

	agents := make(map[string]*AgentInfo)
	for k, v := range s.agents {
		// Create copy to avoid data races
		agentCopy := *v
		agents[k] = &agentCopy
	}
	return agents
}

// GetStats returns server statistics
func (s *MetricsServer) GetStats() map[string]interface{} {
	s.agentsMutex.RLock()
	defer s.agentsMutex.RUnlock()

	return map[string]interface{}{
		"active_agents":           len(s.agents),
		"total_latency_metrics":   s.totalLatencyMetrics,
		"total_connection_events": s.totalConnectionEvts,
		"uptime":                  time.Since(time.Now()).String(),
	}
}

// Helper methods

func (s *MetricsServer) updateAgentActiveStreams(agentName string, delta int32) {
	s.agentsMutex.Lock()
	defer s.agentsMutex.Unlock()

	if agent, exists := s.agents[agentName]; exists {
		agent.ActiveStreams += delta
		if agent.ActiveStreams < 0 {
			agent.ActiveStreams = 0
		}
	}
}

func (s *MetricsServer) updateAgentEventCount(agentName string, delta uint64) {
	s.agentsMutex.Lock()
	defer s.agentsMutex.Unlock()

	if agent, exists := s.agents[agentName]; exists {
		agent.EventsReceived += delta
		agent.LastHeartbeat = time.Now()
	}
}

func (s *MetricsServer) updateAgentHeartbeat(agentName string) {
	s.agentsMutex.Lock()
	defer s.agentsMutex.Unlock()

	if agent, exists := s.agents[agentName]; exists {
		agent.LastHeartbeat = time.Now()
	}
}

func (s *MetricsServer) getActiveAgentCount() int {
	s.agentsMutex.RLock()
	defer s.agentsMutex.RUnlock()
	return len(s.agents)
}

func (s *MetricsServer) cleanupRoutine(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.cleanupStaleAgents()
		}
	}
}

func (s *MetricsServer) cleanupStaleAgents() {
	s.agentsMutex.Lock()
	defer s.agentsMutex.Unlock()

	staleThreshold := time.Now().Add(-5 * time.Minute)
	var staleAgents []string

	for name, agent := range s.agents {
		if agent.LastHeartbeat.Before(staleThreshold) {
			staleAgents = append(staleAgents, name)
		}
	}

	for _, name := range staleAgents {
		delete(s.agents, name)
		log.Printf("🗑️  Removed stale agent: %s", name)
	}

	if len(staleAgents) > 0 {
		log.Printf("🧹 Cleanup complete, active agents: %d", len(s.agents))
	}
}

func httpMethodToString(method uint32) string {
	switch method {
	case 1:
		return "GET"
	case 2:
		return "POST"
	case 3:
		return "PUT"
	case 4:
		return "DELETE"
	default:
		return "UNKNOWN"
	}
}

// Close gracefully shuts down the server
func (s *MetricsServer) Close() error {
	if s.grpcServer != nil {
		log.Println("🛑 Gracefully stopping gRPC server...")
		s.grpcServer.GracefulStop()
	}

	// Close channels
	close(s.latencyMetricsChan)
	close(s.connectionEvtsChan)
	close(s.flowEventsChan)
	close(s.dependencyEvtsChan)

	log.Printf("✅ gRPC server closed (processed %d latency metrics, %d connection events)",
		s.totalLatencyMetrics, s.totalConnectionEvts)
	return nil
}
