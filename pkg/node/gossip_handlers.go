package node

import (
	"encoding/json"
	"log/slog"
	"net"
	"time"

	"github.com/ryandielhenn/zephyrcache/pkg/gossip"
	"github.com/ryandielhenn/zephyrcache/pkg/peer"
)

const GOSSIP_PORT_DEFAULT string = "4000"

func (n *Node) handleGossip(msg *gossip.Message) {
	if msg == nil {
		return
	}

	slog.Debug("Received Message", "message", *msg)

	n.mu.Lock()
	defer n.mu.Unlock()

	if msg.Payload != nil {
		n.handlePayload(msg.Payload, msg.SourceId)
	}

	switch msg.Type {
	case gossip.Ping:
		n.handlePing(msg)
	case gossip.PingReq:
		n.handlePingReq(msg)
	case gossip.PingAck:
		n.handlePingAck(msg)
	}
}

func (n *Node) handlePing(msg *gossip.Message) {
	peerBody, ok := n.peers[msg.SourceId]
	if !ok {
		return
	}
	payload := n.removeGossip()
	if peerBody.Status == peer.Dead {
		if payload == nil {
			peers := make(map[string]peer.Peer)
			payload = gossip.NewPayload(peers, false)
		}
		payload.Peers[msg.SourceId] = peer.Peer{
			Addr:        peerBody.Addr,
			Status:      peer.Suspected,
			Incarnation: peerBody.Incarnation,
			GossipAddr:  peerBody.GossipAddr,
		}
	}
	message := gossip.NewMessage(
		gossip.PingAck,
		msg.SubjectId,
		n.config.id,
		msg.OriginId,
		payload,
	)
	n.sendGossip(message, peerBody.GossipAddr)
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
		n.config.id,
		msg.OriginId,
		payload,
	)
	n.sendGossip(message, peerBody.GossipAddr)
}

func (n *Node) handlePingAck(msg *gossip.Message) {
	// handle ack at node that requested it
	if msg.OriginId == n.config.id {
		if n.targetPeer == msg.SubjectId {
			if n.timeout != nil {
				n.timeout.Stop()
				n.timeout = nil
			}
			n.targetPeer = ""
		}
		return
	}

	// handle forwarding ack when ping req
	peerBody, ok := n.peers[msg.OriginId]
	if !ok {
		return
	}
	payload := n.removeGossip()
	message := gossip.NewMessage(
		gossip.PingAck,
		msg.SubjectId,
		n.config.id,
		msg.OriginId,
		payload,
	)
	n.sendGossip(message, peerBody.GossipAddr)
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
		case peer.Suspected:
			n.handleSuspectedStatus(id, updatedPeer)
		case peer.Dead:
			n.handleDeadStatus(id, updatedPeer)
		}
	}
}

func (n *Node) handleAliveStatus(id string, updatedPeer peer.Peer, sourceId string) {
	// drop payloads about yourself
	if id == n.config.id {
		return
	}

	// handle join requests
	// when new nodes sends alive status for itself respond with peers
	currentPeer, ok := n.peers[id]
	if !ok && id == sourceId {
		peers := n.getPeerMap()
		peers[n.config.id] = peer.Peer{
			Addr:        n.config.addr,
			GossipAddr:  n.selfGossipAddr(),
			Status:      peer.Alive,
			Incarnation: n.incarnation,
		}
		delete(peers, id)
		payload := gossip.NewPayload(peers, false)
		n.prependGossip(payload)
	}

	// determine whether message is stale or not
	// update peer status if not stale and propagate update to other nodes
	shouldUpdate := !ok || updatedPeer.Supersedes(currentPeer)
	if shouldUpdate {
		n.setPeer(id, updatedPeer)
		peers := map[string]peer.Peer{
			id: updatedPeer,
		}
		payload := gossip.NewPayload(peers, true)
		n.enqGossip(payload)
	}
}

