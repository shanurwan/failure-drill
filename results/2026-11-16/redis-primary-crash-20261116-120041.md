# Drill: redis-primary (crash)

Run on 16 November 2026 at 12:00 UTC in namespace `faildrill-lab`: 50 requests per second per target, 2s caller timeout, 15s of baseline and 1m0s of observation.

| Target | Requests | Failed | Impact | Longest gap | p99 before | p99 after | Slowest after |
|---|---:|---:|---:|---:|---:|---:|---:|
| open | 3767 | 0 | none | none | 2.3ms | 2.4ms | 7.0ms |
| keyed | 3767 | 6 | 0.1s | 0.1s | 2.5ms | 2.8ms | 2001ms |

Impact is the time from the first failed request to the last. Longest gap is the longest stretch in which every request failed.

- **open**: 3767 × HTTP 200
- **keyed**: 3761 × HTTP 200, 6 × timeout

## What the system did

- `old_primary_returned_s`: 18.0
- `primary_after`: redis-1
- `primary_before`: redis-0
- `promotion_s`: 6.8

## Timeline

Seconds are relative to the fault.

```
  -15.0s  baseline: 15s of traffic with nothing broken
   +0.0s  fault injected
   +0.8s  redis-0 stopped answering
   +1.7s  redis-0 is answering again, still as primary
   +6.8s  redis-1 promoted to primary
  +18.0s  redis-0 is back, now as a replica
  +60.0s  observation window over
```
