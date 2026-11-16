# Drill: rolling-restart (rollout)

Run on 16 November 2026 at 11:57 UTC in namespace `faildrill-lab`: 50 requests per second per target, 2s caller timeout, 15s of baseline and 1m0s of observation.

| Target | Requests | Failed | Impact | Longest gap | p99 before | p99 after | Slowest after |
|---|---:|---:|---:|---:|---:|---:|---:|
| open | 3750 | 0 | none | none | 0.8ms | 2.4ms | 7.0ms |
| keyed | 3750 | 0 | none | none | 0.9ms | 2.6ms | 10.0ms |

Impact is the time from the first failed request to the last. Longest gap is the longest stretch in which every request failed.

- **open**: 3750 × HTTP 200
- **keyed**: 3750 × HTTP 200

## What the system did

- `deployment`: gateway-tyk-tyk-gateway
- `pods`: 2
- `rollout_s`: 9.0

## Timeline

Seconds are relative to the fault.

```
  -15.0s  baseline: 15s of traffic with nothing broken
   +0.0s  fault injected
   +0.0s  gateway-tyk-tyk-gateway-5c5f65f47c-lk4vv no longer ready
   +1.2s  gateway-tyk-tyk-gateway-56c6f95d97-t2v95 created
   +1.2s  gateway-tyk-tyk-gateway-56c6f95d97-vrctl created
   +3.4s  gateway-tyk-tyk-gateway-56c6f95d97-t2v95 ready
   +3.4s  gateway-tyk-tyk-gateway-56c6f95d97-vrctl ready
   +3.4s  gateway-tyk-tyk-gateway-5c5f65f47c-lk4vv gone
   +8.2s  gateway-tyk-tyk-gateway-5c5f65f47c-xjs6b no longer ready
   +9.0s  gateway-tyk-tyk-gateway-5c5f65f47c-xjs6b gone
   +9.0s  rollout complete: 2 new pods ready
  +60.0s  observation window over
```
