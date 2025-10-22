//go:build ignore

#include <linux/bpf.h>
#include <linux/ptrace.h>
#include <linux/types.h>
#include <linux/if_ether.h>
#include <linux/ip.h>
#include <linux/tcp.h>
#include <linux/udp.h>
#include <linux/in.h>
#include <linux/socket.h>
#include <linux/pkt_cls.h>
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_tracing.h>
#include <bpf/bpf_endian.h>

// Minimal sys_enter ctx for tracepoints
struct trace_event_raw_sys_enter {
    unsigned long __unused[4];
    long id;
    unsigned long args[6];
};

// Define missing types
typedef unsigned long size_t;

#define MAX_ENTRIES 10000
#define MAX_FLOW_ENTRIES 50000
#define MAX_SERVICE_ENTRIES 1000
#define MAX_DEPENDENCY_ENTRIES 5000

// Flow identification structure
struct flow_key {
    __u32 saddr;
    __u32 daddr;
    __u16 sport;
    __u16 dport;
    __u8 protocol;
};

// Flow statistics and metadata
struct flow_info {
    __u64 first_seen;
    __u64 last_seen;
    __u64 bytes_sent;
    __u64 packets_sent;
    __u64 bytes_recv;
    __u64 packets_recv;
    __u32 src_pid;
    __u32 dst_pid;
    char src_comm[16];
    char dst_comm[16];
    char http_method[8];
    char http_path[32];
    __u8 flow_state; // 0: new, 1: established, 2: finished
};

// Service identification
struct service_key {
    __u32 ip;
    __u16 port;
};

struct service_info {
    __u64 first_seen;
    __u64 last_seen;
    __u64 total_requests;
    __u64 total_responses;
    char service_name[32];
    char process_name[16];
    __u32 pid;
    __u8 service_type; // 0: unknown, 1: http, 2: database, 3: cache
};

// Dependency relationship
struct dependency_key {
    struct service_key source;
    struct service_key dest;
};

struct dependency_info {
    __u64 first_seen;
    __u64 last_seen;
    __u64 request_count;
    __u64 total_latency_ns;
    __u64 avg_latency_ns;
    __u64 error_count;
    char protocol[8];
    __u8 relationship_strength; // 1-10 based on frequency
};


// Event structures for userspace
struct flow_event {
    __u64 timestamp;
    struct flow_key key;
    struct flow_info info;
    __u8 event_type; // 0: new_flow, 1: flow_update, 2: flow_close
};

struct dependency_event {
    __u64 timestamp;
    struct dependency_key key;
    struct dependency_info info;
    __u8 event_type; // 0: new_dependency, 1: dependency_update
};

// Maps for flow tracking
struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, MAX_FLOW_ENTRIES);
    __type(key, struct flow_key);
    __type(value, struct flow_info);
} flow_map SEC(".maps");

// Maps for service tracking
struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, MAX_SERVICE_ENTRIES);
    __type(key, struct service_key);
    __type(value, struct service_info);
} service_map SEC(".maps");

// Maps for dependency tracking
struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, MAX_DEPENDENCY_ENTRIES);
    __type(key, struct dependency_key);
    __type(value, struct dependency_info);
} dependency_map SEC(".maps");

// Cookie map to correlate connect4 (dest) with sockops (established)
struct cookie_meta {
    __u16 dport;
    __u32 daddr;
    __u32 pid;
    char comm[16];
};

struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, MAX_FLOW_ENTRIES);
    __type(key, __u64);
    __type(value, struct cookie_meta);
} cookie_map SEC(".maps");


// PID map as fallback correlation when cookie is unavailable
struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, MAX_FLOW_ENTRIES);
    __type(key, __u32);
    __type(value, struct cookie_meta);
} pid_map SEC(".maps");

// Event output maps
struct {
    __uint(type, BPF_MAP_TYPE_PERF_EVENT_ARRAY);
    __uint(key_size, sizeof(__u32));
    __uint(value_size, sizeof(__u32));
} flow_events SEC(".maps");

