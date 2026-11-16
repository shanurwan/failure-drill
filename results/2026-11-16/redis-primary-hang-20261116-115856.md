# Drill: redis-primary (hang)

Run on 16 November 2026 at 11:58 UTC in namespace `faildrill-lab`: 50 requests per second per target, 2s caller timeout, 15s of baseline and 1m0s of observation.

| Target | Requests | Failed | Impact | Longest gap | p99 before | p99 after | Slowest after |
|---|---:|---:|---:|---:|---:|---:|---:|
| open | 3762 | 0 | none | none | 1.3ms | 3.2ms | 12.4ms |
| keyed | 3762 | 19 | 0.4s | 0.4s | 1.4ms | 1795ms | 2001ms |

Impact is the time from the first failed request to the last. Longest gap is the longest stretch in which every request failed.

- **open**: 3762 × HTTP 200
- **keyed**: 3743 × HTTP 200, 19 × timeout

## What the system did

- `old_primary_returned_s`: 41.1
- `primary_after`: redis-0
- `primary_before`: redis-1
- `promotion_s`: 8.1

## Timeline

Seconds are relative to the fault.

```
  -15.0s  baseline: 15s of traffic with nothing broken
   +0.0s  fault injected
   +2.1s  redis-1 stopped answering
   +8.1s  redis-0 promoted to primary
  +30.1s  redis-1 is answering again, still as primary
  +41.1s  redis-1 is back, now as a replica
  +60.0s  observation window over
```
