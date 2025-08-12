
package ring

// Placeholder for a consistent hashing ring with virtual nodes.
// In MVP, this is not used yet; Week 2 will implement:
//  - Node struct {ID, Addr}
//  - Hash(key) -> vnode -> primary + replicas
//  - Rebalance on join/leave

type Node struct {
    ID   string
    Addr string
}
