# eBPF Dependency Tracker

Minimal, Hubble-inspired local dependency graph for microservices using eBPF (CGROUP_SOCK_ADDR + SOCK_OPS) with a built-in operator and dashboard. Optionally enriches edges with HTTP paths using a userspace libpcap sniffer.

Quick start
- Install libpcap (for HTTP path enrichment):
  - Ubuntu/Debian: sudo apt-get install -y libpcap-dev
- Build: sudo go generate ./internal/ebpf/... && sudo ./scripts/build.sh
- Start core (operator + agent): sudo ./scripts/start-tracking.sh
- Start demo microservices: ./scripts/start-microservices.sh
- Generate traffic: ./scripts/generate-traffic.sh
- Open UI: http://localhost:8080 (Dependency Graph)
- Stop everything: sudo ./scripts/stop-all.sh
- One-shot restart + demo: ./scripts/restart.sh --with-microservices --with-traffic
- Clean: ./scripts/clean.sh --deep

What you get
- L4 service dependencies resolved via cgroup connect and sockops (loopback supported)
- Optional L7: HTTP request paths per edge via userspace sniffing on loopback (no app changes)
- Enrichment to service names for known ports (8000..8010)
- Live REST API: /health, /api/dependency-graph, /api/metrics, /api/http-metrics

Requirements
- Linux with cgroup v2 mounted at /sys/fs/cgroup
- Run agent as root (sudo) with CAP_BPF/CAP_NET_ADMIN
- Go 1.23+ and protoc (scripts/build.sh checks)
- libpcap (libpcap-dev on Debian/Ubuntu) for the HTTP path sniffer

Project layout
- cmd/operator, cmd/agent: binaries
- internal/ebpf/dependency_tracer.*: eBPF programs and userspace wiring
- internal/pathsniff/sniffer.go: libpcap HTTP path collector on loopback
- internal/graph/*: in-memory graph + enrichment
- pkg/proto/*: gRPC/Protobuf
- scripts/*: build, start, stop, restart, demo traffic
- web/dashboard.html: dashboard with HTTP Path filter

Dashboard tips
- Use the HTTP Path Filter controls (method + path) to highlight edges and nodes involved in a specific route.
- Hover an edge to see request_count, relationship strength, and any HTTP paths seen.
- Click “Reset Layout” to re-run the force simulation; pan/zoom with mouse.

API
- GET /api/dependency-graph → nodes[], edges[] with http_paths when available
- GET /api/metrics → aggregate metrics (legacy)
- GET /api/http-metrics → recent per-request samples (if enabled)

Troubleshooting
- No HTTP paths: ensure libpcap is installed; agent running with sudo; generate HTTP traffic to ports 8000–8010.
- No edges: ensure agent is running as root; generate traffic; check logs/*.log.
- Ports busy: sudo fuser -k 8080/tcp 9090/tcp 8000-8010/tcp
- Graph cramped: press “Reset Layout” in UI (auto-resizes on window change)
