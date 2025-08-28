#!/usr/bin/env bash
set -euo pipefail

# Kill the first running Zephyrcache container
target=$(docker ps --format '{{.Names}}' | grep zephyrcache | head -n1 || true)

if [[ -z "${target}" ]]; then
  echo "No Zephyrcache container is running."
  exit 1
fi

echo "Killing container: ${target}"
  docker kill "${target}"
