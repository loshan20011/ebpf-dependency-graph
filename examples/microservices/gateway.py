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
            # Auth then forward to user service
            try:
                requests.get('http://localhost:8008/login', timeout=2)
            except:
                pass
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
            # Auth then forward to order service
            try:
                requests.get('http://localhost:8008/login', timeout=2)
            except:
                pass
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
        
        if self.path.startswith('/api/products'):
            # Auth then forward to product service
            try:
                requests.get('http://localhost:8008/login', timeout=2)
            except:
                pass
            try:
                resp = requests.get('http://localhost:8004/products', timeout=5)
                self.send_response(resp.status_code)
                self.send_header('Content-type', 'application/json')
                self.end_headers()
                self.wfile.write(resp.content)
            except Exception as e:
                self.send_response(500)
                self.end_headers()
                self.wfile.write(f"Error: {e}".encode())
            return
        
        if self.path.startswith('/api/catalog'):
            # Auth then forward to catalog service
            try:
                requests.get('http://localhost:8008/login', timeout=2)
            except:
                pass
            try:
                resp = requests.get('http://localhost:8009/catalog', timeout=5)
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
