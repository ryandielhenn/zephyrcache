// gencerts generates a CA, per-node replica certs (mTLS for node-to-node
// traffic), and a shared client cert (one-way TLS for client-to-node traffic).
//
// Usage:
//
//	gencerts [-out <dir>] [-nodes node0,node1,node2] [-hosts 127.0.0.1,localhost]
//
// Example — 3-node local cluster:
//
//	gencerts -out ./certs -nodes node0,node1,node2 -hosts 127.0.0.1
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/ryandielhenn/zephyrcache/pkg/node"
)

func main() {
	outDir := flag.String("out", "certs", "directory to write cert files into")
	nodesFlag := flag.String("nodes", "node0,node1,node2", "comma-separated node IDs")
	hostsFlag := flag.String("hosts", "127.0.0.1,localhost", "comma-separated IPs/hostnames to include in every cert SAN")
	flag.Parse()

	nodeIDs := splitTrim(*nodesFlag)
	hosts := splitTrim(*hostsFlag)

	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		log.Fatalf("create output dir: %v", err)
	}

	ca, err := node.GenerateCA()
	if err != nil {
		log.Fatalf("generate CA: %v", err)
	}
	write(*outDir, "ca.crt", ca.CertPEM)
	write(*outDir, "ca.key", ca.KeyPEM)
	fmt.Printf("wrote %s/ca.crt  %s/ca.key\n", *outDir, *outDir)

	abs, _ := filepath.Abs(*outDir)

	fmt.Println()
	fmt.Println("Per-node environment variables (node-to-node mTLS):")
	fmt.Println()

	for _, id := range nodeIDs {
		creds, err := node.GenerateNodeCert(ca, hosts)
		if err != nil {
			log.Fatalf("generate cert for %s: %v", id, err)
		}
		certFile := id + ".crt"
		keyFile := id + ".key"
		write(*outDir, certFile, creds.CertPEM)
		write(*outDir, keyFile, creds.KeyPEM)

		fmt.Printf("# %s\n", id)
		fmt.Printf("export REPLICA_CA_FILE=%s/ca.crt\n", abs)
		fmt.Printf("export REPLICA_CERT_FILE=%s/%s\n", abs, certFile)
		fmt.Printf("export REPLICA_KEY_FILE=%s/%s\n", abs, keyFile)
		fmt.Println()
	}

	// Dedicated cert for client-facing TLS (one-way TLS, shared across all nodes).
	clientCreds, err := node.GenerateNodeCert(ca, hosts)
	if err != nil {
		log.Fatalf("generate client cert: %v", err)
	}
	write(*outDir, "client.crt", clientCreds.CertPEM)
	write(*outDir, "client.key", clientCreds.KeyPEM)
	fmt.Printf("wrote %s/client.crt  %s/client.key\n", *outDir, *outDir)
	fmt.Println()
	fmt.Println("Client-facing environment variables (shared across all nodes):")
	fmt.Println()
	fmt.Printf("export CLIENT_CERT_FILE=%s/client.crt\n", abs)
	fmt.Printf("export CLIENT_KEY_FILE=%s/client.key\n", abs)
	fmt.Println()
}

func write(dir, name string, data []byte) {
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		log.Fatalf("write %s: %v", path, err)
	}
}

func splitTrim(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}
