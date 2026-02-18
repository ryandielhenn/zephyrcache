package gossip

// Defenitions of the wire protocol: heartbeat messages, membership updates, suspicion signals etc.
// Likely small structs with Matshal/Unmarhsal helpers
// Keeps network encoding/decoding logic isolated
type NodeID string

type State uint8

const (
	StateAlive State = iota
	StateSuspect
	StateDead
)

type MsgType uint8

const (
	MsgPing MsgType = iota
	MsgAck
	MsgIndirectPing
	MsgPushPullReq
	MsgPushPullResp
)

type Delta struct {
	Member Member
	// Optional: tombstone deadline for StateDead to garbage-collect
}

type GossipMsg struct {
	Type    MsgType
	From    NodeID
	Target  *NodeID // for indirect pings
	Deltas  []Delta // piggyback
	Nonce   uint64  // match ping/ack
	SchemaV uint16  // forward-compat
	// Optional: HMAC/nonce for auth later
}
