#!/usr/bin/env python3
import json
from http.server import HTTPServer, BaseHTTPRequestHandler

class NotificationHandler(BaseHTTPRequestHandler):
    def do_GET(self):
        print(f"Notification service received: {self.path}")
        
        if self.path == '/health':
            self.send_response(200)
            self.send_header('Content-type', 'application/json')
            self.end_headers()
            self.wfile.write(json.dumps({"service": "notification-service", "status": "healthy"}).encode())
            return
            
        if self.path == '/notify':
            self.send_response(200)
            self.send_header('Content-type', 'application/json')
            self.end_headers()
            response = {"message": "Notification sent", "timestamp": "2023-10-21T10:30:00Z"}
            self.wfile.write(json.dumps(response).encode())
            return
            
        self.send_response(404)
        self.end_headers()

if __name__ == '__main__':
    server = HTTPServer(('0.0.0.0', 8003), NotificationHandler)
    print("Notification service starting on port 8003...")
    server.serve_forever()
