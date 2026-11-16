# Drill: rolling-restart (rollout)

Run on 16 November 2026 at 12:14 UTC in namespace `faildrill-lab`: 50 requests per second per target, 2s caller timeout, 15s of baseline and 1m0s of observation.

| Target | Requests | Failed | Impact | Longest gap | p99 before | p99 after | Slowest after |
|---|---:|---:|---:|---:|---:|---:|---:|
| open | 3750 | 0 | none | none | 0.9ms | 4.5ms | 10.8ms |
| keyed | 3750 | 1 | 0.0s | 0.0s | 1.1ms | 4.3ms | 10.6ms |

Impact is the time from the first failed request to the last. Longest gap is the longest stretch in which every request failed.

- **open**: 3750 × HTTP 200
- **keyed**: 3749 × HTTP 200, 1 × refused

## What the system did

- `deployment`: gateway-tyk-tyk-gateway
- `pods`: 2
- `rollout_s`: 10.0

## Timeline

Seconds are relative to the fault.

```
  -15.0s  baseline: 15s of traffic with nothing broken
   +0.0s  fault injected
   +0.0s  gateway-tyk-tyk-gateway-79dfbdbf5-52vjt created
   +1.2s  gateway-tyk-tyk-gateway-65d6784795-g6rqc no longer ready
   +1.2s  gateway-tyk-tyk-gateway-79dfbdbf5-6vpzv created
   +2.1s  gateway-tyk-tyk-gateway-65d6784795-g6rqc gone
   +3.1s  gateway-tyk-tyk-gateway-79dfbdbf5-52vjt ready
   +3.1s  gateway-tyk-tyk-gateway-79dfbdbf5-6vpzv ready
   +8.0s  gateway-tyk-tyk-gateway-65d6784795-x9wvd no longer ready
  +10.0s  gateway-tyk-tyk-gateway-65d6784795-x9wvd gone
  +10.0s  rollout complete: 2 new pods ready
  +60.0s  observation window over
```
