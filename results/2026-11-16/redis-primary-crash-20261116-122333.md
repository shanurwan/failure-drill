# Drill: redis-primary (crash)

Run on 16 November 2026 at 12:23 UTC in namespace `faildrill-lab`: 50 requests per second per target, 2s caller timeout, 15s of baseline and 1m0s of observation.

| Target | Requests | Failed | Impact | Longest gap | p99 before | p99 after | Slowest after |
|---|---:|---:|---:|---:|---:|---:|---:|
| open | 3765 | 0 | none | none | 0.9ms | 2.0ms | 7.0ms |
| keyed | 3765 | 0 | none | none | 1.1ms | 2.1ms | 6.9ms |

Impact is the time from the first failed request to the last. Longest gap is the longest stretch in which every request failed.

- **open**: 3765 × HTTP 200
- **keyed**: 3765 × HTTP 200

## What the system did

- `old_primary_returned_s`: 19.0
- `primary_after`: redis-0
- `primary_before`: redis-1
- `promotion_s`: 8.0

## Timeline

Seconds are relative to the fault.

```
  -15.0s  baseline: 15s of traffic with nothing broken
   +0.0s  fault injected
   +1.0s  redis-1 stopped answering
   +1.7s  redis-1 is answering again, still as primary
   +8.0s  redis-0 promoted to primary
  +19.0s  redis-1 is back, now as a replica
  +60.0s  observation window over
```
