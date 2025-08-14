// TODO etcd-backed discovery
package discovery

import (
	"context"
	"fmt"
	"time"

	"go.etcd.io/etcd/client/v3"
)

func NewClient(endpoints []string) (*clientv3.Client, error) {
	return clientv3.New(clientv3.Config{
		Endpoints: endpoints,
		DialTimeout: 5 * time.Second,
	})
}

//TODO func RegisterNode
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

//TODO func GetPeers

//TODO func WatchPeers
