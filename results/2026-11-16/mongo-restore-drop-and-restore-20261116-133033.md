# Drill: mongo-restore (drop-and-restore)

Run on 16 November 2026 at 13:30 UTC in namespace `faildrill-lab`: 50 requests per second per target, 2s caller timeout, 15s of baseline and 1m0s of observation.

| Target | Requests | Failed | Impact | Longest gap | p99 before | p99 after | Slowest after |
|---|---:|---:|---:|---:|---:|---:|---:|
| open | 4173 | 0 | none | none | 15.3ms | 27.3ms | 78.0ms |
| keyed | 4183 | 0 | none | none | 17.7ms | 28.3ms | 77.5ms |

Impact is the time from the first failed request to the last. Longest gap is the longest stretch in which every request failed.

- **open**: 4173 × HTTP 200
- **keyed**: 4183 × HTTP 200

## What the system did

- `backup_bytes`: 3666394
- `backup_s`: 3.6
- `lost_window_s`: 26.4
- `records_at_disaster`: 94833
- `records_lost`: 2385
- `records_restored`: 92448
- `restore_s`: 31.2
- `verified`: yes

## Timeline

Seconds are relative to the fault.

```
  -15.0s  baseline: 15s of traffic with nothing broken
   +0.0s  fault injected
   +3.6s  backup taken: 3666394 bytes in 3.6s, kept at results/2026-11-16/mongo-restore-20261116-133029.archive.gz
  +28.3s  database tyk_analytics dropped with 94833 records in tyk_analytics_pump
  +57.5s  restore finished: 92448 records back, 31.2s after the drop
  +57.5s  2385 records written in the 26.4s between the backup and the disaster are gone
  +59.8s  verified: all 91843 records older than the backup are back
  +69.8s  observation window over
```
