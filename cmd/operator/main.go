package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/ebpf-dependency-tracker/internal/graph"
	pb "github.com/ebpf-dependency-tracker/pkg/proto"
)
// EnhancedMetricsStore aggregates metrics and manages dependency graphs
type EnhancedMetricsStore struct {
	mu                   sync.RWMutex
	serviceMetrics       map[string]*ServiceMetrics
	recentLatencyMetrics []*pb.LatencyMetric
	maxRecentMetrics     int
	
	// New graph-based dependency tracking
	graphManager    *graph.GraphManager
	dependencyAnalyzer *graph.DependencyAnalyzer
}

// ServiceMetrics tracks aggregated metrics per service (legacy)
type ServiceMetrics struct {
	ServiceName    string
	TotalRequests  uint64
	AvgLatencyMs   float64
	ErrorCount     uint64
	LastSeen       time.Time
	RequestCounts  map[string]uint64  // Path -> count
}

// NewEnhancedMetricsStore creates a new enhanced metrics store
func NewEnhancedMetricsStore() *EnhancedMetricsStore {
	graphManager := graph.NewGraphManager()
	dependencyAnalyzer := graph.NewDependencyAnalyzer(graphManager)
	
	return &EnhancedMetricsStore{
		serviceMetrics:       make(map[string]*ServiceMetrics),
		recentLatencyMetrics: make([]*pb.LatencyMetric, 0, 100),
		maxRecentMetrics:     100,
		graphManager:         graphManager,
		dependencyAnalyzer:   dependencyAnalyzer,
	}
}

// ProcessFlowEvent processes incoming flow events from eBPF agent
func (ms *EnhancedMetricsStore) ProcessFlowEvent(flowEvent *pb.FlowEvent) {
	if flowEvent == nil {
		return
	}
	
	ms.mu.Lock()
	defer ms.mu.Unlock()
	
	// Process with graph manager
	ms.graphManager.ProcessFlowEvent(flowEvent)
	
	// Run analysis periodically
	go func() {
		ms.dependencyAnalyzer.AnalyzeGraph()
	}()
	
	log.Printf("🌊 Processed flow event: %s:%d -> %s:%d", 
		flowEvent.SourceIp, flowEvent.SourcePort,
		flowEvent.DestIp, flowEvent.DestPort)
}

// ProcessDependencyEvent processes incoming dependency events
func (ms *EnhancedMetricsStore) ProcessDependencyEvent(depEvent *pb.DependencyEvent) {
	if depEvent == nil {
		return
	}
	
	ms.mu.Lock()
	defer ms.mu.Unlock()
	
	// Add dependency to graph
	ms.graphManager.AddDependency(depEvent.Dependency)
	
	// Run analysis
	go func() {
		ms.dependencyAnalyzer.AnalyzeGraph()
	}()
	
	log.Printf("🔗 Processed dependency: %s:%d -> %s:%d (strength: %d)", 
		depEvent.Dependency.Source.Ip, depEvent.Dependency.Source.Port,
		depEvent.Dependency.Dest.Ip, depEvent.Dependency.Dest.Port,
		depEvent.Dependency.RelationshipStrength)
}

// ProcessLatencyMetric processes incoming latency metrics from eBPF agent (legacy)
func (ms *EnhancedMetricsStore) ProcessLatencyMetric(metric *pb.LatencyMetric) {
	if metric == nil {
		return
	}
	
	ms.mu.Lock()
	defer ms.mu.Unlock()

	// Store recent metrics (keep last 100)
	ms.recentLatencyMetrics = append(ms.recentLatencyMetrics, metric)
	if len(ms.recentLatencyMetrics) > ms.maxRecentMetrics {
		ms.recentLatencyMetrics = ms.recentLatencyMetrics[1:]
	}

	// Enrich source service name with PID lookup
	sourceService := getServiceNameFromPID(metric.Pid, metric.ProcessName)
	if sourceService == "" {
		sourceService = extractServiceName(metric.ProcessName, metric.SourceIp)
	}
	
	// Use port information for better service identification
	targetService := portToService(metric.DestPort)
	if targetService == "" {
		targetService = extractServiceName("", metric.DestIp)
	}

	// Update source service metrics
	if sourceService != "" && sourceService != "external" {
		ms.updateServiceMetrics(sourceService, metric.LatencyMs)
	}

	// Update target service metrics
	if targetService != "" {
		ms.updateServiceMetrics(targetService, metric.LatencyMs)
	}

	// Update dependency graph
	ms.updateDependencyGraph(sourceService, targetService, metric.LatencyMs)
}

