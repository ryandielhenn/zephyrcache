package node

import (
	"context"
	"log"
	"log/slog"
	"strings"

	discovery "github.com/ryandielhenn/zephyrcache/pkg/registry"
	clientv3 "go.etcd.io/etcd/client/v3"
)

func BootstrapPeers(node *Node, cli *clientv3.Client) func() {
	// Bootstrap peers into this ring
	resp, err := cli.Get(context.TODO(), "/zephyr/nodes", clientv3.WithPrefix())
	if err != nil {
		log.Fatal(err)
	}
	for _, kv := range resp.Kvs {
		nodeID := strings.TrimPrefix(string(kv.Key), "/zephyr/nodes/")
		peerHP := NormalizeHostPort(string(kv.Value), "8080")
		slog.Info("[Bootstrap]", "node", nodeID, "peer", peerHP)
		node.addPeer(nodeID, peerHP)
	}

	config := node.config
	slog.Info("[Boot] registering, with etcd", "node.id", config.id, "node.addr", config.addr)
	leaseId, cancel, err := discovery.RegisterNode(cli, config.id, config.addr, 10)
	if err != nil {
		log.Fatal(err)
	}

	cleanup := func() {
		cancel()
		_, _ = cli.Revoke(context.TODO(), leaseId)
	}
	return cleanup

}

func WatchPeers(node *Node, cli *clientv3.Client) {
	// Watch for updates about peers
	slog.Info("[Boot] before watch peers")
	discovery.WatchPeers(cli, func(peers map[string]string) {
		normalizedPeers := make(map[string]string, len(peers))
		for id, addr := range peers {
			normalizedPeers[id] = NormalizeHostPort(addr, "8080")
		}
		node.syncPeers(normalizedPeers)
		slog.Info("[WatchPeers Callback] synced", "peers", len(peers))
	})
	slog.Info("[BOOT] after WatchPeers")
}
