# Architecture & Data Flow

This system builds a service dependency graph using eBPF with Cilium/Hubble-style hooks. It favors correct L4 edges on localhost without relying on blocked perf tracepoints.

Program types
- CGROUP_SOCK_ADDR (connect4): Captures outgoing connect() destination IP/port per process; updates service_map and cookie_map.
- CGROUP_SOCK_OPS (active-established): Emits FlowEvents with reliable 4‑tuple at TCP establishment; correlates socket via cookie_map and a PID fallback.
- XDP (optional): Observes TCP packets on an interface (best-effort; not required for localhost correctness).

Key eBPF files
- internal/ebpf/dependency_tracer.c
  - Maps:
    - service_map: {ip,port} -> service_info
    - flow_map: {4‑tuple} -> flow_info
    - dependency_map: {src_svc,dst_svc} -> dependency_info
    - cookie_map: cookie -> {daddr,dport} (connect4→sockops correlation)
    - pid_map: pid -> {daddr,dport} (fallback)
  - Programs:
    - SEC("cgroup/connect4") cgroup_connect_tracker
    - SEC("sockops") sockops_tracker
    - SEC("xdp") xdp_dependency_tracker (optional)
- internal/ebpf/dependency_tracer.go
  - Loads/attaches programs; sets up perf readers; converts C structs to protobuf; streams FlowEvents/DependencyEvents to operator.

Userspace components
- cmd/agent: Loads eBPF, streams to operator via gRPC (pkg/proto/metrics.proto).
- cmd/operator:
  - Receives streams, updates an in-memory graph (internal/graph/*).
  - Exposes REST API: /health, /api/dependency-graph, /api/metrics.
  - Runs a simple dashboard (web/dashboard.html).
- internal/graph:
  - GraphManager: nodes (services) and edges (dependencies), counters, timestamps, tags.
  - DependencyAnalyzer: rules for strength/tiers/name inference.
  - ServiceRegistry: enriches nodes (e.g., ports 8000..8007 → gateway/user/order/notification/payment).

Data flow (happy path)
1) Client opens TCP to server (e.g., 127.0.0.1:8001).
2) cgroup/connect4 stores {daddr,dport} in cookie_map + pid_map and touches service_map.
3) sockops active-established reads cookie_map/pid_map, produces FlowEvent with stable 4‑tuple.
4) Agent forwards FlowEvent → Operator.
5) Operator updates GraphManager (services by ip:port, edges source→dest); ServiceRegistry names common ports.
6) Dashboard fetches /api/dependency-graph and renders the graph.

Edge correctness on localhost
- We avoid sys_enter_connect tracepoints (often blocked) and instead rely on CGROUP_SOCK_ADDR + SOCK_OPS with cookie and PID correlation to get accurate destination ports on loopback.

Operational notes
- Agent must run as root to attach cgroup/sockops (scripts/start-tracking.sh handles via sudo).
- cgroup v2 must be mounted at /sys/fs/cgroup.
- XDP may fail to attach on some interfaces; it’s optional and logged as a warning.

Important source files
- Agent entry: cmd/agent/main.go
- Operator entry + HTTP API: cmd/operator/main.go, cmd/operator/grpc_server.go
- eBPF core: internal/ebpf/dependency_tracer.c, internal/ebpf/dependency_tracer.go
- Graph logic: internal/graph/*.go
- Protobuf: pkg/proto/metrics.proto
- UI: web/dashboard.html

API endpoints
- GET /health → { service, status }
- GET /api/dependency-graph → nodes[], edges[], totals, stats
- GET /api/metrics → legacy cumulative metrics (optional)

Limitations / future work
- L7 (HTTP) tagging is minimal by design; consider userspace sniffing if needed.
- RTT/latency from TCP sockops can be added with further sockops callbacks.
- Kubernetes enrichment can be added by watching Services/Endpoints and replacing ip:port labels with (ns/svc/pod).
