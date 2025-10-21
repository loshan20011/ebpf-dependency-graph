package graph

import (
	"fmt"
	"log"
	"sort"
	"strings"
	"time"
)

// DependencyAnalyzer analyzes network flows and builds service dependency graphs
type DependencyAnalyzer struct {
	graphManager *GraphManager
	rules        []AnalysisRule
}

// AnalysisRule defines how to analyze and infer service relationships
type AnalysisRule interface {
	Apply(graph *DependencyGraph) error
	Name() string
}

// ServiceInferenceRule infers service names and types based on network patterns
type ServiceInferenceRule struct{}

func (r *ServiceInferenceRule) Name() string {
	return "ServiceInference"
}

func (r *ServiceInferenceRule) Apply(graph *DependencyGraph) error {
	for _, service := range graph.Services {
		// Infer service names based on common patterns
		if service.Name == "" || strings.HasPrefix(service.Name, "service-") {
			service.Name = r.inferServiceName(service)
		}

		// Enhance service type detection
		if service.Type == ServiceTypeUnknown {
			service.Type = r.inferServiceTypeAdvanced(service, graph)
		}

		// Add service tags based on patterns
		r.addServiceTags(service, graph)
	}
	return nil
}

func (r *ServiceInferenceRule) inferServiceName(service *Service) string {
	// Common microservice port patterns
	portToName := map[uint32]string{
		8000: "gateway",
		8001: "user-service",
		8002: "product-service", 
		8003: "order-service",
		8004: "payment-service",
		8005: "notification-service",
		8006: "analytics-service",
		8007: "inventory-service",
		3000: "frontend",
		5000: "api-server",
		9000: "metrics-server",
	}

	if name, exists := portToName[service.Port]; exists {
		return name
	}

	// Process-based inference
	if service.ProcessName != "" {
		processName := strings.ToLower(service.ProcessName)
		switch {
		case strings.Contains(processName, "gateway"):
			return "api-gateway"
		case strings.Contains(processName, "user"):
			return "user-service"
		case strings.Contains(processName, "product"):
			return "product-service"
		case strings.Contains(processName, "order"):
			return "order-service"
		case strings.Contains(processName, "payment"):
			return "payment-service"
		case strings.Contains(processName, "notification"):
			return "notification-service"
		case strings.Contains(processName, "analytics"):
			return "analytics-service"
		case strings.Contains(processName, "inventory"):
			return "inventory-service"
		case strings.Contains(processName, "nginx"):
			return "reverse-proxy"
		case strings.Contains(processName, "python"):
			return fmt.Sprintf("python-app-%d", service.Port)
		case strings.Contains(processName, "node"):
			return fmt.Sprintf("node-app-%d", service.Port)
		case strings.Contains(processName, "java"):
			return fmt.Sprintf("java-app-%d", service.Port)
		default:
			return fmt.Sprintf("%s-%d", processName, service.Port)
		}
	}

	return fmt.Sprintf("service-%s-%d", service.IP, service.Port)
}

func (r *ServiceInferenceRule) inferServiceTypeAdvanced(service *Service, graph *DependencyGraph) ServiceType {
	// First check port-based inference
	switch {
	case service.Port == 80 || service.Port == 8080 || service.Port == 443 || (service.Port >= 8000 && service.Port <= 8999):
		return ServiceTypeHTTP
	case service.Port == 3306 || service.Port == 5432 || service.Port == 27017:
		return ServiceTypeDatabase
	case service.Port == 6379 || service.Port == 11211:
		return ServiceTypeCache
	}

	// Analyze traffic patterns to infer service type
	incomingDeps := 0
	outgoingDeps := 0
	
	for _, dep := range graph.Dependencies {
		if dep.Dest.ID == service.ID {
			incomingDeps++
		}
		if dep.Source.ID == service.ID {
			outgoingDeps++
		}
	}

	// Services with many incoming connections and few outgoing are likely servers
	if incomingDeps > outgoingDeps*2 && incomingDeps > 2 {
		return ServiceTypeHTTP
	}

	// Services with many outgoing connections might be gateways or clients
	if outgoingDeps > incomingDeps*2 {
		if strings.Contains(strings.ToLower(service.Name), "gateway") {
			return ServiceTypeHTTP
		}
	}

	return ServiceTypeUnknown
}

func (r *ServiceInferenceRule) addServiceTags(service *Service, graph *DependencyGraph) {
	if service.Tags == nil {
		service.Tags = make(map[string]string)
	}

	// Add role tags based on traffic patterns
	incomingCount := 0
	outgoingCount := 0
	
	for _, dep := range graph.Dependencies {
		if dep.Dest.ID == service.ID {
			incomingCount++
		}
		if dep.Source.ID == service.ID {
			outgoingCount++
		}
	}

	if incomingCount > 3 && outgoingCount == 0 {
		service.Tags["role"] = "leaf-service"
	} else if outgoingCount > 3 && incomingCount <= 1 {
		service.Tags["role"] = "gateway"
	} else if incomingCount > 0 && outgoingCount > 0 {
		service.Tags["role"] = "middleware"
	}

	// Add activity level tags
	if service.Requests+service.Responses > 1000 {
		service.Tags["activity"] = "high"
	} else if service.Requests+service.Responses > 100 {
		service.Tags["activity"] = "medium"
	} else {
		service.Tags["activity"] = "low"
	}
}

