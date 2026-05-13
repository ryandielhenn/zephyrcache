package node

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"testing"
	"time"
)

// tlsNode is a test node that serves /kv/ and /replica/ over HTTPS.
type tlsNode struct {
	n       *Node
	id      string
	addr    string // host:port of the HTTPS server
	cleanup func()
}

// newTLSNode starts a node whose replication endpoint uses mTLS.
// ca is the shared cluster CA; hosts is the list of IPs/hostnames the node's
// cert should be valid for (typically []string{"127.0.0.1"}).
func newTLSNode(t *testing.T, id string, ca *CA, hosts []string, nReplicas int, seedGossipAddr string) tlsNode {
	t.Helper()
	parent := context.Background()

	creds, err := GenerateNodeCert(ca, hosts)
	if err != nil {
		t.Fatalf("GenerateNodeCert: %v", err)
	}
	serverTLS, err := ServerTLSConfig(ca.CertPEM, creds)
	if err != nil {
		t.Fatalf("ServerTLSConfig: %v", err)
	}
	clientTLS, err := ClientTLSConfig(ca.CertPEM, creds)
	if err != nil {
		t.Fatalf("ClientTLSConfig: %v", err)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()

	// Claim a random UDP port, note it, release it, then let StartGossipListener
	// bind to it. The window between Close and ListenUDP is negligible in tests.
	uconn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("udp listen: %v", err)
	}
	gossipPort := strconv.Itoa(uconn.LocalAddr().(*net.UDPAddr).Port)
	gossipAddr := uconn.LocalAddr().String()
	_ = uconn.Close()

	cfg := ConfigWithOpts(id, addr, gossipPort, nReplicas, 50, 50, 150*time.Millisecond)
	n := NewNode(cfg)
	n.SetReplicaTLS(serverTLS, clientTLS)

	ctx, cancel := context.WithCancel(parent)

	go StartGossipListener(ctx, n)
	go StartGossipPinger(ctx, n,
		WithPeriod(50*time.Millisecond),
		WithPingTimeout(25*time.Millisecond),
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
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/replica/", func(w http.ResponseWriter, req *http.Request) {
		switch req.Method {
		case http.MethodPut, http.MethodPost:
			n.PutReplica(w, req)
		case http.MethodDelete:
			n.DelReplica(w, req)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	srv := &http.Server{Handler: mux}
	go func() {
		if err := srv.Serve(tls.NewListener(ln, serverTLS)); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Info("test TLS server error", "node", id, "err", err)
		}
	}()

	return tlsNode{
		n:    n,
		id:   id,
		addr: gossipAddr,
		cleanup: func() {
			_ = srv.Close()
			cancel()
			n.Cleanup()
		},
	}
}

// newClusterClient returns an HTTPS client that trusts the given CA and
// presents the given node credentials.
func newClusterClient(t *testing.T, ca *CA, creds *NodeTLSCreds) *http.Client {
	t.Helper()
	clientTLS, err := ClientTLSConfig(ca.CertPEM, creds)
	if err != nil {
		t.Fatalf("ClientTLSConfig: %v", err)
	}
	return &http.Client{
		Timeout:   5 * time.Second,
		Transport: &http.Transport{TLSClientConfig: clientTLS},
	}
}

// waitConverged blocks until every node in the cluster sees all peers as alive.
func waitConverged(t *testing.T, nodes []tlsNode, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		ok := true
		for _, nd := range nodes {
			nd.n.mu.Lock()
			alive := nd.n.countPeers()
			nd.n.mu.Unlock()
			if alive < len(nodes)-1 {
				ok = false
				break
			}
		}
		if ok {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("cluster did not converge within", timeout)
}

func TestReplicationPut(t *testing.T) {
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))

	ca, err := GenerateCA()
	if err != nil {
		t.Fatalf("GenerateCA: %v", err)
	}

	const numNodes = 10
	nodes := make([]tlsNode, numNodes)
	nodes[0] = newTLSNode(t, "node0", ca, []string{"127.0.0.1"}, numNodes, "")
	for i := 1; i < numNodes; i++ {
		nodes[i] = newTLSNode(t, fmt.Sprintf("node%d", i), ca, []string{"127.0.0.1"}, numNodes, nodes[0].addr)
	}
	t.Cleanup(func() {
		for _, nd := range nodes {
			nd.cleanup()
		}
	})

	waitConverged(t, nodes, 3*time.Second)

	clientCreds, err := GenerateNodeCert(ca, []string{"127.0.0.1"})
	if err != nil {
		t.Fatalf("GenerateNodeCert for client: %v", err)
	}
	client := newClusterClient(t, ca, clientCreds)

	key := "hello"
	value := []byte("world")

	// PUT via node 0's HTTPS endpoint.
	url := "https://" + nodes[0].n.config.addr + "/kv/" + key
	resp, err := client.Post(url, "application/octet-stream", bytes.NewReader(value))
	if err != nil {
		t.Fatalf("PUT: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("PUT status = %d, want 204", resp.StatusCode)
	}

	// Give async replication a moment to land.
	time.Sleep(100 * time.Millisecond)

	// Every node should have the key in its local store.
	for i, nd := range nodes {
		got, ok := nd.n.LocalGet(key)
		if !ok {
			t.Errorf("node%d: key %q not found after replication", i, key)
			continue
		}
		if !bytes.Equal(got, value) {
			t.Errorf("node%d: got %q, want %q", i, got, value)
		}
	}
}

func TestReplicationDelete(t *testing.T) {
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))

	ca, err := GenerateCA()
	if err != nil {
		t.Fatalf("GenerateCA: %v", err)
	}

	const numNodes = 10
	nodes := make([]tlsNode, numNodes)
	nodes[0] = newTLSNode(t, "node0", ca, []string{"127.0.0.1"}, numNodes, "")
	for i := 1; i < numNodes; i++ {
		nodes[i] = newTLSNode(t, fmt.Sprintf("node%d", i), ca, []string{"127.0.0.1"}, numNodes, nodes[0].addr)
	}
	t.Cleanup(func() {
		for _, nd := range nodes {
			nd.cleanup()
		}
	})

	waitConverged(t, nodes, 3*time.Second)

	clientCreds, err := GenerateNodeCert(ca, []string{"127.0.0.1"})
	if err != nil {
		t.Fatalf("GenerateNodeCert for client: %v", err)
	}
	client := newClusterClient(t, ca, clientCreds)

	key := "todelete"
	value := []byte("temporary")
	url := "https://" + nodes[0].n.config.addr + "/kv/" + key

	// PUT first, wait for replication.
	resp, err := client.Post(url, "application/octet-stream", bytes.NewReader(value))
	if err != nil {
		t.Fatalf("PUT: %v", err)
	}
	_ = resp.Body.Close()
	time.Sleep(100 * time.Millisecond)

	// DELETE via node 0.
	req, _ := http.NewRequest(http.MethodDelete, url, nil)
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("DELETE: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("DELETE status = %d, want 204", resp.StatusCode)
	}

	time.Sleep(100 * time.Millisecond)

	// The key should be absent on every node.
	for i, nd := range nodes {
		if _, ok := nd.n.LocalGet(key); ok {
			t.Errorf("node%d: key %q still present after delete replication", i, key)
		}
	}
}

