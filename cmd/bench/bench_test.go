package bench

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"math/rand"
	"net"
	"net/http"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ryandielhenn/zephyrcache/pkg/node"
)

var (
	numNodes  = flag.Int("n", 3, "number of cache nodes to spin up")
	conc      = flag.Int("c", 32, "concurrency")
	valSize   = flag.Int("b", 128, "value size in bytes")
	nReplicas = flag.Int("r", 3, "Replication Factor")
)

type cacheNode struct {
	httpAddr   string
	gossipAddr string
	n          *node.Node
	cleanup    func()
}

func startNode(id, seedGossipAddr string) cacheNode {
	parent := context.Background()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}
	httpAddr := listener.Addr().String()

	uconn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}
	gossipPort := strconv.Itoa(uconn.LocalAddr().(*net.UDPAddr).Port)
	gossipAddr := uconn.LocalAddr().String()
	_ = uconn.Close()

	config := node.ConfigWithOpts(id, httpAddr, gossipPort, *nReplicas, 50, 50, 150*time.Millisecond)

	ctx, cancel := context.WithCancel(parent)

	n := node.NewNode(config)
	go node.StartGossipListener(ctx, n)
	go node.StartGossipPinger(ctx, n,
		node.WithPeriod(50*time.Millisecond),
		node.WithPingTimeout(25*time.Millisecond),
	)
	if seedGossipAddr != "" {
		go n.ConnectToCluster(seedGossipAddr, 50*time.Millisecond)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/kv/", n.KvEventCallback())
	mux.HandleFunc("/replica/", n.ReplicaEventCallback())
	srv := &http.Server{Handler: mux}
	go func() {
		if err := srv.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Info("HTTP server error", "node", id, "err", err.Error())
		}
	}()

	return cacheNode{
		httpAddr:   httpAddr,
		gossipAddr: gossipAddr,
		n:          n,
		cleanup: func() {
			_ = srv.Close()
			cancel()
			n.Cleanup()
		},
	}
}

var (
	testAddrs  []string
	testClient *http.Client
)

func TestMain(m *testing.M) {
	flag.Parse()
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))

	nodes := make([]cacheNode, *numNodes)
	nodes[0] = startNode("node0", "")
	for i := 1; i < *numNodes; i++ {
		nodes[i] = startNode(fmt.Sprintf("node%d", i), nodes[0].gossipAddr)
	}

	// Wait for cluster convergence: every node must see all other peers.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		converged := true
		for _, nd := range nodes {
			if nd.n.PeerCount() < *numNodes-1 {
				converged = false
				break
			}
		}
		if converged {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	testAddrs = make([]string, *numNodes)
	for i, nd := range nodes {
		testAddrs[i] = "http://" + nd.httpAddr
	}

	testClient = &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			MaxIdleConnsPerHost: *conc,
			MaxIdleConns:        *conc * *numNodes,
		},
	}

	code := m.Run()

	for _, nd := range nodes {
		nd.cleanup()
	}
	os.Exit(code)
}

func BenchmarkPutGet(b *testing.B) {
	var counter atomic.Uint64
	payload := bytes.Repeat([]byte{byte(rand.Intn(255))}, *valSize)
	sem := make(chan struct{}, *conc)
	var wg sync.WaitGroup

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sem <- struct{}{}
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			defer func() { <-sem }()
			addr := testAddrs[counter.Add(1)%uint64(len(testAddrs))]
			key := fmt.Sprintf("k%d", i)
			putResp, _ := testClient.Post(addr+"/kv/"+key, "application/octet-stream", bytes.NewReader(payload))
			if putResp != nil {
				_, _ = io.Copy(io.Discard, putResp.Body)
				_ = putResp.Body.Close()
			}
			getResp, _ := testClient.Get(addr + "/kv/" + key)
			if getResp != nil {
				_, _ = io.Copy(io.Discard, getResp.Body)
				_ = getResp.Body.Close()
			}
		}(i)
	}
	wg.Wait()

	elapsed := b.Elapsed()
	b.ReportMetric(float64(b.N*2)/elapsed.Seconds(), "ops/s")
}
