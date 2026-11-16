# Drill: mongo-primary (crash)

Run on 16 November 2026 at 13:15 UTC in namespace `faildrill-lab`: 50 requests per second per target, 2s caller timeout, 15s of baseline and 1m0s of observation.

| Target | Requests | Failed | Impact | Longest gap | p99 before | p99 after | Slowest after |
|---|---:|---:|---:|---:|---:|---:|---:|
| open | 3846 | 0 | none | none | 8.3ms | 7.9ms | 18.6ms |
| keyed | 3846 | 0 | none | none | 8.4ms | 7.5ms | 22.2ms |

Impact is the time from the first failed request to the last. Longest gap is the longest stretch in which every request failed.

- **open**: 3846 × HTTP 200
- **keyed**: 3846 × HTTP 200

## What the system did

- `analytics_missing`: 0
- `analytics_recorded`: 7692
- `election_s`: 1.3
- `primary_after`: mongo-1
- `primary_before`: mongo-0
- `requests_answered`: 7692

## Timeline

Seconds are relative to the fault.

```
  -15.0s  baseline: 15s of traffic with nothing broken
   +0.0s  fault injected
   +1.3s  mongo-1 elected primary
   +6.0s  mongo-0 is starting up
   +7.2s  mongo-0 stopped answering
   +8.4s  mongo-0 is starting up
  +23.1s  mongo-0 is back as a secondary
  +60.0s  observation window over
  +74.8s  the gateway answered 7692 requests; MongoDB has a record of 7692: none missing
```
