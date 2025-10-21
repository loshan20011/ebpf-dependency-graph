#!/bin/bash

# Start Test Microservices for eBPF Dependency Tracking
echo "🚀 Starting test microservices..."

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
NC='\033[0m'

# Project root
PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$PROJECT_ROOT"

# Create examples directory if it doesn't exist
mkdir -p examples/microservices
cd examples/microservices

echo -e "${BLUE}📁 Working in: $(pwd)${NC}"

# Create simple Python microservices for testing
echo -e "${BLUE}🐍 Creating test microservices...${NC}"

# Gateway Service (Port 8000)
cat > gateway.py << 'EOF'
#!/usr/bin/env python3
import time
import requests
from http.server import HTTPServer, BaseHTTPRequestHandler
import threading
import json

class GatewayHandler(BaseHTTPRequestHandler):
    def do_GET(self):
        print(f"Gateway received: {self.path}")
        
        if self.path == '/health':
            self.send_response(200)
            self.send_header('Content-type', 'application/json')
            self.end_headers()
            self.wfile.write(json.dumps({"service": "gateway", "status": "healthy"}).encode())
            return
            
        if self.path.startswith('/api/users'):
            # Forward to user service
            try:
                resp = requests.get('http://localhost:8001/users', timeout=5)
                self.send_response(resp.status_code)
                self.send_header('Content-type', 'application/json')
                self.end_headers()
                self.wfile.write(resp.content)
            except Exception as e:
                self.send_response(500)
                self.end_headers()
                self.wfile.write(f"Error: {e}".encode())
            return
            
        if self.path.startswith('/api/orders'):
            # Forward to order service
            try:
                resp = requests.get('http://localhost:8002/orders', timeout=5)
                self.send_response(resp.status_code)
                self.send_header('Content-type', 'application/json')
                self.end_headers()
                self.wfile.write(resp.content)
            except Exception as e:
                self.send_response(500)
                self.end_headers()
                self.wfile.write(f"Error: {e}".encode())
            return
            
        self.send_response(200)
        self.send_header('Content-type', 'application/json')
        self.end_headers()
        self.wfile.write(json.dumps({"service": "gateway", "message": "Welcome to API Gateway"}).encode())

if __name__ == '__main__':
    server = HTTPServer(('0.0.0.0', 8000), GatewayHandler)
    print("Gateway service starting on port 8000...")
    server.serve_forever()
EOF

# User Service (Port 8001)
cat > user-service.py << 'EOF'
#!/usr/bin/env python3
import json
from http.server import HTTPServer, BaseHTTPRequestHandler
import requests
import time

class UserHandler(BaseHTTPRequestHandler):
    def do_GET(self):
        print(f"User service received: {self.path}")
        
        if self.path == '/health':
            self.send_response(200)
            self.send_header('Content-type', 'application/json')
            self.end_headers()
            self.wfile.write(json.dumps({"service": "user-service", "status": "healthy"}).encode())
            return
            
        if self.path == '/users':
            # Simulate calling notification service
            try:
                requests.get('http://localhost:8003/notify', timeout=2)
            except:
                pass
                
            self.send_response(200)
            self.send_header('Content-type', 'application/json')
            self.end_headers()
            users = [
                {"id": 1, "name": "Alice", "email": "alice@example.com"},
                {"id": 2, "name": "Bob", "email": "bob@example.com"}
            ]
            self.wfile.write(json.dumps(users).encode())
            return
            
        self.send_response(404)
        self.end_headers()

if __name__ == '__main__':
    server = HTTPServer(('0.0.0.0', 8001), UserHandler)
    print("User service starting on port 8001...")
    server.serve_forever()
EOF

# Order Service (Port 8002)
cat > order-service.py << 'EOF'
#!/usr/bin/env python3
import json
from http.server import HTTPServer, BaseHTTPRequestHandler
import requests
import time

class OrderHandler(BaseHTTPRequestHandler):
    def do_GET(self):
        print(f"Order service received: {self.path}")
        
        if self.path == '/health':
            self.send_response(200)
            self.send_header('Content-type', 'application/json')
            self.end_headers()
            self.wfile.write(json.dumps({"service": "order-service", "status": "healthy"}).encode())
            return
            
        if self.path == '/orders':
            # Simulate calling user service and payment service
            try:
                requests.get('http://localhost:8001/users', timeout=2)
                requests.get('http://localhost:8007/payment', timeout=2)
            except:
                pass
                
            self.send_response(200)
            self.send_header('Content-type', 'application/json')
            self.end_headers()
            orders = [
                {"id": 1, "user_id": 1, "amount": 99.99, "status": "completed"},
                {"id": 2, "user_id": 2, "amount": 149.99, "status": "pending"}
            ]
            self.wfile.write(json.dumps(orders).encode())
            return
            
        self.send_response(404)
        self.end_headers()

