package main

import (
	"log"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/ryandielhenn/zephyrcache/internal/telemetry"
	"github.com/ryandielhenn/zephyrcache/pkg/node"
	clientv3 "go.etcd.io/etcd/client/v3"
)

func main() {
	membershipService := os.Getenv("DISCOVERY")
	var level slog.Level
	if lvl := os.Getenv("LOG_LEVEL"); lvl != "" {
		err := level.UnmarshalText([]byte(lvl))
		if err != nil {
			slog.Info("Error reading LOG_LEVEL configuration")
		}
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: level,
	})))
	// 2. Connect to cluster
	n := node.NewNode(node.Config())
	switch membershipService {
	case "etcd":
		slog.Info("[Boot] creating etcd client")
		etcdEndpoints := os.Getenv("ETCD_ENDPOINTS")
		cli, err := clientv3.New(clientv3.Config{
			Endpoints:   strings.Split(etcdEndpoints, ","),
			DialTimeout: 5 * time.Second,
		})
		slog.Info("[Boot] created etcd client with", "endpoints", cli.Endpoints())
		if err != nil {
			log.Fatal(err)
		}
		defer func() { _ = cli.Close() }()
		defer node.BootstrapPeers(n, cli)()
		node.WatchPeers(n, cli)
	case "gossip":
		go node.StartGossipListener(n)
		seedAddr := os.Getenv("SEED_ADDR")
		if seedAddr != "" {
			n.ConnectToCluster(seedAddr, 200*time.Millisecond)
		}
		go node.StartGossipPinger(
			n,
			node.WithPeriod(200*time.Millisecond),
			node.WithPingTimeout(100*time.Millisecond),
			node.WithSuspectedTimeout(600*time.Millisecond),
		)
	default:
		slog.Info("DISCOVERY must be set.")
		return
	}

	// 3. Wire up client facing endpoints
	go func() {
		clientMux := http.NewServeMux()
		clientMux.HandleFunc("/healthz", n.Healthz)
		clientMux.HandleFunc("/info", n.Info)
		clientMux.Handle("/metrics", telemetry.MetricsHandler())
		clientMux.HandleFunc("/kv/", func(w http.ResponseWriter, req *http.Request) {
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
		clientFacingAddr := ":8080"
		slog.Info("ZephyrCache node listening to clients on", "addr", clientFacingAddr)
		if err := http.ListenAndServe(clientFacingAddr, clientMux); err != nil {
			log.Fatal(err.Error())
		}

	}()

	// 3. Wire up node facing endpoints
	peerMux := http.NewServeMux()
	peerMux.HandleFunc("/replica/", func(w http.ResponseWriter, req *http.Request) {
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

	peerFacingAddr := ":8081"
	slog.Info("ZephyrCache node listening to peers on", "addr", peerFacingAddr)
	if err := http.ListenAndServe(peerFacingAddr, peerMux); err != nil {
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
