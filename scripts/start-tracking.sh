#!/bin/bash

# Start eBPF Dependency Tracking System
echo "🚀 Starting eBPF Dependency Tracking System..."

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
NC='\033[0m'

# Project root
PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$PROJECT_ROOT"

echo -e "${BLUE}📁 Project root: $PROJECT_ROOT${NC}"

# Ensure we have logs directory
mkdir -p logs

# Check if binaries exist
if [ ! -f "bin/operator" ] || [ ! -f "bin/agent" ]; then
    echo -e "${YELLOW}⚠️  Binaries not found. Building...${NC}"
    ./scripts/build.sh
fi

# Start operator first
echo -e "${BLUE}🎯 Starting Operator...${NC}"
sudo nohup ./bin/operator > logs/operator.log 2>&1 &
OPERATOR_PID=$!

# Wait a moment for operator to start
sleep 3

# Check if operator is running
if kill -0 $OPERATOR_PID 2>/dev/null; then
    echo -e "${GREEN}✅ Operator started (PID: $OPERATOR_PID)${NC}"
else
    echo -e "${RED}❌ Operator failed to start${NC}"
    echo -e "${BLUE}📋 Check logs: tail -f logs/operator.log${NC}"
    exit 1
fi

# Start agent
echo -e "${BLUE}🕵️  Starting Agent...${NC}"
sudo nohup ./bin/agent > logs/agent.log 2>&1 &
AGENT_PID=$!

# Wait a moment for agent to start
sleep 3

# Check if agent is running
if kill -0 $AGENT_PID 2>/dev/null; then
    echo -e "${GREEN}✅ Agent started (PID: $AGENT_PID)${NC}"
else
    echo -e "${RED}❌ Agent failed to start${NC}"
    echo -e "${BLUE}📋 Check logs: tail -f logs/agent.log${NC}"
    exit 1
fi

echo -e "${GREEN}🎉 eBPF Tracking System is running!${NC}"
echo ""

# Show endpoints
echo -e "${BLUE}📊 API Endpoints:${NC}"
echo "   • Health Check: http://localhost:8080/health"
echo "   • Dependency Graph: http://localhost:8080/api/dependency-graph"
echo "   • Services: http://localhost:8080/api/services"
echo "   • Metrics: http://localhost:8080/api/metrics"

echo ""
echo -e "${BLUE}🔧 gRPC Endpoint:${NC}"
echo "   • Agent Connection: localhost:9090"

echo ""
echo -e "${BLUE}📋 Process Information:${NC}"
echo "   • Operator PID: $OPERATOR_PID"
echo "   • Agent PID: $AGENT_PID"

echo ""
echo -e "${BLUE}📝 Log Files:${NC}"
echo "   • Operator: logs/operator.log"
echo "   • Agent: logs/agent.log"

echo ""
echo -e "${BLUE}🧪 Quick Test:${NC}"
sleep 2
if curl -s http://localhost:8080/health > /dev/null; then
    echo -e "${GREEN}✅ Operator API responding${NC}"
    curl -s http://localhost:8080/health | python3 -m json.tool 2>/dev/null || echo "Health check successful"
else
    echo -e "${YELLOW}⚠️  Operator API not responding yet (may need more time)${NC}"
fi

echo ""
echo -e "${BLUE}📖 Next Steps:${NC}"
echo "   1. Start microservices: ./scripts/start-microservices.sh"
echo "   2. Generate traffic: ./scripts/generate-traffic.sh"
echo "   3. View logs: tail -f logs/*.log"
echo "   4. Stop system: ./scripts/stop-all.sh"