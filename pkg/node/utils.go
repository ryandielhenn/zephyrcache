package node

import (
	"cmp"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// normalizeHostPort cuts the http:// https:// prefixes from the input address
// adds a default port
func NormalizeHostPort(addr, defPort string) string {
	if rest, ok := strings.CutPrefix(addr, "http://"); ok {
		addr = rest
	} else if rest, ok := strings.CutPrefix(addr, "https://"); ok {
		addr = rest
	}

	if _, _, err := net.SplitHostPort(addr); err == nil {
		return addr
	}

	return addr + ":" + defPort
}

func OverrideHostPort(addr, port string) string {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return addr + ":" + port
	}
	return net.JoinHostPort(host, port)
}

// ownerForKey looks up the owner for a key and normalizes the address of the owner
func (n *Node) OwnerForKey(key string) (ownerHP, selfHP string, ok bool) {
	n.mu.Lock()
	defer n.mu.Unlock()
	ownerID := n.ring.Lookup([]byte(key)) // e.g. "Node3"
	ownerAddr, ok := n.ring.Addr(ownerID) // e.g. "Node3:8080" (what you stored)
	if !ok || ownerAddr == "" {
		return "", "", false
	}
	return NormalizeHostPort(ownerAddr, "8080"), NormalizeHostPort(n.config.addr, "8080"), true
}

// replicas looks up the replicas for a key and normalizes their addresses
func (n *Node) ReplicasForKey(key string) (replicaAddrs []string) {
	replicaIds := n.ring.LookupN([]byte(key), n.config.nReplicas) // e.g. "Node3"

	addrs := make([]string, len(replicaIds))
	for i := range len(replicaIds) {
		addr, ok := n.ring.Addr(replicaIds[i]) // e.g. "Node3:8080" (what you stored)
		if !ok || addr == "" {
			return nil
		}
		addrs[i] = NormalizeHostPort(addr, "8080")

	}
	return addrs
}

func readBody(req *http.Request) ([]byte, error) {
	b, err := io.ReadAll(req.Body)
	if err != nil && err.Error() != "EOF" {
		return nil, err
	}
	return b, nil
}

func parseTTL(req *http.Request) (time.Duration, error) {
	ttlStr := req.URL.Query().Get("ttl")
	if ttlStr == "" {
		return 0, nil
	}
	sec, err := strconv.Atoi(ttlStr)
	if err != nil {
		return 0, fmt.Errorf("invalid ttl")
	}
	return time.Duration(sec) * time.Second, nil
}

// Generate node config from environment variables
func Config() *NodeConfig {
	id := os.Getenv("SELF_ID")
	addr := os.Getenv("SELF_ADDR")
	gossipPort := cmp.Or(os.Getenv("GOSSIP_PORT"), "4000")
	replicationFactor, err := strconv.Atoi(os.Getenv("REPLICATION_FACTOR"))
	if err != nil {
		slog.Warn("REPLICATION_FACTOR should be an int, could not parse, defaulting to 3")
		replicationFactor = 3
	}
	return &NodeConfig{
		maxGossipLen: 50,
		id:           id,
		addr:         addr,
		nReplicas:    replicationFactor,
		gossipPort:   gossipPort,
	}
}

// Generate node config, manual passing of configs for tests/benchmarks
func ConfigWithOpts(id, addr, gossipPort string, nReplicas, gossipQueueLen int) *NodeConfig {
	return &NodeConfig{
		maxGossipLen: gossipQueueLen,
		id:           id,
		addr:         addr,
		nReplicas:    nReplicas,
		gossipPort:   gossipPort,
	}
}
