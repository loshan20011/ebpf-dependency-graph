#!/usr/bin/env python3

import requests
import json
import time

def test_manual_dependency_injection():
    """
    Manually test the dependency graph by simulating what the eBPF system should capture
    """
    
    operator_url = "http://localhost:8080"
    
    # Test that operator is running
    try:
        health_response = requests.get(f"{operator_url}/health")
        print(f"✅ Operator health check: {health_response.status_code}")
    except Exception as e:
        print(f"❌ Operator not responding: {e}")
        return False
        
    # Check current dependency graph
    try:
        deps_response = requests.get(f"{operator_url}/api/dependency-graph")
        current_deps = deps_response.json()
        print(f"📊 Current dependency graph:")
        print(f"   Nodes: {current_deps.get('total_nodes', 0)}")
        print(f"   Edges: {current_deps.get('total_edges', 0)}")
        
        if current_deps.get('edges'):
            for edge in current_deps['edges']:
                print(f"   🔗 {edge['source']} → {edge['target']}")
        
        return current_deps
        
    except Exception as e:
        print(f"❌ Failed to get dependency graph: {e}")
        return False

def generate_real_traffic():
    """
    Generate real HTTP traffic to the microservices and see what gets captured
    """
    services = {
        'gateway': 'http://localhost:8000',
        'user-service': 'http://localhost:8001', 
        'product-service': 'http://localhost:8002',
        'order-service': 'http://localhost:8003',
        'analytics-service': 'http://localhost:8007'
    }
    
    print("🚦 Generating real traffic to microservices...")
    
    for service_name, base_url in services.items():
        try:
            # Health check
            response = requests.get(f"{base_url}/health", timeout=2)
            print(f"✅ {service_name}: {response.status_code}")
            
            # Service-specific endpoints
            if service_name == 'gateway':
                requests.get(f"{base_url}/api/users", timeout=2)
                requests.get(f"{base_url}/api/products", timeout=2)
                requests.get(f"{base_url}/api/orders", timeout=2)
            elif service_name == 'user-service':
                requests.get(f"{base_url}/users", timeout=2)
                requests.get(f"{base_url}/users/1", timeout=2)
            elif service_name == 'product-service':
                requests.get(f"{base_url}/products", timeout=2)
                requests.get(f"{base_url}/products/1", timeout=2)
            elif service_name == 'order-service':
                requests.get(f"{base_url}/orders", timeout=2)
                requests.get(f"{base_url}/orders/1", timeout=2)
            elif service_name == 'analytics-service':
                requests.get(f"{base_url}/analytics", timeout=2)
                
        except Exception as e:
            print(f"❌ {service_name}: {e}")
            
    # Wait for eBPF to process events
    print("⏳ Waiting 5 seconds for eBPF processing...")
    time.sleep(5)

if __name__ == "__main__":
    print("🔧 Manual Dependency Graph Testing")
    print("=" * 50)
    
    # Check initial state
    print("\n1. Initial State:")
    initial_deps = test_manual_dependency_injection()
    
    # Generate traffic
    print("\n2. Generating Traffic:")
    generate_real_traffic()
    
    # Check final state
    print("\n3. Final State:")
    final_deps = test_manual_dependency_injection()
    
    print("\n📊 Summary:")
    if initial_deps and final_deps:
        initial_edges = initial_deps.get('total_edges', 0)
        final_edges = final_deps.get('total_edges', 0)
        print(f"   Initial edges: {initial_edges}")
        print(f"   Final edges: {final_edges}")
        print(f"   New edges detected: {final_edges - initial_edges}")
        
        if final_edges > initial_edges:
            print("🎉 SUCCESS: New dependencies detected!")
        else:
            print("⚠️  No new dependencies detected - eBPF capture may need improvement")
    else:
        print("❌ Could not complete test")