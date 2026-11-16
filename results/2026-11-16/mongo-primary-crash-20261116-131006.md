# Drill: mongo-primary (crash)

Run on 16 November 2026 at 13:10 UTC in namespace `faildrill-lab`: 50 requests per second per target, 2s caller timeout, 15s of baseline and 1m0s of observation.

| Target | Requests | Failed | Impact | Longest gap | p99 before | p99 after | Slowest after |
|---|---:|---:|---:|---:|---:|---:|---:|
| open | 3829 | 0 | none | none | 8.7ms | 10.0ms | 27.0ms |
| keyed | 3830 | 0 | none | none | 7.6ms | 9.9ms | 29.8ms |

Impact is the time from the first failed request to the last. Longest gap is the longest stretch in which every request failed.

- **open**: 3829 × HTTP 200
- **keyed**: 3830 × HTTP 200

## What the system did

- `analytics_missing`: 0
- `analytics_recorded`: 7659
- `election_s`: 1.3
- `primary_after`: mongo-1
- `primary_before`: mongo-0
- `requests_answered`: 7659

## Timeline

Seconds are relative to the fault.

```
  -15.0s  baseline: 15s of traffic with nothing broken
   +0.0s  fault injected
   +1.3s  mongo-1 elected primary
   +4.7s  mongo-0 is starting up
   +5.8s  mongo-0 stopped answering
   +7.3s  mongo-0 is starting up
  +22.3s  mongo-0 is back as a secondary
  +34.6s  mongo-1 stopped answering
  +34.6s  mongo-2 stopped answering
  +36.1s  mongo-1 elected primary
  +36.1s  mongo-2 is back as a secondary
  +60.0s  observation window over
  +73.8s  the gateway answered 7659 requests; MongoDB has a record of 7659: none missing
```