// ProcessConnectionEvent processes connection events (legacy)
func (ms *EnhancedMetricsStore) ProcessConnectionEvent(event *pb.ConnectionEvent) {
	if event == nil {
		return
	}
	
	ms.mu.Lock()
	defer ms.mu.Unlock()

	// Enrich process name with service name from PID
	sourceService := getServiceNameFromPID(event.Pid, event.ProcessName)
	if sourceService == "" {
		sourceService = event.ProcessName
	}
	if sourceService == "" {
		sourceService = event.SourceIp
	}

	// Target: map port to service or use IP
	targetService := portToService(event.DestPort)
	if targetService == "" {
		targetService = event.DestIp
	}

	// Add to dependency graph using new system (skip loopback to same service)
	if sourceService != "" && targetService != "" && sourceService != targetService {
		// Create a basic flow event from connection event
		flowEvent := &pb.FlowEvent{
			SourceIp:   event.SourceIp,
			DestIp:     event.DestIp,
			SourcePort: event.SourcePort,
			DestPort:   event.DestPort,
			Protocol:   event.Protocol,
			SrcComm:    event.ProcessName,
			Timestamp:  event.Timestamp,
		}
		ms.graphManager.ProcessFlowEvent(flowEvent)
		log.Printf("📊 Edge created: %s → %s", sourceService, targetService)
	}
}

func (ms *EnhancedMetricsStore) updateServiceMetrics(serviceName string, latencyMs float64) {
	if ms.serviceMetrics[serviceName] == nil {
		ms.serviceMetrics[serviceName] = &ServiceMetrics{
			ServiceName:   serviceName,
			TotalRequests: 0,
			AvgLatencyMs:  0,
			ErrorCount:    0,
			LastSeen:      time.Now(),
			RequestCounts: make(map[string]uint64),
		}
	}

	metrics := ms.serviceMetrics[serviceName]
	metrics.TotalRequests++
	metrics.LastSeen = time.Now()

	// Exponential moving average for latency
	alpha := 0.1
	metrics.AvgLatencyMs = alpha*latencyMs + (1-alpha)*metrics.AvgLatencyMs
}

func (ms *EnhancedMetricsStore) updateDependencyGraph(source, target string, latency float64) {
	// Legacy method - now handled by the new graph manager
	// Convert to new system by creating service infos and adding them
	if source != "" && source != "external" {
		sourceService := &pb.ServiceInfo{
			Ip:           "0.0.0.0", // Would need actual IP resolution
			Port:         0,
			ServiceName:  source,
			ProcessName:  source,
			FirstSeen:    uint64(time.Now().UnixNano()),
			LastSeen:     uint64(time.Now().UnixNano()),
		}
		ms.graphManager.AddService(sourceService)
	}
	
	if target != "" && target != "external" {
		targetService := &pb.ServiceInfo{
			Ip:           "0.0.0.0", // Would need actual IP resolution
			Port:         0,
			ServiceName:  target,
			ProcessName:  target,
			FirstSeen:    uint64(time.Now().UnixNano()),
			LastSeen:     uint64(time.Now().UnixNano()),
		}
		ms.graphManager.AddService(targetService)
	}
}

// HTTP API handlers

func (ms *EnhancedMetricsStore) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"service":   "ebpf-operator",
		"status":    "healthy",
		"timestamp": time.Now().Unix(),
	})
}

