# Drill: mongo-restore (drop-and-restore)

Run on 16 November 2026 at 13:28 UTC in namespace `faildrill-lab`: 50 requests per second per target, 2s caller timeout, 15s of baseline and 1m0s of observation.

| Target | Requests | Failed | Impact | Longest gap | p99 before | p99 after | Slowest after |
|---|---:|---:|---:|---:|---:|---:|---:|
| open | 4182 | 0 | none | none | 3.6ms | 23.6ms | 96.8ms |
| keyed | 4177 | 0 | none | none | 2.9ms | 25.6ms | 82.0ms |

Impact is the time from the first failed request to the last. Longest gap is the longest stretch in which every request failed.

- **open**: 4182 × HTTP 200
- **keyed**: 4177 × HTTP 200

## What the system did

- `backup_bytes`: 3426124
- `backup_s`: 1.3
- `lost_window_s`: 25.8
- `records_at_disaster`: 89224
- `records_lost`: 2596
- `records_restored`: 86628
- `restore_s`: 31.6
- `verified`: yes

## Timeline

Seconds are relative to the fault.

```
  -15.0s  baseline: 15s of traffic with nothing broken
   +0.0s  fault injected
   +1.3s  backup taken: 3426124 bytes in 1.3s, kept at results/2026-11-16/mongo-restore-20261116-132833.archive.gz
  +27.8s  database tyk_analytics dropped with 89224 records in tyk_analytics_pump
  +57.4s  restore finished: 86628 records back, 31.6s after the drop
  +57.4s  2596 records written in the 25.8s between the backup and the disaster are gone
  +59.3s  verified: all 86282 records older than the backup are back
  +69.3s  observation window over
```
