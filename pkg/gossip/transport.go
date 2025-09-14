package gossip

import "context"

// Interface for sending/receiving gossip messages (e.g. over UDP, TCP, or even in-proc channel for testing)
// Concrete implementations: UDPTransport, ChannelTransport
// Helps you swap out transport without touching membership logic

type Transport interface {
    Send(addr string, msg GossipMsg) error
    Recv(ctx context.Context) (GossipMsg, net.Addr, error)
    Close() error
}