func (n *Node) handleSuspectedStatus(id string, updatedPeer peer.Peer) {
	// drop payloads about yourself
	if id == n.config.id {
		// refute updates saying you are suspected
		if updatedPeer.Incarnation == n.incarnation {
			n.incarnation += 1
		}
		peers := map[string]peer.Peer{
			n.config.id: {
				Addr:        n.config.addr,
				GossipAddr:  n.selfGossipAddr(),
				Status:      peer.Alive,
				Incarnation: n.incarnation,
			},
		}
		payload := gossip.NewPayload(peers, true)
		n.prependGossip(payload)
		return
	}

	// determine whether message is stale or not
	// update peer status if not stale and propagate update to other nodes
	currentPeer, ok := n.peers[id]
	shouldUpdate := !ok || updatedPeer.Supersedes(currentPeer)
	if shouldUpdate {
		n.setPeer(id, updatedPeer)
		peers := map[string]peer.Peer{
			id: updatedPeer,
		}
		payload := gossip.NewPayload(peers, true)
		n.enqGossip(payload)
		// TODO REFACTOR TO USE A CONFIGURED TIMEOUT
		time.AfterFunc(600*time.Millisecond, func() {
			n.mu.Lock()
			defer n.mu.Unlock()

			peerBody, ok := n.peers[id]
			if !ok || peerBody.Status != peer.Suspected {
				return
			}
			peerBody.Status = peer.Dead
			n.setPeer(id, peerBody)
			peers := map[string]peer.Peer{
				id: peerBody,
			}
			payload := gossip.NewPayload(peers, true)
			n.enqGossip(payload)
		})
	}
}

func (n *Node) handleDeadStatus(id string, updatedPeer peer.Peer) {
	// determine whether message is stale or not
	// update peer status if not stale and propagate update to other nodes
	currentPeer, ok := n.peers[id]
	shouldUpdate := !ok || updatedPeer.Supersedes(currentPeer)
	if shouldUpdate {
		n.setPeer(id, updatedPeer)
		peers := map[string]peer.Peer{
			id: updatedPeer,
		}
		payload := gossip.NewPayload(peers, true)
		n.enqGossip(payload)
	}
}

func (n *Node) selfGossipAddr() string {
	return OverrideHostPort(n.config.addr, n.config.gossipPort)
}

func (n *Node) sendGossip(msg *gossip.Message, addr string) {
	data, err := json.Marshal(msg)
	if err != nil {
		return
	}

	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return
	}

	conn, err := net.DialUDP("udp", nil, udpAddr)
	if err != nil {
		return
	}
	defer func() { _ = conn.Close() }()

	_, err = conn.Write(data)
	if err != nil {
		return
	}
}

func (n *Node) attemptConnectToCluster(addr string) bool {
	n.mu.Lock()
	defer n.mu.Unlock()

	if len(n.peers) > 0 {
		return true
	}

	peers := map[string]peer.Peer{
		n.config.id: {
			Addr:        n.config.addr,
			GossipAddr:  n.selfGossipAddr(),
			Status:      peer.Alive,
			Incarnation: 0,
		},
	}
	payload := gossip.NewPayload(peers, true)
	message := gossip.NewMessage(
		gossip.Ping,
		"",
		n.config.id,
		n.config.id,
		payload,
	)
	n.sendGossip(message, addr)

	return false
}

func (n *Node) ConnectToCluster(addr string, attemptPeriod time.Duration) {
	ticker := time.NewTicker(attemptPeriod)
	for range ticker.C {
		if n.attemptConnectToCluster(addr) {
			break
		}
	}
}

