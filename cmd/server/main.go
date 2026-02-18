package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"go.etcd.io/etcd/client/v3"

	"github.com/ryandielhenn/zephyrcache/internal/telemetry"
	"github.com/ryandielhenn/zephyrcache/pkg/kv"
	"github.com/ryandielhenn/zephyrcache/pkg/registry"
	"github.com/ryandielhenn/zephyrcache/pkg/ring"
)

type server struct {
	kv    *kv.Store
	ring  *ring.HashRing
	peers []string
	addr  string
}

// healthz returns 200 OK to indicate the server is alive.
func (s *server) healthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

// info writes a JSON payload with the process ID, current time, and KV item count.
func (s *server) info(w http.ResponseWriter, _ *http.Request) {
	type resp struct {
		PID   int       `json:"pid"`
		Now   time.Time `json:"now"`
		Items int       `json:"items"`
	}
	data, _ := json.Marshal(resp{PID: os.Getpid(), Now: time.Now(), Items: s.kv.Len()})
	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}

// normalizeHostPort cuts the http:// https:// prefixes from the input address
// adds a default port
func normalizeHostPort(addr, defPort string) string {
	if rest, ok := strings.CutPrefix(addr, "http://"); ok {
		addr = rest
	} else if rest, ok := strings.CutPrefix(addr, "https://"); ok {
		addr = rest
	}

	if _, _, err := net.SplitHostPort(addr); err == nil {
		return addr
	}

	return addr + ":" + defPort
}

// ownerForKey looks up the owner for a key and normalizes the address of the owner
func (s *server) ownerForKey(key string) (ownerHP, selfHP string, ok bool) {
	ownerID := s.ring.Lookup([]byte(key)) // e.g. "node3"
	ownerAddr, ok := s.ring.Addr(ownerID) // e.g. "node3:8080" (what you stored)
	if !ok || ownerAddr == "" {
		return "", "", false
	}
	return normalizeHostPort(ownerAddr, "8080"), normalizeHostPort(s.addr, "8080"), true
}

