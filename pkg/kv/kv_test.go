package kv

import (
	"bytes"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestPutGetDelete_NoTTL(t *testing.T) {
	s := NewStore(1 << 20) // 1MB

	type row struct {
		k string
		v []byte
	}
	data := []row{
		{"a", []byte("alpha")},
		{"b", []byte("beta")},
		{"c", []byte("gamma")},
	}

	for _, r := range data {
		s.Put(r.k, r.v, 0) // ttl=0 → no expiry
	}

	if got := s.Len(); got != len(data) {
		t.Fatalf("Len = %d, want %d", got, len(data))
	}

	for _, r := range data {
		got, ok := s.Get(r.k)
		if !ok {
			t.Fatalf("Get(%q) !ok", r.k)
		}
		if !bytes.Equal(got, r.v) {
			t.Fatalf("Get(%q) = %q, want %q", r.k, got, r.v)
		}
	}

	if ok := s.Delete("b"); !ok {
		t.Fatalf("Delete(b) = false, want true")
	}
	if _, ok := s.Get("b"); ok {
		t.Fatalf("Get(b) ok after delete")
	}
}

func TestOverwriteKeepsLen(t *testing.T) {
	s := NewStore(1 << 20)
	s.Put("x", []byte("one"), 0)
	s.Put("x", []byte("two"), 0)
	if got := s.Len(); got != 1 {
		t.Fatalf("Len after overwrite = %d, want 1", got)
	}
	v, ok := s.Get("x")
	if !ok || string(v) != "two" {
		t.Fatalf("Get(x) = %q,%v want two,true", v, ok)
	}
}

func TestTTLExpiry(t *testing.T) {
	s := NewStore(1 << 20)

	s.Put("k", []byte("v"), 50*time.Millisecond)
	// Small buffer beyond TTL to avoid flakiness in CI
	time.Sleep(90 * time.Millisecond)

	if _, ok := s.Get("k"); ok {
		t.Fatalf("expected key to expire")
	}
}

func TestEvictionByCapacity_LRU(t *testing.T) {
	// Small cap to force eviction.
	s := NewStore(9)

	s.Put("a", []byte("1234"), 0) // ~4
	s.Put("b", []byte("56"), 0)   // ~2  total ~6

	// Touch "a" so it's the most-recent.
	if _, ok := s.Get("a"); !ok {
		t.Fatalf("precondition failed: expected to get a before eviction")
	}

	// Insert "c" → should evict least-recent ("b").
	s.Put("c", []byte("7890"), 0) // ~4

	if _, ok := s.Get("a"); !ok {
		t.Fatalf("expected a to remain")
	}
	if _, ok := s.Get("c"); !ok {
		t.Fatalf("expected c to be present")
	}
	if _, ok := s.Get("b"); ok {
		t.Fatalf("expected b to be evicted")
	}
}

func TestConcurrentAccess_NoRaces(t *testing.T) {
	s := NewStore(1 << 20)

	var wg sync.WaitGroup
	const G = 32
	const N = 2000

	errCh := make(chan error, G)
	var stop atomic.Bool // requires Go 1.19+

	for gid := range G {
		wg.Add(1)
		go func(gid int) {
			defer wg.Done()
			for i := range N {
				if stop.Load() {
					return
				}
				k := fmt.Sprintf("k-%d-%d", gid, i)
				v := fmt.Appendf(nil, "v-%d", i)

				s.Put(k, v, 0)

				got, ok := s.Get(k)
				if !ok {
					errCh <- fmt.Errorf("missing key=%s right after Put", k)
					stop.Store(true)
					return
				}
				if !bytes.Equal(got, v) {
					errCh <- fmt.Errorf("mismatch for key=%s", k)
					stop.Store(true)
					return
				}

				if i%7 == 0 {
					s.Delete(k)
				}
			}
		}(gid)
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Fatalf("concurrency test failed: %v", err)
	}
	// Tip: run with race detector:
	//   go test -race ./kv -v
}

func TestLRU_GetUpdatesRecency(t *testing.T) {
	s := NewStore(100)

	a := bytes.Repeat([]byte("a"), 40)
	b := bytes.Repeat([]byte("b"), 40)
	c := bytes.Repeat([]byte("c"), 40)

	s.Put("a", a, 0)              // ~40
	s.Put("b", b, 0)              // ~80
	if _, ok := s.Get("a"); !ok { // touch a → should be MRU now
		t.Fatalf("precondition: a missing")
	}
	s.Put("c", c, 0) // ~120 → must evict LRU "b" (not "a")

	if _, ok := s.Get("a"); !ok {
		t.Fatalf("expected a to remain after eviction (Get must update recency)")
	}
	if _, ok := s.Get("c"); !ok {
		t.Fatalf("expected c present")
	}
	if _, ok := s.Get("b"); ok {
		t.Fatalf("expected b to be evicted (LRU victim)")
	}
}

func TestOverwrite_SizeAndLen(t *testing.T) {
	s := NewStore(200)

	orig := bytes.Repeat([]byte("x"), 50)
	big := bytes.Repeat([]byte("y"), 90)   // grows
	small := bytes.Repeat([]byte("z"), 10) // shrinks

	s.Put("k", orig, 0)
	if got, ok := s.Get("k"); !ok || !bytes.Equal(got, orig) {
		t.Fatalf("wrote orig wrong")
	}
	if s.Len() != 1 {
		t.Fatalf("Len = %d, want 1", s.Len())
	}

	// grow
	s.Put("k", big, 0)
	if got, ok := s.Get("k"); !ok || !bytes.Equal(got, big) {
		t.Fatalf("overwrite (grow) failed")
	}
	if s.Len() != 1 {
		t.Fatalf("Len after grow overwrite = %d, want 1", s.Len())
	}

	// shrink
	s.Put("k", small, 0)
	if got, ok := s.Get("k"); !ok || !bytes.Equal(got, small) {
		t.Fatalf("overwrite (shrink) failed")
	}
	if s.Len() != 1 {
		t.Fatalf("Len after shrink overwrite = %d, want 1", s.Len())
	}
}

func TestTTL_Expires(t *testing.T) {
	s := NewStore(1 << 20)

	s.Put("short", []byte("v"), 40*time.Millisecond)
	if _, ok := s.Get("short"); !ok {
		t.Fatalf("fresh key with TTL should be readable")
	}
	time.Sleep(80 * time.Millisecond)
	if _, ok := s.Get("short"); ok {
		t.Fatalf("expected TTL expiry for key 'short'")
	}

	// No TTL (0) should not expire
	s.Put("noTTL", []byte("v2"), 0)
	time.Sleep(80 * time.Millisecond)
	if _, ok := s.Get("noTTL"); !ok {
		t.Fatalf("noTTL key unexpectedly missing")
	}
}
