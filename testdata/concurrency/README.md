# Concurrent index regression gate

The ordinary Go PR job runs each Linux process-level concurrency test once with
the race detector. The tests share one DevSpecs database across real `map`,
`find`, `recent`, `task`, and `scan` processes, verify queued writes and usable
committed reads, and inspect SQLite integrity, foreign keys, FTS rows, and scan
freshness after completion.

The `Concurrent Index Stress` workflow repeats the same bounded scenarios ten
times every day and on demand. It is intentionally separate from PR checks so
runner variance does not turn a deterministic release gate into a long default
job.

Run the heavier variant locally on Linux with:

```sh
go test -race ./cmd/ds \
  -run '^(TestMain_WhenConcurrentCommandsShareIndex_SerializesWritesAndPreservesReads|TestMain_WhenCPUActiveMapRefreshIsTerminated_ReapsGitAndPreservesIndex)$' \
  -count=10 \
  -timeout=25m
```
