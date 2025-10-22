package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ebpf-dependency-tracker/internal/ebpf"
	"github.com/ebpf-dependency-tracker/internal/grpc"
	"github.com/ebpf-dependency-tracker/internal/pathsniff"
)

const (
	defaultOperatorAddress = "localhost:9090"
	defaultNodeName        = "unknown"
	agentVersion          = "v0.1.0"
)

func main() {
	var (
		operatorAddr = flag.String("operator-addr", defaultOperatorAddress, "Address of the operator gRPC server")
		nodeName     = flag.String("node-name", getNodeName(), "Name of the Kubernetes node")
		nodeIP       = flag.String("node-ip", getNodeIP(), "IP address of the Kubernetes node")
	)
	flag.Parse()

	log.Printf("Starting eBPF autoscaler agent v%s", agentVersion)
	log.Printf("Node: %s (%s)", *nodeName, *nodeIP)
	log.Printf("Operator address: %s", *operatorAddr)

	// Create context that will be cancelled on shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle shutdown signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Initialize eBPF dependency tracker
	tracer, err := ebpf.NewDependencyTracker()
	if err != nil {
		log.Fatalf("Failed to create eBPF dependency tracker: %v", err)
	}
	defer tracer.Close()

	// Userspace HTTP path collector on loopback ports 8000-8010 (no app changes)
	ports := []int{8000,8001,8002,8003,8004,8005,8006,8007,8008,8009,8010}
	pc := pathsniff.NewCollector(ports)
	_ = pc.Start(ctx)
	tracer.WithPathCollector(pc)

	log.Println("Dependency tracker initialized successfully")

	// Initialize gRPC client
	client, err := grpc.NewClient(*operatorAddr, &grpc.NodeInfo{
		NodeName:      *nodeName,
		NodeIP:        *nodeIP,
		AgentVersion:  agentVersion,
		KernelVersion: getKernelVersion(),
	})
	if err != nil {
		log.Fatalf("Failed to create gRPC client: %v", err)
	}
	defer client.Close()

	log.Println("gRPC client connected to operator")

	// Start the dependency tracker in a separate goroutine
	go func() {
		if err := tracer.Start(ctx, client); err != nil {
			log.Printf("dependency tracker error: %v", err)
			cancel()
		}
	}()

	// Start health check reporting
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := client.HealthCheck(ctx); err != nil {
					log.Printf("Health check failed: %v", err)
				}
			}
		}
	}()

	log.Println("eBPF agent is running... Press Ctrl+C to stop")

	// Wait for shutdown signal
	select {
	case sig := <-sigCh:
		log.Printf("Received signal %s, shutting down...", sig)
		cancel()
	case <-ctx.Done():
		log.Println("Context cancelled, shutting down...")
	}

	// Give some time for graceful shutdown
	time.Sleep(2 * time.Second)
	log.Println("eBPF agent stopped")
}

// getNodeName returns the node name from environment or hostname
func getNodeName() string {
	if name := os.Getenv("NODE_NAME"); name != "" {
		return name
	}
	
	hostname, err := os.Hostname()
	if err != nil {
		return defaultNodeName
	}
	return hostname
}

// getNodeIP returns the node IP from environment
func getNodeIP() string {
	if ip := os.Getenv("NODE_IP"); ip != "" {
		return ip
	}
	return "unknown"
}

// getKernelVersion returns the kernel version
func getKernelVersion() string {
	data, err := os.ReadFile("/proc/version")
	if err != nil {
		return "unknown"
	}
	
	version := string(data)
	if len(version) > 100 {
		version = version[:100]
	}
	return version
}