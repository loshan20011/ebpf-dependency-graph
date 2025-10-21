#!/usr/bin/env python3
"""
Kubernetes-native Dependency Graph Visualizer
Connects to metrics collector API for real-time dependency visualization
"""

from flask import Flask, render_template, jsonify, request
import requests
from datetime import datetime
import os
import logging

# Configure logging
logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

app = Flask(__name__)

# Configuration
METRICS_COLLECTOR_URL = os.getenv('METRICS_COLLECTOR_URL', 'http://localhost:8080')

def fetch_from_metrics_collector(endpoint: str, timeout: int = 5):
    """Fetch data from metrics collector service"""
    try:
        url = f"{METRICS_COLLECTOR_URL}{endpoint}"
        logger.debug(f"Fetching {url}")
        
        response = requests.get(url, timeout=timeout)
        if response.status_code == 200:
            return response.json()
        else:
            logger.warning(f"Metrics collector returned {response.status_code} for {endpoint}")
            
    except requests.exceptions.RequestException as e:
        logger.error(f"Error fetching from metrics collector: {e}")
    except Exception as e:
        logger.error(f"Unexpected error: {e}")
    
    return None

@app.route('/')
def index():
    """Main dashboard page"""
    return render_template('k8s_dependency_graph.html')

@app.route('/api/dependencies')
def get_dependencies():
    """Get current service dependencies from metrics collector"""
    data = fetch_from_metrics_collector('/api/dependency-graph')
    if data:
        # Transform the data for our visualization format
        dependencies = {}
        services = set()
        
        # Extract dependencies from nodes and edges
        for edge in data.get('edges', []):
            source = edge['source']
            target = edge['target']
            
            services.add(source)
            services.add(target)
            
            if source not in dependencies:
                dependencies[source] = []
            dependencies[source].append(target)
        
        return jsonify({
            'dependencies': dependencies,
            'services': list(services),
            'raw_data': data,
            'timestamp': datetime.utcnow().isoformat()
        })
    
    return jsonify({
        'dependencies': {},
        'services': [],
        'timestamp': datetime.utcnow().isoformat(),
        'error': 'Could not fetch dependency data'
    })

@app.route('/api/service-latencies')
def get_service_latencies():
    """Get service latencies from metrics collector"""
    data = fetch_from_metrics_collector('/api/metrics')
    if data:
        latency_summary = {}
        
        # Transform service metrics into latency summary
        for service_name, service_data in data.get('services', {}).items():
            if service_data.get('avg_latency_ms') is not None:
                latency_summary[service_name] = {
                    'avg_ms': service_data['avg_latency_ms'],
                    'metric_count': service_data.get('metric_count', 0)
                }
        
        return jsonify({
            'latencies': latency_summary,
            'timestamp': datetime.utcnow().isoformat()
        })
    
    return jsonify({
        'latencies': {},
        'timestamp': datetime.utcnow().isoformat(),
        'error': 'Could not fetch latency data'
    })

@app.route('/api/service-calls')
def get_service_calls():
    """Get recent service calls - simulated for now"""
    # For now, return empty calls since we don't have real-time call tracking
    # In the future, this could be enhanced with actual call logs
    return jsonify({
        'calls': [],
        'count': 0,
        'timestamp': datetime.utcnow().isoformat()
    })

@app.route('/api/scaling-events')
def get_scaling_events():
    """Get recent scaling events - would come from operator in real implementation"""
    # For now, return empty events
    # In the future, this could fetch from Kubernetes events or operator logs
    return jsonify({
        'events': [],
        'count': 0,
        'timestamp': datetime.utcnow().isoformat()
    })

@app.route('/api/service-replicas')
def get_service_replicas():
    """Get current replica counts from Kubernetes"""
    try:
        # Try to get deployment info
        from kubernetes import client, config
        
        try:
            config.load_incluster_config()
        except:
            config.load_kube_config()
        
        apps_v1 = client.AppsV1Api()
        deployments = apps_v1.list_namespaced_deployment(namespace="enhanced-microservices")
        
        replicas = {}
        for deployment in deployments.items:
            replicas[deployment.metadata.name] = deployment.spec.replicas
            
        return jsonify({
            'replicas': replicas,
            'timestamp': datetime.utcnow().isoformat()
        })
        
    except Exception as e:
        logger.error(f"Error getting replica counts: {e}")
        return jsonify({
            'replicas': {},
            'timestamp': datetime.utcnow().isoformat(),
            'error': 'Could not fetch replica data'
        })

