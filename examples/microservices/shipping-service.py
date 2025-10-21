#!/usr/bin/env python3
import json
from http.server import HTTPServer, BaseHTTPRequestHandler
import requests

class ShippingHandler(BaseHTTPRequestHandler):
    def do_GET(self):
        print(f"Shipping service received: {self.path}")
        if self.path == '/health':
            self.send_response(200)
            self.send_header('Content-type', 'application/json')
            self.end_headers()
            self.wfile.write(json.dumps({"service": "shipping-service", "status": "healthy"}).encode())
            return
        if self.path == '/shipping':
            # Notify on shipping quote
            try:
                requests.get('http://localhost:8003/notify', timeout=2)
            except:
                pass
            self.send_response(200)
            self.send_header('Content-type', 'application/json')
            self.end_headers()
            quote = {"carrier": "FastShip", "cost": 4.99}
            self.wfile.write(json.dumps(quote).encode())
            return
        self.send_response(404)
        self.end_headers()

if __name__ == '__main__':
    server = HTTPServer(('0.0.0.0', 8006), ShippingHandler)
    print("Shipping service starting on port 8006...")
    server.serve_forever()
