package gossip

import (
	"github.com/ryandielhenn/zephyrcache/pkg/peer"
)

type Message struct {
	Type      MessageType     `json:"type"`
	SubjectId string          `json:"sub_id"`
	SourceId  string          `json:"src_id"`
	OriginId  string          `json:"orig_id"`
	Payload   *MessagePayload `json:"payload"`
}

type MessagePayload struct {
	Peers         map[string]peer.Peer `json:"peers"`
	TransmitCount int                  `json:"-"`
}

type MessageType string

const (
	Ping    MessageType = "ping"
	PingReq MessageType = "ping_request"
	PingAck MessageType = "ping_ack"
)

func NewMessage(msgType MessageType, subjectId, sourceId, originId string, payload *MessagePayload) *Message {
	return &Message{
		Type:      msgType,
		SubjectId: subjectId,
		SourceId:  sourceId,
		OriginId:  originId,
		Payload:   payload,
	}
}

func NewPayload(peers map[string]peer.Peer, retransmit bool) *MessagePayload {
	var transmitCount int
	if retransmit {
		transmitCount = 1
	} else {
		transmitCount = 0
	}
	return &MessagePayload{
		Peers:         peers,
		TransmitCount: transmitCount,
	}
}
