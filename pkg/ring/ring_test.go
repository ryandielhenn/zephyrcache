package ring

import (
	"hash/fnv"
	"math"
	"testing"
)

// test hasher: FNV-1a 32-bit
func fnv32a(b []byte) uint32 {
	h := fnv.New32a()
	_, _ = h.Write(b)
	return h.Sum32()
}

func TestAddAddrLookup(t *testing.T) {
	r := New(128, fnv32a)

	r.Add("node1", "127.0.0.1:8080")
	r.Add("node2", "127.0.0.1:8081")
	r.Add("node3", "127.0.0.1:8082")

	// Addr should return what we inserted
	for id, want := range map[string]string{
		"node1": "127.0.0.1:8080",
		"node2": "127.0.0.1:8081",
		"node3": "127.0.0.1:8082",
	} {
		got, ok := r.Addr(id)
		if !ok || got != want {
			t.Fatalf("Addr(%s) = (%q,%v), want (%q,true)", id, got, ok, want)
		}
	}

	// Lookup should return one of our node IDs; stable for same key
	keys := [][]byte{[]byte("foo"), []byte("bar"), []byte("baz")}
	for _, k := range keys {
		id1 := r.Lookup(k)
		id2 := r.Lookup(k)
		if id1 == "" {
			t.Fatalf("Lookup(%q) returned empty id", k)
		}
		if id1 != id2 {
			t.Fatalf("Lookup(%q) not stable: %q != %q", k, id1, id2)
		}
	}
}

func TestRemoveAffectsLookup(t *testing.T) {
	r := New(128, fnv32a)
	r.Add("n1", "a:1")
	r.Add("n2", "a:2")
	r.Add("n3", "a:3")

	key := []byte("hot-key-123")
	before := r.Lookup(key)
	if before == "" {
		t.Fatal("Lookup empty before remove")
	}

	// Remove the owner; Lookup should move to a different node
	r.Remove(before)
	after := r.Lookup(key)
	if after == "" || after == before {
		t.Fatalf("Lookup did not change after removing %q: got %q", before, after)
	}
}

func TestDistributionRoughlyBalanced(t *testing.T) {
	// Not a strict test—just sanity: with replicas, distribution shouldn’t be wildly skewed
	r := New(128, fnv32a)
	r.Add("n1", "a:1")
	r.Add("n2", "a:2")
	r.Add("n3", "a:3")

	const N = 6000
	counts := map[string]int{}
	for i := range N {
		id := r.Lookup([]byte{byte(i >> 24), byte(i >> 16), byte(i >> 8), byte(i)})
		counts[id]++
	}
	// Expect near-uniform: allow 2x deviation from perfect split
	ideal := float64(N) / 3.0
	for id, c := range counts {
		if c == 0 {
			t.Fatalf("node %s got zero keys", id)
		}
		if diff := math.Abs(float64(c)-ideal) / ideal; diff > 1.0 { // >100% off
			t.Fatalf("distribution too skewed: node %s has %d (ideal %.1f)", id, c, ideal)
		}
	}
}

func TestIdempotentRemove(t *testing.T) {
	r := New(128, fnv32a)
	r.Add("n1", "a:1")
	r.Remove("n1")
	// Removing again should not panic
	r.Remove("n1")
}

func TestRemoveNonExistentNode(t *testing.T) {
	r := New(128, fnv32a)
	r.Add("n1", "a:1")
	r.Add("n2", "a:2")

	// Record state before removing non-existent node
	beforeCount := len(r.Nodes())

	// Remove a node that doesn't exist
	r.Remove("non-existent")

	// Verify nothing changed
	afterCount := len(r.Nodes())
	if beforeCount != afterCount {
		t.Fatalf("removing non-existent node changed node count: before=%d, after=%d", beforeCount, afterCount)
	}

	// Verify original nodes are still there
	if _, ok := r.Addr("n1"); !ok {
		t.Fatal("n1 should still exist")
	}
	if _, ok := r.Addr("n2"); !ok {
		t.Fatal("n2 should still exist")
	}
}

func TestNodes(t *testing.T) {
	r := New(128, fnv32a)
	r.Add("n1", "a:1")
	r.Add("n2", "a:2")

	nodes := r.Nodes()
	if len(nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(nodes))
	}
	if nodes["n1"] != "a:1" || nodes["n2"] != "a:2" {
		t.Fatalf("Nodes() returned incorrect data: %v", nodes)
	}

	// Verify it's a copy (modifying doesn't affect original)
	nodes["n3"] = "a:3"
	if _, ok := r.Nodes()["n3"]; ok {
		t.Fatal("Nodes() returned a reference, not a copy")
	}
}

func TestRemoveOnlyAffectsTargetNode(t *testing.T) {
	r := New(128, fnv32a)
	r.Add("n1", "a:1")
	r.Add("n2", "a:2")
	r.Add("n3", "a:3")

	// Record lookups before removal
	keys := [][]byte{[]byte("key1"), []byte("key2"), []byte("key3")}
	before := make(map[string]string)
	for _, k := range keys {
		before[string(k)] = r.Lookup(k)
	}

	// Remove n2
	r.Remove("n2")

	// Verify n2 is gone
	if _, ok := r.Addr("n2"); ok {
		t.Fatal("n2 should have been removed")
	}

	// Verify n1 and n3 are still present
	if _, ok := r.Addr("n1"); !ok {
		t.Fatal("n1 should still exist")
	}
	if _, ok := r.Addr("n3"); !ok {
		t.Fatal("n3 should still exist")
	}

	// Verify lookups for keys that were on n1 or n3 haven't changed
	for _, k := range keys {
		after := r.Lookup(k)
		beforeNode := before[string(k)]
		if beforeNode != "n2" && after != beforeNode {
			t.Fatalf("key %q moved from %s to %s, should stay on %s", k, beforeNode, after, beforeNode)
		}
	}
}
