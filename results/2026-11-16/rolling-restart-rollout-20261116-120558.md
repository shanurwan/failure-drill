# Drill: rolling-restart (rollout)

Run on 16 November 2026 at 12:05 UTC in namespace `faildrill-lab`: 50 requests per second per target, 2s caller timeout, 15s of baseline and 1m0s of observation.

| Target | Requests | Failed | Impact | Longest gap | p99 before | p99 after | Slowest after |
|---|---:|---:|---:|---:|---:|---:|---:|
| open | 3751 | 0 | none | none | 1.7ms | 2.6ms | 8.5ms |
| keyed | 3751 | 0 | none | none | 1.7ms | 2.7ms | 13.3ms |

Impact is the time from the first failed request to the last. Longest gap is the longest stretch in which every request failed.

- **open**: 3751 × HTTP 200
- **keyed**: 3751 × HTTP 200

## What the system did

- `deployment`: gateway-tyk-tyk-gateway
- `pods`: 2
- `rollout_s`: 9.1

## Timeline

Seconds are relative to the fault.

```
  -15.0s  baseline: 15s of traffic with nothing broken
   +0.0s  fault injected
   +1.0s  gateway-tyk-tyk-gateway-56c6f95d97-swjn9 no longer ready
   +1.0s  gateway-tyk-tyk-gateway-65d6784795-pvfnx created
   +1.0s  gateway-tyk-tyk-gateway-65d6784795-x9wvd created
   +2.2s  gateway-tyk-tyk-gateway-56c6f95d97-swjn9 gone
   +3.2s  gateway-tyk-tyk-gateway-65d6784795-pvfnx ready
   +3.2s  gateway-tyk-tyk-gateway-65d6784795-x9wvd ready
   +8.2s  gateway-tyk-tyk-gateway-56c6f95d97-wg24l no longer ready
   +9.1s  gateway-tyk-tyk-gateway-56c6f95d97-wg24l gone
   +9.1s  rollout complete: 2 new pods ready
  +60.0s  observation window over
```
