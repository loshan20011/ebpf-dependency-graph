# Architecture & Data Flow

This system builds a service dependency graph using eBPF with Cilium/Hubble-style hooks. It favors correct L4 edges on localhost without relying on blocked perf tracepoints, and optionally enriches L7 HTTP paths via a lightweight userspace sniffer.

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
- internal/pathsniff/sniffer.go (new): libpcap-based HTTP request line sniffer on loopback.
  - Captures first-line of HTTP requests (METHOD PATH) for configured microservice ports.
  - Uses pcap.OpenLive("lo") with a broad "tcp" BPF and in-process filtering.
  - Caches recent observations keyed by 4‑tuple and also by destination port (TTL ~30s) for correlation.
  - Integrated into agent via DependencyTracker.WithPathCollector; enriches FlowEvents before sending.
- cmd/operator:
  - Receives streams, updates an in-memory graph (internal/graph/*).
  - Exposes REST API: /health, /api/dependency-graph, /api/metrics, /api/http-metrics.
  - Runs a dashboard (web/dashboard.html) with an HTTP Path filter to highlight paths across the graph.
- internal/graph:
  - GraphManager: nodes (services) and edges (dependencies), counters, timestamps, tags.
  - DependencyAnalyzer: rules for strength/tiers/name inference.
  - ServiceRegistry: enriches nodes (e.g., ports 8000..8010 → gateway/user/order/product/...).

Data flow (happy path)
1) Client opens TCP to server (e.g., 127.0.0.1:8001).
2) cgroup/connect4 stores {daddr,dport} in cookie_map + pid_map and touches service_map.
3) sockops active-established reads cookie_map/pid_map, produces FlowEvent with stable 4‑tuple.
4) Agent forwards FlowEvent → Operator. If userspace sniffer observed an HTTP request for the same 4‑tuple (or same destination port recently), it sets FlowEvent.http_method/path.
5) Operator updates GraphManager (services by ip:port, edges source→dest); attaches any discovered HTTP paths to edges.
6) Dashboard fetches /api/dependency-graph and renders the graph. Users can filter/highlight edges by METHOD and/or PATH.

Edge correctness on localhost
- We avoid sys_enter_* tracepoints and rely on CGROUP_SOCK_ADDR + SOCK_OPS with cookie and PID correlation for accurate destination ports on loopback.
- L7 paths are added in userspace to avoid BPF stack/verification limits from parsing HTTP in-kernel.

Operational notes
- Agent must run as root to attach cgroup/sockops (scripts/start-tracking.sh uses sudo).
- cgroup v2 must be mounted at /sys/fs/cgroup.
- XDP may fail to attach on some interfaces; it’s optional and logged as a warning.
- libpcap is required for HTTP path sniffing (install: apt-get install -y libpcap-dev).

Important source files
- Agent entry: cmd/agent/main.go
- Operator entry + HTTP API: cmd/operator/main.go, cmd/operator/grpc_server.go
- eBPF core: internal/ebpf/dependency_tracer.c, internal/ebpf/dependency_tracer.go
- Userspace HTTP sniffer: internal/pathsniff/sniffer.go
- Graph logic: internal/graph/*.go
- Protobuf: pkg/proto/metrics.proto
- UI: web/dashboard.html

API endpoints
- GET /health → { service, status }
- GET /api/dependency-graph → nodes[], edges[], totals, stats (edges include http_paths when available)
- GET /api/metrics → legacy cumulative metrics (optional)
- GET /api/http-metrics → recent per-request samples (optional)

UI features
- Pan/zoom force-directed graph with node sizing from traffic.
- Edge tooltips show request_count, strength, and any HTTP paths seen.
- HTTP Path Filter: highlight edges and participating services by METHOD/PATH and show a service chain summary.

Limitations / future work
- HTTP parsing is best-effort: only the first request line is parsed; bodies and TLS are not parsed.
- Loopback IPv4 is targeted; IPv6 and non-loopback captures may need additional handling.
- Correlation falls back to destination-port cache if the client ephemeral port changes quickly.
- RTT/latency from TCP sockops can be added with further sockops callbacks.
- Kubernetes enrichment can map ip:port to (ns/svc/pod) using informers.
