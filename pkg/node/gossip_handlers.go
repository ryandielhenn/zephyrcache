package node

import (
	"encoding/json"
	"log/slog"
	"net"
	"time"

	"github.com/ryandielhenn/zephyrcache/pkg/gossip"
	"github.com/ryandielhenn/zephyrcache/pkg/peer"
)

func (n *Node) handleGossip(msg *gossip.Message, addr string) {
	if msg == nil {
		return
	}

	slog.Debug("Received Message", "message", *msg)

	if msg.Payload != nil {
		n.handlePayload(msg.Payload, msg.SourceId)
	}

	switch msg.Type {
	case gossip.Ping:
		n.handlePing(msg, addr)
	case gossip.PingReq:
		n.handlePingReq(msg)
	case gossip.PingAck:
		n.handlePingAck(msg)
	}
}

func (n *Node) handlePing(msg *gossip.Message, addr string) {
	payload := n.removeGossip()
	message := gossip.NewMessage(
		gossip.PingAck,
		msg.SubjectId,
		n.id,
		msg.OriginId,
		payload,
	)
	n.sendGossip(message, addr)
}

func (n *Node) handlePingReq(msg *gossip.Message) {
	peerBody, ok := n.peers[msg.SubjectId]
	if !ok {
		return
	}
	payload := n.removeGossip()
	message := gossip.NewMessage(
		gossip.Ping,
		msg.SubjectId,
		n.id,
		msg.OriginId,
		payload,
	)
	n.sendGossip(message, peerBody.Addr)
}

func (n *Node) handlePingAck(msg *gossip.Message) {
	// when a ping ack for suspected node is received
	// we stop the ping req timeout and reset the suspected peer
	if msg.OriginId == n.id && n.suspectPeer == msg.SubjectId {
		if n.timeout != nil {
			n.timeout.Stop()
			n.timeout = nil
		}
		n.suspectPeer = ""
		return
	}
	peerBody, ok := n.peers[msg.OriginId]
	if !ok {
		return
	}
	payload := n.removeGossip()
	message := gossip.NewMessage(
		gossip.PingAck,
		msg.SubjectId,
		n.id,
		msg.OriginId,
		payload,
	)
	n.sendGossip(message, peerBody.Addr)
}

func (n *Node) handlePayload(msg *gossip.MessagePayload, sourceId string) {
	if msg == nil {
		return
	}

	slog.Debug("Received Message Payload", "payload", *msg)

	for id, updatedPeer := range msg.Peers {
		switch updatedPeer.Status {
		case peer.Alive:
			n.handleAliveStatus(id, updatedPeer, sourceId)
		case peer.Dead:
			n.handleDeadStatus(id, updatedPeer)
		}
	}
}

func (n *Node) handleAliveStatus(id string, updatedPeer peer.Peer, sourceId string) {
	// drop payloads about yourself
	if id == n.id {
		return
	}

	// handle join requests
	// when new nodes sends alive status for itself respond with peers
	currentPeer, ok := n.peers[id]
	if !ok && id == sourceId {
		peers := n.getPeerMap()
		peers[n.id] = peer.Peer{
			Addr:        n.addr,
			Status:      peer.Alive,
			Incarnation: n.incarnation,
		}
		delete(peers, id)
		payload := gossip.NewPayload(peers, false)
		n.prependGossip(payload)
	}

	// determine whether message is stale or not
	// update peer status if not stale and propagate update to other nodes
	shouldUpdate := !ok || (updatedPeer.Incarnation > currentPeer.Incarnation)
	if shouldUpdate {
		n.setPeer(id, updatedPeer)
		peers := map[string]peer.Peer{
			id: updatedPeer,
		}
		payload := gossip.NewPayload(peers, true)
		n.addGossip(payload)
	}
}

func (n *Node) handleDeadStatus(id string, updatedPeer peer.Peer) {
	// drop payloads about yourself
	if id == n.id {
		return
	}
	currentPeer, ok := n.peers[id]

	// determine whether message is stale or not
	// update peer status if not stale and propagate update to other nodes
	// dead status has precedence over alive messages for equal incarnation
	shouldUpdate := !ok || (updatedPeer.Incarnation > currentPeer.Incarnation ||
		updatedPeer.Incarnation == currentPeer.Incarnation && currentPeer.Status == peer.Alive)
	if shouldUpdate {
		n.setPeer(id, updatedPeer)
		peers := map[string]peer.Peer{
			id: updatedPeer,
		}
		payload := gossip.NewPayload(peers, true)
		n.addGossip(payload)
	}
}

