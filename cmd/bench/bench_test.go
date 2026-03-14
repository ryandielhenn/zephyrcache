package bench

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ryandielhenn/zephyrcache/pkg/kv"
	"github.com/ryandielhenn/zephyrcache/pkg/node"
	"github.com/ryandielhenn/zephyrcache/pkg/ring"
)

var (
	numNodes = flag.Int("nodes", 3, "number of cache nodes to spin up")
	numReqs  = flag.Int("req", 5000, "number of requests")
	conc     = flag.Int("c", 32, "concurrency")
	valSize  = flag.Int("val", 128, "value size in bytes")
)

type cacheNode struct {
	httpAddr   string
	gossipAddr string
	cleanup    func()
	uconn      net.PacketConn
}

func startNode(id, seedGossipAddr string) cacheNode {
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

	store := kv.NewStore(64 << 20)
	r := ring.New(128, ring.FNV32a)
	r.Add(id, httpAddr)

	n := node.NewNode(store, r, id, httpAddr, gossipPort)
	go node.StartGossipListener(n)
	go node.StartGossipPinger(n,
		node.WithPeriod(50*time.Millisecond),
		node.WithTimeout(50*time.Millisecond),
	)
	if seedGossipAddr != "" {
		go n.ConnectToCluster(seedGossipAddr, 50*time.Millisecond)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/kv/", func(w http.ResponseWriter, req *http.Request) {
		switch req.Method {
		case http.MethodPut, http.MethodPost:
			n.Put(w, req)
		case http.MethodGet:
			n.Get(w, req)
		case http.MethodDelete:
			n.Del(w, req)
		}
	})
	srv := &http.Server{Handler: mux}
	go srv.Serve(listener)

	return cacheNode{
		httpAddr:   httpAddr,
		gossipAddr: gossipAddr,
		uconn:      uconn,
		cleanup: func() {
			srv.Close()
			uconn.Close()
		},
	}
}

func BenchmarkPutGet(b *testing.B) {
	nodes := make([]cacheNode, *numNodes)
	nodes[0] = startNode("node0", "")
	for i := 1; i < *numNodes; i++ {
		nodes[i] = startNode(fmt.Sprintf("node%d", i), nodes[0].gossipAddr)
	}
	defer func() {
		for _, nd := range nodes {
			nd.cleanup()
		}
	}()

	b.Logf("waiting for %d-node cluster to converge...", *numNodes)
	time.Sleep(500 * time.Millisecond)

	addrs := make([]string, *numNodes)
	for i, nd := range nodes {
		addrs[i] = "http://" + nd.httpAddr
	}
	b.Logf("cluster ready: %v", addrs)

	client := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			MaxIdleConnsPerHost: *conc,
			MaxIdleConns:        *conc * *numNodes,
		},
	}
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
			addr := addrs[counter.Add(1)%uint64(len(addrs))]
			key := fmt.Sprintf("k%d", i)
			_, _ = client.Post(addr+"/kv/"+key, "application/octet-stream", bytes.NewReader(payload))
			resp, _ := client.Get(addr + "/kv/" + key)
			if resp != nil {
				io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
			}
		}(i)
	}
	wg.Wait()

	elapsed := b.Elapsed()
	b.ReportMetric(float64(b.N*2)/elapsed.Seconds(), "ops/s")
}
