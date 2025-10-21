#!/usr/bin/env python3
import json
from http.server import HTTPServer, BaseHTTPRequestHandler
import requests

class CatalogHandler(BaseHTTPRequestHandler):
    def do_GET(self):
        print(f"Catalog service received: {self.path}")
        if self.path == '/health':
            self.send_response(200)
            self.send_header('Content-type', 'application/json')
            self.end_headers()
            self.wfile.write(json.dumps({"service": "catalog-service", "status": "healthy"}).encode())
            return
        if self.path == '/catalog':
            # Aggregate products and users
            try:
                requests.get('http://localhost:8004/products', timeout=2)
                requests.get('http://localhost:8001/users', timeout=2)
            except:
                pass
            self.send_response(200)
            self.send_header('Content-type', 'application/json')
            self.end_headers()
            self.wfile.write(json.dumps({"ok": True}).encode())
            return
        self.send_response(404)
        self.end_headers()

if __name__ == '__main__':
    server = HTTPServer(('0.0.0.0', 8009), CatalogHandler)
    print("Catalog service starting on port 8009...")
    server.serve_forever()