struct {
    __uint(type, BPF_MAP_TYPE_PERF_EVENT_ARRAY);
    __uint(key_size, sizeof(__u32));
    __uint(value_size, sizeof(__u32));
} dependency_events SEC(".maps");

// Helper functions
static __always_inline int is_microservice_port(__u16 port) {
    return (port >= 8000 && port <= 9999) ||
           (port >= 3000 && port <= 3999) ||
           (port >= 5000 && port <= 5999) ||
           port == 80 || port == 8080 || port == 443;
}

static __always_inline void identify_service_type(struct service_info *service, __u16 port) {
    if (port == 80 || port == 8080 || port == 443 || (port >= 8000 && port <= 8999)) {
        service->service_type = 1; // HTTP service
    } else if (port == 3306 || port == 5432 || port == 27017) {
        service->service_type = 2; // Database
    } else if (port == 6379 || port == 11211) {
        service->service_type = 3; // Cache
    } else {
        service->service_type = 0; // Unknown
    }
}

static __always_inline void extract_http_info(const char *data, int len, struct flow_info *flow) {
    // Simple HTTP method extraction
    if (len >= 4) {
        if (data[0] == 'G' && data[1] == 'E' && data[2] == 'T' && data[3] == ' ') {
            __builtin_memcpy(flow->http_method, "GET", 4);
        } else if (data[0] == 'P' && data[1] == 'O' && data[2] == 'S' && data[3] == 'T') {
            __builtin_memcpy(flow->http_method, "POST", 4);
        } else if (data[0] == 'P' && data[1] == 'U' && data[2] == 'T' && data[3] == ' ') {
            __builtin_memcpy(flow->http_method, "PUT", 4);
        } else if (data[0] == 'D' && data[1] == 'E' && data[2] == 'L') {
            __builtin_memcpy(flow->http_method, "DELETE", 6);
        }
        
        // Extract HTTP path
        for (int i = 4; i < len - 1 && i < 67; i++) {
            if (data[i] == ' ' && data[i+1] == '/') {
                int path_len = 0;
                for (int j = i+1; j < len && j < i+64 && data[j] != ' ' && data[j] != '\n'; j++, path_len++) {
                    flow->http_path[path_len] = data[j];
                }
                flow->http_path[path_len] = '\0';
                break;
            }
        }
    }
}

// XDP program for early packet processing (Cilium-style)
SEC("xdp")
int xdp_dependency_tracker(struct xdp_md *ctx) {
    void *data_end = (void *)(long)ctx->data_end;
    void *data = (void *)(long)ctx->data;
    
    struct ethhdr *eth = data;
    if ((void *)(eth + 1) > data_end)
        return XDP_PASS;
    
    if (eth->h_proto != __constant_htons(ETH_P_IP))
        return XDP_PASS;
        
    struct iphdr *ip = (void *)(eth + 1);
    if ((void *)(ip + 1) > data_end)
        return XDP_PASS;
    
    // Only track TCP traffic for HTTP services
    if (ip->protocol != IPPROTO_TCP)
        return XDP_PASS;
        
    struct tcphdr *tcp = (void *)(ip + 1);
    if ((void *)(tcp + 1) > data_end)
        return XDP_PASS;
    
    __u16 sport = __bpf_ntohs(tcp->source);
    __u16 dport = __bpf_ntohs(tcp->dest);
    
    // Only track microservice traffic
    if (!is_microservice_port(sport) && !is_microservice_port(dport))
        return XDP_PASS;
    
    struct flow_key key = {
        .saddr = ip->saddr,
        .daddr = ip->daddr,
        .sport = sport,
        .dport = dport,
        .protocol = ip->protocol
    };
    
    struct flow_info *flow = bpf_map_lookup_elem(&flow_map, &key);
    __u64 now = bpf_ktime_get_ns();
    
    if (!flow) {
        // New flow
        struct flow_info new_flow = {
            .first_seen = now,
            .last_seen = now,
            .bytes_sent = __bpf_ntohs(ip->tot_len),
            .packets_sent = 1,
            .bytes_recv = 0,
            .packets_recv = 0,
            .flow_state = 0
        };
        
        bpf_map_update_elem(&flow_map, &key, &new_flow, BPF_ANY);
        
        // Emit new flow event
        struct flow_event event = {
            .timestamp = now,
            .key = key,
            .info = new_flow,
            .event_type = 0
        };
        bpf_perf_event_output(ctx, &flow_events, BPF_F_CURRENT_CPU, &event, sizeof(event));
    } else {
        // Update existing flow
        flow->last_seen = now;
        flow->bytes_sent += __bpf_ntohs(ip->tot_len);
        flow->packets_sent++;
        
        bpf_map_update_elem(&flow_map, &key, flow, BPF_EXIST);
    }
    
    return XDP_PASS;
}

