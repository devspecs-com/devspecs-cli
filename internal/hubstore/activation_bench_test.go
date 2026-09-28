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

// Run explicitly with DEVSPECS_HUB_ACTIVATION_BENCH=1 and -benchtime=1x.
// Each cohort has its own repository and authority; normal test runs skip it.
func BenchmarkHubColdActivation(b *testing.B) {
	if os.Getenv("DEVSPECS_HUB_ACTIVATION_BENCH") != "1" {
		b.Skip("opt-in disk and latency probe")
	}
	for _, total := range []int{100, 1000, 10000} {
		b.Run(fmt.Sprintf("%d", total), func(b *testing.B) {
			if b.N != 1 {
				b.Skip("use -benchtime=1x to bound writes and preserve one raw sample")
			}
			b.StopTimer()
			ctx := context.Background()
			root := b.TempDir()
			free, err := availableDiskBytes(root)
			require.NoError(b, err)
			if free < 2<<30 {
				b.Skipf("disk preflight: %d free bytes, need at least 2 GiB", free)
			}
			repo := filepath.Join(root, "repo")
			require.NoError(b, os.Mkdir(repo, 0o700))
			out, err := exec.Command("git", "init", "-q", repo).CombinedOutput()
			require.NoError(b, err, string(out))
			home := filepath.Join(root, "home")
			d, err := Open(ctx, Options{Home: home})
			require.NoError(b, err)
			defer d.Close()
			_, err = d.EnrollRepo(ctx, repo)
			require.NoError(b, err)
			_, err = d.EnrollActor(ctx, "owner")
			require.NoError(b, err)
			_, err = d.EnrollActor(ctx, "author")
			require.NoError(b, err)
			topic, err := d.CreateTopic(ctx, repo, "owner", TopicInput{Key: "activation", Name: "Activation", Description: "Cold activation measurement"})
			require.NoError(b, err)
			_, err = d.EnrollConsumer(ctx, d.AuthorityID(), "reader")
			require.NoError(b, err)
			sub, err := d.Subscribe(ctx, repo, SubscribeInput{AuthorityID: d.AuthorityID(), ConsumerID: "reader", TopicIDs: []string{topic.ID}, FromBeginning: true})
			require.NoError(b, err)

			seedStart := time.Now()
			for i := 0; i < total; i++ {
				_, err := d.PostMessage(ctx, repo, topic.ID, "author", MessageInput{
					AuthorityID: d.AuthorityID(), Text: strings.Repeat("x", 256), IdempotencyKey: fmt.Sprintf("seed-%d", i),
				})
				require.NoError(b, err)
			}
			seedWall := time.Since(seedStart)
			beforeDB := benchmarkFileSize(b, d.Path())
			beforeWAL := benchmarkFileSize(b, d.Path()+"-wal")

			b.StartTimer()
			coldStart := time.Now()
			reader, err := OpenReadOnly(ctx, Options{Home: home})
			require.NoError(b, err)
			topics, err := reader.ListTopics(ctx, repo, TopicList{Limit: 20})
			require.NoError(b, err)
			require.Len(b, topics, 1)
			require.NoError(b, reader.Close())
			coldTopics := time.Since(coldStart)

			pullStart := time.Now()
			reader, err = OpenReadOnly(ctx, Options{Home: home})
			require.NoError(b, err)
			page, err := reader.Pull(ctx, repo, "reader", sub.ID, 100)
			require.NoError(b, err)
			require.Len(b, page.Entries, 100)
			require.NoError(b, reader.Close())
			firstPull := time.Since(pullStart)

			publishStart := time.Now()
			published, err := d.PostMessage(ctx, repo, topic.ID, "author", MessageInput{
				AuthorityID: d.AuthorityID(), Text: "new publication", IdempotencyKey: "measured-publish",
			})
			require.NoError(b, err)
			require.Equal(b, int64(total+1), published.Sequence)
			publish := time.Since(publishStart)
			b.StopTimer()

			afterDB := benchmarkFileSize(b, d.Path())
			afterWAL := benchmarkFileSize(b, d.Path()+"-wal")
			b.ReportMetric(float64(coldTopics.Microseconds()), "cold_topics_us")
			b.ReportMetric(float64(firstPull.Microseconds()), "first_pull_us")
			b.ReportMetric(float64(publish.Microseconds()), "publish_us")
			b.ReportMetric(float64(beforeDB+beforeWAL), "seed_db_wal_B")
			b.ReportMetric(float64(afterDB+afterWAL), "final_db_wal_B")
			b.Logf("cohort=%d seed=%s cold_topics=%s first_pull=%s publish=%s seed_db_B=%d seed_wal_B=%d final_db_B=%d final_wal_B=%d",
				total, seedWall, coldTopics, firstPull, publish, beforeDB, beforeWAL, afterDB, afterWAL)
		})
	}
}

func benchmarkFileSize(b *testing.B, path string) int64 {
	b.Helper()
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return 0
	}
	require.NoError(b, err)
	return info.Size()
}
