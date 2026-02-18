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
	tokens   []uint32          // tokens give each node many placements on the ring and sorted token list defines ring order
	owners   map[uint32]string // token -> nodeID
	nodes    map[string]string // nodeID -> addr (metadata)
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
		pt := r.hash(tokenKey(nodeID, i))
		r.owners[pt] = nodeID
		r.tokens = append(r.tokens, pt)
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
	// rebuild tokens/owners (simplest, MVP)
	r.tokens = r.tokens[:0]
	clear(r.owners)
	for id := range r.nodes {
		for i := 0; i < r.replicas; i++ {
			tok := r.hash(tokenKey(id, i))
			r.owners[tok] = id
			r.tokens = append(r.tokens, tok)
		}
	}
	slices.Sort(r.tokens)
}

func (r *HashRing) Lookup(key []byte) string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if len(r.tokens) == 0 {
		return ""
	}
	h := r.hash(key)
	// first token >= h, wrap if needed
	idx := sort.Search(len(r.tokens), func(i int) bool { return r.tokens[i] >= h })
	if idx == len(r.tokens) {
		idx = 0
	}
	return r.owners[r.tokens[idx]]
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
