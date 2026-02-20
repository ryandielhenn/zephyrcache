package ring

import (
	"encoding/binary"
	"hash/fnv"
	"slices"
	"sort"
	"sync"
)

type Hasher func([]byte) uint32

type HashRing struct {
	mu       sync.RWMutex
	replicas int
	hash     Hasher
	// tokens give each node many placements on the ring and
	// sorted token list defines ring order
	tokens []uint32
	owners map[uint32]string // token -> nodeID
	nodes  map[string]string // nodeID -> addr (metadata)
}

func New(replicas int, h Hasher) *HashRing {
	if replicas <= 0 {
		replicas = 128
	}
	if h == nil {
		h = FNV32a
	}
	return &HashRing{
		replicas: replicas,
		hash:     h,
		owners:   make(map[uint32]string),
		nodes:    make(map[string]string),
	}
}

func (r *HashRing) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tokens = r.tokens[:0]
	r.owners = make(map[uint32]string)
	r.nodes = make(map[string]string)
}

func (r *HashRing) Add(nodeID, addr string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.nodes[nodeID]; ok {
		return
	}
	r.nodes[nodeID] = addr
	// add virtual nodes
	for i := 0; i < r.replicas; i++ {
		tok := r.hash(tokenKey(nodeID, i))
		r.owners[tok] = nodeID
		r.tokens = append(r.tokens, tok)
	}
	slices.Sort(r.tokens)
}

func (r *HashRing) Remove(nodeID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.nodes[nodeID]; !ok {
		return
	}
	delete(r.nodes, nodeID)

	// Remove only this node's tokens (O(replicas) instead of O(nodes * replicas))
	newTokens := make([]uint32, 0, len(r.tokens)-r.replicas)
	for _, tok := range r.tokens {
		if owner, ok := r.owners[tok]; !ok || owner != nodeID {
			newTokens = append(newTokens, tok)
		}
	}
	r.tokens = newTokens

	// Clean up owners map for this node's tokens
	for i := 0; i < r.replicas; i++ {
		tok := r.hash(tokenKey(nodeID, i))
		delete(r.owners, tok)
	}
}

func (r *HashRing) Lookup(key []byte) string {
	nodes := r.LookupN(key, 1)
	if len(nodes) == 0 {
		return ""
	}
	return nodes[0]
}

func (r *HashRing) LookupN(key []byte, n int) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if len(r.tokens) == 0 || n <= 0 {
		return nil
	}
	h := r.hash(key)
	idx := sort.Search(len(r.tokens), func(i int) bool { return r.tokens[i] >= h })
	if idx == len(r.tokens) {
		idx = 0
	}

	seen := make(map[string]bool, n)
	out := make([]string, 0, n)
	for i := 0; i < len(r.tokens) && len(out) < n; i++ {
		p := r.tokens[(idx+i)%len(r.tokens)]
		id := r.owners[p]
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

func (r *HashRing) Addr(nodeID string) (string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	a, ok := r.nodes[nodeID]
	return a, ok
}

// Nodes returns a copy of the nodes map for iteration.
func (r *HashRing) Nodes() map[string]string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	nodes := make(map[string]string, len(r.nodes))
	for id, addr := range r.nodes {
		nodes[id] = addr
	}
	return nodes
}

func FNV32a(b []byte) uint32 {
	h := fnv.New32a()
	_, _ = h.Write(b)
	return h.Sum32()
}

func tokenKey(nodeID string, i int) []byte {
	var buf [4]byte
	binary.LittleEndian.PutUint32(buf[:], uint32(i))
	return append([]byte(nodeID), buf[:]...)
}