// TC program for established connection tracking
SEC("tc")
int tc_dependency_tracker(struct __sk_buff *skb) {
    void *data_end = (void *)(long)skb->data_end;
    void *data = (void *)(long)skb->data;
    
    struct ethhdr *eth = data;
    if ((void *)(eth + 1) > data_end)
        return TC_ACT_OK;
    
    if (eth->h_proto != __constant_htons(ETH_P_IP))
        return TC_ACT_OK;
        
    struct iphdr *ip = (void *)(eth + 1);
    if ((void *)(ip + 1) > data_end)
        return TC_ACT_OK;
    
    if (ip->protocol != IPPROTO_TCP)
        return TC_ACT_OK;
        
    struct tcphdr *tcp = (void *)(ip + 1);
    if ((void *)(tcp + 1) > data_end)
        return TC_ACT_OK;
    
    __u16 sport = __bpf_ntohs(tcp->source);
    __u16 dport = __bpf_ntohs(tcp->dest);
    
    if (!is_microservice_port(sport) && !is_microservice_port(dport))
        return TC_ACT_OK;
    
    // Track service endpoints
    struct service_key src_svc = { .ip = ip->saddr, .port = sport };
    struct service_key dst_svc = { .ip = ip->daddr, .port = dport };
    
    __u64 now = bpf_ktime_get_ns();
    
    // Update source service
    struct service_info *src_info = bpf_map_lookup_elem(&service_map, &src_svc);
    if (!src_info) {
        struct service_info new_svc = {
            .first_seen = now,
            .last_seen = now,
            .total_requests = 1,
            .total_responses = 0
        };
        identify_service_type(&new_svc, sport);
        bpf_map_update_elem(&service_map, &src_svc, &new_svc, BPF_ANY);
    } else {
        src_info->last_seen = now;
        src_info->total_requests++;
        bpf_map_update_elem(&service_map, &src_svc, src_info, BPF_EXIST);
    }
    
    // Update destination service
    struct service_info *dst_info = bpf_map_lookup_elem(&service_map, &dst_svc);
    if (!dst_info) {
        struct service_info new_svc = {
            .first_seen = now,
            .last_seen = now,
            .total_requests = 0,
            .total_responses = 1
        };
        identify_service_type(&new_svc, dport);
        bpf_map_update_elem(&service_map, &dst_svc, &new_svc, BPF_ANY);
    } else {
        dst_info->last_seen = now;
        dst_info->total_responses++;
        bpf_map_update_elem(&service_map, &dst_svc, dst_info, BPF_EXIST);
    }
    
    // Track dependency relationship
    struct dependency_key dep_key = { .source = src_svc, .dest = dst_svc };
    struct dependency_info *dep_info = bpf_map_lookup_elem(&dependency_map, &dep_key);
    
    if (!dep_info) {
        struct dependency_info new_dep = {
            .first_seen = now,
            .last_seen = now,
            .request_count = 1,
            .total_latency_ns = 0,
            .avg_latency_ns = 0,
            .error_count = 0,
            .relationship_strength = 1
        };
        __builtin_memcpy(new_dep.protocol, "TCP", 4);
        bpf_map_update_elem(&dependency_map, &dep_key, &new_dep, BPF_ANY);
        
        // Emit new dependency event
        struct dependency_event event = {
            .timestamp = now,
            .key = dep_key,
            .info = new_dep,
            .event_type = 0
        };
        bpf_perf_event_output(skb, &dependency_events, BPF_F_CURRENT_CPU, &event, sizeof(event));
    } else {
        dep_info->last_seen = now;
        dep_info->request_count++;
        // Calculate relationship strength based on frequency
        if (dep_info->request_count > 100) dep_info->relationship_strength = 10;
        else if (dep_info->request_count > 50) dep_info->relationship_strength = 8;
        else if (dep_info->request_count > 20) dep_info->relationship_strength = 6;
        else if (dep_info->request_count > 10) dep_info->relationship_strength = 4;
        else if (dep_info->request_count > 5) dep_info->relationship_strength = 2;
        
        bpf_map_update_elem(&dependency_map, &dep_key, dep_info, BPF_EXIST);
    }
    
    return TC_ACT_OK;
}