func (n *Node) sendGossip(msg *gossip.Message, addr string) {
	data, err := json.Marshal(msg)
	if err != nil {
		return
	}

	udpAddr, err := net.ResolveUDPAddr("udp", OverrideHostPort(addr, n.gossipPort))
	if err != nil {
		return
	}

	conn, err := net.DialUDP("udp", nil, udpAddr)
	if err != nil {
		return
	}
	defer conn.Close()

	_, err = conn.Write(data)
	if err != nil {
		return
	}
}

func (n *Node) attemptConnectToCluster(addr string) {
	peers := map[string]peer.Peer{
		n.id: {
			Addr:        n.addr,
			Status:      peer.Alive,
			Incarnation: 0,
		},
	}
	payload := gossip.NewPayload(peers, true)
	message := gossip.NewMessage(
		gossip.Ping,
		"",
		n.id,
		n.id,
		payload,
	)
	n.sendGossip(message, addr)
}

func (n *Node) ConnectToCluster(addr string, attemptPeriod time.Duration) {
	ticker := time.NewTicker(attemptPeriod)
	for range ticker.C {
		n.mu.Lock()
		if len(n.peers) > 0 {
			n.mu.Unlock()
			break
		}
		n.attemptConnectToCluster(addr)
		n.mu.Unlock()
	}
}

func StartGossipListener(node *Node) {
	address := net.JoinHostPort("", node.gossipPort)

	addr, err := net.ResolveUDPAddr("udp", address)
	if err != nil {
		return
	}

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return
	}
	defer conn.Close()

	buffer := make([]byte, 1024)
	for {
		n, addr, err := conn.ReadFromUDP(buffer)
		if err != nil {
			continue
		}

		data := make([]byte, n)
		copy(data, buffer[:n])

		var msg gossip.Message
		if err := json.Unmarshal(data, &msg); err != nil {
			continue
		}
		node.mu.Lock()
		node.handleGossip(&msg, addr.String())
		node.mu.Unlock()
	}
}

type pingerConfig struct {
	period  time.Duration
	timeout time.Duration
	k       int
}

type pingerOption func(*pingerConfig)

func WithPeriod(period time.Duration) pingerOption {
	return func(c *pingerConfig) {
		c.period = period
	}
}

func WithTimeout(timeout time.Duration) pingerOption {
	return func(c *pingerConfig) {
		c.timeout = timeout
	}
}

func WithK(k int) pingerOption {
	return func(c *pingerConfig) {
		c.k = k
	}
}

func StartGossipPinger(node *Node, opts ...pingerOption) {
	cfg := &pingerConfig{
		period:  1 * time.Second,
		timeout: 500 * time.Millisecond,
		k:       3,
	}

	for _, opt := range opts {
		opt(cfg)
	}

	ticker := time.NewTicker(cfg.period)
	defer ticker.Stop()

	for range ticker.C {
		// declare peer dead if has not been acked since last ping
		node.mu.Lock()
		if node.suspectPeer != "" {
			peerBody, ok := node.peers[node.suspectPeer]
			if ok {
				peerBody.Status = peer.Dead
				peers := map[string]peer.Peer{
					node.suspectPeer: peerBody,
				}
				payload := gossip.NewPayload(peers, true)
				node.addGossip(payload)
				node.setPeer(node.suspectPeer, peerBody)
			}
		}

		// send ping to new random suspected peer
		payload := node.removeGossip()
		node.suspectPeer = node.getRandomPeer()
		peerBody, ok := node.peers[node.suspectPeer]
		if !ok {
			node.mu.Unlock()
			continue
		}
		message := gossip.NewMessage(
			gossip.Ping,
			node.suspectPeer,
			node.id,
			node.id,
			payload,
		)
		node.sendGossip(message, peerBody.Addr)

		suspect := node.suspectPeer
		// send ping req to k random peers after timeout
		node.timeout = time.AfterFunc(cfg.timeout, func() {
			node.mu.Lock()
			defer node.mu.Unlock()
			for _, id := range node.getKRandomPeers(cfg.k) {
				if id == suspect {
					continue
				}
				peerBody, ok := node.peers[id]
				if !ok {
					continue
				}
				message := gossip.NewMessage(
					gossip.PingReq,
					suspect,
					node.id,
					node.id,
					payload,
				)
				node.sendGossip(message, peerBody.Addr)
			}
		})
		node.mu.Unlock()
	}
}
