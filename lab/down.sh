#!/bin/sh
# Remove the lab and everything in it, the Redis volumes included.
set -eu
NS="${NS:-faildrill-lab}"
helm uninstall tyk -n "$NS" 2>/dev/null || true
kubectl delete namespace "$NS" --ignore-not-found
