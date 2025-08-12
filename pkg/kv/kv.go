
package kv

import (
    "container/list"
    "sync"
    "time"
)

type entry struct {
    key   string
    value []byte
    expireAt time.Time
}

// Store is a minimal in-memory KV with TTL and LRU eviction by bytes capacity.
type Store struct {
    mu   sync.RWMutex
    data map[string]*list.Element
    ll   *list.List
    used int
    cap  int
}

func NewStore(capacityBytes int) *Store {
    return &Store{
        data: make(map[string]*list.Element),
        ll:   list.New(),
        cap:  capacityBytes,
    }
}

func (s *Store) Put(key string, val []byte, ttl time.Duration) {
    s.mu.Lock()
    defer s.mu.Unlock()

    var exp time.Time
    if ttl > 0 {
        exp = time.Now().Add(ttl)
    }

    if el, ok := s.data[key]; ok {
        old := el.Value.(*entry)
        s.used -= len(old.value)
        old.value = append([]byte(nil), val...)
        old.expireAt = exp
        s.used += len(old.value)
        s.ll.MoveToFront(el)
    } else {
        e := &entry{key: key, value: append([]byte(nil), val...), expireAt: exp}
        el := s.ll.PushFront(e)
        s.data[key] = el
        s.used += len(e.value)
    }
    s.evictIfNeeded()
}

func (s *Store) Get(key string) ([]byte, bool) {
    s.mu.Lock()
    defer s.mu.Unlock()

    if el, ok := s.data[key]; ok {
        e := el.Value.(*entry)
        if !e.expireAt.IsZero() && time.Now().After(e.expireAt) {
            s.removeElement(el)
            return nil, false
        }
        s.ll.MoveToFront(el)
        return append([]byte(nil), e.value...), true
    }
    return nil, false
}

func (s *Store) Delete(key string) {
    s.mu.Lock()
    defer s.mu.Unlock()
    if el, ok := s.data[key]; ok {
        s.removeElement(el)
    }
}

func (s *Store) Len() int {
    s.mu.RLock()
    defer s.mu.RUnlock()
    return len(s.data)
}

func (s *Store) evictIfNeeded() {
    for s.used > s.cap && s.ll.Back() != nil {
        s.removeElement(s.ll.Back())
    }
}

func (s *Store) removeElement(el *list.Element) {
    e := el.Value.(*entry)
    delete(s.data, e.key)
    s.used -= len(e.value)
    s.ll.Remove(el)
}