// forward forwards a http request to the node that owns the key
func (s *server) forward(w http.ResponseWriter, req *http.Request, owner string) {
	if owner == "" {
		http.Error(w, "no owner for key", http.StatusServiceUnavailable)
		return
	}

	hostport := normalizeHostPort(owner, "8080")
	if normalizeHostPort(s.addr, "8080") == hostport {
		// last-resort safety; shouldn’t happen if handler compare is correct
		http.Error(w, "refusing to forward to self", http.StatusInternalServerError)
		return
	}
	target := *req.URL
	target.Scheme = "http"
	target.Host = hostport

	out, err := http.NewRequestWithContext(req.Context(), req.Method, target.String(), req.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	out.Header = req.Header.Clone()

	out.Header.Set("X-Forwarded-For", req.RemoteAddr)

	resp, err := http.DefaultClient.Do(out)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	for k, vv := range resp.Header {
		for _, v := range vv {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)

}

// put adds a key/value pair
func (s *server) put(w http.ResponseWriter, req *http.Request) {
	key := req.URL.Path[len("/kv/"):]
	owner, self, ok := s.ownerForKey(key)
	if !ok {
		http.Error(w, "no owner for key", http.StatusServiceUnavailable)
		return
	}

	if owner != self {
		log.Printf("[Forward PUT] key=%q owner=%q self=%q", key, owner, self)
		s.forward(w, req, owner)
		return
	}

	// handle local case
	val, err := io.ReadAll(req.Body)
	if err != nil && err.Error() != "EOF" {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var ttl time.Duration
	if ttlStr := req.URL.Query().Get("ttl"); ttlStr != "" {
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

// get returns the value for a key
func (s *server) get(w http.ResponseWriter, req *http.Request) {
	key := req.URL.Path[len("/kv/"):]
	owner, self, ok := s.ownerForKey(key)
	if !ok {
		http.Error(w, "no owner for key", http.StatusServiceUnavailable)
		return
	}

	if owner != self {
		log.Printf("[Forward GET] key=%q owner=%q self=%q", key, owner, self)
		s.forward(w, req, owner)
		return
	}

	// handle local case
	val, ok := s.kv.Get(key)
	if !ok {
		http.NotFound(w, req)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Write(val)
}

// del removes a key
func (s *server) del(w http.ResponseWriter, req *http.Request) {
	key := req.URL.Path[len("/kv/"):]
	owner, self, ok := s.ownerForKey(key)
	if !ok {
		http.Error(w, "no owner for key", http.StatusServiceUnavailable)
		return
	}

	if owner != self {
		log.Printf("[Forward DEL] key=%q owner=%q self=%q", key, owner, self)
		s.forward(w, req, owner)
		return
	}

	// handle local case
	s.kv.Delete(key)
	w.WriteHeader(http.StatusNoContent)
}

func main() {
	// 1. Initialize this server with routing ring and key value store
	store := kv.NewStore(64 << 20) // 64MB default cap for MVP
	r := ring.New(128, ring.FNV32a)
	id := os.Getenv("SELF_ID")
	addr := os.Getenv("SELF_ADDR")
	s := &server{
		kv:   store,
		ring: r,
		addr: normalizeHostPort(addr, "8080"),
	}
	// 2. Create etcd client
	log.Printf("[Boot] creating etcd client")
	cli, err := clientv3.New(clientv3.Config{
		Endpoints:   []string{"http://etcd:2379"},
		DialTimeout: 5 * time.Second,
	})
	log.Printf("[Boot] created etcd client with endpoints, %s", cli.Endpoints())
	if err != nil {
		log.Fatal(err)
	}
	defer cli.Close()

	// 3. Bootstrap peers into this ring
	resp, err := cli.Get(context.TODO(), "/zephyr/nodes", clientv3.WithPrefix())
	if err != nil {
		log.Fatal(err)
	}
	for _, kv := range resp.Kvs {
		nodeID := strings.TrimPrefix(string(kv.Key), "/zephyr/nodes/")
		peerHP := normalizeHostPort(string(kv.Value), "8080")
		log.Printf("[Bootstrap] %s -> %s", nodeID, peerHP)
		s.ring.Add(nodeID, peerHP)
	}

	// 4. Register this node
	log.Printf("[Boot] registering, %s : %s with etcd", id, s.addr)
	leaseId, cancel, err := discovery.RegisterNode(cli, id, s.addr, 10)
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		cancel()
		_, _ = cli.Revoke(context.TODO(), leaseId)
	}()

	// 5. Watch for updates about peers
	log.Printf("[Boot] before watch peers")
	discovery.WatchPeers(cli, func(peers map[string]string) {
		s.ring.Clear()
		for id, addr := range peers {
			hp := normalizeHostPort(addr, "8080")
			log.Printf("[WatchPeers Callback] %s -> %s\n", id, hp)
			s.ring.Add(id, hp)
		}

	})
	log.Printf("[BOOT] after WatchPeers")

	// 6. Wire up HTTP server endpoints
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.healthz)
	mux.HandleFunc("/info", s.info)
	mux.Handle("/metrics", telemetry.MetricsHandler())
	mux.HandleFunc("/kv/", func(w http.ResponseWriter, req *http.Request) {
		op := methodToOp(req.Method) // "get" | "put" | "post" | "delete" | "other"
		telemetry.Instrument(op, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		})).ServeHTTP(w, req)
	})

	addr = ":8080"
	fmt.Println("ZephyrCache server listening on", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}

func methodToOp(m string) string {
	switch m {
	case http.MethodGet:
		return "get"
	case http.MethodPut:
		return "put"
	case http.MethodPost:
		return "post"
	case http.MethodDelete:
		return "delete"
	default:
		return "other"
	}
}
