# ZephyrCache — a self-healing distributed cache

## Started
- Docker Compose to run a local cluster
- Core KV store
- HTTP API and basic metrics
- etcd-backed membership via leases (ephemeral keys, watch-based join/leave)
- Cluster routing via consistent hash ring and request forwarding
- Gossip-based membership and failure detection (skeleton not working yet defaulting to etcd)
- Prometheus metrics, Grafana dashboards

## Quick start
```bash
# Run with default configuration (1 cache node, 1 etcd node, default docker network)
docker-compose -f deploy/docker-compose.yml up -d

# Test the cluster
curl localhost:8080/healthz
curl -X PUT localhost:8080/kv/foo -d 'bar'
curl localhost:8080/kv/foo

# Scale to more cache nodes
docker-compose -f deploy/docker-compose.yml up -d --scale node=10
docker-compose -f deploy/docker-compose.yml up -d --scale node=50
```


## Viewing logs
```bash
# View logs from all nodes in real-time
docker-compose -f deploy/docker-compose.yml logs -f node

# View logs from all services (etcd + nodes)
docker-compose -f deploy/docker-compose.yml logs -f

# View logs from a specific container
docker logs -f deploy-node-1

# View last 100 lines
docker-compose -f deploy/docker-compose.yml logs --tail 100 node
```

## Stopping the cluster
```bash
# Stop all containers
docker-compose -f deploy/docker-compose.yml down
```

## etcd Lease Sequence
![etcd Lease Sequence](diagrams/etcd-lease-sequence/diagram.png)

- Each node attaches all ephemeral membership keys to a single lease (e.g., `leaseID 0x1234`)
- `KeepAlive` runs as a long-lived gRPC stream; nodes send heartbeat pings every ~TTL/3 to maintain the lease
- When a lease expires or is revoked, etcd automatically deletes all bound keys and emits watch events to peers watching `/zephyrcache/members/`


## Request Forwarding
Clients can send requests to any node in the cluster without needing to know which node owns the data.

![Request Forwarding](diagrams/request-forwarding/diagram.png)

This raises the question: **"How does a node determine which node owns a given key?"**

## Consistent Hashing
Each node uses consistent hashing to route requests to the correct owner:

**How it works:**
- The hash ring spans positions from 0 to 2^m-1
- Each node is assigned one or more token positions (t1, t2, t3, t4) on the ring
- When a key arrives, hash(k) maps it to a position p on the ring
- The owner is the first node found traveling clockwise from position p

**Example:**
![Consistent Hashing Example](diagrams/consistent_hashing/diagram.png)

## Not Started
- Replication factor (N), quorum reads/writes (tunable consistency)
- Rebalancing hooks for node joins/leaves
- Anti-entropy sync (Merkle trees)
- Chaos testing, improved dashboards, and alerts
- Phi accrual failure detector
- Hinted handoff, read-repair, anti-entropy (Merkle)
