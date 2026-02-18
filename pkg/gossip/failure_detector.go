package gossip

import "time"

// Implements the Phi Accrual Failure Detector (or simpler heartbeat timeouts).
// Tracks last heartbeat timestamps per peer.
// Emits suspicion/confirmation events when a node looks dead.

type FailureDetector interface {
	Observe(id NodeID, t time.Time) // Called when ack/ping received
	Phi(id NodeID, now time.Time) float64
	Remove(id NodeID)
}
