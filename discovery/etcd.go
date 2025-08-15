package discovery

import (
	"context"
	"fmt"
	"log"
	"time"

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

func GetPeers(cli *clientv3.Client) (map[string]string, error) {
	resp, err := cli.Get(context.TODO(), "/zephyr/nodes", clientv3.WithPrefix())
	if err != nil {
		return nil, err
	}

	peers := make(map[string]string)
	for _, kv := range resp.Kvs {
		id := string(kv.Key[len("zephyr/nodes/"):]) 
		addr := string(kv.Value)
		peers[id] = addr
	}
	return peers, nil
}


func WatchPeers(cli *clientv3.Client, callback func(map[string]string)) {
	//TODO func WatchPeers
	peers, _ := GetPeers(cli)
	callback(peers)

	go func() {
		watchChan := cli.Watch(context.TODO(), "/zephyr/nodes/", clientv3.WithPrefix())
		for wresp := range watchChan {
			//1. 	Check if there was an error with the watch
			if wresp.Err() != nil {
				log.Printf("watch error: %v", wresp.Err())
				continue
			}
			//2.	Loop over wresp.Events (each PUT or DELETE).
			//3.	Update the local peers map to reflect the new cluster membership.
			//4.	Call your callback (cb) with the updated snapshot so your ring or router knows about the change.


		}
	}()
}
