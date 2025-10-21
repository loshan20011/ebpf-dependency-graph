#!/usr/bin/env python3
"""
Enhanced Dependency Graph Test Script

This script tests the new Cilium Hubble-inspired dependency graph functionality
by simulating various network flow scenarios and validating the graph construction.
"""

import requests
import json
import time
import sys
from typing import Dict, Any

class DependencyGraphTester:
    def __init__(self, operator_url: str = "http://localhost:8080"):
        self.operator_url = operator_url
        self.api_base = f"{operator_url}/api"
    
    def test_health_check(self) -> bool:
        """Test if the operator is running and healthy."""
        try:
            response = requests.get(f"{operator_url}/health", timeout=5)
            if response.status_code == 200:
                data = response.json()
                print(f"✅ Operator health: {data.get('status', 'unknown')}")
                return True
        except Exception as e:
            print(f"❌ Operator health check failed: {e}")
        return False
    
    def get_dependency_graph(self) -> Dict[str, Any]:
        """Fetch the current dependency graph."""
        try:
            response = requests.get(f"{self.api_base}/dependency-graph", timeout=10)
            if response.status_code == 200:
                return response.json()
        except Exception as e:
            print(f"❌ Failed to fetch dependency graph: {e}")
        return {}
    
    def print_dependency_graph(self, graph: Dict[str, Any]):
        """Print the dependency graph in a readable format."""
        print("\n🌐 Current Dependency Graph:")
        print("=" * 50)
        
        # Print statistics
        total_nodes = graph.get('total_nodes', 0)
        total_edges = graph.get('total_edges', 0)
        print(f"📊 Statistics: {total_nodes} services, {total_edges} dependencies")
        
        # Print service statistics if available
        service_stats = graph.get('service_statistics', {})
        if service_stats:
            print(f"📈 Service Stats: {service_stats.get('total_services', 0)} total services")
            services_by_type = service_stats.get('services_by_type', {})
            if services_by_type:
                print("   Service types:")
                for stype, count in services_by_type.items():
                    type_name = {
                        "0": "unknown", "1": "http", "2": "database", "3": "cache"
                    }.get(str(stype), f"type-{stype}")
                    print(f"     - {type_name}: {count}")
        
        # Print dependency statistics
        dep_stats = graph.get('dependency_stats', {})
        if dep_stats:
            health_metrics = dep_stats.get('health_metrics', {})
            if health_metrics:
                print("💊 Health Metrics:")
                avg_latency = health_metrics.get('average_latency', 0)
                error_rate = health_metrics.get('overall_error_rate', 0)
                total_requests = health_metrics.get('total_requests', 0)
                print(f"     - Average Latency: {avg_latency:.2f}ms")
                print(f"     - Error Rate: {error_rate:.2f}%")
                print(f"     - Total Requests: {int(total_requests)}")
        
        print("\n🏗️ Services (Nodes):")
        nodes = graph.get('nodes', [])
        for node in nodes:
            service_id = node.get('id', 'unknown')
            service_name = node.get('label', service_id)
            service_type = node.get('type', 'unknown')
            ip = node.get('ip', 'unknown')
            port = node.get('port', 'unknown')
            requests = node.get('requests', 0)
            tags = node.get('tags', {})
            
            print(f"  📦 {service_name} ({service_id})")
            print(f"      - Address: {ip}:{port}")
            print(f"      - Type: {service_type}")
            print(f"      - Requests: {requests}")
            if tags:
                print(f"      - Tags: {tags}")
        
        print("\n🔗 Dependencies (Edges):")
        edges = graph.get('edges', [])
        for edge in edges:
            source = edge.get('source', 'unknown')
            target = edge.get('target', 'unknown')
            request_count = edge.get('request_count', 0)
            avg_latency = edge.get('avg_latency_ms', 0)
            error_rate = edge.get('error_rate', 0)
            strength = edge.get('relationship_strength', 0)
            protocol = edge.get('protocol', 'unknown')
            tags = edge.get('tags', [])
            http_paths = edge.get('http_paths', [])
            
            print(f"  🔗 {source} → {target}")
            print(f"      - Requests: {request_count}")
            print(f"      - Avg Latency: {avg_latency:.2f}ms")
            print(f"      - Error Rate: {error_rate:.2f}%")
            print(f"      - Strength: {strength}/10")
            print(f"      - Protocol: {protocol}")
            if tags:
                print(f"      - Tags: {tags}")
            if http_paths:
                print(f"      - HTTP Paths: {http_paths}")
        
        if graph.get('last_updated'):
            last_updated = graph.get('last_updated', 0)
            print(f"\n⏰ Last Updated: {time.ctime(last_updated)}")
    
    def analyze_graph_quality(self, graph: Dict[str, Any]) -> Dict[str, Any]:
        """Analyze the quality of the dependency graph."""
        nodes = graph.get('nodes', [])
        edges = graph.get('edges', [])
        
        analysis = {
            'node_count': len(nodes),
            'edge_count': len(edges),
            'connectivity_ratio': len(edges) / max(len(nodes), 1),
            'services_with_names': 0,
            'services_with_tags': 0,
            'dependencies_with_metrics': 0,
            'high_strength_dependencies': 0,
            'error_prone_dependencies': 0,
        }
        
        # Analyze nodes
        for node in nodes:
            if node.get('label') and node.get('label') != node.get('id'):
                analysis['services_with_names'] += 1
            if node.get('tags'):
                analysis['services_with_tags'] += 1
        
        # Analyze edges
        for edge in edges:
            if edge.get('request_count', 0) > 0:
                analysis['dependencies_with_metrics'] += 1
            if edge.get('relationship_strength', 0) >= 7:
                analysis['high_strength_dependencies'] += 1
            if edge.get('error_rate', 0) > 5:
                analysis['error_prone_dependencies'] += 1
        
        return analysis
    
    def print_analysis(self, analysis: Dict[str, Any]):
        """Print the graph analysis results."""
        print("\n📊 Graph Quality Analysis:")
        print("=" * 30)
        print(f"Nodes: {analysis['node_count']}")
        print(f"Edges: {analysis['edge_count']}")
        print(f"Connectivity Ratio: {analysis['connectivity_ratio']:.2f}")
        print(f"Services with Names: {analysis['services_with_names']}/{analysis['node_count']}")
        print(f"Services with Tags: {analysis['services_with_tags']}/{analysis['node_count']}")
        print(f"Dependencies with Metrics: {analysis['dependencies_with_metrics']}/{analysis['edge_count']}")
        print(f"High Strength Dependencies: {analysis['high_strength_dependencies']}/{analysis['edge_count']}")
        print(f"Error-prone Dependencies: {analysis['error_prone_dependencies']}/{analysis['edge_count']}")
        
        # Quality score
        if analysis['node_count'] > 0 and analysis['edge_count'] > 0:
            quality_score = (
                (analysis['services_with_names'] / analysis['node_count']) * 0.3 +
                (analysis['dependencies_with_metrics'] / analysis['edge_count']) * 0.4 +
                (analysis['services_with_tags'] / analysis['node_count']) * 0.2 +
                min(analysis['connectivity_ratio'], 1.0) * 0.1
            )
            print(f"📈 Quality Score: {quality_score:.2%}")
        
    def test_microservices_scenario(self):
        """Test microservices dependency detection scenario."""
        print("\n🧪 Testing Microservices Scenario...")
        
        # Check for expected microservices
        expected_services = [
            'gateway', 'user-service', 'product-service', 
            'order-service', 'analytics-service'
        ]
        
        graph = self.get_dependency_graph()
        if not graph:
            print("❌ Could not fetch dependency graph")
            return False
        
        nodes = graph.get('nodes', [])
        found_services = set()
        
        for node in nodes:
            service_name = node.get('label', '').lower()
            for expected in expected_services:
                if expected in service_name:
                    found_services.add(expected)
                    break
        
        print(f"🔍 Expected services: {expected_services}")
        print(f"✅ Found services: {list(found_services)}")
        
        missing_services = set(expected_services) - found_services
        if missing_services:
            print(f"⚠️  Missing services: {list(missing_services)}")
        
        # Check for typical dependency patterns
        edges = graph.get('edges', [])
        gateway_dependencies = []
        
        for edge in edges:
            source = edge.get('source', '').lower()
            target = edge.get('target', '').lower()
            
            if 'gateway' in source:
                gateway_dependencies.append(target)
        
        print(f"🌐 Gateway dependencies: {gateway_dependencies}")
        
        return len(found_services) >= 3  # Consider success if we find at least 3 services
    
    def monitor_real_time_updates(self, duration: int = 30):
        """Monitor real-time dependency graph updates."""
        print(f"\n⏱️  Monitoring real-time updates for {duration} seconds...")
        
        start_time = time.time()
        previous_graph = {}
        update_count = 0
        
        while time.time() - start_time < duration:
            current_graph = self.get_dependency_graph()
            
            if current_graph and current_graph != previous_graph:
                update_count += 1
                print(f"📱 Update #{update_count} detected at {time.strftime('%H:%M:%S')}")
                
                # Show what changed
                if previous_graph:
                    prev_nodes = len(previous_graph.get('nodes', []))
                    curr_nodes = len(current_graph.get('nodes', []))
                    prev_edges = len(previous_graph.get('edges', []))
                    curr_edges = len(current_graph.get('edges', []))
                    
                    if curr_nodes != prev_nodes:
                        print(f"   Services: {prev_nodes} → {curr_nodes}")
                    if curr_edges != prev_edges:
                        print(f"   Dependencies: {prev_edges} → {curr_edges}")
                
                previous_graph = current_graph
            
            time.sleep(2)  # Check every 2 seconds
        
        print(f"📊 Monitoring completed. Detected {update_count} updates.")
        return update_count

