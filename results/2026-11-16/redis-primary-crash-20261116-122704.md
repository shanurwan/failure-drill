# Drill: redis-primary (crash)

Run on 16 November 2026 at 12:27 UTC in namespace `faildrill-lab`: 50 requests per second per target, 2s caller timeout, 15s of baseline and 1m0s of observation.

| Target | Requests | Failed | Impact | Longest gap | p99 before | p99 after | Slowest after |
|---|---:|---:|---:|---:|---:|---:|---:|
| open | 3750 | 0 | none | none | 3.2ms | 1.5ms | 4.9ms |
| keyed | 3750 | 16 | 0.3s | 0.3s | 3.6ms | 1.5ms | 8.3ms |

Impact is the time from the first failed request to the last. Longest gap is the longest stretch in which every request failed.

- **open**: 3750 × HTTP 200
- **keyed**: 3734 × HTTP 200, 16 × HTTP 403

## What the system did

- `old_primary_returned_s`: 19.2
- `primary_after`: redis-2
- `primary_before`: redis-1
- `promotion_s`: 8.2

## Timeline

Seconds are relative to the fault.

```
  -15.0s  baseline: 15s of traffic with nothing broken
   +0.0s  fault injected
   +1.2s  redis-1 is answering again, still as primary
   +8.2s  redis-2 promoted to primary
  +19.2s  redis-1 is back, now as a replica
  +60.0s  observation window over
```
