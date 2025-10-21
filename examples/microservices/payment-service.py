#!/usr/bin/env python3
import json
from http.server import HTTPServer, BaseHTTPRequestHandler

class PaymentHandler(BaseHTTPRequestHandler):
    def do_GET(self):
        print(f"Payment service received: {self.path}")
        
        if self.path == '/health':
            self.send_response(200)
            self.send_header('Content-type', 'application/json')
            self.end_headers()
            self.wfile.write(json.dumps({"service": "payment-service", "status": "healthy"}).encode())
            return
            
        if self.path == '/payment':
            self.send_response(200)
            self.send_header('Content-type', 'application/json')
            self.end_headers()
            response = {"status": "success", "transaction_id": "txn_123456"}
            self.wfile.write(json.dumps(response).encode())
            return
            
        self.send_response(404)
        self.end_headers()

if __name__ == '__main__':
    server = HTTPServer(('0.0.0.0', 8007), PaymentHandler)
    print("Payment service starting on port 8007...")
    server.serve_forever()
