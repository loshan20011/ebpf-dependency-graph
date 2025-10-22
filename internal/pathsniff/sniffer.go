package pathsniff

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
)

// Key identifies a flow tuple
type key struct {
	srcIP   string
	srcPort uint16
	dstIP   string
	dstPort uint16
}

// entry stores parsed HTTP request line with TTL
type entry struct {
	method string
	path   string
	expiry time.Time
}

// Collector sniffs HTTP request lines on loopback and caches method/path per flow
type Collector struct {
	iface      string
	ports      map[uint16]struct{}
	ttl        time.Duration
	mu         sync.RWMutex
	cache      map[key]entry
	byDst      map[uint16]entry
	stopOnce   sync.Once
}

// NewCollector creates a path collector for localhost ports
func NewCollector(ports []int) *Collector {
	m := make(map[uint16]struct{}, len(ports))
	for _, p := range ports {
		m[uint16(p)] = struct{}{}
	}
	return &Collector{
		iface: "lo",
		ports: m,
		ttl:   30 * time.Second,
		cache: make(map[key]entry, 1024),
		byDst: make(map[uint16]entry, 64),
	}
}

// Start begins sniffing on loopback; non-fatal if pcap open fails
func (c *Collector) Start(ctx context.Context) error {
	go c.cleanupLoop(ctx)
	go c.sniffLoop(ctx)
	return nil
}

// Lookup returns a recent method/path for the given tuple
func (c *Collector) Lookup(srcIP string, srcPort uint16, dstIP string, dstPort uint16) (string, string, bool) {
	k := key{srcIP: srcIP, srcPort: srcPort, dstIP: dstIP, dstPort: dstPort}
	c.mu.RLock()
	e, ok := c.cache[k]
	if ok && !time.Now().After(e.expiry) {
		c.mu.RUnlock()
		return e.method, e.path, true
	}
	// Fallback: match by destination port only
	e2, ok2 := c.byDst[dstPort]
	c.mu.RUnlock()
	if ok2 && !time.Now().After(e2.expiry) {
		return e2.method, e2.path, true
	}
	return "", "", false
}

func (c *Collector) cleanupLoop(ctx context.Context) {
	t := time.NewTicker(2 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			now := time.Now()
			c.mu.Lock()
			for k, v := range c.cache {
				if now.After(v.expiry) {
					delete(c.cache, k)
				}
			}
			for p, v := range c.byDst {
				if now.After(v.expiry) {
					delete(c.byDst, p)
				}
			}
			c.mu.Unlock()
		}
	}
}

func (c *Collector) sniffLoop(ctx context.Context) {
	h, err := pcap.OpenLive(c.iface, 65535, true, pcap.BlockForever)
	if err != nil {
		return
	}
	defer h.Close()

	// Broad TCP filter; we filter ports in-process
	_ = h.SetBPFFilter("tcp")


	src := gopacket.NewPacketSource(h, h.LinkType())
	src.NoCopy = true
	for {
		select {
		case <-ctx.Done():
			return
		case pkt, ok := <-src.Packets():
			if !ok {
				return
			}
			c.process(pkt)
		}
	}
}

func (c *Collector) process(pkt gopacket.Packet) {
	ip4 := pkt.Layer(layers.LayerTypeIPv4)
	if ip4 == nil {
		return
	}
	ipv4 := ip4.(*layers.IPv4)
	tcpL := pkt.Layer(layers.LayerTypeTCP)
	if tcpL == nil {
		return
	}
	tcp := tcpL.(*layers.TCP)
	if _, ok := c.ports[uint16(tcp.DstPort)]; !ok {
		return
	}
	// Parse first line of HTTP request from payload
	payload := tcp.Payload
	if len(payload) < 8 {
		return
	}
	method, path := parseRequestLine(payload)
	if method == "" || path == "" {
		return
	}
	k := key{
		srcIP:   ipv4.SrcIP.String(),
		srcPort: uint16(tcp.SrcPort),
		dstIP:   ipv4.DstIP.String(),
		dstPort: uint16(tcp.DstPort),
	}
	now := time.Now()
	c.mu.Lock()
	ent := entry{method: method, path: path, expiry: now.Add(c.ttl)}
	c.cache[k] = ent
	c.byDst[uint16(tcp.DstPort)] = ent
	c.mu.Unlock()
}

func parseRequestLine(b []byte) (string, string) {
	// METHOD SP PATH SP
	space := bytesIndexByte(b, ' ')
	if space <= 0 || space > 8 {
		return "", ""
	}
	method := string(b[:space])
	if !isMethod(method) {
		return "", ""
	}
	b = b[space+1:]
	space2 := bytesIndexByte(b, ' ')
	if space2 <= 0 {
		return method, ""
	}
	path := string(b[:space2])
	if !strings.HasPrefix(path, "/") {
		return method, ""
	}
	if len(path) > 64 {
		path = path[:64]
	}
	return method, path
}

func isMethod(m string) bool {
	switch m {
	case "GET", "POST", "PUT", "DELETE", "PATCH", "HEAD":
		return true
	default:
		return false
	}
}

func bytesIndexByte(b []byte, c byte) int {
	for i := 0; i < len(b); i++ {
		if b[i] == c {
			return i
		}
	}
	return -1
}
