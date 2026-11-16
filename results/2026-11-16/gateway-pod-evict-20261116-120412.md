# Drill: gateway-pod (evict)

Run on 16 November 2026 at 12:04 UTC in namespace `faildrill-lab`: 50 requests per second per target, 2s caller timeout, 15s of baseline and 1m0s of observation.

| Target | Requests | Failed | Impact | Longest gap | p99 before | p99 after | Slowest after |
|---|---:|---:|---:|---:|---:|---:|---:|
| open | 3750 | 0 | none | none | 2.5ms | 1.9ms | 5.5ms |
| keyed | 3750 | 1 | 0.0s | 0.0s | 2.6ms | 2.0ms | 5.1ms |

Impact is the time from the first failed request to the last. Longest gap is the longest stretch in which every request failed.

- **open**: 3750 × HTTP 200
- **keyed**: 3749 × HTTP 200, 1 × refused

## What the system did

- `deployment`: gateway-tyk-tyk-gateway
- `full_strength_s`: 2.0
- `pod_removed`: gateway-tyk-tyk-gateway-56c6f95d97-vrctl
- `ready_before`: 2

## Timeline

Seconds are relative to the fault.

```
  -15.0s  baseline: 15s of traffic with nothing broken
   +0.0s  fault injected
   +0.0s  gateway-tyk-tyk-gateway-56c6f95d97-vrctl no longer ready
   +1.2s  gateway-tyk-tyk-gateway-56c6f95d97-swjn9 created
   +2.0s  gateway-tyk-tyk-gateway-56c6f95d97-swjn9 ready
   +2.0s  gateway-tyk-tyk-gateway-56c6f95d97-vrctl gone
   +2.0s  back to 2 ready pods
  +60.0s  observation window over
```
