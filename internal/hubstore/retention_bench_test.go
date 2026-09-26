package hubstore

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Run with -benchtime=1x to bound disk use and keep each reported sample distinct.
// Example: go test ./internal/hubstore -run '^$' -bench '^BenchmarkHubPruneVacuum/100$' -benchtime=1x -count=1 -v
func BenchmarkHubPruneVacuum(b *testing.B) {
	for _, total := range []int{100, 500, 1000} {
		b.Run(fmt.Sprintf("%d", total), func(b *testing.B) {
			for iteration := 0; iteration < b.N; iteration++ {
				b.StopTimer()
				ctx := context.Background()
				root := b.TempDir()
				repo := filepath.Join(root, "repo")
				require.NoError(b, os.Mkdir(repo, 0o700))
				cmd := exec.Command("git", "init", "-q", repo)
				out, err := cmd.CombinedOutput()
				require.NoError(b, err, string(out))

				now := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
				d, err := Open(ctx, Options{Home: filepath.Join(root, "home"), Now: func() time.Time { return now }})
				require.NoError(b, err)
				_, err = d.EnrollRepo(ctx, repo)
				require.NoError(b, err)
				_, err = d.EnrollActor(ctx, "author")
				require.NoError(b, err)
				_, err = d.EnrollActor(ctx, "owner")
				require.NoError(b, err)
				topic, err := d.CreateTopic(ctx, repo, "owner", TopicInput{Key: "retention-bench", Name: "Retention benchmark", Description: "Synthetic one-shot messages"})
				require.NoError(b, err)

				oldCount := total * 9 / 10
				payload := strings.Repeat("x", 4*1024)
				setupStart := time.Now()
				for i := 0; i < total; i++ {
					if i == oldCount {
						now = now.Add(2 * time.Hour)
					}
					_, err := d.PostMessage(ctx, repo, topic.ID, "author", MessageInput{
						AuthorityID: d.AuthorityID(), Text: payload, IdempotencyKey: fmt.Sprintf("bench-%d", i),
					})
					require.NoError(b, err)
				}
				setupWall := time.Since(setupStart)
				cutoff := now.Add(-time.Minute)
				plan, err := d.PlanHubPrune(ctx, cutoff)
				require.NoError(b, err)
				require.Equal(b, oldCount, plan.Entries)
				require.False(b, plan.More)
				require.NoError(b, d.checkpointForVacuum(ctx))
				before, err := os.Stat(d.Path())
				require.NoError(b, err)

				backupStart := time.Now()
				probeBackup, err := d.backupForPrune(ctx)
				backupWall := time.Since(backupStart)
				require.NoError(b, err)
				backupInfo, err := os.Stat(probeBackup)
				require.NoError(b, err)
				require.NoError(b, os.Remove(probeBackup))

				b.StartTimer()
				wallStart := time.Now()
				pruneStart := time.Now()
				pruned, err := d.PruneHub(ctx, cutoff)
				pruneWall := time.Since(pruneStart)
				require.NoError(b, err)
				require.NoError(b, d.checkpointForVacuum(ctx))
				afterPrune, err := os.Stat(d.Path())
				require.NoError(b, err)
				vacuumStart := time.Now()
				vacuumed, err := d.VacuumHub(ctx)
				vacuumWall := time.Since(vacuumStart)
				require.NoError(b, err)
				wall := time.Since(wallStart)
				b.StopTimer()

				require.Equal(b, plan.Entries, pruned.Entries)
				require.Equal(b, plan.PayloadBytes, pruned.PayloadBytes)
				require.Equal(b, afterPrune.Size(), vacuumed.BytesBefore)
				require.NoError(b, d.CheckIntegrity(ctx))
				b.ReportMetric(float64(plan.Entries), "old_rows")
				b.ReportMetric(float64(plan.PayloadBytes), "old_payload_B")
				b.ReportMetric(float64(before.Size()), "before_B")
				b.ReportMetric(float64(vacuumed.BytesAfter), "after_B")
				b.ReportMetric(float64(vacuumed.ReclaimedBytes), "reclaimed_B")
				b.Logf("total=%d old_rows=%d old_payload_B=%d setup=%s backup_probe=%s backup_B=%d prune=%s vacuum=%s maintenance_wall=%s file_before_B=%d file_after_prune_B=%d file_after_vacuum_B=%d reclaimed_B=%d",
					total, plan.Entries, plan.PayloadBytes, setupWall, backupWall, backupInfo.Size(), pruneWall, vacuumWall, wall,
					before.Size(), afterPrune.Size(), vacuumed.BytesAfter, vacuumed.ReclaimedBytes)
				require.NoError(b, d.Close())
			}
		})
	}
}