// DependencyStrengthRule calculates and updates dependency strength scores
type DependencyStrengthRule struct{}

func (r *DependencyStrengthRule) Name() string {
	return "DependencyStrength"
}

func (r *DependencyStrengthRule) Apply(graph *DependencyGraph) error {
	// Find max request count for normalization
	maxRequests := uint64(1)
	for _, dep := range graph.Dependencies {
		if dep.RequestCount > maxRequests {
			maxRequests = dep.RequestCount
		}
	}

	for _, dep := range graph.Dependencies {
		// Calculate strength based on multiple factors
		requestScore := float64(dep.RequestCount) / float64(maxRequests) * 4.0
		
		// Frequency score (recent activity gets higher score)
		timeSinceLastSeen := time.Since(dep.LastSeen)
		frequencyScore := 1.0
		if timeSinceLastSeen < time.Minute {
			frequencyScore = 3.0
		} else if timeSinceLastSeen < time.Hour {
			frequencyScore = 2.0
		} else if timeSinceLastSeen < time.Hour*24 {
			frequencyScore = 1.5
		}

		// Error rate penalty
		errorPenalty := 1.0
		if dep.ErrorRate > 10 {
			errorPenalty = 0.5
		} else if dep.ErrorRate > 5 {
			errorPenalty = 0.7
		}

		// Latency factor
		latencyFactor := 1.0
		if dep.AverageLatencyMS > 1000 {
			latencyFactor = 0.8
		} else if dep.AverageLatencyMS < 100 {
			latencyFactor = 1.2
		}

		finalScore := requestScore * frequencyScore * errorPenalty * latencyFactor
		dep.RelationshipStrength = uint32(min(finalScore, 10))
	}

	return nil
}

// CriticalPathRule identifies critical service paths
type CriticalPathRule struct{}

func (r *CriticalPathRule) Name() string {
	return "CriticalPath"
}

func (r *CriticalPathRule) Apply(graph *DependencyGraph) error {
	// Find services that are on critical paths (many services depend on them)
	dependencyCount := make(map[string]int)
	
	for _, dep := range graph.Dependencies {
		dependencyCount[dep.Dest.ID]++
	}

	// Mark services as critical if many services depend on them
	for serviceID, count := range dependencyCount {
		if service, exists := graph.Services[serviceID]; exists {
			if service.Tags == nil {
				service.Tags = make(map[string]string)
			}
			
			if count >= 3 {
				service.Tags["criticality"] = "critical"
			} else if count >= 2 {
				service.Tags["criticality"] = "important" 
			} else {
				service.Tags["criticality"] = "normal"
			}
		}
	}

	return nil
}

// ServiceTopologyRule analyzes service topology patterns
type ServiceTopologyRule struct{}

func (r *ServiceTopologyRule) Name() string {
	return "ServiceTopology"
}

func (r *ServiceTopologyRule) Apply(graph *DependencyGraph) error {
	// Identify service tiers based on dependency depth
	tiers := r.calculateServiceTiers(graph)
	
	for serviceID, tier := range tiers {
		if service, exists := graph.Services[serviceID]; exists {
			if service.Tags == nil {
				service.Tags = make(map[string]string)
			}
			service.Tags["tier"] = fmt.Sprintf("tier-%d", tier)
			
			// Add architectural pattern tags
			switch tier {
			case 0:
				service.Tags["pattern"] = "entry-point"
			case 1:
				service.Tags["pattern"] = "business-logic"
			default:
				service.Tags["pattern"] = "data-layer"
			}
		}
	}

	return nil
}

func (r *ServiceTopologyRule) calculateServiceTiers(graph *DependencyGraph) map[string]int {
	tiers := make(map[string]int)
	visited := make(map[string]bool)
	
	// Find entry points (services with no incoming dependencies)
	entryPoints := make([]string, 0)
	for serviceID := range graph.Services {
		hasIncoming := false
		for _, dep := range graph.Dependencies {
			if dep.Dest.ID == serviceID {
				hasIncoming = true
				break
			}
		}
		if !hasIncoming {
			entryPoints = append(entryPoints, serviceID)
		}
	}

	// Calculate tiers using BFS from entry points
	queue := make([]string, 0)
	for _, serviceID := range entryPoints {
		tiers[serviceID] = 0
		visited[serviceID] = true
		queue = append(queue, serviceID)
	}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		currentTier := tiers[current]

		// Find all services this service depends on
		for _, dep := range graph.Dependencies {
			if dep.Source.ID == current && !visited[dep.Dest.ID] {
				tiers[dep.Dest.ID] = currentTier + 1
				visited[dep.Dest.ID] = true
				queue = append(queue, dep.Dest.ID)
			}
		}
	}

	return tiers
}

