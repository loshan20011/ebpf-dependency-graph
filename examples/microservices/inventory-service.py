#!/usr/bin/env python3
import json
from http.server import HTTPServer, BaseHTTPRequestHandler
import requests

class InventoryHandler(BaseHTTPRequestHandler):
    def do_GET(self):
        print(f"Inventory service received: {self.path}")
        if self.path == '/health':
            self.send_response(200)
            self.send_header('Content-type', 'application/json')
            self.end_headers()
            self.wfile.write(json.dumps({"service": "inventory-service", "status": "healthy"}).encode())
            return
        if self.path == '/inventory':
            # Use cache
            try:
                requests.get('http://localhost:8010/cache/ping', timeout=2)
            except:
                pass
            self.send_response(200)
            self.send_header('Content-type', 'application/json')
            self.end_headers()
            stock = {"101": 42, "102": 17}
            self.wfile.write(json.dumps(stock).encode())
            return
        self.send_response(404)
        self.end_headers()

if __name__ == '__main__':
    server = HTTPServer(('0.0.0.0', 8005), InventoryHandler)
    print("Inventory service starting on port 8005...")
    server.serve_forever()
