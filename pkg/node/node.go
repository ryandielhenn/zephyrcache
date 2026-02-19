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

func (n *Node) ClearPeers() {
	n.ring.Clear()
}

func (n *Node) Addr() string {
	return n.addr
}
