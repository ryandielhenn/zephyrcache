package main

import (
	"context"
	"log"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/ryandielhenn/zephyrcache/internal/telemetry"
	"github.com/ryandielhenn/zephyrcache/pkg/node"
)

func main() {
	parent := context.Background()

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
	defer n.Cleanup()

	if err := node.ConfigureTLS(n); err != nil {
		log.Fatal(err)
	}
	switch membershipService {
	case "etcd":
		cleanup := n.JoinEtcd()
		defer cleanup()
	default:
		slog.Info("DISCOVERY defaulting to the gossip protocol.")

		ctx, cancel := context.WithCancel(parent)
		defer cancel()

		go node.StartGossipListener(ctx, n)
		seedAddr := os.Getenv("SEED_ADDR")
		if seedAddr != "" {
			n.ConnectToCluster(seedAddr, 200*time.Millisecond)
		}
		go node.StartGossipPinger(ctx, n,
			node.WithPeriod(200*time.Millisecond),
			node.WithPingTimeout(100*time.Millisecond),
			node.WithSuspectedTimeout(600*time.Millisecond),
		)
	}

	// 3. Wire up node facing endpoints
	go func() {
		peerMux := http.NewServeMux()
		peerMux.HandleFunc("/replica/", n.ReplicaEventCallback())
		// /kv/ also lives on the peer mux so inter-node forwards reuse the
		// same (optionally TLS) channel as /replica/.
		peerMux.HandleFunc("/kv/", n.KvEventCallback())

		peerFacingAddr := ":443"
		slog.Info("ZephyrCache node listening to peers on", "addr", peerFacingAddr)

		srv := &http.Server{
			Addr:      peerFacingAddr,
			Handler:   peerMux,
			TLSConfig: n.ServerTLSConfig(),
		}
		if n.ServerTLSConfig() != nil {
			log.Fatal(srv.ListenAndServeTLS("", ""))
		} else {
			log.Fatal(srv.ListenAndServe())
		}
	}()

	// 4. Wire up client facing endpoints
	clientMux := http.NewServeMux()
	clientMux.HandleFunc("/healthz", n.Healthz)
	clientMux.HandleFunc("/info", n.Info)
	clientMux.Handle("/metrics", telemetry.MetricsHandler())
	clientMux.HandleFunc("/kv/",
		func(w http.ResponseWriter, req *http.Request) {
			op := strings.ToLower(req.Method)
			telemetry.Instrument(op, http.HandlerFunc(n.KvEventCallback())).ServeHTTP(w, req)
		})
	clientFacingAddr := ":8080"
	slog.Info("ZephyrCache node listening to clients on", "addr", clientFacingAddr)
	clientSrv := &http.Server{
		Addr:      clientFacingAddr,
		Handler:   clientMux,
		TLSConfig: n.ClientFacingTLSConfig(),
	}
	var clientErr error
	if n.ClientFacingTLSConfig() != nil {
		clientErr = clientSrv.ListenAndServeTLS("", "")
	} else {
		clientErr = clientSrv.ListenAndServe()
	}
	if clientErr != nil {
		slog.Error("ZephyrCache client api error", "err", clientErr)
	}
}