func TestMTLSRejectsUnknownClient(t *testing.T) {
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))

	ca, err := GenerateCA()
	if err != nil {
		t.Fatalf("GenerateCA: %v", err)
	}

	nd := newTLSNode(t, "node0", ca, []string{"127.0.0.1"}, 1, "")
	t.Cleanup(nd.cleanup)

	// Build a rogue CA and present a cert signed by it — the server must reject it.
	rogueCA, err := GenerateCA()
	if err != nil {
		t.Fatalf("GenerateCA (rogue): %v", err)
	}
	rogueCreds, err := GenerateNodeCert(rogueCA, []string{"127.0.0.1"})
	if err != nil {
		t.Fatalf("GenerateNodeCert (rogue): %v", err)
	}
	// Client trusts the real CA (so it accepts the server cert) but presents a
	// rogue cert — the server should reject it at the TLS handshake.
	rogueTLS, err := ClientTLSConfig(ca.CertPEM, rogueCreds)
	if err != nil {
		t.Fatalf("ClientTLSConfig (rogue): %v", err)
	}
	rogueClient := &http.Client{
		Timeout:   3 * time.Second,
		Transport: &http.Transport{TLSClientConfig: rogueTLS},
	}

	url := "https://" + nd.n.config.addr + "/replica/testkey"
	req, _ := http.NewRequest(http.MethodPut, url, bytes.NewReader([]byte("val")))
	_, err = rogueClient.Do(req)
	if err == nil {
		t.Fatal("expected TLS error for rogue client, got nil")
	}
}