// NewDependencyAnalyzer creates a new dependency analyzer
func NewDependencyAnalyzer(graphManager *GraphManager) *DependencyAnalyzer {
	analyzer := &DependencyAnalyzer{
		graphManager: graphManager,
		rules: []AnalysisRule{
			&ServiceInferenceRule{},
			&DependencyStrengthRule{},
			&CriticalPathRule{},
			&ServiceTopologyRule{},
		},
	}
	return analyzer
}

// AnalyzeGraph runs all analysis rules on the current graph
func (a *DependencyAnalyzer) AnalyzeGraph() error {
	graph := a.graphManager.GetGraph()
	
	for _, rule := range a.rules {
		if err := rule.Apply(graph); err != nil {
			log.Printf("Error applying rule %s: %v", rule.Name(), err)
			continue
		}
		log.Printf("Applied analysis rule: %s", rule.Name())
	}

	return nil
}

// GetServiceStatistics returns comprehensive service statistics
func (a *DependencyAnalyzer) GetServiceStatistics() *ServiceStatistics {
	graph := a.graphManager.GetGraph()
	
	stats := &ServiceStatistics{
		TotalServices:     len(graph.Services),
		TotalDependencies: len(graph.Dependencies),
		ServicesByType:    make(map[ServiceType]int),
		ServicesByTier:    make(map[string]int),
		TopServices:       make([]*ServiceRanking, 0),
	}

	// Count services by type
	for _, service := range graph.Services {
		stats.ServicesByType[service.Type]++
		
		if tier, exists := service.Tags["tier"]; exists {
			stats.ServicesByTier[tier]++
		}
	}

	// Rank services by various criteria
	stats.TopServices = a.rankServices(graph)

	return stats
}

// GetDependencyStatistics returns dependency analysis results
func (a *DependencyAnalyzer) GetDependencyStatistics() *DependencyStatistics {
	graph := a.graphManager.GetGraph()
	
	stats := &DependencyStatistics{
		TotalDependencies: len(graph.Dependencies),
		CriticalPaths:     make([]*DependencyPath, 0),
		HealthMetrics:     make(map[string]float64),
	}

	// Calculate health metrics
	totalLatency := float64(0)
	totalErrors := uint64(0)
	totalRequests := uint64(0)

	for _, dep := range graph.Dependencies {
		totalLatency += dep.AverageLatencyMS
		totalErrors += dep.ErrorCount
		totalRequests += dep.RequestCount
	}

	if len(graph.Dependencies) > 0 {
		stats.HealthMetrics["average_latency"] = totalLatency / float64(len(graph.Dependencies))
		stats.HealthMetrics["total_requests"] = float64(totalRequests)
		stats.HealthMetrics["total_errors"] = float64(totalErrors)
		if totalRequests > 0 {
			stats.HealthMetrics["overall_error_rate"] = float64(totalErrors) / float64(totalRequests) * 100
		}
	}

	return stats
}

// Data structures for statistics

type ServiceStatistics struct {
	TotalServices     int                       `json:"total_services"`
	TotalDependencies int                       `json:"total_dependencies"`
	ServicesByType    map[ServiceType]int       `json:"services_by_type"`
	ServicesByTier    map[string]int            `json:"services_by_tier"`
	TopServices       []*ServiceRanking         `json:"top_services"`
}

type ServiceRanking struct {
	Service     *Service `json:"service"`
	Score       float64  `json:"score"`
	Criteria    string   `json:"criteria"`
}

type DependencyStatistics struct {
	TotalDependencies int                    `json:"total_dependencies"`
	CriticalPaths     []*DependencyPath      `json:"critical_paths"`
	HealthMetrics     map[string]float64     `json:"health_metrics"`
}

type DependencyPath struct {
	Services   []*Service    `json:"services"`
	TotalHops  int           `json:"total_hops"`
	Criticality string       `json:"criticality"`
}

// Helper functions

func (a *DependencyAnalyzer) rankServices(graph *DependencyGraph) []*ServiceRanking {
	rankings := make([]*ServiceRanking, 0)

	// Rank by incoming dependency count
	for _, service := range graph.Services {
		incomingCount := 0
		for _, dep := range graph.Dependencies {
			if dep.Dest.ID == service.ID {
				incomingCount++
			}
		}

		score := float64(incomingCount) + float64(service.Requests+service.Responses)/1000.0
		rankings = append(rankings, &ServiceRanking{
			Service:  service,
			Score:    score,
			Criteria: "importance",
		})
	}

	// Sort by score
	sort.Slice(rankings, func(i, j int) bool {
		return rankings[i].Score > rankings[j].Score
	})

	// Return top 10
	if len(rankings) > 10 {
		rankings = rankings[:10]
	}

	return rankings
}

func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}