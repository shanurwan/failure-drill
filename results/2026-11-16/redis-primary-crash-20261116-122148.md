# Drill: redis-primary (crash)

Run on 16 November 2026 at 12:21 UTC in namespace `faildrill-lab`: 50 requests per second per target, 2s caller timeout, 15s of baseline and 1m0s of observation.

| Target | Requests | Failed | Impact | Longest gap | p99 before | p99 after | Slowest after |
|---|---:|---:|---:|---:|---:|---:|---:|
| open | 3750 | 0 | none | none | 2.5ms | 1.8ms | 6.3ms |
| keyed | 3750 | 0 | none | none | 2.8ms | 1.8ms | 6.0ms |

Impact is the time from the first failed request to the last. Longest gap is the longest stretch in which every request failed.

- **open**: 3750 × HTTP 200
- **keyed**: 3750 × HTTP 200

## What the system did

- `old_primary_returned_s`: 19.1
- `primary_after`: redis-1
- `primary_before`: redis-0
- `promotion_s`: 8.1

## Timeline

Seconds are relative to the fault.

```
  -15.0s  baseline: 15s of traffic with nothing broken
   +0.0s  fault injected
   +1.1s  redis-0 is answering again, still as primary
   +8.1s  redis-1 promoted to primary
  +19.1s  redis-0 is back, now as a replica
  +60.0s  observation window over
```
