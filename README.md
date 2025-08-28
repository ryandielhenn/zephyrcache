
# ZephyrCache — a self-healing distributed cache (MVP)
# Started Fall 2025
**Status:** Scaffold. First milestone compiles & runs a single-node HTTP API with pluggable ring/membership stubs.

## Features (planned)
- Consistent hashing ring with virtual nodes
- Gossip/etcd-based membership
- phi accrual failure detector
- Tunable consistency (R/W quorums), replication factor
- Hinted handoff, read-repair, anti-entropy (Merkle)
- TTL + LRU eviction
- Prometheus metrics, Grafana dashboards
- Docker Compose to run a local cluster

## Quick start
```bash
# inside the deploy folder
docker compose up # build and run nodes
curl localhost:8080/healthz
curl -X PUT localhost:8080/kv/foo -d 'bar'
curl localhost:8080/kv/foo #TODO coordinator/proxy If the node is not the owner, it forwards the request to the correct node internally and returns the response.
```

## Roadmap

### Done
- Core KV store with TTL + LRU eviction
- HTTP API and basic metrics

### In Progress
- Cluster routing via consistent hash ring and request forwarding
- Next: replication factor (N), quorum reads/writes (R/W), hinted handoff, read repair

### Not Started
- Gossip-based membership and failure detection
- Rebalancing hooks for node joins/leaves
- Anti-entropy sync (Merkle trees)
- Chaos testing, dashboards, and alerts

## etcd Lease & KeepAlive Sequence
```text
ETCD LEASE LIFECYCLE (for membership keys like /zephyrcache/members/<node>)

Client (node1)                 etcd cluster                       Peers / Watchers
      |                              |                                    |
      |-- LeaseGrant(TTL=10s) ------>|                                    |
      |<-- LeaseGrantResp -----------|  (leaseID=0x1234, TTL=10s)         |
      |                              |                                    |
      |-- Put(/members/node1,        |                                    |
      |         value=...,           |                                    |
      |         lease=0x1234) ------>|                                    |
      |<------------- OK ------------|                                    |
      |                              |                                    |
      |==== bi-di stream: LeaseKeepAlive(0x1234) ====                     |
      |                              |                                    |
      |-- KeepAlive(0x1234) -------->|                                    |
      |<-- KeepAliveResp(TTL≈10s) ---|   (repeat every ~TTL/3)            |
      |-- KeepAlive(0x1234) -------->|                                    |
      |<-- KeepAliveResp(TTL≈10s) ---|                                    |
      |   ...continues while healthy...                                   |
      |                              |                                    |
      |   (node crash / network partition / process exit)                 |
      |   X                          |                                    |
      |                              |-- TTL counts down ---------------->|
      |                              |-- EXPIRE lease 0x1234 ------------>|
      |                              |-- DELETE /members/node1 ---------->|
      |                              |== WATCH EVENT ====================>|  notify: DELETE
      |                              |   (/members/node1, reason=EXPIRED) |  peers react (evict, rebalance)
      |                              |                                    |
      |                              |                                    |
      |   (optional graceful shutdown)                                    |
      |-- LeaseRevoke(0x1234) ------>|                                    |
      |<------------- OK ------------|-- DELETE /members/node1 ---------->|
      |                              |== WATCH EVENT ====================>|  notify: DELETE (reason=REVOKED)
      |                              |                                    |
```
```markdown
Notes:
- Attach all ephemeral membership/heartbeat keys to the same lease (e.g., leaseID 0x1234).
- KeepAlive is a long-lived gRPC stream; send pings around TTL/3 to maintain headroom.
- On lease expiry or revoke, etcd deletes all keys bound to that lease and emits watch events.
- Peers watch the prefix (/zephyrcache/members/) to detect joins/leaves promptly.
```

