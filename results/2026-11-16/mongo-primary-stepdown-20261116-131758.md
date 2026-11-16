# Drill: mongo-primary (stepdown)

Run on 16 November 2026 at 13:17 UTC in namespace `faildrill-lab`: 50 requests per second per target, 2s caller timeout, 15s of baseline and 1m0s of observation.

| Target | Requests | Failed | Impact | Longest gap | p99 before | p99 after | Slowest after |
|---|---:|---:|---:|---:|---:|---:|---:|
| open | 3839 | 0 | none | none | 9.6ms | 8.2ms | 26.3ms |
| keyed | 3839 | 0 | none | none | 9.0ms | 8.6ms | 25.9ms |

Impact is the time from the first failed request to the last. Longest gap is the longest stretch in which every request failed.

- **open**: 3839 × HTTP 200
- **keyed**: 3839 × HTTP 200

## What the system did

- `analytics_missing`: 0
- `analytics_recorded`: 7678
- `election_s`: 2.2
- `primary_after`: mongo-0
- `primary_before`: mongo-1
- `requests_answered`: 7678

## Timeline

Seconds are relative to the fault.

```
  -15.0s  baseline: 15s of traffic with nothing broken
   +0.9s  fault injected
   +2.2s  mongo-0 elected primary
   +2.2s  mongo-1 stepped down, now a secondary
  +60.0s  observation window over
  +74.1s  the gateway answered 7678 requests; MongoDB has a record of 7678: none missing
```
