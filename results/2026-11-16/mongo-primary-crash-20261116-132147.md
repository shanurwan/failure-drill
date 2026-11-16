# Drill: mongo-primary (crash)

Run on 16 November 2026 at 13:21 UTC in namespace `faildrill-lab`: 50 requests per second per target, 2s caller timeout, 15s of baseline and 1m0s of observation.

| Target | Requests | Failed | Impact | Longest gap | p99 before | p99 after | Slowest after |
|---|---:|---:|---:|---:|---:|---:|---:|
| open | 3877 | 0 | none | none | 9.6ms | 13.7ms | 45.2ms |
| keyed | 3875 | 0 | none | none | 9.3ms | 13.4ms | 45.1ms |

Impact is the time from the first failed request to the last. Longest gap is the longest stretch in which every request failed.

- **open**: 3877 × HTTP 200
- **keyed**: 3875 × HTTP 200

## What the system did

- `analytics_missing`: 0
- `analytics_recorded`: 7752
- `election_s`: 1.5
- `primary_after`: mongo-1
- `primary_before`: mongo-0
- `requests_answered`: 7752

## Timeline

Seconds are relative to the fault.

```
  -15.0s  baseline: 15s of traffic with nothing broken
   +0.0s  fault injected
   +1.5s  mongo-1 elected primary
   +4.4s  mongo-0 is starting up
  +23.1s  mongo-0 is back as a secondary
  +60.0s  observation window over
  +85.9s  the gateway answered 7752 requests; MongoDB has a record of 7752: none missing
```
