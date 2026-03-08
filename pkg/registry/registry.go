package discovery

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"maps"
	"strings"
	"sync"
	"time"

	"go.etcd.io/etcd/api/v3/mvccpb"
	"go.etcd.io/etcd/client/v3"
)

func NewClient(endpoints []string) (*clientv3.Client, error) {
	return clientv3.New(clientv3.Config{
		Endpoints:   endpoints,
		DialTimeout: 5 * time.Second,
	})
}

func RegisterNode(cli *clientv3.Client, id, addr string, ttl int64) (clientv3.LeaseID, context.CancelFunc, error) {
	slog.Info("RegisterNode - granting lease")
	ctx, cancel := context.WithCancel(context.Background())
	lease, err := cli.Grant(ctx, ttl)
	if err != nil {
		return 0, cancel, err
	}
	key := fmt.Sprintf("/zephyr/nodes/%s", id)
	slog.Info("RegisterNode - putting", "key", key, "addr", addr, "lease", lease.ID)
	_, err = cli.Put(ctx, key, addr, clientv3.WithLease(lease.ID))
	if err != nil {
		return 0, cancel, err
	}

	slog.Info("Sending keepalive to", "lease", lease.ID)
	ch, err := cli.KeepAlive(ctx, lease.ID)
	if err != nil {
		log.Fatal(err)
	}
	go func() {
		for resp := range ch {
			if resp == nil {
				slog.Info("keepalive channel closed")
				return
			}
		}
	}()

	return lease.ID, cancel, nil
}

func GetPeers(cli *clientv3.Client, prefix string) (map[string]string, error) {
	resp, err := cli.Get(context.TODO(), prefix, clientv3.WithPrefix())
	if err != nil {
		return nil, err
	}

	peers := make(map[string]string)
	for _, kv := range resp.Kvs {
		id := strings.TrimPrefix(string(kv.Key), prefix)
		peers[id] = string(kv.Value)
	}
	return peers, nil
}

func WatchPeers(cli *clientv3.Client, callback func(map[string]string)) {
	const prefix = "/zephyr/nodes/"
	slog.Info("[WATCH] starting WatchPeers on", "prefix", prefix)
	peers, err := GetPeers(cli, prefix)
	if err != nil {
		slog.Error("[WATCH] GetPeers failed", "error", err.Error())
	} else {
		slog.Info("[WATCH] bootstrap snapshot", "peers", len(peers))
	}
	callback(maps.Clone(peers))

	var (
		mu sync.Mutex
	)

	go func() {
		slog.Info("[WATCH] establishing watch on", "etcd prefix", prefix)
		watchChan := cli.Watch(context.TODO(), prefix, clientv3.WithPrefix())
		slog.Info("[WATCH] watch established")
		for wresp := range watchChan {
			if wresp.Err() != nil {
				slog.Error("watch error: ", "error", wresp.Err().Error())
				continue
			}

			mu.Lock()
			for _, ev := range wresp.Events {
				switch ev.Type {
				case mvccpb.PUT:
					id := strings.TrimPrefix(string(ev.Kv.Key), prefix)
					addr := string(ev.Kv.Value)
					peers[id] = addr
				case mvccpb.DELETE:
					id := strings.TrimPrefix(string(ev.Kv.Key), prefix)
					delete(peers, id)
				}
			}
			snap := maps.Clone(peers)
			mu.Unlock()
			callback(snap)
		}
	}()
}
