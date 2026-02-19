package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"

	"github.com/ryandielhenn/zephyrcache/internal/telemetry"
	"github.com/ryandielhenn/zephyrcache/pkg/kv"
	"github.com/ryandielhenn/zephyrcache/pkg/node"
	discovery "github.com/ryandielhenn/zephyrcache/pkg/registry"
	"github.com/ryandielhenn/zephyrcache/pkg/ring"
)

func main() {
	// 1. Initialize this node with routing ring and key value store
	store := kv.NewStore(64 << 20) // 64MB default cap for MVP
	r := ring.New(128, ring.FNV32a)
	id := os.Getenv("SELF_ID")
	addr := os.Getenv("SELF_ADDR")

	rf := 2
	if v := os.Getenv("REPLICATION_FACTOR"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			rf = n
		}
	}
	n := node.NewNodeRF(store, r, id, rf)
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
		peerHP := node.NormalizeHostPort(string(kv.Value), "8080")
		log.Printf("[Bootstrap] %s -> %s", nodeID, peerHP)
		n.AddPeer(nodeID, peerHP)
	}

	// 4. Register this node
	log.Printf("[Boot] registering, %s : %s with etcd", id, n.Addr())
	leaseId, cancel, err := discovery.RegisterNode(cli, id, n.Addr(), 10)
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
		n.ClearPeers()
		for id, addr := range peers {
			hp := node.NormalizeHostPort(addr, "8080")
			log.Printf("[WatchPeers Callback] %s -> %s\n", id, hp)
			n.AddPeer(id, hp)
		}

	})
	log.Printf("[BOOT] after WatchPeers")

	// 6. Wire up HTTP node endpoints
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", n.Healthz)
	mux.HandleFunc("/info", n.Info)
	mux.Handle("/metrics", telemetry.MetricsHandler())
	mux.HandleFunc("/kv/", func(w http.ResponseWriter, req *http.Request) {
		op := methodToOp(req.Method) // "get" | "put" | "post" | "delete" | "other"
		telemetry.Instrument(op, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodPut, http.MethodPost:
				n.Put(w, r)
			case http.MethodGet:
				n.Get(w, r)
			case http.MethodDelete:
				n.Del(w, r)
			default:
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			}
		})).ServeHTTP(w, req)
	})

	addr = ":8080"
	fmt.Println("ZephyrCache node listening on", addr)
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
