# Drill: mongo-primary (stepdown)

Run on 16 November 2026 at 13:24 UTC in namespace `faildrill-lab`: 50 requests per second per target, 2s caller timeout, 15s of baseline and 1m0s of observation.

| Target | Requests | Failed | Impact | Longest gap | p99 before | p99 after | Slowest after |
|---|---:|---:|---:|---:|---:|---:|---:|
| open | 3912 | 0 | none | none | 30.4ms | 16.7ms | 46.2ms |
| keyed | 3914 | 0 | none | none | 29.1ms | 17.5ms | 46.6ms |

Impact is the time from the first failed request to the last. Longest gap is the longest stretch in which every request failed.

- **open**: 3912 × HTTP 200
- **keyed**: 3914 × HTTP 200

## What the system did

- `analytics_missing`: 0
- `analytics_recorded`: 7826
- `election_s`: 4.2
- `primary_after`: mongo-0
- `primary_before`: mongo-1
- `requests_answered`: 7826

## Timeline

Seconds are relative to the fault.

```
  -15.0s  baseline: 15s of traffic with nothing broken
  -12.9s  mongo-1 stopped answering
   -6.3s  mongo-1 elected primary
   -4.6s  mongo-0 is back as a secondary
   -4.6s  mongo-2 is back as a secondary
   +1.6s  mongo-0 stopped answering
   +1.6s  mongo-2 stopped answering
   +2.1s  fault injected
   +3.8s  mongo-1 stopped answering
   +4.2s  mongo-0 elected primary
   +5.9s  mongo-1 is back as a secondary
   +5.9s  mongo-2 is back as a secondary
   +9.6s  mongo-1 stopped answering
   +9.6s  mongo-2 stopped answering
  +11.9s  mongo-2 is back as a secondary
  +13.7s  mongo-1 is back as a secondary
  +59.7s  mongo-0 stopped answering
  +59.7s  mongo-1 stopped answering
  +59.7s  mongo-2 stopped answering
  +60.0s  observation window over
  +92.5s  the gateway answered 7826 requests; MongoDB has a record of 7826: none missing
```
