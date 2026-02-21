package node

import (
	"github.com/ryandielhenn/zephyrcache/pkg/gossip"
	"github.com/ryandielhenn/zephyrcache/pkg/kv"
	"github.com/ryandielhenn/zephyrcache/pkg/ring"
)

type Node struct {
	kv    *kv.Store
	ring  *ring.HashRing
	peers []string
	addr  string
	gsp   gossip.Gossip
	rf    int
}

func NewNode(store *kv.Store, r *ring.HashRing, addr string) *Node {
	return NewNodeRF(store, r, addr, 3)
}

func NewNodeRF(store *kv.Store, r *ring.HashRing, addr string, replicationFactor int) *Node {
	return &Node{
		kv:   store,
		ring: r,
		addr: addr,
		rf:   replicationFactor,
	}
}

func (n *Node) AddPeer(id string, hostport string) {
	n.ring.Add(id, hostport)
}

func (n *Node) RemovePeer(id string) {
	n.ring.Remove(id)
}

func (n *Node) ClearPeers() {
	n.ring.Clear()
}

// SyncPeers updates the ring incrementally by computing diff between current and new peers.
// Added peers are added to the ring, removed peers are removed.
// This is O((added + removed) * replicas) instead of O(all_peers * replicas).
func (n *Node) SyncPeers(newPeers map[string]string) {
	// Find removed peers (in current ring but not in new peers)
	for id := range n.ring.Nodes() {
		if _, ok := newPeers[id]; !ok {
			n.ring.Remove(id)
		}
	}

	// Find added peers (in new peers but not in current ring)
	for id, addr := range newPeers {
		if _, ok := n.ring.Addr(id); !ok {
			n.ring.Add(id, addr)
		}
	}
}

func (n *Node) Addr() string {
	return n.addr
}
