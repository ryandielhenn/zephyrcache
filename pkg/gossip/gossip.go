package gossip

import (
	"go.uber.org/zap"
	"math/rand"
	"time"
)

// Entry point for the gossip subsystem
// defines the Gossiper struct, config, lifecycle methods (e.g. Start(), Stop() etc)
// Wires together membership, message handling and transport

type Config struct {
	// --- Gossip loop ---
	ProbeInterval   time.Duration // how often to ping a random peer
	ProbeTimeout    time.Duration // wait time for ack before suspecting
	IndirectK       int           // # of indirect peers for failure suspicion
	IndirectTimeout time.Duration // wait time for indirect ack
	Fanout          int           // how many peers get piggybacked deltas

	// --- Failure detection ---
	SuspectTimeout time.Duration // how long before Suspect → Dead
	DeadTimeout    time.Duration // how long to keep Dead before tombstone GC
	PhiThreshold   float64       // phi value at which we suspect a node

	// --- Anti-entropy ---
	PushPullEvery int // every N probes, run a full state sync

	// --- Misc ---
	BindAddr       string      // local address for transport
	SeedNodes      []string    // known peers to join cluster
	Logger         *zap.Logger // or stdlib logger, for observability
	MetricsEnabled bool
}

type Gossip struct {
	ml  MemberList
	tx  Transport
	fd  FailureDetector
	cfg Config
	rnd *rand.Rand
}
