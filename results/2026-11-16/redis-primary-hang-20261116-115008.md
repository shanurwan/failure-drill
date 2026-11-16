# Drill: redis-primary (hang)

Run on 16 November 2026 at 11:50 UTC in namespace `faildrill-lab`: 50 requests per second per target, 2s caller timeout, 15s of baseline and 1m0s of observation.

| Target | Requests | Failed | Impact | Longest gap | p99 before | p99 after | Slowest after |
|---|---:|---:|---:|---:|---:|---:|---:|
| open | 3737 | 0 | none | none | 3.5ms | 12.7ms | 73.8ms |
| keyed | 3738 | 32 | 1.1s | 0.1s | 3.7ms | 2000ms | 2001ms |

Impact is the time from the first failed request to the last. Longest gap is the longest stretch in which every request failed.

- **open**: 3737 × HTTP 200
- **keyed**: 3706 × HTTP 200, 32 × timeout

## What the system did

- `old_primary_returned_s`: 40.3
- `primary_after`: redis-0
- `primary_before`: redis-1
- `promotion_s`: 7.4

## Timeline

Seconds are relative to the fault.

```
  -15.0s  baseline: 15s of traffic with nothing broken
   +0.0s  fault injected
   +1.2s  redis-1 stopped answering
   +7.4s  redis-0 promoted to primary
  +30.1s  redis-1 is answering again, still as primary
  +40.3s  redis-1 is back, now as a replica
  +60.0s  observation window over
```