if __name__ == '__main__':
    server = HTTPServer(('0.0.0.0', 8002), OrderHandler)
    print("Order service starting on port 8002...")
    server.serve_forever()
EOF

# Notification Service (Port 8003)
cat > notification-service.py << 'EOF'
#!/usr/bin/env python3
import json
from http.server import HTTPServer, BaseHTTPRequestHandler

class NotificationHandler(BaseHTTPRequestHandler):
    def do_GET(self):
        print(f"Notification service received: {self.path}")
        
        if self.path == '/health':
            self.send_response(200)
            self.send_header('Content-type', 'application/json')
            self.end_headers()
            self.wfile.write(json.dumps({"service": "notification-service", "status": "healthy"}).encode())
            return
            
        if self.path == '/notify':
            self.send_response(200)
            self.send_header('Content-type', 'application/json')
            self.end_headers()
            response = {"message": "Notification sent", "timestamp": "2023-10-21T10:30:00Z"}
            self.wfile.write(json.dumps(response).encode())
            return
            
        self.send_response(404)
        self.end_headers()

if __name__ == '__main__':
    server = HTTPServer(('0.0.0.0', 8003), NotificationHandler)
    print("Notification service starting on port 8003...")
    server.serve_forever()
EOF

# Payment Service (Port 8007)
cat > payment-service.py << 'EOF'
#!/usr/bin/env python3
import json
from http.server import HTTPServer, BaseHTTPRequestHandler

class PaymentHandler(BaseHTTPRequestHandler):
    def do_GET(self):
        print(f"Payment service received: {self.path}")
        
        if self.path == '/health':
            self.send_response(200)
            self.send_header('Content-type', 'application/json')
            self.end_headers()
            self.wfile.write(json.dumps({"service": "payment-service", "status": "healthy"}).encode())
            return
            
        if self.path == '/payment':
            self.send_response(200)
            self.send_header('Content-type', 'application/json')
            self.end_headers()
            response = {"status": "success", "transaction_id": "txn_123456"}
            self.wfile.write(json.dumps(response).encode())
            return
            
        self.send_response(404)
        self.end_headers()

if __name__ == '__main__':
    server = HTTPServer(('0.0.0.0', 8007), PaymentHandler)
    print("Payment service starting on port 8007...")
    server.serve_forever()
EOF

# Make scripts executable
chmod +x *.py

echo -e "${GREEN}✅ Microservice scripts created${NC}"

# Start services in background
echo -e "${BLUE}🚀 Starting services...${NC}"

# Use virtual environment if available
PYTHON_CMD="python3"
if [ -f "../../.venv/bin/python" ]; then
    PYTHON_CMD="../../.venv/bin/python"
    echo -e "${BLUE}   Using virtual environment${NC}"
fi

# Install requests in case it's missing
$PYTHON_CMD -m pip install requests >/dev/null 2>&1 || true

# Start each service
echo -e "${BLUE}   Starting Gateway (port 8000)...${NC}"
nohup $PYTHON_CMD gateway.py > ../../logs/gateway.log 2>&1 &
sleep 1

echo -e "${BLUE}   Starting User Service (port 8001)...${NC}"
nohup $PYTHON_CMD user-service.py > ../../logs/user-service.log 2>&1 &
sleep 1

echo -e "${BLUE}   Starting Order Service (port 8002)...${NC}"
nohup $PYTHON_CMD order-service.py > ../../logs/order-service.log 2>&1 &
sleep 1

echo -e "${BLUE}   Starting Notification Service (port 8003)...${NC}"
nohup $PYTHON_CMD notification-service.py > ../../logs/notification-service.log 2>&1 &
sleep 1

echo -e "${BLUE}   Starting Payment Service (port 8007)...${NC}"
nohup $PYTHON_CMD payment-service.py > ../../logs/payment-service.log 2>&1 &
sleep 2

echo -e "${GREEN}✅ All microservices started!${NC}"
echo ""

# Test services
echo -e "${BLUE}🧪 Testing services...${NC}"
for port in 8000 8001 8002 8003 8007; do
    if curl -s http://localhost:$port/health >/dev/null; then
        echo -e "${GREEN}   ✅ Port $port: Service responding${NC}"
    else
        echo -e "${YELLOW}   ⚠️  Port $port: Service not responding yet${NC}"
    fi
done

echo ""
echo -e "${BLUE}📊 Service endpoints:${NC}"
echo "   • Gateway:      http://localhost:8000"
echo "   • User Service: http://localhost:8001" 
echo "   • Order Service: http://localhost:8002"
echo "   • Notification: http://localhost:8003"
echo "   • Payment:      http://localhost:8007"

echo ""
echo -e "${BLUE}📋 Test commands:${NC}"
echo "   • Health: curl http://localhost:8000/health"
echo "   • Users:  curl http://localhost:8000/api/users"
echo "   • Orders: curl http://localhost:8000/api/orders"
echo "   • Logs:   tail -f ../../logs/*.log"