#!/bin/sh
set -e

# Get the container's hostname (Docker sets this to node-1, node-2, etc.)
CONTAINER_HOSTNAME=$(hostname)

# Set SELF_ID if not already set
if [ -z "$SELF_ID" ]; then
    export SELF_ID="$CONTAINER_HOSTNAME"
fi

# Set SELF_ADDR if not already set
if [ -z "$SELF_ADDR" ]; then
    export SELF_ADDR="http://$CONTAINER_HOSTNAME:8080"
fi

echo "Starting node with:"
echo "  SELF_ID=$SELF_ID"
echo "  SELF_ADDR=$SELF_ADDR"
echo "  ETCD_ENDPOINTS=$ETCD_ENDPOINTS"
echo "  CLUSTER=$CLUSTER"

# Execute the main application
exec "$@"
