package discovery

import (
	"context"
	"fmt"
	"log"
	"maps"
	"strings"
	"sync"
	"time"

	"go.etcd.io/etcd/api/v3/mvccpb"
	"go.etcd.io/etcd/client/v3"
)

func NewClient(endpoints []string) (*clientv3.Client, error) {
	return clientv3.New(clientv3.Config{
		Endpoints: endpoints,
		DialTimeout: 5 * time.Second,
	})
}

func RegisterNode(cli *clientv3.Client, id, addr string, ttl int64) (clientv3.LeaseID, error) {
	lease, err := cli.Grant(context.TODO(), ttl)
	if err != nil {
		return 0, err
	}
	key := fmt.Sprintf("/zephyr/nodes/%s", id)
    _, err = cli.Put(context.TODO(), key, addr, clientv3.WithLease(lease.ID))
    if err != nil {
        return 0, err
    }

    go cli.KeepAlive(context.TODO(), lease.ID)

    return lease.ID, nil
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
	//TODO func WatchPeers
	const prefix = "/zephyr/nodes/"
	peers, _ := GetPeers(cli, prefix)
	callback(maps.Clone(peers))

	var(
		mu sync.Mutex
	)

	go func() {
		watchChan := cli.Watch(context.TODO(), prefix, clientv3.WithPrefix())
		for wresp := range watchChan {
			//1. 	Check if there was an error with the watch
			if wresp.Err() != nil {
				log.Printf("watch error: %v", wresp.Err())
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
