# Local hub retention scale probe (2026-09-26)

## Reproduce

`BenchmarkHubPruneVacuum` in `internal/hubstore/retention_bench_test.go`
creates an isolated local Git repository and SQLite hub. It posts 4 KiB
one-shot messages, with 90% older than the explicit cutoff and 10% newer,
then plans, prunes, and explicitly vacuums. It verifies row counts, file
sizes, and database integrity. Run one size at a time:

```bash
go test ./internal/hubstore -run '^$' -bench '^BenchmarkHubPruneVacuum/1000$' -benchtime=1x -count=1 -v
```

## One-shot observations

Windows/amd64, Go 1.25.0, Intel i5-6600K, with other local Go work occurring.
The 100- and 500-message samples were run separately by the benchmark author;
the 1,000-message sample was rerun from the parent task. Values are one sample
each, not a confidence interval or regression threshold.

| Messages | Old publications | Old payload | Backup probe | Prune | Vacuum | File before -> after vacuum | Vacuum reclaimed |
| ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| 100 | 90 | 369,000 B | 161 ms | 180 ms | 207 ms | 876,544 -> 352,256 B | 532,480 B |
| 500 | 450 | 1,845,000 B | 126 ms | 422 ms | 113 ms | 3,297,280 -> 684,032 B | 2,625,536 B |
| 1,000 | 900 | 3,690,000 B | 229 ms | 909 ms | 311 ms | 6,299,648 -> 1,081,344 B | 5,230,592 B |

The 1,000-message setup took 82.9 seconds; this is an additional publish-path
signal, not part of the prune/vacuum times. The 1,000-message verified backup
probe occupied 6,205,440 bytes. The tool creates another verified backup for
prune and one for vacuum, retaining the most recent tool-owned snapshot when
cleanup succeeds. Large-hub and repeated-batch peak disk, latency, and
metadata growth are not established by these samples. A single group above
10,000 publications is currently rejected. No fat-repo activation quality
claim follows from this local hub benchmark.
