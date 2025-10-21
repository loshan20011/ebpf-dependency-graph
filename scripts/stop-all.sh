#!/bin/bash

# Stop All Services - eBPF Dependency Tracker
echo "🛑 Stopping all services..."

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
NC='\033[0m'

# Function to stop process by name
stop_process() {
    local name="$1"
    local pids=$(pgrep -f "$name" 2>/dev/null)
    if [ -n "$pids" ]; then
        echo -e "${BLUE}🛑 Stopping $name (PIDs: $pids)...${NC}"
        sudo pkill -f "$name" 2>/dev/null || true
        sleep 2
        # Force kill if still running
        local remaining=$(pgrep -f "$name" 2>/dev/null)
        if [ -n "$remaining" ]; then
            echo -e "${RED}   Force killing $name...${NC}"
            sudo pkill -9 -f "$name" 2>/dev/null || true
        fi
        echo -e "${GREEN}   ✅ $name stopped${NC}"
    else
        echo -e "${BLUE}   $name not running${NC}"
    fi
}

# Stop eBPF components
echo -e "${BLUE}📡 Stopping eBPF components...${NC}"
stop_process "bin/operator"
stop_process "bin/agent"
stop_process "./operator"
stop_process "./agent"

# Stop test microservices (if running via Python/Docker)
echo -e "${BLUE}🐍 Stopping test microservices...${NC}"
stop_process "python.*gateway"
stop_process "python.*user-service"
stop_process "python.*order-service"
stop_process "python.*payment-service"
stop_process "python.*notification"

# Stop nginx services on microservice ports
echo -e "${BLUE}🌐 Checking nginx services...${NC}"
for port in 8000 8001 8002 8003 8007; do
    pid=$(lsof -ti :$port 2>/dev/null)
    if [ -n "$pid" ]; then
        process_name=$(ps -p $pid -o comm= 2>/dev/null)
        echo -e "${BLUE}   Port $port: $process_name (PID: $pid)${NC}"
        if [[ "$process_name" == *"nginx"* ]]; then
            echo -e "${BLUE}   Stopping nginx on port $port...${NC}"
            sudo kill $pid 2>/dev/null || true
        fi
    fi
done

# Stop any Docker containers with microservice names
echo -e "${BLUE}🐳 Stopping Docker containers...${NC}"
docker stop $(docker ps -q --filter "name=gateway") 2>/dev/null || true
docker stop $(docker ps -q --filter "name=user-service") 2>/dev/null || true
docker stop $(docker ps -q --filter "name=order-service") 2>/dev/null || true
docker stop $(docker ps -q --filter "name=payment-service") 2>/dev/null || true
docker stop $(docker ps -q --filter "name=notification") 2>/dev/null || true

# Stop Kubernetes pods (if any)
echo -e "${BLUE}☸️  Stopping Kubernetes services...${NC}"
kubectl delete deployment gateway user-service order-service payment-service notification-service 2>/dev/null || true

# Clean up any remaining processes
echo -e "${BLUE}🧹 Final cleanup...${NC}"
sudo pkill -f "ebpf" 2>/dev/null || true

echo -e "${GREEN}✅ All services stopped!${NC}"
echo ""
echo -e "${BLUE}📊 Current port status:${NC}"
netstat -tlnp 2>/dev/null | grep -E ":800[0-9]|:900[0-9]" | head -10 || echo "No services on ports 8000-8009 or 9000-9009"