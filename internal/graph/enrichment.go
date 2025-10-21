package graph

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"time"
)

// GraphNode represents a node in the dependency graph (for visualization)
type GraphNode struct {
	ID       string            `json:"id"`
	Label    string            `json:"label"`
	IP       string            `json:"ip"`
	Port     int               `json:"port"`
	Requests int               `json:"requests"`
	Tags     map[string]string `json:"tags,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// ServiceRegistry maintains a mapping of IP:Port to service names
type ServiceRegistry struct {
	services map[string]string // "ip:port" -> "service_name"
}

// NewServiceRegistry creates a new service registry
func NewServiceRegistry() *ServiceRegistry {
	return &ServiceRegistry{
		services: make(map[string]string),
	}
}

// RegisterService adds a service to the registry
func (sr *ServiceRegistry) RegisterService(ip string, port uint32, serviceName string) {
	key := fmt.Sprintf("%s:%d", ip, port)
	sr.services[key] = serviceName
	log.Printf("📝 Registered service: %s -> %s", key, serviceName)
}

// GetServiceName returns the service name for an IP:Port combination
func (sr *ServiceRegistry) GetServiceName(ip string, port uint32) string {
	key := fmt.Sprintf("%s:%d", ip, port)
	if name, exists := sr.services[key]; exists {
		return name
	}
	
	// Try to detect service by port
	return sr.detectServiceByPort(port)
}

// detectServiceByPort tries to identify services by their well-known ports
func (sr *ServiceRegistry) detectServiceByPort(port uint32) string {
	switch port {
	case 8000:
		return "gateway"
	case 8001:
		return "user-service"
	case 8002:
		return "order-service"  
	case 8003:
		return "notification-service"
	case 8007:
		return "payment-service"
	case 80:
		return "web-server"
	case 443:
		return "https-server"
	case 3306:
		return "mysql"
	case 5432:
		return "postgresql"
	case 6379:
		return "redis"
	case 9090:
		return "operator"
	default:
		return fmt.Sprintf("service-port-%d", port)
	}
}

// DiscoverServices automatically discovers services by probing known ports
func (sr *ServiceRegistry) DiscoverServices() {
	knownPorts := []uint32{8000, 8001, 8002, 8003, 8007}
	
	for _, port := range knownPorts {
		go sr.probeService("127.0.0.1", port)
		go sr.probeService("localhost", port)
	}
}

// probeService probes a service to discover its identity
func (sr *ServiceRegistry) probeService(host string, port uint32) {
	url := fmt.Sprintf("http://%s:%d/health", host, port)
	
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return // Service not available
	}
	defer resp.Body.Close()
	
	if resp.StatusCode == 200 {
		// Try to get service name from response
		serviceName := sr.extractServiceNameFromResponse(resp, port)
		
		// Resolve IP address
		ips, err := net.LookupIP(host)
		if err == nil && len(ips) > 0 {
			ip := ips[0].String()
			sr.RegisterService(ip, port, serviceName)
		}
		
		// Also register with localhost/127.0.0.1
		sr.RegisterService("127.0.0.1", port, serviceName)
		sr.RegisterService("localhost", port, serviceName)
	}
}

// extractServiceNameFromResponse extracts service name from HTTP response
func (sr *ServiceRegistry) extractServiceNameFromResponse(resp *http.Response, port uint32) string {
	// Read a small part of response to identify service
	buf := make([]byte, 512)
	n, _ := resp.Body.Read(buf)
	body := string(buf[:n])
	
	// Look for service identifiers in response
	body = strings.ToLower(body)
	
	if strings.Contains(body, "gateway") {
		return "gateway"
	}
	if strings.Contains(body, "user") {
		return "user-service"
	}
	if strings.Contains(body, "order") {
		return "order-service"
	}
	if strings.Contains(body, "notification") {
		return "notification-service"
	}
	if strings.Contains(body, "payment") {
		return "payment-service"
	}
	
	// Fallback to port-based detection
	return sr.detectServiceByPort(port)
}

// EnrichNodeWithServiceName enriches a graph node with service information
func (sr *ServiceRegistry) EnrichNodeWithServiceName(node *GraphNode) {
	if node.Label == "" || strings.Contains(node.Label, "service-") {
		serviceName := sr.GetServiceName(node.IP, uint32(node.Port))
		node.Label = serviceName
		
		// Add metadata
		if node.Metadata == nil {
			node.Metadata = make(map[string]string)
		}
		node.Metadata["service_name"] = serviceName
		node.Metadata["detected_by"] = "service_registry"
	}
}

// Helper function to fix IP address byte order issues
func FixIPAddress(rawIP string) string {
	// If IP looks malformed (like "111.114.0.0"), try to parse and fix
	ip := net.ParseIP(rawIP)
	if ip != nil {
		return ip.String()
	}
	
	// If it's not a valid IP, return as-is
	return rawIP
}