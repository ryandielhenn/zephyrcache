// Package gossip implements a gossip-based membership and failure detection
// subsystem for zephyrcache. It defines an abstract Transport interface and
// provides simple message types (heartbeat, membership rumors, etc.) and
// a pluggable failure detector. The goal is to allow nodes in a cluster
// to discover each other, share liveness information, and react to changes.
//
// Typical usage:
//
//   g, _ := gossip.New(gossip.Config{NodeID: "node1"})
//   g.Start()
//   defer g.Stop()
//
// By default it can run with an in-process channel transport (for testing),
// but production deployments should swap in a UDP transport.
package gossip