def main():
    print("🚀 Enhanced eBPF Dependency Graph Tester")
    print("=" * 50)
    
    if len(sys.argv) > 1:
        operator_url = sys.argv[1]
    else:
        operator_url = "http://localhost:8080"
    
    tester = DependencyGraphTester(operator_url)
    
    # Test health
    if not tester.test_health_check():
        print("❌ Operator is not healthy. Please start the operator first.")
        return 1
    
    # Get and display current graph
    graph = tester.get_dependency_graph()
    if graph:
        tester.print_dependency_graph(graph)
        
        # Analyze graph quality
        analysis = tester.analyze_graph_quality(graph)
        tester.print_analysis(analysis)
        
        # Test microservices scenario
        success = tester.test_microservices_scenario()
        
        # Monitor real-time updates (optional)
        try:
            print("\n👀 Press Ctrl+C to skip real-time monitoring...")
            time.sleep(2)
            tester.monitor_real_time_updates(15)  # Monitor for 15 seconds
        except KeyboardInterrupt:
            print("\n⏹️  Real-time monitoring skipped.")
        
        if success and analysis['node_count'] > 0 and analysis['edge_count'] > 0:
            print("\n🎉 Dependency graph test PASSED!")
            return 0
        else:
            print("\n⚠️  Dependency graph test completed with limited results.")
            return 1
    else:
        print("❌ Could not fetch dependency graph.")
        return 1

if __name__ == "__main__":
    sys.exit(main())