// Enhanced connection tracking with HTTP payload analysis
SEC("tracepoint/syscalls/sys_enter_sendto")
int trace_sendto_enhanced(void *raw_ctx) {
    // Disabled HTTP path capture to reduce stack usage
    return 0;
}

// Socket address tracking for cgroup programs
SEC("cgroup/connect4")
int cgroup_connect_tracker(struct bpf_sock_addr *ctx) {
    __u32 pid = bpf_get_current_pid_tgid() >> 32;
    
    // Only track connections to microservice ports
    __u16 port = ctx->user_port;
    if (!is_microservice_port(port))
        return 1; // Allow connection
    
    __u64 now = bpf_ktime_get_ns();
    
    // Track the service being connected to
    struct service_key svc_key = {
        .ip = ctx->user_ip4,
        .port = port
    };

    // Save cookie metadata for later established callback
    __u64 cookie = bpf_get_socket_cookie(ctx);
    struct cookie_meta meta = { .dport = port, .daddr = ctx->user_ip4, .pid = pid };
    bpf_get_current_comm(&meta.comm, sizeof(meta.comm));
    if (cookie) {
        bpf_map_update_elem(&cookie_map, &cookie, &meta, BPF_ANY);
    }
    // Also store by PID as fallback
    bpf_map_update_elem(&pid_map, &pid, &meta, BPF_ANY);
    
    struct service_info *svc = bpf_map_lookup_elem(&service_map, &svc_key);
    if (!svc) {
        struct service_info new_svc = {
            .first_seen = now,
            .last_seen = now,
            .total_requests = 1,
            .pid = pid
        };
        identify_service_type(&new_svc, port);
        bpf_get_current_comm(&new_svc.process_name, sizeof(new_svc.process_name));
        bpf_map_update_elem(&service_map, &svc_key, &new_svc, BPF_ANY);
    } else {
        svc->last_seen = now;
        svc->total_requests++;
        bpf_map_update_elem(&service_map, &svc_key, svc, BPF_EXIST);
    }

    // Emit a minimal flow event for dependency tracking (best-effort)
    struct flow_key key = {
        .saddr = 0,                 // unknown at connect time
        .daddr = ctx->user_ip4,
        .sport = 0,                 // kernel-assigned later
        .dport = port,
        .protocol = IPPROTO_TCP,
    };

    // Do not emit incomplete flow event here; we only use connect4 to seed maps
    // (cookie_map/pid_map/service_map). Sockops will emit established flows.
    
    return 1; // Allow connection
}

