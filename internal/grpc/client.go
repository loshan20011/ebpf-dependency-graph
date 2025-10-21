package grpc

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "github.com/ebpf-dependency-tracker/pkg/proto"
)

// NodeInfo contains information about the node where the agent is running
type NodeInfo struct {
	NodeName      string
	NodeIP        string
	AgentVersion  string
	KernelVersion string
}

// Client manages gRPC communication with the operator
type Client struct {
	conn     *grpc.ClientConn
	client   pb.MetricsStreamClient
	nodeInfo *pb.NodeInfo
	mutex    sync.RWMutex

	// Streaming clients
	latencyStream    pb.MetricsStream_StreamLatencyMetricsClient
	connectionStream pb.MetricsStream_StreamConnectionEventsClient
	flowStream       pb.MetricsStream_StreamFlowEventsClient
	dependencyStream pb.MetricsStream_StreamDependencyEventsClient
}

// NewClient creates a new gRPC client
func NewClient(operatorAddr string, nodeInfo *NodeInfo) (*Client, error) {
	// Set up gRPC connection with retry
	var conn *grpc.ClientConn
	var err error
	
	// Try to connect with retries
	for i := 0; i < 5; i++ {
		conn, err = grpc.Dial(operatorAddr, 
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithBlock(),
		)
		if err == nil {
			break
		}
		
		log.Printf("Failed to connect to operator (attempt %d/5): %v", i+1, err)
		if i < 4 {
			time.Sleep(time.Duration(i+1) * time.Second)
		}
	}
	
	if err != nil {
		return nil, fmt.Errorf("failed to connect to operator: %w", err)
	}

	client := pb.NewMetricsStreamClient(conn)

	// Convert NodeInfo to protobuf
	pbNodeInfo := &pb.NodeInfo{
		NodeName:      nodeInfo.NodeName,
		NodeIp:        nodeInfo.NodeIP,
		AgentVersion:  nodeInfo.AgentVersion,
		KernelVersion: nodeInfo.KernelVersion,
	}

	grpcClient := &Client{
		conn:     conn,
		client:   client,
		nodeInfo: pbNodeInfo,
	}

	// Initialize streaming connections
	if err := grpcClient.initStreams(context.Background()); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to initialize streams: %w", err)
	}

	return grpcClient, nil
}

// initStreams initializes the streaming connections
func (c *Client) initStreams(ctx context.Context) error {
	// Initialize latency metrics stream (bidirectional)
	latencyStream, err := c.client.StreamLatencyMetrics(ctx)
	if err != nil {
		return fmt.Errorf("failed to create latency stream: %w", err)
	}
	c.latencyStream = latencyStream

	// Initialize connection events stream (bidirectional)
	connectionStream, err := c.client.StreamConnectionEvents(ctx)
	if err != nil {
		return fmt.Errorf("failed to create connection stream: %w", err)
	}
	c.connectionStream = connectionStream

	// Initialize flow events stream
	flowStream, err := c.client.StreamFlowEvents(ctx)
	if err != nil {
		return fmt.Errorf("failed to create flow stream: %w", err)
	}
	c.flowStream = flowStream

	// Initialize dependency events stream
	dependencyStream, err := c.client.StreamDependencyEvents(ctx)
	if err != nil {
		return fmt.Errorf("failed to create dependency stream: %w", err)
	}
	c.dependencyStream = dependencyStream

	log.Println("gRPC streams initialized successfully")
	return nil
}

// SendLatencyMetric sends a latency metric to the operator
func (c *Client) SendLatencyMetric(ctx context.Context, metric *pb.LatencyMetric) error {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	if c.latencyStream == nil {
		return fmt.Errorf("latency stream not available")
	}

	// Send metric to the stream
	if err := c.latencyStream.Send(metric); err != nil {
		return fmt.Errorf("failed to send latency metric: %w", err)
	}
	
	return nil
}

// SendConnectionEvent sends a connection event to the operator
func (c *Client) SendConnectionEvent(ctx context.Context, event *pb.ConnectionEvent) error {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	if c.connectionStream == nil {
		return fmt.Errorf("connection stream not available")
	}

	// Send event to the stream
	if err := c.connectionStream.Send(event); err != nil {
		return fmt.Errorf("failed to send connection event: %w", err)
	}
	
	return nil
}

// SendFlowEvent sends a flow event to the operator
func (c *Client) SendFlowEvent(ctx context.Context, event *pb.FlowEvent) error {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	if c.flowStream == nil {
		return fmt.Errorf("flow stream not available")
	}

	// Send event to the stream
	if err := c.flowStream.Send(event); err != nil {
		return fmt.Errorf("failed to send flow event: %w", err)
	}
	
	return nil
}

// SendDependencyEvent sends a dependency event to the operator
func (c *Client) SendDependencyEvent(ctx context.Context, event *pb.DependencyEvent) error {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	if c.dependencyStream == nil {
		return fmt.Errorf("dependency stream not available")
	}

	// Send event to the stream
	if err := c.dependencyStream.Send(event); err != nil {
		return fmt.Errorf("failed to send dependency event: %w", err)
	}
	
	return nil
}

// HealthCheck performs a health check with the operator
func (c *Client) HealthCheck(ctx context.Context) error {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	request := &pb.HealthCheckRequest{
		Service: "ebpf-agent",
	}

	response, err := c.client.HealthCheck(ctx, request)
	if err != nil {
		return fmt.Errorf("health check failed: %w", err)
	}

	if response.Status != pb.HealthCheckResponse_SERVING {
		return fmt.Errorf("operator not serving, status: %v", response.Status)
	}

	return nil
}

// Close closes the gRPC connection
func (c *Client) Close() error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	// Close streams
	if c.latencyStream != nil {
		c.latencyStream.CloseSend()
		c.latencyStream = nil
	}
	if c.connectionStream != nil {
		c.connectionStream.CloseSend()
		c.connectionStream = nil
	}
	if c.flowStream != nil {
		c.flowStream.CloseSend()
		c.flowStream = nil
	}
	if c.dependencyStream != nil {
		c.dependencyStream.CloseSend()
		c.dependencyStream = nil
	}

	// Close connection
	if c.conn != nil {
		if err := c.conn.Close(); err != nil {
			return fmt.Errorf("failed to close gRPC connection: %w", err)
		}
		c.conn = nil
	}

	log.Println("gRPC client closed")
	return nil
}