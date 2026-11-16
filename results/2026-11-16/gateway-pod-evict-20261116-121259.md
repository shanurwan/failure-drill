# Drill: gateway-pod (evict)

Run on 16 November 2026 at 12:12 UTC in namespace `faildrill-lab`: 50 requests per second per target, 2s caller timeout, 15s of baseline and 1m0s of observation.

| Target | Requests | Failed | Impact | Longest gap | p99 before | p99 after | Slowest after |
|---|---:|---:|---:|---:|---:|---:|---:|
| open | 3750 | 0 | none | none | 1.1ms | 1.9ms | 5.3ms |
| keyed | 3750 | 0 | none | none | 1.2ms | 2.0ms | 6.3ms |

Impact is the time from the first failed request to the last. Longest gap is the longest stretch in which every request failed.

- **open**: 3750 × HTTP 200
- **keyed**: 3750 × HTTP 200

## What the system did

- `deployment`: gateway-tyk-tyk-gateway
- `full_strength_s`: 3.3
- `pod_removed`: gateway-tyk-tyk-gateway-65d6784795-qxmt8
- `ready_before`: 2

## Timeline

Seconds are relative to the fault.

```
  -15.0s  baseline: 15s of traffic with nothing broken
   +0.0s  fault injected
   +0.0s  gateway-tyk-tyk-gateway-65d6784795-qxmt8 no longer ready
   +1.2s  gateway-tyk-tyk-gateway-65d6784795-g6rqc created
   +2.1s  gateway-tyk-tyk-gateway-65d6784795-qxmt8 gone
   +3.3s  gateway-tyk-tyk-gateway-65d6784795-g6rqc ready
   +3.3s  back to 2 ready pods
  +60.0s  observation window over
```
