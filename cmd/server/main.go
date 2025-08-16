package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"go.etcd.io/etcd/client/v3"

	"github.com/ryandielhenn/zephyrcache/discovery"
	"github.com/ryandielhenn/zephyrcache/pkg/kv"
	"github.com/ryandielhenn/zephyrcache/pkg/ring"
)

// --- TODOs (bite-size next steps) ---
// TODO(ryan): wire config: parse nodes, self ID, replicas from env/JSON
// TODO(ryan): initialize ring, store, router; start HTTP server
// TODO(ryan): register /debug/pprof and Prometheus metrics
// TODO(ryan): handlers: PUT/GET/DELETE /v1/cache/{key}?ttl=...
// TODO(ryan): validate key/ttl/value size; return appropriate status codes
// TODO(ryan): structured logging: op, key hash, duration, nodeID
// --- end TODOs ---

type server struct {
	kv *kv.Store
	ring *ring.HashRing
	peers []string
}

func (s *server) healthz(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

func (s *server) put(w http.ResponseWriter, r *http.Request) {
	key := r.URL.Path[len("/kv/"):]

	val := make([]byte, r.ContentLength)
	_, err := r.Body.Read(val)
	if err != nil && err.Error() != "EOF" {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	ttlStr := r.URL.Query().Get("ttl")
	var ttl time.Duration
	if ttlStr != "" {
		sec, err := strconv.Atoi(ttlStr)
		if err != nil {
			http.Error(w, "invalid ttl", http.StatusBadRequest)
			return
		}
		ttl = time.Duration(sec) * time.Second
	}
	s.kv.Put(key, val, ttl)
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) get(w http.ResponseWriter, r *http.Request) {
	key := r.URL.Path[len("/kv/"):]

	val, ok := s.kv.Get(key)
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Write(val)
}

func (s *server) del(w http.ResponseWriter, r *http.Request) {
	key := r.URL.Path[len("/kv/"):]

	s.kv.Delete(key)
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) info(w http.ResponseWriter, r *http.Request) {
	type resp struct {
		PID   int       `json:"pid"`
		Now   time.Time `json:"now"`
		Items int       `json:"items"`
	}
	data, _ := json.Marshal(resp{PID: os.Getpid(), Now: time.Now(), Items: s.kv.Len()})
	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}

func main() {
	store := kv.NewStore(64 << 20) // 64MB default cap for MVP
	s := &server{kv: store}
	// TODO populate s.peers using etcd client
	cli, err := clientv3.New(clientv3.Config{
		Endpoints:   []string{"localhost:2379"},
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		log.Fatal(err)
	}
	defer cli.Close()
	id := os.Getenv("SELF_ID")
	addr := os.Getenv("SELF_ADDR")
	
	leaseId, err := discovery.RegisterNode(cli, id, addr, 10)
	if err != nil {
		log.Fatal(err)
	}
	defer cli.Revoke(context.TODO(), leaseId)

	discovery.WatchPeers(cli, func(peers map[string]string) {
		//TODO placeholder until I figure out how to respond to watches
		for id, addr := range(peers) {
			fmt.Printf("%s -> %s\n", id, addr)
		}
	})

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.healthz)
	mux.HandleFunc("/info", s.info)
	mux.HandleFunc("/kv/", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPut, http.MethodPost:
			s.put(w, r)
		case http.MethodGet:
			s.get(w, r)
		case http.MethodDelete:
			s.del(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	addr = ":8080"
	fmt.Println("ZephyrCache server listening on", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}
