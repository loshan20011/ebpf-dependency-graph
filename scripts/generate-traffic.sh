#!/bin/bash

# Generate Test Traffic for eBPF Dependency Tracking
echo "🚦 Generating test traffic..."

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
NC='\033[0m'

# Project root
PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$PROJECT_ROOT"

# Function to make HTTP request and show result
make_request() {
    local url="$1"
    local description="$2"
    
    echo -e "${BLUE}📡 $description${NC}"
    echo -e "${BLUE}   URL: $url${NC}"
    
    response=$(curl -s -w "\n%{http_code}" "$url" 2>/dev/null)
    http_code=$(echo "$response" | tail -n1)
    body=$(echo "$response" | head -n -1)
    
    if [ "$http_code" = "200" ]; then
        echo -e "${GREEN}   ✅ Success ($http_code)${NC}"
        # Pretty print JSON if possible
        echo "$body" | python3 -m json.tool 2>/dev/null || echo "   Response: $body"
    else
        echo -e "${RED}   ❌ Failed ($http_code)${NC}"
        echo "   Response: $body"
    fi
    echo ""
}

# Check if services are running
echo -e "${BLUE}🔍 Checking if services are running...${NC}"

services_running=0
for port in 8000 8001 8002 8003 8004 8005 8006 8007 8008 8009 8010; do
    if curl -s http://localhost:$port/health >/dev/null 2>&1; then
        services_running=$((services_running + 1))
        echo -e "${GREEN}   ✅ Port $port: Running${NC}"
    else
        echo -e "${YELLOW}   ⚠️  Port $port: Not responding${NC}"
    fi
done

if [ $services_running -eq 0 ]; then
    echo -e "${RED}❌ No microservices are running!${NC}"
    echo -e "${BLUE}💡 Start microservices first: ./scripts/start-microservices.sh${NC}"
    exit 1
fi

echo ""
echo -e "${GREEN}✅ Found $services_running running services${NC}"
echo ""

# Wait a moment
sleep 1

# Test individual service health endpoints
echo -e "${BLUE}🏥 Testing health endpoints...${NC}"
make_request "http://localhost:8000/health" "Gateway health check"
make_request "http://localhost:8001/health" "User service health check"
make_request "http://localhost:8002/health" "Order service health check"
make_request "http://localhost:8003/health" "Notification service health check"
make_request "http://localhost:8004/health" "Product service health check"
make_request "http://localhost:8005/health" "Inventory service health check"
make_request "http://localhost:8006/health" "Shipping service health check"
make_request "http://localhost:8007/health" "Payment service health check"
make_request "http://localhost:8008/health" "Auth service health check"
make_request "http://localhost:8009/health" "Catalog service health check"
make_request "http://localhost:8010/health" "Cache service health check"

# Generate inter-service traffic
echo -e "${BLUE}🔄 Generating inter-service traffic...${NC}"

for i in {1..3}; do
    echo -e "${BLUE}   Round $i/3${NC}"
    
    # Gateway -> User Service -> Notification Service
    make_request "http://localhost:8000/api/users" "Gateway calling User Service (triggers notification)"
    sleep 1
    
    # Gateway -> Order Service -> User Service + Payment Service
    make_request "http://localhost:8000/api/orders" "Gateway calling Order Service (triggers user + payment)"
    sleep 1

    # Gateway -> Product Service -> Inventory + Shipping (+ Cache)
    make_request "http://localhost:8000/api/products" "Gateway calling Product Service (triggers inventory + shipping + cache)"
    sleep 1

    # Gateway -> Catalog Service -> Product + User
    make_request "http://localhost:8000/api/catalog" "Gateway calling Catalog Service (aggregates product + user)"
    sleep 1
    
    # Direct service calls to create more dependencies
    make_request "http://localhost:8001/users" "Direct User Service call"
    sleep 0.3
    
    make_request "http://localhost:8002/orders" "Direct Order Service call"
    sleep 0.3
    
    make_request "http://localhost:8003/notify" "Direct Notification Service call"
    sleep 0.3

    make_request "http://localhost:8004/products" "Direct Product Service call"
    sleep 0.3

    make_request "http://localhost:8005/inventory" "Direct Inventory Service call"
    sleep 0.3

    make_request "http://localhost:8006/shipping" "Direct Shipping Service call"
    sleep 0.3
    
    make_request "http://localhost:8007/payment" "Direct Payment Service call"
    sleep 0.3

    make_request "http://localhost:8008/login" "Direct Auth Service call"
    sleep 0.3

    make_request "http://localhost:8009/catalog" "Direct Catalog Service call"
    sleep 0.3

    make_request "http://localhost:8010/cache/ping" "Direct Cache Service call"
    sleep 0.5
    
    echo -e "${BLUE}   Completed round $i${NC}"
    echo ""
