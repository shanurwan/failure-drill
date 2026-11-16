# faildrill

Failure drills for [Tyk](https://github.com/TykTechnologies/tyk) on Kubernetes.

`faildrill` breaks one thing in a running Tyk install while traffic flows through the gateway, and reports what the callers saw: how many requests failed, as what, for how long, and how slow the rest became.

There is a short hands-on course built from recorded runs: **https://shanurwan.github.io/failure-drill/**. You type the commands and the page replays what the real command printed, next to a 3D view of what the system did.

## Why

A checklist can tell you that your gateways have no disruption budget or that Redis has no replica. It cannot tell you what a caller gets when the Redis primary freezes for thirty seconds: an error, a slow answer, or nothing they would notice. The only way to know is to do it and count.

This is one run on a lab cluster, with a Redis primary made to stop answering:

```
drill      redis-primary (hang)
namespace  faildrill-lab
traffic    open   50 req/s  http://192.168.188.116:32472/open/get
traffic    keyed  50 req/s  http://192.168.188.116:32472/keyed/get
plan       make redis-1 (primary, 2 replicas) stop answering for 30s

    -15.0s  baseline: 15s of traffic with nothing broken
     +0.0s  fault injected
     +1.2s  redis-1 stopped answering
     +7.4s  redis-0 promoted to primary
    +30.1s  redis-1 is answering again, still as primary
    +40.3s  redis-1 is back, now as a replica
    +60.0s  observation window over

TARGET    REQUESTS   FAILED     IMPACT  LONGEST GAP P99 BEFORE  P99 AFTER
open          3737        0       none         none      3.5ms     12.7ms
keyed         3738       32       1.1s         0.1s      3.7ms     2000ms

open     3737 × HTTP 200
keyed    3706 × HTTP 200, 32 × timeout
         failures from +5.0s to +6.1s after the fault
```

The API that needs no key lost nothing. The API that needs a key held 32 callers until they gave up, in the second before Sentinel promoted a replica. And for ten seconds after the old primary woke up, two pods both believed they were primary.

Run the same drill again and the count is different: across three recorded runs, 0, 19 and 32 keyed requests timed out. That is why the tool keeps every record.

A second drill kills the primary's pod and lets Kubernetes restart it. It was recorded ten times. Six runs lost nothing and one timed out six requests. In three, callers holding a valid key were answered with **HTTP 403**, up to 85 requests in one run. The gateway's log for those seconds reads `storage: Redis is either down or was not configured` followed by `Attempted access with non-existent key.`: a lookup that failed was treated as a key that does not exist. The records and two of those logs are in [`results/2026-11-16/`](results/2026-11-16/).

## Install

```sh
go install github.com/shanurwan/failure-drill/cmd/faildrill@latest
```

It uses your kubeconfig the way `kubectl` does.

## Use

Read the install first. This changes nothing:

```sh
faildrill preflight -n tyk
```

Then break one thing. Give it the addresses to call during the drill, each with a name:

```sh
faildrill run redis-primary --fault hang -n tyk \
  --url open=https://gw.example.com/status \
  --url keyed=https://gw.example.com/orders \
  --header 'keyed=Authorization: YOUR-KEY'
```

It asks you to type the namespace before it breaks anything (`--yes` skips the question). When the drill ends it prints the table above and writes a JSON record and a Markdown report to `drills/`.

| Drill | `--fault` | What it does |
|---|---|---|
| `redis-primary` | `hang` | Makes the primary stop answering for `--hold` (30s) with `CLIENT PAUSE`. It stays up, like a frozen node or a stalled disk. |
| `redis-primary` | `crash` | Kills the primary's pod without warning and lets Kubernetes restart it. |
| `gateway-pod` | `crash` | Kills one gateway pod without warning. |
| `gateway-pod` | `evict` | Deletes one gateway pod with its grace period, as a node drain does. |
| `rolling-restart` | | Restarts every gateway pod, as `kubectl rollout restart` and every chart upgrade do. |

Flags worth knowing:

| Flag | Default | |
|---|---|---|
| `--rps` | 50 | Requests per second, per target |
| `--timeout` | 2s | How long a caller waits for one request |
| `--baseline` | 15s | Traffic before the fault |
| `--observe` | 60s | Traffic after the fault |
| `--redis-selector` | `app.kubernetes.io/name=redis` | Label selector of the Redis pods |
| `--redis-cli-args` | | Extra `redis-cli` arguments, such as `-a $PASSWORD` |
| `--gateway` | found by image | Gateway Deployment name |
| `--out` | `drills` | Where records and reports go |

## What it touches

`faildrill` needs to list pods, Deployments and PodDisruptionBudgets in the namespace. Depending on the drill it also deletes one pod, runs `redis-cli` in the Redis pods, or patches the gateway Deployment's restart annotation. It creates nothing and leaves nothing behind. Every fault is one the cluster recovers from by itself: a deleted pod is replaced by its controller, and `CLIENT PAUSE` ends on its own.

Start in a cluster you are allowed to break.

## How it measures

- **The rate is open-loop.** A request is sent on every tick whether or not earlier ones have returned. A gateway that stalls shows up as slow answers and timeouts, as it does for real callers.
- **Times are counted from the fault.** Fifteen seconds of traffic come first. A failure during that baseline is reported separately, because it means the system was not healthy to begin with.
- **Impact** is the time from the first failed request to the last. **Longest gap** is the longest stretch in which every request failed. Scattered failures give a long impact and a short gap.
- **Outcomes keep the exact status.** A 403 for a valid key and a 503 are different things to tell a caller, so they are counted separately.

## The lab

`lab/` brings up something to practise on: three Redis pods with three Sentinels, a small upstream, and two gateways installed from Tyk's own open-source chart (`tyk-helm/tyk-oss`).

```sh
lab/up.sh       # needs kubectl and helm
lab/down.sh
```

## The records

`results/2026-11-16/` holds 22 runs: the full JSON record of each drill, with every request, the Markdown report the tool wrote, and the gateway logs of two runs. `site/data/runs.json` is made from the records by `faildrill export`, and CI fails if the two disagree.

They were recorded on a single-node k3s cluster with Tyk Gateway v5.13.5. They describe that lab on that day. One node means a lost node cannot be imitated, and open-source Tyk means nothing here covers a multi-data-centre setup.

## Licence

MIT. This is an independent project and is not affiliated with Tyk Technologies.