func StartGossipListener(node *Node) {
	address := net.JoinHostPort("", node.config.gossipPort)

	addr, err := net.ResolveUDPAddr("udp", address)
	if err != nil {
		return
	}

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return
	}
	defer func() { _ = conn.Close() }()

	buffer := make([]byte, 1024)
	for {
		n, _, err := conn.ReadFromUDP(buffer)
		if err != nil {
			continue
		}

		data := make([]byte, n)
		copy(data, buffer[:n])

		var msg gossip.Message
		if err := json.Unmarshal(data, &msg); err != nil {
			continue
		}
		node.handleGossip(&msg)
	}
}

type pingerConfig struct {
	period           time.Duration
	pingTimeout      time.Duration
	suspectedTimeout time.Duration
	k                int
}

type pingerOption func(*pingerConfig)

func WithPeriod(period time.Duration) pingerOption {
	return func(c *pingerConfig) {
		c.period = period
	}
}

func WithPingTimeout(timeout time.Duration) pingerOption {
	return func(c *pingerConfig) {
		c.pingTimeout = timeout
	}
}

func WithSuspectedTimeout(timeout time.Duration) pingerOption {
	return func(c *pingerConfig) {
		c.suspectedTimeout = timeout
	}
}

func WithK(k int) pingerOption {
	return func(c *pingerConfig) {
		c.k = k
	}
}

func runGossipPing(node *Node, cfg *pingerConfig) {
	node.mu.Lock()
	defer node.mu.Unlock()

	// propagate SUSPECTED if ALIVE target not been acked since last ping
	if node.targetPeer != "" {
		peerBody, ok := node.peers[node.targetPeer]
		if ok && peerBody.Status == peer.Alive {
			peerBody.Status = peer.Suspected
			node.setPeer(node.targetPeer, peerBody)
			peers := map[string]peer.Peer{
				node.targetPeer: peerBody,
			}
			payload := gossip.NewPayload(peers, true)
			node.enqGossip(payload)

			// set timeout to declare dead if SUSPECTED for long enough
			targetPeer := node.targetPeer
			time.AfterFunc(cfg.suspectedTimeout, func() {
				node.mu.Lock()
				defer node.mu.Unlock()

				peerBody, ok := node.peers[targetPeer]
				if !ok || peerBody.Status != peer.Suspected {
					return
				}
				peerBody.Status = peer.Dead
				node.setPeer(targetPeer, peerBody)
				peers := map[string]peer.Peer{
					targetPeer: peerBody,
				}
				payload := gossip.NewPayload(peers, true)
				node.enqGossip(payload)
			})
		}
	}

	// send ping to new random target peer
	node.targetPeer = node.getRandomPeer()
	peerBody, ok := node.peers[node.targetPeer]
	if !ok {
		return
	}
	payload := node.removeGossip()
	message := gossip.NewMessage(
		gossip.Ping,
		node.targetPeer,
		node.config.id,
		node.config.id,
		payload,
	)
	node.sendGossip(message, peerBody.GossipAddr)

	// send ping req to k random peers after timeout
	targetPeer := node.targetPeer
	node.timeout = time.AfterFunc(cfg.pingTimeout, func() {
		node.mu.Lock()
		defer node.mu.Unlock()

		for _, id := range node.getKRandomPeers(cfg.k) {
			if id == targetPeer {
				continue
			}
			peerBody, ok := node.peers[id]
			if !ok {
				continue
			}
			payload := node.removeGossip()
			message := gossip.NewMessage(
				gossip.PingReq,
				targetPeer,
				node.config.id,
				node.config.id,
				payload,
			)
			node.sendGossip(message, peerBody.GossipAddr)
		}
	})
}

func StartGossipPinger(node *Node, opts ...pingerOption) {
	cfg := &pingerConfig{
		period:           1 * time.Second,
		pingTimeout:      500 * time.Millisecond,
		suspectedTimeout: 3 * time.Second,
		k:                3,
	}

	for _, opt := range opts {
		opt(cfg)
	}

	ticker := time.NewTicker(cfg.period)
	defer ticker.Stop()

	for range ticker.C {
		runGossipPing(node, cfg)
	}
}
