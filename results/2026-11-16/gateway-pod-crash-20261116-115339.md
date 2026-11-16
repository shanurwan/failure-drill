# Drill: gateway-pod (crash)

Run on 16 November 2026 at 11:53 UTC in namespace `faildrill-lab`: 50 requests per second per target, 2s caller timeout, 15s of baseline and 1m0s of observation.

| Target | Requests | Failed | Impact | Longest gap | p99 before | p99 after | Slowest after |
|---|---:|---:|---:|---:|---:|---:|---:|
| open | 3751 | 0 | none | none | 0.9ms | 2.0ms | 6.2ms |
| keyed | 3751 | 0 | none | none | 0.9ms | 2.1ms | 7.1ms |

Impact is the time from the first failed request to the last. Longest gap is the longest stretch in which every request failed.

- **open**: 3751 × HTTP 200
- **keyed**: 3751 × HTTP 200

## What the system did

- `deployment`: gateway-tyk-tyk-gateway
- `full_strength_s`: 4.0
- `pod_removed`: gateway-tyk-tyk-gateway-5c5f65f47c-7qxhz
- `ready_before`: 2

## Timeline

Seconds are relative to the fault.

```
  -15.0s  baseline: 15s of traffic with nothing broken
   +0.0s  fault injected
   +1.0s  gateway-tyk-tyk-gateway-5c5f65f47c-2dzj6 created
   +1.0s  gateway-tyk-tyk-gateway-5c5f65f47c-7qxhz gone
   +4.0s  gateway-tyk-tyk-gateway-5c5f65f47c-2dzj6 ready
   +4.0s  back to 2 ready pods
  +60.0s  observation window over
```
