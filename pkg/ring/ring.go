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
	points   []uint32          // sorted
	owners   map[uint32]string // point -> nodeID
	nodes    map[string]string // nodeID -> addr (metadata)
}

func New(replicas int, h Hasher) *HashRing {
	if replicas <= 0 { replicas = 128 }
	if h == nil { h = fnv32a }
	return &HashRing{
		replicas: replicas,
		hash:     h,
		owners:   make(map[uint32]string),
		nodes:    make(map[string]string),
	}
}

func (r *HashRing) Add(nodeID, addr string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.nodes[nodeID]; ok { return }
	r.nodes[nodeID] = addr
	// add virtual nodes
	for i := 0; i < r.replicas; i++ {
		pt := r.hash(pointKey(nodeID, i))
		r.owners[pt] = nodeID
		r.points = append(r.points, pt)
	}
	slices.Sort(r.points)
}

func (r *HashRing) Remove(nodeID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.nodes[nodeID]; !ok { return }
	delete(r.nodes, nodeID)
	// rebuild points/owners (simplest, MVP)
	r.points = r.points[:0]
	clear(r.owners)
	for id := range r.nodes {
		for i := 0; i < r.replicas; i++ {
			pt := r.hash(pointKey(id, i))
			r.owners[pt] = id
			r.points = append(r.points, pt)
		}
	}
	slices.Sort(r.points)
}

func (r *HashRing) Lookup(key []byte) string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if len(r.points) == 0 { return "" }
	h := r.hash(key)
	// first point >= h, wrap if needed
	idx := sort.Search(len(r.points), func(i int) bool { return r.points[i] >= h })
	if idx == len(r.points) { idx = 0 }
	return r.owners[r.points[idx]]
}

func (r *HashRing) LookupN(key []byte, n int) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if len(r.points) == 0 || n <= 0 { return nil }
	h := r.hash(key)
	idx := sort.Search(len(r.points), func(i int) bool { return r.points[i] >= h })
	if idx == len(r.points) { idx = 0 }

	seen := make(map[string]struct{}, n)
	out := make([]string, 0, n)
	for i := 0; i < len(r.points) && len(out) < n; i++ {
		p := r.points[(idx+i)%len(r.points)]
		id := r.owners[p]
		if _, ok := seen[id]; !ok {
			seen[id] = struct{}{}
			out = append(out, id)
		}
	}
	return out
}

func (r *HashRing) Addr(nodeID string) (string, bool) {
	r.mu.RLock(); defer r.mu.RUnlock()
	a, ok := r.nodes[nodeID]
	return a, ok
}

func fnv32a(b []byte) uint32 {
	h := fnv.New32a()
	_, _ = h.Write(b)
	return h.Sum32()
}

func pointKey(nodeID string, i int) []byte {
	var buf [4]byte
	binary.LittleEndian.PutUint32(buf[:], uint32(i))
	return append([]byte(nodeID), buf[:]...)
}
