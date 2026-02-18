package gossip

import "time"

// Tracks the cluster's view of active members
// Data structures for Member (id, address, state, incarnation number)
// Add/remove/update logic
// Maybe a background routine that spreads membership rumors

type Member struct {
	ID          NodeID    // unique, usually UUID or host:port string
	Addr        string    // current reachable address
	Incarnation uint64    // version number for this member’s state
	State       State     // Alive, Suspect, Dead
	LastUpdate  time.Time // (optional) for metrics/GC
}

type MemberList interface {
	Self() Member
	All() []Member
	Get(id NodeID) (Member, bool)
	ApplyDelta(d Delta) bool
	BumpIncarnation() uint64
}
