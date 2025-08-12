
# Architecture (MVP → Full)

- MVP: single-node in-memory KV, TTL + LRU, HTTP API, bench tool.
- Next: consistent hashing ring, replication factor, quorum R/W.
- Later: gossip membership, hinted handoff, anti-entropy with Merkle trees.
