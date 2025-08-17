
# ZephyrCache — a self-healing distributed cache (MVP)
# Started Fall 2025
**Status:** Scaffold. First milestone compiles & runs a single-node HTTP API with pluggable ring/membership stubs.

## Features (planned)
- Consistent hashing ring with virtual nodes
- Gossip-based membership + phi accrual failure detector
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
- Week 1: Single-node KV with TTL + LRU, HTTP API, metrics.
- Week 2: Ring + replication factor, quorum reads/writes.
- Week 3: Gossip membership, hinted handoff, rebalancing hooks.
- Week 4: Anti-entropy (Merkle), chaos tests, dashboards.
