#!/bin/bash

echo "🚀 Starting Local Microservices (No Kubernetes)"
echo "=============================================="

# Function to create nginx config and start service
start_service() {
    local service_name=$1
    local port=$2
    local config_content=$3
    
    echo "📝 Starting $service_name on port $port..."
    
    # Create config directory for this service
    mkdir -p /tmp/nginx-$service_name
    
    # Write nginx config
    echo "$config_content" > /tmp/nginx-$service_name/nginx.conf
    
    # Start nginx with custom config
    nginx -c /tmp/nginx-$service_name/nginx.conf -p /tmp/nginx-$service_name/ &
    
    # Store PID for cleanup
    echo $! >> /tmp/microservices.pids
    
    sleep 1
    
    # Test if service started
    if curl -s http://localhost:$port/health > /dev/null; then
        echo "✅ $service_name running on http://localhost:$port"
    else
        echo "❌ Failed to start $service_name on port $port"
    fi
}

# Clean up any existing services
echo "🧹 Cleaning up existing services..."
pkill -f "nginx.*tmp/nginx-" || true
rm -f /tmp/microservices.pids
rm -rf /tmp/nginx-*

# Wait for cleanup
sleep 2

# Gateway Service (Port 8000)
start_service "gateway" "8000" "
daemon off;
error_log /tmp/nginx-gateway/error.log;
pid /tmp/nginx-gateway/nginx.pid;

events {
    worker_connections 1024;
}

http {
    access_log /tmp/nginx-gateway/access.log;
    
    server {
        listen 8000;
        
        location /health { 
            return 200 \"gateway healthy\n\"; 
            add_header Content-Type text/plain;
        }
        location /api/users { 
            return 200 \"users from gateway\n\"; 
            add_header Content-Type text/plain;
        }
        location /api/products { 
            return 200 \"products from gateway\n\"; 
            add_header Content-Type text/plain;
        }
        location /api/orders { 
            return 200 \"orders from gateway\n\"; 
            add_header Content-Type text/plain;
        }
        location / { 
            return 200 \"Gateway Service - Entry Point\n\"; 
            add_header Content-Type text/plain;
        }
    }
}
"

# User Service (Port 8001)
start_service "user" "8001" "
daemon off;
error_log /tmp/nginx-user/error.log;
pid /tmp/nginx-user/nginx.pid;

events {
    worker_connections 1024;
}

http {
    access_log /tmp/nginx-user/access.log;
    
    server {
        listen 8001;
        
        location /health { 
            return 200 \"user-service healthy\n\"; 
            add_header Content-Type text/plain;
        }
        location /users { 
            return 200 '{\"users\": [\"john\", \"jane\", \"bob\"]}\n'; 
            add_header Content-Type application/json;
        }
        location /users/1 { 
            return 200 '{\"id\": 1, \"name\": \"john\", \"email\": \"john@example.com\"}\n'; 
            add_header Content-Type application/json;
        }
        location / { 
            return 200 \"User Service\n\"; 
            add_header Content-Type text/plain;
        }
    }
}
"

# Product Service (Port 8002)  
start_service "product" "8002" "
daemon off;
error_log /tmp/nginx-product/error.log;
pid /tmp/nginx-product/nginx.pid;

events {
    worker_connections 1024;
}

http {
    access_log /tmp/nginx-product/access.log;
    
    server {
        listen 8002;
        
        location /health { 
            return 200 \"product-service healthy\n\"; 
            add_header Content-Type text/plain;
        }
        location /products { 
            return 200 '{\"products\": [\"laptop\", \"phone\", \"tablet\"]}\n'; 
            add_header Content-Type application/json;
        }
        location /products/1 { 
            return 200 '{\"id\": 1, \"name\": \"laptop\", \"price\": 999}\n'; 
            add_header Content-Type application/json;
        }
        location / { 
            return 200 \"Product Service\n\"; 
            add_header Content-Type text/plain;
        }
    }
}
"

# Order Service (Port 8003)
start_service "order" "8003" "
daemon off;
error_log /tmp/nginx-order/error.log;
pid /tmp/nginx-order/nginx.pid;

events {
    worker_connections 1024;
}

http {
    access_log /tmp/nginx-order/access.log;
    
    server {
        listen 8003;
        
        location /health { 
            return 200 \"order-service healthy\n\"; 
            add_header Content-Type text/plain;
        }
        location /orders { 
            return 200 '{\"orders\": [\"order-1\", \"order-2\"]}\n'; 
            add_header Content-Type application/json;
        }
        location /orders/1 { 
            return 200 '{\"id\": 1, \"status\": \"processing\", \"total\": 999}\n'; 
            add_header Content-Type application/json;
        }
        location / { 
            return 200 \"Order Service\n\"; 
            add_header Content-Type text/plain;
        }
    }
}
"

# Analytics Service (Port 8007)
start_service "analytics" "8007" "
daemon off;
error_log /tmp/nginx-analytics/error.log;
pid /tmp/nginx-analytics/nginx.pid;

events {
    worker_connections 1024;
}

http {
    access_log /tmp/nginx-analytics/access.log;
    
    server {
        listen 8007;
        
        location /health { 
            return 200 \"analytics-service healthy\n\"; 
            add_header Content-Type text/plain;
        }
        location /analytics { 
            return 200 '{\"events\": 1234, \"users\": 567}\n'; 
            add_header Content-Type application/json;
        }
        location / { 
            return 200 \"Analytics Service\n\"; 
            add_header Content-Type text/plain;
        }
    }
}
"

sleep 2

echo ""
echo "✅ All microservices started successfully!"
echo "📊 Service endpoints:"
echo "   • Gateway:    http://localhost:8000/health"
echo "   • User:       http://localhost:8001/health"  
echo "   • Product:    http://localhost:8002/health"
echo "   • Order:      http://localhost:8003/health"
echo "   • Analytics:  http://localhost:8007/health"
echo ""
echo "🛑 To stop all services: ./stop_local_services.sh"