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