func (ms *EnhancedMetricsStore) handleDependencyGraph(w http.ResponseWriter, r *http.Request) {
	ms.mu.RLock()
	defer ms.mu.RUnlock()

	// Get the enhanced dependency graph
	depGraph := ms.graphManager.GetGraph()
	serviceStats := ms.dependencyAnalyzer.GetServiceStatistics()
	depStats := ms.dependencyAnalyzer.GetDependencyStatistics()

	// Convert to API-friendly format
	nodes := make([]map[string]interface{}, 0)
	for _, service := range depGraph.Services {
		nodes = append(nodes, map[string]interface{}{
			"id":       service.ID,
			"label":    service.Name,
			"type":     service.Type,
			"ip":       service.IP,
			"port":     service.Port,
			"requests": service.Requests,
			"tags":     service.Tags,
			"metadata": service.Metadata,
		})
	}

	edges := make([]map[string]interface{}, 0)
	for _, dep := range depGraph.Dependencies {
		edges = append(edges, map[string]interface{}{
			"source":               dep.Source.ID,
			"target":               dep.Dest.ID,
			"type":                 "dependency",
			"request_count":        dep.RequestCount,
			"avg_latency_ms":       dep.AverageLatencyMS,
			"error_rate":           dep.ErrorRate,
			"relationship_strength": dep.RelationshipStrength,
			"protocol":             dep.Protocol,
			"tags":                 dep.Tags,
			"http_paths":           dep.HTTPPaths,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"nodes":              nodes,
		"edges":              edges,
		"total_nodes":        len(nodes),
		"total_edges":        len(edges),
		"service_statistics": serviceStats,
		"dependency_stats":   depStats,
		"last_updated":       depGraph.LastUpdated.Unix(),
		"timestamp":          time.Now().Unix(),
	})
}

// handleDependencyGraphEnriched returns enriched dependency graph with service names
func (ms *EnhancedMetricsStore) handleDependencyGraphEnriched(w http.ResponseWriter, r *http.Request, serviceRegistry *graph.ServiceRegistry) {
	ms.mu.RLock()
	defer ms.mu.RUnlock()

	// Get the enhanced dependency graph
	depGraph := ms.graphManager.GetGraph()
	serviceStats := ms.dependencyAnalyzer.GetServiceStatistics()
	depStats := ms.dependencyAnalyzer.GetDependencyStatistics()

	// Convert to API-friendly format with enrichment
	nodes := make([]map[string]interface{}, 0)
	for _, service := range depGraph.Services {
		// Create a graph node for enrichment
		graphNode := &graph.GraphNode{
			ID:       service.ID,
			Label:    service.Name,
			IP:       service.IP,
			Port:     int(service.Port),
			Requests: int(service.Requests),
			Tags:     service.Tags,
			Metadata: service.Metadata,
		}
		
		// Enrich with service registry
		serviceRegistry.EnrichNodeWithServiceName(graphNode)
		
		// Fix IP address if needed
		cleanIP := graph.FixIPAddress(graphNode.IP)
		
		nodes = append(nodes, map[string]interface{}{
			"id":       graphNode.ID,
			"label":    graphNode.Label,
			"type":     service.Type,
			"ip":       cleanIP,
			"port":     graphNode.Port,
			"requests": graphNode.Requests,
			"tags":     graphNode.Tags,
			"metadata": graphNode.Metadata,
		})
	}

	edges := make([]map[string]interface{}, 0)
	for _, dep := range depGraph.Dependencies {
		// Get enriched service names for source and target
		sourceServiceName := serviceRegistry.GetServiceName(dep.Source.IP, uint32(dep.Source.Port))
		targetServiceName := serviceRegistry.GetServiceName(dep.Dest.IP, uint32(dep.Dest.Port))
		
		edges = append(edges, map[string]interface{}{
			"source":               dep.Source.ID,
			"target":               dep.Dest.ID,
			"source_service":       sourceServiceName,
			"target_service":       targetServiceName,
			"type":                 "dependency",
			"request_count":        dep.RequestCount,
			"avg_latency_ms":       dep.AverageLatencyMS,
			"error_rate":           dep.ErrorRate,
			"relationship_strength": dep.RelationshipStrength,
			"protocol":             dep.Protocol,
			"tags":                 dep.Tags,
			"http_paths":           dep.HTTPPaths,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"nodes":              nodes,
		"edges":              edges,
		"total_nodes":        len(nodes),
		"total_edges":        len(edges),
		"service_statistics": serviceStats,
		"dependency_stats":   depStats,
		"last_updated":       depGraph.LastUpdated.Unix(),
		"timestamp":          time.Now().Unix(),
	})
}

func (ms *EnhancedMetricsStore) handleMetrics(w http.ResponseWriter, r *http.Request) {
	ms.mu.RLock()
	defer ms.mu.RUnlock()

	totalRequests := uint64(0)
	totalLatency := 0.0
	serviceCount := 0

	for _, metrics := range ms.serviceMetrics {
		totalRequests += metrics.TotalRequests
		totalLatency += metrics.AvgLatencyMs
		serviceCount++
	}

	avgOverallLatency := 0.0
	if serviceCount > 0 {
		avgOverallLatency = totalLatency / float64(serviceCount)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"total_requests":       totalRequests,
		"avg_overall_latency":  avgOverallLatency,
		"services":             ms.serviceMetrics,
		"timestamp":            time.Now().Unix(),
	})
}

func (ms *EnhancedMetricsStore) handleHTTPMetrics(w http.ResponseWriter, r *http.Request) {
	ms.mu.RLock()
	defer ms.mu.RUnlock()

	// Convert protobuf metrics to JSON-friendly format
	metrics := make([]map[string]interface{}, 0, len(ms.recentLatencyMetrics))
	for _, m := range ms.recentLatencyMetrics {
		metrics = append(metrics, map[string]interface{}{
			"source_ip":      m.SourceIp,
			"dest_ip":        m.DestIp,
			"source_port":    m.SourcePort,
			"dest_port":      m.DestPort,
			"http_path":      m.HttpPath,
			"latency_ms":     m.LatencyMs,
			"process_name":   m.ProcessName,
			"target_service": extractServiceName("", m.DestIp),
			"timestamp":      m.Timestamp,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"metrics":     metrics,
		"total_count": len(ms.recentLatencyMetrics),
		"timestamp":   time.Now().Unix(),
	})
}

// Helper: extract service name from process name or IP
func extractServiceName(processName, ip string) string {
	// Try to extract from process name first
	if processName != "" {
		// Remove "-process" suffix if exists
		if len(processName) > 8 && processName[len(processName)-8:] == "-process" {
			return processName[:len(processName)-8]
		}
		return processName
	}

	// Fallback to IP-based mapping with actual pod IPs
	ipToService := map[string]string{
		// Pod IPs (from kubectl get pods -o wide)
		"10.244.0.3":  "gateway",
		"10.244.0.5":  "gateway", 
		"10.244.0.4":  "user-service",
		"10.244.0.6":  "user-service",
		"10.244.0.7":  "product-service",
		"10.244.0.8":  "product-service",
		"10.244.0.9":  "order-service",
		"10.244.0.10": "order-service",
		"10.244.0.11": "analytics-service",
		"10.244.0.12": "analytics-service",
		// Service IPs (from kubectl get services)
		"10.99.236.135":  "gateway",
		"10.97.243.159":  "user-service",
		"10.111.236.70":  "product-service",
		"10.107.187.176": "order-service",
		"10.108.147.204": "analytics-service",
		// NodePort IP (Minikube IP)
		"192.168.49.2": "minikube-node",
		"127.0.0.1": "localhost",
	}

	if service, ok := ipToService[ip]; ok {
		return service
	}

	return ""
}

// Helper: map port number to service name
func portToService(port uint32) string {
	portMap := map[uint32]string{
		// System services
		80:   "http",
		443:  "https",
		8080: "operator",
		9090: "grpc",
		5000: "visualizer",
		3000: "frontend",
		6379: "redis",
		5432: "postgres",
		// Microservice ports
		8000: "gateway",
		8001: "user-service",
		8002: "product-service",
		8003: "order-service",
		8007: "analytics-service",
		// NodePort mappings (from kubectl get svc)
		31207: "gateway",
		31250: "user-service",
		31992: "product-service",
		31944: "order-service",
		32132: "analytics-service",
	}

	if service, ok := portMap[port]; ok {
		return service
	}
	return ""
}

// getServiceNameFromPID reads /proc/[pid]/cmdline to extract microservice name
func getServiceNameFromPID(pid uint32, fallback string) string {
	if pid == 0 {
		return fallback
	}

	// Read /proc/[pid]/cmdline
	cmdlinePath := fmt.Sprintf("/proc/%d/cmdline", pid)
	data, err := ioutil.ReadFile(cmdlinePath)
	if err != nil {
		// Process may have exited or we don't have permissions
		return fallback
	}

	// cmdline is null-separated, convert to space-separated
	cmdline := string(data)
	cmdline = strings.ReplaceAll(cmdline, "\x00", " ")
	cmdline = strings.TrimSpace(cmdline)

	// Look for patterns like "python /app/svc1.py" or "svc1.py"
	if strings.Contains(cmdline, "/app/svc") {
		// Extract svc number
		parts := strings.Split(cmdline, "/app/")
		if len(parts) > 1 {
			// Get the script name, e.g., "svc1.py"
			scriptParts := strings.Fields(parts[1])
			if len(scriptParts) > 0 {
				scriptName := scriptParts[0]
				// Remove .py extension
				scriptName = strings.TrimSuffix(scriptName, ".py")
				return scriptName
			}
		}
	}

	// Look for service names in typical paths
	if strings.Contains(cmdline, "service") || strings.Contains(cmdline, "svc") {
		// Try to extract from common patterns
		for _, part := range strings.Fields(cmdline) {
			if strings.HasSuffix(part, ".py") {
				// Extract basename
				basename := filepath.Base(part)
				return strings.TrimSuffix(basename, ".py")
			}
		}
	}

	return fallback
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Println("🚀 Starting eBPF Enhanced Microservices Operator")

	// Create enhanced metrics store with dependency graph support
	metricsStore := NewEnhancedMetricsStore()

	// Create gRPC server
	grpcServer := NewMetricsServer()

	// Start metrics processor goroutine
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Process HTTP metrics from gRPC server
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case metric := <-grpcServer.GetLatencyMetricsChannel():
				metricsStore.ProcessLatencyMetric(metric)
			}
		}
	}()

	// Process connection events from gRPC server
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case event := <-grpcServer.GetConnectionEventsChannel():
				metricsStore.ProcessConnectionEvent(event)
			}
		}
	}()

	// Process flow events from gRPC server (new dependency graph system)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case flowEvent := <-grpcServer.GetFlowEventsChannel():
				metricsStore.ProcessFlowEvent(flowEvent)
			}
		}
	}()

	// Process dependency events from gRPC server
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case depEvent := <-grpcServer.GetDependencyEventsChannel():
				metricsStore.ProcessDependencyEvent(depEvent)
			}
		}
	}()

	// Start gRPC server
	grpcAddr := ":9090"
	go func() {
		log.Printf("🔌 Starting gRPC server on %s", grpcAddr)
		if err := grpcServer.Start(ctx, grpcAddr); err != nil {
			log.Printf("❌ gRPC server error: %v", err)
		}
	}()

	// Setup HTTP API server (for visualizer)
	httpAddr := ":8080"
	mux := http.NewServeMux()

	// CORS middleware
	corsHandler := func(h http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			if r.Method == "OPTIONS" {
				w.WriteHeader(http.StatusOK)
				return
			}
			h(w, r)
		}
	}

	// Create service registry for enrichment
	serviceRegistry := graph.NewServiceRegistry()
	go serviceRegistry.DiscoverServices() // Auto-discover services

	// API Routes
	mux.HandleFunc("/health", corsHandler(metricsStore.handleHealth))
	mux.HandleFunc("/api/dependency-graph", corsHandler(func(w http.ResponseWriter, r *http.Request) {
		metricsStore.handleDependencyGraphEnriched(w, r, serviceRegistry)
	}))
	mux.HandleFunc("/api/metrics", corsHandler(metricsStore.handleMetrics))
	mux.HandleFunc("/api/http-metrics", corsHandler(metricsStore.handleHTTPMetrics))
	
	// Dashboard Route
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.ServeFile(w, r, "web/dashboard.html")
		} else {
			http.NotFound(w, r)
		}
	})

	httpServer := &http.Server{
		Addr:    httpAddr,
		Handler: mux,
	}

	// Start HTTP server
	go func() {
		log.Printf("🌐 Starting HTTP API server on %s", httpAddr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("❌ HTTP server error: %v", err)
		}
	}()

	log.Println("✅ Operator started successfully")
	log.Printf("📊 gRPC endpoint: localhost%s (for eBPF agents)", grpcAddr)
	log.Printf("🌐 HTTP API endpoint: http://localhost%s (for visualizer)", httpAddr)

	// Wait for interrupt signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	<-sigCh

	log.Println("🛑 Shutting down operator...")

	// Shutdown HTTP server
	httpCtx, httpCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer httpCancel()
	if err := httpServer.Shutdown(httpCtx); err != nil {
		log.Printf("❌ HTTP server shutdown error: %v", err)
	}

	// Shutdown gRPC server
	grpcServer.Close()

	log.Println("✅ Operator shut down cleanly")
}
