# Drill: gateway-pod (evict)

Run on 16 November 2026 at 11:55 UTC in namespace `faildrill-lab`: 50 requests per second per target, 2s caller timeout, 15s of baseline and 1m0s of observation.

| Target | Requests | Failed | Impact | Longest gap | p99 before | p99 after | Slowest after |
|---|---:|---:|---:|---:|---:|---:|---:|
| open | 3750 | 1 | 0.0s | 0.0s | 1.1ms | 2.1ms | 4.0ms |
| keyed | 3750 | 0 | none | none | 1.1ms | 2.2ms | 4.2ms |

Impact is the time from the first failed request to the last. Longest gap is the longest stretch in which every request failed.

- **open**: 3749 × HTTP 200, 1 × refused
- **keyed**: 3750 × HTTP 200

## What the system did

- `deployment`: gateway-tyk-tyk-gateway
- `full_strength_s`: 3.2
- `pod_removed`: gateway-tyk-tyk-gateway-5c5f65f47c-2dzj6
- `ready_before`: 2

## Timeline

Seconds are relative to the fault.

```
  -15.0s  baseline: 15s of traffic with nothing broken
   +0.0s  fault injected
   +0.0s  gateway-tyk-tyk-gateway-5c5f65f47c-2dzj6 no longer ready
   +1.1s  gateway-tyk-tyk-gateway-5c5f65f47c-lk4vv created
   +2.3s  gateway-tyk-tyk-gateway-5c5f65f47c-2dzj6 gone
   +3.2s  gateway-tyk-tyk-gateway-5c5f65f47c-lk4vv ready
   +3.2s  back to 2 ready pods
  +60.0s  observation window over
```
