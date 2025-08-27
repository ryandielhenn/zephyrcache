package gossip

// Interface for sending/receiving gossip messages (e.g. over UDP, TCP, or even in-proc channel for testing)
// Concrete implementations: UDPTransport, ChannelTransport
// Helps you swap out transport without touching membership logic
