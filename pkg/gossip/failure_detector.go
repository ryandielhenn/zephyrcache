package gossip

// Implements the Phi Accrual Failure Detector (or simpler heartbeat timeouts).
// Tracks last heartbeat timestamps per peer.
// Emits suspicion/confirmation events when a node looks dead.