done

# Continuous traffic generation (optional)
echo -e "${BLUE}🔄 Generating continuous background traffic for 30 seconds...${NC}"
echo -e "${YELLOW}   This will create ongoing dependencies for eBPF to track${NC}"

# Background traffic generation
generate_background_traffic() {
    local duration=30
    local end_time=$(($(date +%s) + duration))
    
    while [ $(date +%s) -lt $end_time ]; do
        # Random endpoint selection
        case $((RANDOM % 10)) in
            0) curl -s http://localhost:8000/api/users >/dev/null 2>&1 ;;
            1) curl -s http://localhost:8000/api/orders >/dev/null 2>&1 ;;
            2) curl -s http://localhost:8000/api/products >/dev/null 2>&1 ;;
            3) curl -s http://localhost:8000/api/catalog >/dev/null 2>&1 ;;
            4) curl -s http://localhost:8001/users >/dev/null 2>&1 ;;
            5) curl -s http://localhost:8002/orders >/dev/null 2>&1 ;;
            6) curl -s http://localhost:8004/products >/dev/null 2>&1 ;;
            7) curl -s http://localhost:8005/inventory >/dev/null 2>&1 ;;
            8) curl -s http://localhost:8006/shipping >/dev/null 2>&1 ;;
            9) curl -s http://localhost:8008/login >/dev/null 2>&1 ;;
        esac
        
        # Random delay between requests (0.5-2 seconds)
        sleep $(awk 'BEGIN{srand(); print 0.5 + rand() * 1.5}')
    done
}

# Start background traffic
generate_background_traffic &
TRAFFIC_PID=$!

# Show progress
for i in {1..30}; do
    echo -ne "\r${BLUE}   Generating traffic... ${i}/30 seconds${NC}"
    sleep 1
done

# Stop background traffic
kill $TRAFFIC_PID 2>/dev/null || true
wait $TRAFFIC_PID 2>/dev/null || true

echo -e "\n${GREEN}✅ Traffic generation completed!${NC}"
echo ""

# Check eBPF system response
echo -e "${BLUE}🔍 Checking eBPF system response...${NC}"

# Test operator health
if curl -s http://localhost:8080/health >/dev/null 2>&1; then
    echo -e "${GREEN}✅ Operator is responding${NC}"
    make_request "http://localhost:8080/health" "eBPF Operator health"
    
    # Check if we have any metrics
    make_request "http://localhost:8080/api/metrics" "Current metrics"
    make_request "http://localhost:8080/api/dependency-graph" "Dependency graph"
    
else
    echo -e "${RED}❌ Operator is not responding${NC}"
    echo -e "${BLUE}💡 Check operator status: ./scripts/start-tracking.sh${NC}"
fi

echo ""
echo -e "${GREEN}🎉 Traffic generation completed!${NC}"
echo ""
echo -e "${BLUE}📊 Next Steps:${NC}"
echo "   • Check logs: tail -f logs/*.log"
echo "   • View metrics: curl http://localhost:8080/api/metrics | jq"
echo "   • View graph: curl http://localhost:8080/api/dependency-graph | jq"
echo "   • Stop system: ./scripts/stop-all.sh"

echo ""
echo -e "${BLUE}📈 Expected Dependencies:${NC}"
echo "   Gateway (8000) → User Service (8001)"
echo "   Gateway (8000) → Order Service (8002)"
echo "   User Service (8001) → Notification (8003)"
echo "   Order Service (8002) → User Service (8001)"
echo "   Order Service (8002) → Payment (8007)"