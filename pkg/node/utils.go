package node

import (
	"net"
	"strings"
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
func (s *Node) OwnerForKey(key string) (ownerHP, selfHP string, ok bool) {
	ownerID := s.ring.Lookup([]byte(key)) // e.g. "Node3"
	ownerAddr, ok := s.ring.Addr(ownerID) // e.g. "Node3:8080" (what you stored)
	if !ok || ownerAddr == "" {
		return "", "", false
	}
	return NormalizeHostPort(ownerAddr, "8080"), NormalizeHostPort(s.addr, "8080"), true
}

// replicas looks up the replicas for a key and normalizes their addresses
func (n *Node) ReplicasForKey(key string, replicas int) (replicaAddrs []string) {
	replicaIds := n.ring.LookupN([]byte(key), replicas) // e.g. "Node3"

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
