package main

import (
	"cmp"
	"log"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/ryandielhenn/zephyrcache/internal/telemetry"
	"github.com/ryandielhenn/zephyrcache/pkg/kv"
	"github.com/ryandielhenn/zephyrcache/pkg/node"
	"github.com/ryandielhenn/zephyrcache/pkg/ring"
	clientv3 "go.etcd.io/etcd/client/v3"
)

func main() {
	// 1. Initialize this node with routing ring and key value store
	store := kv.NewStore(64 << 20) // 64MB default cap for MVP
	r := ring.New(128, ring.FNV32a)
	id := os.Getenv("SELF_ID")
	addr := os.Getenv("SELF_ADDR")
	seedAddr := os.Getenv("SEED_ADDR")
	etcdEndpoints := os.Getenv("ETCD_ENDPOINTS")
	membershipService := os.Getenv("DISCOVERY")
	gossipPort := cmp.Or(os.Getenv("GOSSIP_PORT"), "4000")
	var level slog.Level
	if lvl := os.Getenv("LOG_LEVEL"); lvl != "" {
		level.UnmarshalText([]byte(lvl))
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: level,
	})))

	// 2. Connect to cluster
	r.Add(id, addr)
	n := node.NewNode(store, r, id, node.NormalizeHostPort(addr, "8080"), gossipPort)
	switch membershipService {
	case "etcd":
		// Create etcd client
		slog.Info("[Boot] creating etcd client")
		cli, err := clientv3.New(clientv3.Config{
			Endpoints:   strings.Split(etcdEndpoints, ","),
			DialTimeout: 5 * time.Second,
		})
		slog.Info("[Boot] created etcd client with", "endpoints", cli.Endpoints())
		if err != nil {
			log.Fatal(err)
		}
		defer cli.Close()
		defer node.BootstrapPeers(n, cli)()
		node.WatchPeers(n, cli)
	case "gossip":
		go node.StartGossipListener(n)
		if seedAddr != "" {
			n.ConnectToCluster(seedAddr, 200*time.Millisecond)
		}
		go node.StartGossipPinger(
			n,
			node.WithPeriod(200*time.Millisecond),
			node.WithTimeout(100*time.Millisecond),
		)
	default:
		slog.Info("DISCOVERY must be set.")
		return
	}

	// 3. Wire up HTTP node endpoints
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", n.Healthz)
	mux.HandleFunc("/info", n.Info)
	mux.Handle("/metrics", telemetry.MetricsHandler())
	mux.HandleFunc("/kv/", func(w http.ResponseWriter, req *http.Request) {
		op := methodToOp(req.Method) // "get" | "put" | "post" | "delete" | "other"
		telemetry.Instrument(op, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			slog.Info("Received HTTP KV Request", "type", r.Method)
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
	mux.HandleFunc("/replica/", func(w http.ResponseWriter, req *http.Request) {
		slog.Info("Received HTTP Replica Request", "type", req.Method)
		switch req.Method {
		case http.MethodPut, http.MethodPost:
			n.PutReplica(w, req)
		case http.MethodDelete:
			n.DelReplica(w, req)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	addr = ":8080"
	slog.Info("ZephyrCache node listening on", "addr", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err.Error())
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
