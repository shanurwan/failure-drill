# Drill: gateway-pod (crash)

Run on 16 November 2026 at 12:02 UTC in namespace `faildrill-lab`: 50 requests per second per target, 2s caller timeout, 15s of baseline and 1m0s of observation.

| Target | Requests | Failed | Impact | Longest gap | p99 before | p99 after | Slowest after |
|---|---:|---:|---:|---:|---:|---:|---:|
| open | 3750 | 1 | 0.0s | 0.0s | 3.5ms | 3.1ms | 14.9ms |
| keyed | 3750 | 1 | 0.0s | 0.0s | 3.5ms | 3.2ms | 18.7ms |

Impact is the time from the first failed request to the last. Longest gap is the longest stretch in which every request failed.

- **open**: 3749 × HTTP 200, 1 × refused
- **keyed**: 3749 × HTTP 200, 1 × refused

## What the system did

- `deployment`: gateway-tyk-tyk-gateway
- `full_strength_s`: 3.0
- `pod_removed`: gateway-tyk-tyk-gateway-56c6f95d97-t2v95
- `ready_before`: 2

## Timeline

Seconds are relative to the fault.

```
  -15.0s  baseline: 15s of traffic with nothing broken
   +0.0s  fault injected
   +0.2s  gateway-tyk-tyk-gateway-56c6f95d97-t2v95 gone
   +1.0s  gateway-tyk-tyk-gateway-56c6f95d97-wg24l created
   +3.0s  gateway-tyk-tyk-gateway-56c6f95d97-wg24l ready
   +3.0s  back to 2 ready pods
  +60.0s  observation window over
```
