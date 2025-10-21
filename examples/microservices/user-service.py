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
