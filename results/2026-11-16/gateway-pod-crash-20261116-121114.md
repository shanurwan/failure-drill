# Drill: gateway-pod (crash)

Run on 16 November 2026 at 12:11 UTC in namespace `faildrill-lab`: 50 requests per second per target, 2s caller timeout, 15s of baseline and 1m0s of observation.

| Target | Requests | Failed | Impact | Longest gap | p99 before | p99 after | Slowest after |
|---|---:|---:|---:|---:|---:|---:|---:|
| open | 3766 | 0 | none | none | 1.1ms | 1.7ms | 4.6ms |
| keyed | 3766 | 1 | 0.0s | 0.0s | 1.2ms | 1.8ms | 4.7ms |

Impact is the time from the first failed request to the last. Longest gap is the longest stretch in which every request failed.

- **open**: 3766 × HTTP 200
- **keyed**: 3765 × HTTP 200, 1 × refused

## What the system did

- `deployment`: gateway-tyk-tyk-gateway
- `full_strength_s`: 2.7
- `pod_removed`: gateway-tyk-tyk-gateway-65d6784795-pvfnx
- `ready_before`: 2

## Timeline

Seconds are relative to the fault.

```
  -15.0s  baseline: 15s of traffic with nothing broken
   +0.0s  fault injected
   +0.7s  gateway-tyk-tyk-gateway-65d6784795-pvfnx gone
   +0.7s  gateway-tyk-tyk-gateway-65d6784795-qxmt8 created
   +2.7s  gateway-tyk-tyk-gateway-65d6784795-qxmt8 ready
   +2.7s  back to 2 ready pods
  +60.0s  observation window over
```