@app.route('/api/health')
def health():
    """Health check endpoint"""
    # Check if metrics collector is reachable
    collector_health = fetch_from_metrics_collector('/health')
    
    return jsonify({
        'service': 'dependency-visualizer',
        'status': 'healthy',
        'metrics_collector_status': 'healthy' if collector_health else 'unhealthy',
        'metrics_collector_url': METRICS_COLLECTOR_URL,
        'timestamp': datetime.utcnow().isoformat()
    })

def create_template():
    """Create the HTML template for the dependency graph"""
    os.makedirs('templates', exist_ok=True)
    
    html_content = '''
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Enhanced Microservices - Dependency Graph</title>
    <script src="https://d3js.org/d3.v7.min.js"></script>
    <style>
        body {
            font-family: 'Segoe UI', Tahoma, Geneva, Verdana, sans-serif;
            margin: 0;
            padding: 20px;
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            min-height: 100vh;
            color: #333;
        }
        .container {
            max-width: 1400px;
            margin: 0 auto;
        }
        .header {
            text-align: center;
            margin-bottom: 30px;
        }
        .header h1 {
            color: white;
            text-shadow: 2px 2px 4px rgba(0,0,0,0.3);
            margin-bottom: 10px;
        }
        .status-bar {
            background: rgba(255,255,255,0.1);
            backdrop-filter: blur(10px);
            border-radius: 10px;
            padding: 15px;
            margin-bottom: 20px;
            color: white;
        }
        .dashboard {
            display: grid;
            grid-template-columns: 2fr 1fr;
            gap: 20px;
        }
        .graph-container, .metrics-panel {
            background: rgba(255,255,255,0.95);
            backdrop-filter: blur(10px);
            border-radius: 15px;
            padding: 25px;
            box-shadow: 0 8px 25px rgba(0,0,0,0.15);
        }
        .stats {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
            gap: 15px;
            margin-bottom: 20px;
        }
        .stat-card {
            background: linear-gradient(145deg, #e6f3ff, #cce7ff);
            padding: 20px;
            border-radius: 12px;
            text-align: center;
            box-shadow: 0 4px 8px rgba(0,0,0,0.1);
            transition: transform 0.2s ease;
        }
        .stat-card:hover {
            transform: translateY(-2px);
        }
        .stat-label {
            font-size: 12px;
            color: #666;
            text-transform: uppercase;
            margin-bottom: 8px;
            font-weight: 600;
        }
        .stat-value {
            font-size: 24px;
            font-weight: bold;
            color: #2c3e50;
        }
        .graph-controls {
            margin-bottom: 20px;
        }
        .refresh-btn {
            background: linear-gradient(145deg, #3498db, #2980b9);
            color: white;
            border: none;
            padding: 12px 20px;
            border-radius: 8px;
            cursor: pointer;
            font-weight: 600;
            transition: all 0.3s ease;
        }
        .refresh-btn:hover {
            transform: translateY(-1px);
            box-shadow: 0 4px 12px rgba(52, 152, 219, 0.3);
        }
        .node {
            cursor: pointer;
            transition: all 0.3s ease;
        }
        .node:hover {
            stroke: #2c3e50;
            stroke-width: 3px;
        }
        .link {
            stroke: #95a5a6;
            stroke-opacity: 0.8;
            stroke-width: 2;
            transition: all 0.3s ease;
        }
        .link:hover {
            stroke: #3498db;
            stroke-width: 3;
        }
        .tooltip {
            position: absolute;
            padding: 12px;
            background: rgba(0, 0, 0, 0.9);
            color: white;
            border-radius: 8px;
            pointer-events: none;
            font-size: 13px;
            z-index: 1000;
            max-width: 200px;
        }
        .metric-item {
            margin-bottom: 15px;
            padding: 15px;
            background: #f8f9fa;
            border-radius: 8px;
            border-left: 4px solid #3498db;
        }
        .connection-status {
            display: flex;
            align-items: center;
            justify-content: space-between;
        }
        .status-indicator {
            width: 10px;
            height: 10px;
            border-radius: 50%;
            margin-left: 8px;
        }
        .status-healthy { background-color: #27ae60; }
        .status-unhealthy { background-color: #e74c3c; }
        .error-message {
            background: rgba(231, 76, 60, 0.1);
            border: 1px solid rgba(231, 76, 60, 0.3);
            color: #c0392b;
            padding: 10px;
            border-radius: 6px;
            margin-bottom: 15px;
        }
        .loading {
            text-align: center;
            padding: 40px;
            color: #7f8c8d;
        }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>🔗 Enhanced Microservices</h1>
            <p style="color: rgba(255,255,255,0.8); margin: 0;">Real-time Service Dependency Graph & Latency-based Autoscaling</p>
        </div>
        
        <div class="status-bar">
            <div class="connection-status">
                <span>Metrics Collector Connection</span>
                <div style="display: flex; align-items: center;">
                    <span id="collector-status">Checking...</span>
                    <div class="status-indicator" id="collector-indicator"></div>
                </div>
            </div>
        </div>

        <div class="stats">
            <div class="stat-card">
                <div class="stat-label">Services</div>
                <div class="stat-value" id="service-count">-</div>
            </div>
            <div class="stat-card">
                <div class="stat-label">Dependencies</div>
                <div class="stat-value" id="dependency-count">-</div>
            </div>
            <div class="stat-card">
                <div class="stat-label">Avg Latency</div>
                <div class="stat-value" id="avg-latency">-</div>
            </div>
            <div class="stat-card">
                <div class="stat-label">Active Replicas</div>
                <div class="stat-value" id="total-replicas">-</div>
            </div>
        </div>

        <div class="dashboard">
            <div class="graph-container">
                <div class="graph-controls">
                    <button class="refresh-btn" onclick="refreshData()">🔄 Refresh Data</button>
                    <span style="margin-left: 15px; color: #7f8c8d; font-size: 14px;">Auto-refresh every 10s</span>
                </div>
                <div id="error-container"></div>
                <div id="graph-loading" class="loading">
                    <div>📊 Loading dependency graph...</div>
                </div>
                <svg id="dependency-graph"></svg>
            </div>
            
            <div class="metrics-panel">
                <h2 style="color: #2c3e50; margin-top: 0;">📊 Service Metrics</h2>
                <div id="metrics-content"></div>
                
                <h2 style="color: #2c3e50;">🎯 Service Replicas</h2>
                <div id="service-replicas"></div>
            </div>
        </div>
    </div>

    <div class="tooltip" id="tooltip"></div>

    <script>
        let graphData = { nodes: [], links: [] };
        let svg, simulation;
        let width, height;

        function setGraphSize() {
            // Make the graph fill most of the viewport, with some margin
            width = Math.max(document.documentElement.clientWidth, window.innerWidth || 0) - 100;
            height = Math.max(document.documentElement.clientHeight, window.innerHeight || 0) - 250;
            if (svg) {
                svg.attr('width', width).attr('height', height);
                if (simulation) {
                    simulation.force('center', d3.forceCenter(width / 2, height / 2));
                    simulation.alpha(1).restart();
                }
            }
        }

        function initGraph() {
            setGraphSize();
            svg = d3.select('#dependency-graph')
                .attr('width', width)
                .attr('height', height);

            simulation = d3.forceSimulation()
                .force('link', d3.forceLink().id(d => d.id).distance(150))
                .force('charge', d3.forceManyBody().strength(-400))
                .force('center', d3.forceCenter(width / 2, height / 2))
                .force('collision', d3.forceCollide(35));
        }

        function getNodeColor(service) {
            const colors = {
                'gateway': '#e74c3c',
                'user-service': '#3498db',
                'product-service': '#2ecc71',
                'analytics-service': '#f39c12',
                'order-service': '#9b59b6',
                'payment-service': '#1abc9c',
                'inventory-service': '#34495e',
                'notification-service': '#e67e22'
            };
            
            for (const [key, color] of Object.entries(colors)) {
                if (service.includes(key)) return color;
            }
            return '#95a5a6';
        }

        function updateGraph(dependencies) {
            const nodes = [];
            const links = [];
            
            // Create nodes
            const services = new Set();
            Object.keys(dependencies).forEach(source => {
                services.add(source);
                if (Array.isArray(dependencies[source])) {
                    dependencies[source].forEach(target => services.add(target));
                }
            });
            
            services.forEach(service => {
                nodes.push({ 
                    id: service, 
                    color: getNodeColor(service)
                });
            });
            
            // Create links
            Object.entries(dependencies).forEach(([source, targets]) => {
                if (Array.isArray(targets)) {
                    targets.forEach(target => {
                        links.push({ source, target, value: 1 });
                    });
                }
            });
            
            graphData = { nodes, links };
            renderGraph();
        }

        function renderGraph() {
            if (graphData.nodes.length === 0) {
                document.getElementById('graph-loading').innerHTML = '📭 No service dependencies detected';
                return;
            }

            document.getElementById('graph-loading').style.display = 'none';
            svg.selectAll('*').remove();
            
            const link = svg.append('g')
                .selectAll('line')
                .data(graphData.links)
                .enter().append('line')
                .attr('class', 'link');

            const node = svg.append('g')
                .selectAll('circle')
                .data(graphData.nodes)
                .enter().append('circle')
                .attr('class', 'node')
                .attr('r', 25)
                .attr('fill', d => d.color)
                .call(d3.drag()
                    .on('start', dragstarted)
                    .on('drag', dragged)
                    .on('end', dragended))
                .on('mouseover', showTooltip)
                .on('mouseout', hideTooltip);

            const labels = svg.append('g')
                .selectAll('text')
                .data(graphData.nodes)
                .enter().append('text')
                .text(d => d.id.replace('-service', ''))
                .attr('font-size', 11)
                .attr('font-weight', 'bold')
                .attr('text-anchor', 'middle')
                .attr('dy', 4)
                .attr('fill', 'white');

            simulation.nodes(graphData.nodes).on('tick', ticked);
            simulation.force('link').links(graphData.links);
            simulation.alpha(1).restart();

            function ticked() {
                link
                    .attr('x1', d => d.source.x)
                    .attr('y1', d => d.source.y)
                    .attr('x2', d => d.target.x)
                    .attr('y2', d => d.target.y);

                node
                    .attr('cx', d => d.x)
                    .attr('cy', d => d.y);

                labels
                    .attr('x', d => d.x)
                    .attr('y', d => d.y);
            }
        }

        function dragstarted(event, d) {
            if (!event.active) simulation.alphaTarget(0.3).restart();
            d.fx = d.x;
            d.fy = d.y;
        }

        function dragged(event, d) {
            d.fx = event.x;
            d.fy = event.y;
        }

        function dragended(event, d) {
            if (!event.active) simulation.alphaTarget(0);
            d.fx = null;
            d.fy = null;
        }

        function showTooltip(event, d) {
            const tooltip = document.getElementById('tooltip');
            tooltip.style.display = 'block';
            tooltip.style.left = event.pageX + 10 + 'px';
            tooltip.style.top = event.pageY - 10 + 'px';
            tooltip.innerHTML = `<strong>${d.id}</strong><br>Microservice<br>Click and drag to reposition`;
        }

        function hideTooltip() {
            document.getElementById('tooltip').style.display = 'none';
        }

        function showError(message) {
            const errorContainer = document.getElementById('error-container');
            errorContainer.innerHTML = `<div class="error-message">⚠️ ${message}</div>`;
        }

        function clearError() {
            document.getElementById('error-container').innerHTML = '';
        }

        async function fetchData() {
            try {
                clearError();
                
                const [healthRes, depsRes, latencyRes, replicasRes] = await Promise.all([
                    fetch('/api/health'),
                    fetch('/api/dependencies'),
                    fetch('/api/service-latencies'),
                    fetch('/api/service-replicas')
                ]);

                const health = await healthRes.json();
                const dependencies = await depsRes.json();
                const latencies = await latencyRes.json();
                const replicas = await replicasRes.json();

                updateConnectionStatus(health);
                updateGraph(dependencies.dependencies);
                updateMetrics(latencies);
                updateServiceReplicas(replicas.replicas);
                updateStats(dependencies, latencies, replicas);
                
            } catch (error) {
                console.error('Error fetching data:', error);
                showError('Failed to fetch data from metrics collector');
                updateConnectionStatus({metrics_collector_status: 'unhealthy'});
            }
        }

        function updateConnectionStatus(health) {
            const statusSpan = document.getElementById('collector-status');
            const indicator = document.getElementById('collector-indicator');
            
            if (health.metrics_collector_status === 'healthy') {
                statusSpan.textContent = 'Connected';
                indicator.className = 'status-indicator status-healthy';
            } else {
                statusSpan.textContent = 'Disconnected';
                indicator.className = 'status-indicator status-unhealthy';
            }
        }

        function updateStats(dependencies, latencies, replicas) {
            const serviceCount = dependencies.services ? dependencies.services.length : 0;
            const dependencyCount = Object.values(dependencies.dependencies || {}).reduce((acc, deps) => acc + deps.length, 0);
            
            document.getElementById('service-count').textContent = serviceCount;
            document.getElementById('dependency-count').textContent = dependencyCount;
            
            const avgLatencies = Object.values(latencies.latencies || {}).map(l => l.avg_ms).filter(l => l != null);
            const avgLatency = avgLatencies.length > 0 ? 
                (avgLatencies.reduce((a, b) => a + b, 0) / avgLatencies.length).toFixed(1) + 'ms' : 'N/A';
            document.getElementById('avg-latency').textContent = avgLatency;
            
            const totalReplicas = Object.values(replicas.replicas || {}).reduce((acc, count) => acc + count, 0);
            document.getElementById('total-replicas').textContent = totalReplicas || 0;
        }

        function updateMetrics(latencies) {
            const metricsContent = document.getElementById('metrics-content');
            let html = '';
            
            const latencyData = latencies.latencies || {};
            if (Object.keys(latencyData).length === 0) {
                html = '<div class="metric-item">📊 No latency data available yet</div>';
            } else {
                html = '<div class="metric-item"><strong>Service Latencies:</strong><br>';
                Object.entries(latencyData).forEach(([service, data]) => {
                    html += `${service}: ${data.avg_ms ? data.avg_ms.toFixed(1) + 'ms' : 'N/A'}<br>`;
                });
                html += '</div>';
            }
            
            metricsContent.innerHTML = html;
        }

        function updateServiceReplicas(replicasData) {
            const replicasDiv = document.getElementById('service-replicas');
            let html = '';
            
            if (Object.keys(replicasData || {}).length === 0) {
                html = '<div class="metric-item">🎯 No replica data available</div>';
            } else {
                Object.entries(replicasData).forEach(([service, count]) => {
                    html += `<div class="metric-item">
                        <strong>${service}</strong><br>
                        Replicas: ${count}
                    </div>`;
                });
            }
            
            replicasDiv.innerHTML = html;
        }

        function refreshData() {
            fetchData();
        }

        // Initialize
        initGraph();
        fetchData();

        // Auto-refresh every 10 seconds
        setInterval(fetchData, 10000);

        // Resize graph on window resize
        window.addEventListener('resize', () => {
            setGraphSize();
            renderGraph();
        });
    </script>
</body>
</html>
    '''
    
    with open('templates/k8s_dependency_graph.html', 'w') as f:
        f.write(html_content)

if __name__ == '__main__':
    # Create templates
    create_template()
    
    logger.info("🚀 Starting Enhanced Microservices Dependency Visualizer")
    logger.info(f"📊 Dashboard available at: http://localhost:5000")
    logger.info(f"🔗 Connecting to metrics collector: {METRICS_COLLECTOR_URL}")
    
    # Run Flask app
    app.run(host='0.0.0.0', port=5000, debug=False)