// Sockops program to capture established 4-tuple on loopback and others
SEC("sockops")
int sockops_tracker(struct bpf_sock_ops *skops) {
    if (skops->op != BPF_SOCK_OPS_ACTIVE_ESTABLISHED_CB) {
        return 0;
    }

    __u64 now = bpf_ktime_get_ns();

    // Build flow key
    struct flow_key key = {};
    if (skops->family == 2 /* AF_INET */) {
        key.saddr = skops->local_ip4;
        key.daddr = skops->remote_ip4;
        key.sport = __bpf_ntohs(skops->local_port);
        // Try to get reliable dport from cookie map (populated at connect4)
        __u64 cookie = bpf_get_socket_cookie(skops);
        __u16 dport = 0;
        if (cookie) {
            struct cookie_meta *m = bpf_map_lookup_elem(&cookie_map, &cookie);
            if (m) {
                dport = m->dport;
                if (m->daddr) {
                    key.daddr = m->daddr;
                }
            }
        }
        if (dport == 0) {
            // Fallback to PID-based correlation
            __u32 pid = bpf_get_current_pid_tgid() >> 32;
            struct cookie_meta *pm = bpf_map_lookup_elem(&pid_map, &pid);
            if (pm) {
                dport = pm->dport;
                if (pm->daddr) {
                    key.daddr = pm->daddr;
                }
            }
        }
        if (dport == 0) {
            // Fallback: remote_port in sockops is 32-bit BE; try both halves
            __u32 rp_be32 = skops->remote_port;
            __u32 rp = __bpf_ntohl(rp_be32);
            __u16 hi = (__u16)(rp >> 16);
            __u16 lo = (__u16)(rp & 0xFFFF);
            if (is_microservice_port(hi)) {
                dport = hi;
            } else if (is_microservice_port(lo)) {
                dport = lo;
            } else {
                dport = hi; // default
            }
        }
        key.dport = dport;
        key.protocol = IPPROTO_TCP;
    } else {
        return 0; // only IPv4 for now
    }

    // Only track microservice ports
    if (!is_microservice_port(key.sport) && !is_microservice_port(key.dport)) {
        return 0;
    }

    struct flow_info info = {
        .first_seen = now,
        .last_seen = now,
        .bytes_sent = 0,
        .packets_sent = 0,
        .bytes_recv = 0,
        .packets_recv = 0,
        .flow_state = 1, // established
    };
    // Fill src pid/comm from cookie/pid mapping if available
    __u64 cookie2 = bpf_get_socket_cookie(skops);
    if (cookie2) {
        struct cookie_meta *m2 = bpf_map_lookup_elem(&cookie_map, &cookie2);
        if (m2) {
            info.src_pid = m2->pid;
            __builtin_memcpy(&info.src_comm, m2->comm, sizeof(info.src_comm));
        }
    }
    if (info.src_pid == 0) {
        __u32 curpid = bpf_get_current_pid_tgid() >> 32;
        struct cookie_meta *pm2 = bpf_map_lookup_elem(&pid_map, &curpid);
        if (pm2) {
            info.src_pid = pm2->pid;
            __builtin_memcpy(&info.src_comm, pm2->comm, sizeof(info.src_comm));
        }
    }

    struct flow_event evt = {
        .timestamp = now,
        .key = key,
        .info = info,
        .event_type = 1, // flow_update/established
    };
    bpf_perf_event_output(skops, &flow_events, BPF_F_CURRENT_CPU, &evt, sizeof(evt));

    return 0;
}

// Record listening ports to map server processes → ports
SEC("cgroup/bind4")
int cgroup_bind_tracker(struct bpf_sock_addr *ctx) {
    __u32 pid = bpf_get_current_pid_tgid() >> 32;
    __u16 port = ctx->user_port;
    if (!is_microservice_port(port))
        return 1;
    __u64 now = bpf_ktime_get_ns();
    struct service_key svc_key = { .ip = ctx->user_ip4, .port = port };
    struct service_info *svc = bpf_map_lookup_elem(&service_map, &svc_key);
    if (!svc) {
        struct service_info new_svc = {
            .first_seen = now,
            .last_seen = now,
            .total_requests = 0,
            .total_responses = 0,
            .pid = pid
        };
        identify_service_type(&new_svc, port);
        bpf_get_current_comm(&new_svc.process_name, sizeof(new_svc.process_name));
        bpf_map_update_elem(&service_map, &svc_key, &new_svc, BPF_ANY);
    } else {
        svc->last_seen = now;
        svc->pid = pid;
        bpf_get_current_comm(&svc->process_name, sizeof(svc->process_name));
        bpf_map_update_elem(&service_map, &svc_key, svc, BPF_EXIST);
    }
    return 1;
}

char _license[] SEC("license") = "GPL";
