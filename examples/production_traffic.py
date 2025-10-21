#!/usr/bin/env python3
"""
Production Traffic Generator for eBPF Dependency Graph
Creates realistic microservice traffic patterns for dependency discovery
"""
import requests
import time
import random
import threading
from datetime import datetime

# Service endpoints - will be auto-detected for Minikube
import os
import subprocess

def get_minikube_service_url(service_name, namespace='enhanced-microservices'):
    """Get Minikube service URL for NodePort services"""
    try:
        result = subprocess.run(
            ['minikube', 'service', service_name, '-n', namespace, '--url'],
            capture_output=True, text=True, timeout=10
        )
        if result.returncode == 0:
            return result.stdout.strip()
    except Exception:
        pass
    return None

# Try to get Minikube service URLs, fallback to localhost port-forward
SERVICES = {}
service_ports = {
    'gateway': 8000,
    'user-service': 8001, 
    'product-service': 8002,
    'order-service': 8003,
    'analytics-service': 8007
}

for service, port in service_ports.items():
    # Check if running in Minikube environment
    minikube_url = get_minikube_service_url(service)
    if minikube_url:
        SERVICES[service] = minikube_url
        print(f"🎯 Using Minikube service URL for {service}: {minikube_url}")
    else:
        SERVICES[service] = f'http://localhost:{port}'
        print(f"🔗 Using port-forward URL for {service}: http://localhost:{port}")

def call_service(service_name, endpoint, max_retries=3):
    """Make HTTP call to service with retries"""
    base_url = SERVICES.get(service_name)
    if not base_url:
        return None
    
    url = f"{base_url}{endpoint}"
    
    for attempt in range(max_retries):
        try:
            response = requests.get(url, timeout=3)
            if response.status_code == 200:
                print(f"✅ {service_name}{endpoint} -> {response.status_code}")
                return response
            else:
                print(f"⚠️  {service_name}{endpoint} -> {response.status_code}")
                return response
        except Exception as e:
            if attempt < max_retries - 1:
                time.sleep(0.5)
            else:
                print(f"❌ {service_name}{endpoint} -> {e}")
    return None

def pattern_1_basic_flow():
    """Basic e-commerce user flow"""
    print("🔄 Pattern 1: Basic User Flow")
    
    # Gateway -> User Service (user lookup)
    call_service('gateway', '/api/users')
    time.sleep(random.uniform(0.1, 0.3))
    
    # Gateway -> Product Service (product browsing)  
    call_service('gateway', '/api/products')
    time.sleep(random.uniform(0.1, 0.3))
    
    # Gateway -> Order Service (order creation)
    call_service('gateway', '/api/orders')

def pattern_2_detailed_journey():
    """Detailed user journey with multiple services"""
    print("🔄 Pattern 2: Detailed Journey")
    
    # User login and profile
    call_service('user-service', '/users/1')
    time.sleep(random.uniform(0.05, 0.2))
    
    # Product browsing
    call_service('product-service', '/products')
    time.sleep(random.uniform(0.1, 0.2))
    
    call_service('product-service', '/products/1')
    time.sleep(random.uniform(0.05, 0.15))
    
    # Order creation
    call_service('order-service', '/orders')
    time.sleep(random.uniform(0.1, 0.2))
    
    # Analytics tracking
    call_service('analytics-service', '/analytics')

def pattern_3_health_checks():
    """Health check pattern across all services"""
    print("🔄 Pattern 3: Health Checks")
    
    for service in SERVICES.keys():
        call_service(service, '/health')
        time.sleep(random.uniform(0.05, 0.1))

def pattern_4_heavy_load():
    """Heavy load simulation"""
    print("🔄 Pattern 4: Heavy Load")
    
    # Multiple concurrent calls
    threads = []
    services_and_endpoints = [
        ('gateway', '/api/users'),
        ('gateway', '/api/products'), 
        ('gateway', '/api/orders'),
        ('user-service', '/users'),
        ('product-service', '/products'),
        ('order-service', '/orders'),
        ('analytics-service', '/analytics')
    ]
    
    for service, endpoint in services_and_endpoints:
        thread = threading.Thread(target=call_service, args=(service, endpoint))
        threads.append(thread)
        thread.start()
        time.sleep(random.uniform(0.02, 0.05))  # Stagger requests
    
    # Wait for all threads
    for thread in threads:
        thread.join()

def external_calls():
    """Make some external calls to show external dependencies"""
    print("🌐 External Dependencies")
    
    external_urls = [
        'http://httpbin.org/get',
        'http://jsonplaceholder.typicode.com/posts/1',
    ]
    
    for url in external_urls:
        try:
            response = requests.get(url, timeout=3)
            print(f"🌍 External: {url} -> {response.status_code}")
        except Exception as e:
            print(f"❌ External: {url} -> {e}")
        time.sleep(random.uniform(0.1, 0.3))

def continuous_traffic_generation():
    """Generate continuous traffic with different patterns"""
    patterns = [
        pattern_1_basic_flow,
        pattern_2_detailed_journey, 
        pattern_3_health_checks,
        pattern_4_heavy_load
    ]
    
    print("🚀 Starting continuous traffic generation...")
    print("   This will create various dependency patterns for eBPF to capture")
    print()
    
    iteration = 0
    
    try:
        while True:
            iteration += 1
            print(f"\n📊 Iteration {iteration} - {datetime.now().strftime('%H:%M:%S')}")
            print("-" * 50)
            
            # Execute random pattern
            pattern = random.choice(patterns)
            pattern()
            
            # Occasionally make external calls
            if iteration % 3 == 0:
                external_calls()
            
            # Wait between iterations
            wait_time = random.uniform(2, 5)
            print(f"⏱️  Waiting {wait_time:.1f}s for next iteration...")
            time.sleep(wait_time)
            
    except KeyboardInterrupt:
        print("\n\n🛑 Traffic generation stopped")

def run_single_iteration():
    """Run one iteration of all patterns"""
    print("🧪 Single iteration of all traffic patterns")
    print("=" * 50)
    
    # Run each pattern once
    pattern_1_basic_flow()
    time.sleep(1)
    
    pattern_2_detailed_journey()
    time.sleep(1)
    
    pattern_3_health_checks()
    time.sleep(1)
    
    pattern_4_heavy_load()
    time.sleep(1)
    
    external_calls()
    
    print("\n✅ Single iteration complete!")

if __name__ == "__main__":
    import sys
    
    if len(sys.argv) > 1 and sys.argv[1] == "--once":
        run_single_iteration()
    else:
        continuous_traffic_generation()