// TODO etcd-backed discovery
package discovery

import (
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

//TODO func GetPeers

//TODO func WatchPeers
