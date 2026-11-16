#!/bin/sh
# Bring up the lab: Redis with Sentinel, a MongoDB replica set, an upstream, and
# two Tyk gateways and a pump from Tyk's own open-source chart. Needs kubectl
# and helm pointed at a cluster you are happy to break things in.
set -eu

NS="${NS:-faildrill-lab}"
here="$(cd "$(dirname "$0")" && pwd)"

kubectl get namespace "$NS" >/dev/null 2>&1 || kubectl create namespace "$NS"
kubectl -n "$NS" apply -f "$here/redis-sentinel.yaml" -f "$here/mongodb.yaml" -f "$here/upstream-and-apis.yaml"
kubectl -n "$NS" rollout status statefulset/redis --timeout=180s
kubectl -n "$NS" rollout status statefulset/sentinel --timeout=180s
kubectl -n "$NS" rollout status statefulset/mongo --timeout=300s

# Form the MongoDB replica set once; a second run finds it already formed.
kubectl -n "$NS" exec mongo-0 -- mongosh --quiet --eval '
  try { rs.status().ok } catch (e) {
    rs.initiate({_id: "rs0", members: [
      {_id: 0, host: "mongo-0.mongo:27017"},
      {_id: 1, host: "mongo-1.mongo:27017"},
      {_id: 2, host: "mongo-2.mongo:27017"}]}).ok
  }' >/dev/null
members="mongodb://mongo-0.mongo,mongo-1.mongo,mongo-2.mongo/?replicaSet=rs0"
until kubectl -n "$NS" exec mongo-0 -- mongosh --quiet "$members" --eval 'db.hello().isWritablePrimary' 2>/dev/null |
  grep -q true; do
  echo "waiting for MongoDB to elect a primary"
  sleep 2
done

helm repo add tyk-helm https://helm.tyk.io/public/helm/charts/ >/dev/null 2>&1 || true
helm repo update tyk-helm >/dev/null
helm upgrade --install tyk tyk-helm/tyk-oss --version 5.5.0 -n "$NS" -f "$here/gateway-values.yaml"
kubectl -n "$NS" rollout status deployment/gateway-tyk-tyk-gateway --timeout=180s
kubectl -n "$NS" rollout status deployment/pump-tyk-tyk-pump --timeout=180s

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
