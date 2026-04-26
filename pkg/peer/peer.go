package peer

type Peer struct {
	Addr        string     `json:"addr"`
	GossipAddr  string     `json:"gossip_addr"`
	Status      PeerStatus `json:"status"`
	Incarnation int        `json:"incarnation"`
}

type PeerStatus string

const (
	Alive     PeerStatus = "alive"
	Suspected PeerStatus = "suspected"
	Dead      PeerStatus = "dead"
)

func (leftPeer Peer) Supersedes(rightPeer Peer) bool {
	if leftPeer.Incarnation > rightPeer.Incarnation {
		return true
	}
	if leftPeer.Incarnation < rightPeer.Incarnation {
		return false
	}
	if leftPeer.Status.supersedes(rightPeer.Status) {
		return true
	}
	return false
}

func (leftStatus PeerStatus) supersedes(rightStatus PeerStatus) bool {
	if rightStatus == Dead {
		return false
	}
	if leftStatus == Dead {
		return true
	}
	if rightStatus == Suspected {
		return false
	}
	if leftStatus == Suspected {
		return true
	}
	return false
}
