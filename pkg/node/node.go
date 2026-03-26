package node

import (
	"log/slog"
	"math"
	"math/rand"
	"sync"
	"time"

	"github.com/ryandielhenn/zephyrcache/pkg/gossip"
	"github.com/ryandielhenn/zephyrcache/pkg/kv"
	"github.com/ryandielhenn/zephyrcache/pkg/peer"
	"github.com/ryandielhenn/zephyrcache/pkg/ring"
)

type Node struct {
	kv           *kv.Store
	ring         *ring.HashRing
	gossipQueue  []*gossip.MessagePayload
	maxGossipLen int
	targetPeer   string
	peers        map[string]peer.Peer
	id           string
	addr         string
	incarnation  int
	timeout      *time.Timer
	gossipPort   string
	mu           sync.Mutex
}

func NewNode(store *kv.Store, r *ring.HashRing, id string, addr string, gossipPort string) *Node {
	return &Node{
		kv:           store,
		ring:         r,
		gossipQueue:  make([]*gossip.MessagePayload, 0),
		maxGossipLen: 50,
		peers:        make(map[string]peer.Peer),
		id:           id,
		addr:         addr,
		incarnation:  0,
		gossipPort:   gossipPort,
	}
}

func (n *Node) enqGossip(newMsg *gossip.MessagePayload) {
	if newMsg == nil {
		return
	}
	for _, oldMsg := range n.gossipQueue {
		if oldMsg == nil {
			continue
		}
		// replace stale updates about peers in old message
		for id, newPeer := range newMsg.Peers {
			if oldPeer, ok := oldMsg.Peers[id]; ok {
				if newPeer.Supersedes(oldPeer) {
					oldMsg.Peers[id] = newPeer
					oldMsg.TransmitCount = 1
				}
				delete(newMsg.Peers, id)
			}
		}
	}
	if len(newMsg.Peers) == 0 || len(n.gossipQueue) == n.maxGossipLen {
		return
	}
	n.gossipQueue = append(n.gossipQueue, newMsg)
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
	count := int(math.Floor(3 * math.Log2(float64(n.countPeers()))))
	if msg.TransmitCount > 0 && msg.TransmitCount <= count {
		msg.TransmitCount += 1
		n.gossipQueue = append(n.gossipQueue, msg)
	}
	return msg
}

func (n *Node) countPeers() int {
	count := 0
	for _, peerBody := range n.peers {
		if peerBody.Status != peer.Dead {
			count += 1
		}
	}
	return count
}

func (n *Node) setPeer(id string, updatedPeer peer.Peer) {
	currentPeer, ok := n.peers[id]

	shouldRemove := ok && currentPeer.Status != peer.Dead &&
		updatedPeer.Status == peer.Dead
	if shouldRemove {
		n.ring.Remove(id)
	}

	shouldAdd := (!ok || currentPeer.Status == peer.Dead) &&
		updatedPeer.Status != peer.Dead
	if shouldAdd {
		n.ring.Add(id, updatedPeer.Addr)
	}

	n.peers[id] = updatedPeer

	if shouldRemove || shouldAdd {
		peerIds := n.getPeerList()
		slog.Info("Peers", "peer ids", peerIds)
	}
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
	peerIds := n.getPeerList()
	if len(peerIds) == 0 {
		return ""
	}
	return peerIds[rand.Intn(len(peerIds))]
}

func (n *Node) getKRandomPeers(k int) []string {
	peerIds := n.getPeerList()
	rand.Shuffle(len(peerIds), func(i, j int) {
		peerIds[i], peerIds[j] = peerIds[j], peerIds[i]
	})
	if k > len(peerIds) {
		k = len(peerIds)
	}
	return peerIds[:k]
}
