package node

import (
	"log"
	"math"
	"math/rand"
	"time"

	"github.com/ryandielhenn/zephyrcache/pkg/gossip"
	"github.com/ryandielhenn/zephyrcache/pkg/kv"
	"github.com/ryandielhenn/zephyrcache/pkg/peer"
	"github.com/ryandielhenn/zephyrcache/pkg/ring"
)

type Node struct {
	kv          *kv.Store
	ring        *ring.HashRing
	gossipQueue []*gossip.MessagePayload
	suspectPeer string
	peers       map[string]peer.Peer
	id          string
	addr        string
	incarnation int
	timeout     *time.Timer
	gossipPort  string
}

func NewNode(store *kv.Store, r *ring.HashRing, id string, addr string, gossipPort string) *Node {
	return &Node{
		kv:          store,
		ring:        r,
		gossipQueue: make([]*gossip.MessagePayload, 0),
		suspectPeer: "",
		peers:       make(map[string]peer.Peer),
		id:          id,
		addr:        addr,
		incarnation: 0,
		gossipPort:  gossipPort,
	}
}

func (n *Node) addGossip(msg *gossip.MessagePayload) {
	n.gossipQueue = append(n.gossipQueue, msg)
}

func (n *Node) prependGossip(msg *gossip.MessagePayload) {
	n.gossipQueue = append([]*gossip.MessagePayload{msg}, n.gossipQueue...)
}

func (n *Node) removeGossip() *gossip.MessagePayload {
	if len(n.gossipQueue) == 0 {
		return nil
	}
	msg := n.gossipQueue[0]
	n.gossipQueue = n.gossipQueue[1:]
	count := int(math.Floor(3 * math.Log2(float64(len(n.peers)))))
	if msg.TransmitCount > 0 && msg.TransmitCount <= count {
		msg.TransmitCount += 1
		n.gossipQueue = append(n.gossipQueue, msg)
	}

	return msg
}

func (n *Node) setPeer(id string, peerBody peer.Peer) {
	_, ok := n.peers[id]
	if ok {
		n.ring.Remove(id)
	}
	if peerBody.Status == peer.Alive {
		n.ring.Add(id, peerBody.Addr)
	}
	n.peers[id] = peerBody
	peerIds := n.getPeerList()
	log.Printf("%+v", peerIds)
}

func (n *Node) addPeer(id string, peerHP string) {
	n.setPeer(id, peer.Peer{Addr: peerHP, Status: peer.Alive, Incarnation: 0})
}

// SyncPeers updates the ring incrementally using the diff between current and new peers.
// This is O((added + removed) * replicas) instead of O(all_peers * replicas).
func (n *Node) syncPeers(newPeers map[string]string) {
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

func (n *Node) getPeerMap() map[string]peer.Peer {
	peerMap := make(map[string]peer.Peer)
	for peerId, peerBody := range n.peers {
		if peerBody.Status == peer.Dead {
			continue
		}
		peerMap[peerId] = peerBody
	}
	return peerMap
}

func (n *Node) getPeerList() []string {
	peerIds := make([]string, 0, len(n.peers))
	for peerId, peerBody := range n.peers {
		if peerBody.Status == peer.Dead {
			continue
		}
		peerIds = append(peerIds, peerId)
	}
	return peerIds
}

func (n *Node) getRandomPeer() string {
	if len(n.peers) == 0 {
		return ""
	}
	peerIds := n.getPeerList()
	if len(peerIds) == 0 {
		return ""
	}
	return peerIds[rand.Intn(len(peerIds))]
}

func (n *Node) getKRandomPeers(k int) []string {
	if len(n.peers) == 0 {
		return make([]string, 0)
	}
	peerIds := n.getPeerList()
	rand.Shuffle(len(peerIds), func(i, j int) {
		peerIds[i], peerIds[j] = peerIds[j], peerIds[i]
	})
	if k > len(peerIds) {
		k = len(peerIds)
	}
	return peerIds[:k]
}
