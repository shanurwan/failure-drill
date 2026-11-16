#!/bin/sh
# Bring up the lab: Redis with Sentinel, an upstream, and two Tyk gateways from
# Tyk's own open-source chart. Needs kubectl and helm pointed at a cluster you
# are happy to break things in.
set -eu

NS="${NS:-faildrill-lab}"
here="$(cd "$(dirname "$0")" && pwd)"

kubectl get namespace "$NS" >/dev/null 2>&1 || kubectl create namespace "$NS"
kubectl -n "$NS" apply -f "$here/redis-sentinel.yaml" -f "$here/upstream-and-apis.yaml"
kubectl -n "$NS" rollout status statefulset/redis --timeout=180s
kubectl -n "$NS" rollout status statefulset/sentinel --timeout=180s

helm repo add tyk-helm https://helm.tyk.io/public/helm/charts/ >/dev/null 2>&1 || true
helm repo update tyk-helm >/dev/null
helm upgrade --install tyk tyk-helm/tyk-oss --version 5.5.0 -n "$NS" -f "$here/gateway-values.yaml"
kubectl -n "$NS" rollout status deployment/gateway-tyk-tyk-gateway --timeout=180s

# One API key for the keyed API. Keys live in Redis, so any gateway can create
# it. The gateway image has no shell, so the request is sent from a Redis pod.
kubectl -n "$NS" exec redis-0 -- \
  wget -q -O - --header "x-tyk-authorization: lab-admin-secret" \
  --post-data '{"org_id":"lab","rate":1000,"per":1,"quota_max":-1,"access_rights":{"keyed":{"api_id":"keyed","api_name":"keyed","versions":["Default"]}}}' \
  http://gateway-svc-tyk-tyk-gateway:8080/tyk/keys/lab-key >/dev/null

port="$(kubectl -n "$NS" get service gateway-svc-tyk-tyk-gateway -o jsonpath='{.spec.ports[0].nodePort}')"
cat <<EOF

The lab is up in namespace $NS. The gateway listens on port $port of every node:

  GW=http://<a node address>:$port
  faildrill preflight -n $NS
  faildrill run redis-primary --fault hang -n $NS \\
    --url open=\$GW/open/get --url keyed=\$GW/keyed/get \\
    --header 'keyed=Authorization: lab-key'
EOF
