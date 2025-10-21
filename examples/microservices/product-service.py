#!/usr/bin/env python3
import json
from http.server import HTTPServer, BaseHTTPRequestHandler
import requests

class ProductHandler(BaseHTTPRequestHandler):
    def do_GET(self):
        print(f"Product service received: {self.path}")
        if self.path == '/health':
            self.send_response(200)
            self.send_header('Content-type', 'application/json')
            self.end_headers()
            self.wfile.write(json.dumps({"service": "product-service", "status": "healthy"}).encode())
            return
        if self.path == '/products':
            # Call inventory and shipping; use cache
            try:
                requests.get('http://localhost:8010/cache/ping', timeout=2)
                requests.get('http://localhost:8005/inventory', timeout=2)
                requests.get('http://localhost:8006/shipping', timeout=2)
            except:
                pass
            self.send_response(200)
            self.send_header('Content-type', 'application/json')
            self.end_headers()
            products = [
                {"id": 101, "name": "Widget", "price": 19.99},
                {"id": 102, "name": "Gadget", "price": 29.99}
            ]
            self.wfile.write(json.dumps(products).encode())
            return
        self.send_response(404)
        self.end_headers()

if __name__ == '__main__':
    server = HTTPServer(('0.0.0.0', 8004), ProductHandler)
    print("Product service starting on port 8004...")
    server.serve_forever()
