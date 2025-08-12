#!/usr/bin/env bash
set -euo pipefail
docker kill $(docker ps --format '{{.Names}}' | head -n1)
