package hubstore

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
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
				b.Skip("use -benchtime=1x to bound writes and preserve one seeded cohort")
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
			coldTopics := make([]time.Duration, 0, 5)
			firstPull := make([]time.Duration, 0, 5)
			publish := make([]time.Duration, 0, 5)
			concurrentRead := make([]time.Duration, 0, 5)
			concurrentWrite := make([]time.Duration, 0, 5)
			for i := 0; i < 5; i++ {
				started := time.Now()
				reader, err := OpenReadOnly(ctx, Options{Home: home})
				require.NoError(b, err)
				topics, err := reader.ListTopics(ctx, repo, TopicList{Limit: 20})
				require.NoError(b, err)
				require.NoError(b, reader.Close())
				coldTopics = append(coldTopics, time.Since(started))
				require.Len(b, topics, 1)
				require.Equal(b, topic.ID, topics[0].ID)

				started = time.Now()
				reader, err = OpenReadOnly(ctx, Options{Home: home})
				require.NoError(b, err)
				page, err := reader.Pull(ctx, repo, "reader", sub.ID, 100)
				require.NoError(b, err)
				require.NoError(b, reader.Close())
				firstPull = append(firstPull, time.Since(started))
				require.Len(b, page.Entries, 100)
				require.NotNil(b, page.Entries[0].Message)
				require.Equal(b, int64(1), page.Entries[0].Message.Sequence)
				require.NotNil(b, page.Entries[99].Message)
				require.Equal(b, int64(100), page.Entries[99].Message.Sequence)
				require.Equal(b, int64(100), page.NextScanPosition)
				require.Equal(b, int64(total+i), page.HighWater)
				require.Empty(b, page.Gaps)

				started = time.Now()
				published, err := d.PostMessage(ctx, repo, topic.ID, "author", MessageInput{
					AuthorityID: d.AuthorityID(), Text: "new publication", IdempotencyKey: fmt.Sprintf("measured-publish-%d", i),
				})
				require.NoError(b, err)
				publish = append(publish, time.Since(started))
				require.Equal(b, int64(total+i+1), published.Sequence)
			}

			for i := 0; i < 5; i++ {
				pairCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
				reader, err := OpenReadOnly(pairCtx, Options{Home: home})
				require.NoError(b, err)
				writer, err := Open(pairCtx, Options{Home: home})
				require.NoError(b, err)
				start := make(chan struct{})
				ready := make(chan struct{}, 2)
				type readResult struct {
					duration time.Duration
					topics   []Topic
					page     PullPage
					err      error
				}
				type writeResult struct {
					duration time.Duration
					message  Message
					err      error
				}
				reads := make(chan readResult, 1)
				writes := make(chan writeResult, 1)
				go func() {
					ready <- struct{}{}
					<-start
					started := time.Now()
					topics, err := reader.ListTopics(pairCtx, repo, TopicList{Limit: 20})
					var page PullPage
					if err == nil {
						page, err = reader.Pull(pairCtx, repo, "reader", sub.ID, 100)
					}
					reads <- readResult{duration: time.Since(started), topics: topics, page: page, err: err}
				}()
				go func(sample int) {
					ready <- struct{}{}
					<-start
					started := time.Now()
					message, err := writer.PostMessage(pairCtx, repo, topic.ID, "author", MessageInput{
						AuthorityID: d.AuthorityID(), Text: "concurrent publication", IdempotencyKey: fmt.Sprintf("concurrent-publish-%d", sample),
					})
					writes <- writeResult{duration: time.Since(started), message: message, err: err}
				}(i)
				<-ready
				<-ready
				close(start)
				read := <-reads
				write := <-writes
				readerClose := reader.Close()
				writerClose := writer.Close()
				cancel()
				require.NoError(b, read.err)
				require.NoError(b, write.err)
				require.NoError(b, readerClose)
				require.NoError(b, writerClose)
				require.Len(b, read.topics, 1)
				require.Equal(b, topic.ID, read.topics[0].ID)
				require.Len(b, read.page.Entries, 100)
				require.NotNil(b, read.page.Entries[0].Message)
				require.Equal(b, int64(1), read.page.Entries[0].Message.Sequence)
				require.NotNil(b, read.page.Entries[99].Message)
				require.Equal(b, int64(100), read.page.Entries[99].Message.Sequence)
				require.Equal(b, int64(100), read.page.NextScanPosition)
				require.GreaterOrEqual(b, read.page.HighWater, int64(total+i+5))
				require.LessOrEqual(b, read.page.HighWater, int64(total+i+6))
				require.Empty(b, read.page.Gaps)
				require.Equal(b, int64(total+i+6), write.message.Sequence)
				concurrentRead = append(concurrentRead, read.duration)
				concurrentWrite = append(concurrentWrite, write.duration)
			}
			b.StopTimer()

			afterDB := benchmarkFileSize(b, d.Path())
			afterWAL := benchmarkFileSize(b, d.Path()+"-wal")
			reportActivationSamples(b, "cold_topics", coldTopics)
			reportActivationSamples(b, "first_pull", firstPull)
			reportActivationSamples(b, "publish", publish)
			reportActivationSamples(b, "concurrent_read", concurrentRead)
			reportActivationSamples(b, "concurrent_write", concurrentWrite)
			b.ReportMetric(float64(beforeDB+beforeWAL), "seed_db_wal_B")
			b.ReportMetric(float64(afterDB+afterWAL), "final_db_wal_B")
			b.Logf("cohort=%d seed=%s seed_db_B=%d seed_wal_B=%d final_db_B=%d final_wal_B=%d",
				total, seedWall, beforeDB, beforeWAL, afterDB, afterWAL)
		})
	}
}

func reportActivationSamples(b *testing.B, name string, samples []time.Duration) {
	b.Helper()
	ordered := append([]time.Duration(nil), samples...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i] < ordered[j] })
	for i, sample := range samples {
		b.Logf("%s[%d]=%s", name, i, sample)
	}
	b.ReportMetric(float64(ordered[len(ordered)/2].Microseconds()), name+"_median_us")
	b.ReportMetric(float64(ordered[len(ordered)-1].Microseconds()), name+"_max_us")
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
