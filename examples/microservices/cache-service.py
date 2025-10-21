#!/usr/bin/env python3
import json
from http.server import HTTPServer, BaseHTTPRequestHandler

class CacheHandler(BaseHTTPRequestHandler):
    def do_GET(self):
        print(f"Cache service received: {self.path}")
        if self.path == '/health':
            self.send_response(200)
            self.send_header('Content-type', 'application/json')
            self.end_headers()
            self.wfile.write(json.dumps({"service": "cache-service", "status": "healthy"}).encode())
            return
        if self.path == '/cache/ping':
            self.send_response(200)
            self.send_header('Content-type', 'application/json')
            self.end_headers()
            self.wfile.write(json.dumps({"pong": True}).encode())
            return
        self.send_response(404)
        self.end_headers()

if __name__ == '__main__':
    server = HTTPServer(('0.0.0.0', 8010), CacheHandler)
    print("Cache service starting on port 8010...")
    server.serve_forever()
