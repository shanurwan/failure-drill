# Drill: redis-primary (crash)

Run on 16 November 2026 at 12:09 UTC in namespace `faildrill-lab`: 50 requests per second per target, 2s caller timeout, 15s of baseline and 1m0s of observation.

| Target | Requests | Failed | Impact | Longest gap | p99 before | p99 after | Slowest after |
|---|---:|---:|---:|---:|---:|---:|---:|
| open | 3750 | 0 | none | none | 1.2ms | 2.2ms | 8.0ms |
| keyed | 3750 | 18 | 0.3s | 0.4s | 1.2ms | 2.4ms | 8.0ms |

Impact is the time from the first failed request to the last. Longest gap is the longest stretch in which every request failed.

- **open**: 3750 × HTTP 200
- **keyed**: 3732 × HTTP 200, 18 × HTTP 403

## What the system did

- `old_primary_returned_s`: 19.2
- `primary_after`: redis-1
- `primary_before`: redis-0
- `promotion_s`: 8.1

## Timeline

Seconds are relative to the fault.

```
  -15.0s  baseline: 15s of traffic with nothing broken
   +0.0s  fault injected
   +2.3s  redis-0 is answering again, still as primary
   +8.1s  redis-1 promoted to primary
  +19.2s  redis-0 is back, now as a replica
  +60.0s  observation window over
```
