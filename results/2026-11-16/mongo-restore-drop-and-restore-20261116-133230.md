# Drill: mongo-restore (drop-and-restore)

Run on 16 November 2026 at 13:32 UTC in namespace `faildrill-lab`: 50 requests per second per target, 2s caller timeout, 15s of baseline and 1m0s of observation.

| Target | Requests | Failed | Impact | Longest gap | p99 before | p99 after | Slowest after |
|---|---:|---:|---:|---:|---:|---:|---:|
| open | 3998 | 0 | none | none | 3.1ms | 19.7ms | 57.2ms |
| keyed | 3997 | 0 | none | none | 3.3ms | 20.8ms | 52.2ms |

Impact is the time from the first failed request to the last. Longest gap is the longest stretch in which every request failed.

- **open**: 3998 × HTTP 200
- **keyed**: 3997 × HTTP 200

## What the system did

- `backup_bytes`: 3892441
- `backup_s`: 1.2
- `lost_window_s`: 24.6
- `records_at_disaster`: 100477
- `records_lost`: 2372
- `records_restored`: 98105
- `restore_s`: 29.1
- `verified`: yes

## Timeline

Seconds are relative to the fault.

```
  -15.0s  baseline: 15s of traffic with nothing broken
   +0.0s  fault injected
   +1.2s  backup taken: 3892441 bytes in 1.2s, kept at results/2026-11-16/mongo-restore-20261116-133227.archive.gz
  +27.1s  database tyk_analytics dropped with 100477 records in tyk_analytics_pump
  +53.7s  restore finished: 98105 records back, 29.1s after the drop
  +53.7s  2372 records written in the 24.6s between the backup and the disaster are gone
  +55.3s  verified: all 97623 records older than the backup are back
  +65.3s  observation window over
```
