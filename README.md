# eBPF Dependency Tracker

Minimal, Hubble-inspired local dependency graph for microservices using eBPF (CGROUP_SOCK_ADDR + SOCK_OPS) with a built-in operator and dashboard.

Quick start
- Build: sudo go generate ./internal/ebpf/... && sudo ./scripts/build.sh
- Start core (operator + agent): sudo ./scripts/start-tracking.sh
- Start demo microservices: ./scripts/start-microservices.sh
- Generate traffic: ./scripts/generate-traffic.sh
- Open UI: http://localhost:8080 (Dependency Graph tab)
- Stop everything: sudo ./scripts/stop-all.sh
- One-shot restart + demo: ./scripts/restart.sh --with-microservices --with-traffic
- Remove all and clean the service: ./scripts/clean.sh --deep

What you get
- L4 service dependencies resolved via cgroup connect and sockops (loopback supported)
- Enrichment to service names for known ports (8000..8007)
- Live REST API: /health, /api/dependency-graph, /api/metrics

Requirements
- Linux with cgroup v2 mounted at /sys/fs/cgroup
- Run agent as root (sudo) with CAP_BPF/CAP_NET_ADMIN (provided by sudo)
- Go 1.23+ and protoc (scripts/build.sh checks)

Project layout
- cmd/operator, cmd/agent: binaries
- internal/ebpf/dependency_tracer.*: eBPF programs and userspace wiring
- internal/graph/*: in-memory graph + enrichment
- pkg/proto/*: gRPC/Protobuf
- scripts/*: build, start, stop, restart, demo traffic
- web/dashboard.html: dashboard

Troubleshooting
- Ports busy: sudo fuser -k 8080/tcp 9090/tcp 8000-8007/tcp
- No edges: ensure agent is running as root; generate traffic; check logs/*.log
- Graph cramped: press “Reset Layout” in UI (also resizes with window)
