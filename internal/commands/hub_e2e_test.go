package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/devspecs-com/devspecs-cli/internal/hubstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Each invocation of this test in a child process runs exactly one hub command.
func TestHubE2EProcess(t *testing.T) {
	if os.Getenv("DEVSPECS_HUB_E2E_CHILD") != "1" {
		return
	}
	for i, arg := range os.Args {
		if arg == "--" {
			cmd := NewHubCmd()
			cmd.SetArgs(os.Args[i+1:])
			if err := cmd.Execute(); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(2)
			}
			os.Exit(0)
		}
	}
	fmt.Fprintln(os.Stderr, "missing hub command separator")
	os.Exit(2)
}

type hubE2EFixture struct {
	home      string
	publisher string
	consumer  string
}

func newHubE2EFixture(t *testing.T) hubE2EFixture {
	t.Helper()
	root := t.TempDir()
	repo := filepath.Join(root, "publisher")
	worktree := filepath.Join(root, "consumer")
	require.NoError(t, os.Mkdir(repo, 0o700))
	runHubE2EGit(t, "init", "-q", repo)
	runHubE2EGit(t, "-C", repo, "-c", "user.name=Hub QA", "-c", "user.email=hub@example.invalid", "commit", "--allow-empty", "-qm", "seed")
	runHubE2EGit(t, "-C", repo, "worktree", "add", "-q", "-b", "hub-consumer", worktree)
	return hubE2EFixture{home: filepath.Join(root, "home"), publisher: repo, consumer: worktree}
}

func runHubE2EGit(t *testing.T, args ...string) {
	t.Helper()
	output, err := exec.Command("git", args...).CombinedOutput()
	require.NoError(t, err, "git %v: %s", args, output)
}

func runHubE2EJSON[T any](t *testing.T, fixture hubE2EFixture, worktree string, args ...string) T {
	t.Helper()
	binary, err := os.Executable()
	require.NoError(t, err)
	childArgs := append([]string{"-test.run=^TestHubE2EProcess$", "--", "--repo", ".", "--json"}, args...)
	cmd := exec.Command(binary, childArgs...)
	cmd.Dir = worktree
	cmd.Env = hubE2EEnv(fixture.home)
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "hub %v in %s: %s", args, worktree, output)
	var response hubJSONResponse[T]
	require.NoError(t, json.Unmarshal(output, &response), "hub %v: %s", args, output)
	require.Equal(t, "devspecs.hub/v1", response.ContractVersion)
	return response.Result
}

func hubE2EEnv(home string) []string {
	env := make([]string, 0, len(os.Environ())+2)
	for _, item := range os.Environ() {
		key, _, _ := strings.Cut(item, "=")
		if !strings.EqualFold(key, "DEVSPECS_HOME") && !strings.EqualFold(key, "DEVSPECS_HUB_E2E_CHILD") {
			env = append(env, item)
		}
	}
	return append(env, "DEVSPECS_HOME="+home, "DEVSPECS_HUB_E2E_CHILD=1")
}

func TestHubE2ECrossWorktreeDeliveryAndDurableAck(t *testing.T) {
	// Arrange
	fixture := newHubE2EFixture(t)
	schemaPath := filepath.Join(t.TempDir(), "schema.json")
	payloadPath := filepath.Join(t.TempDir(), "payload.json")
	require.NoError(t, os.WriteFile(schemaPath, []byte(`{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object","properties":{"status":{"type":"string"}},"required":["status"],"additionalProperties":false}`), 0o600))
	require.NoError(t, os.WriteFile(payloadPath, []byte(`{"status":"ready"}`), 0o600))

	// Act
	actor := runHubE2EJSON[hubstore.Actor](t, fixture, fixture.publisher, "actor", "enroll", "publisher")
	topic := runHubE2EJSON[hubstore.Topic](t, fixture, fixture.publisher, "topic", "create", "release", "--actor", actor.ID, "--name", "Release", "--description", "Cross-worktree coordination")
	schema := runHubE2EJSON[hubstore.EventSchema](t, fixture, fixture.publisher, "type", "register", topic.ID, "release.ready", "1", "--actor", actor.ID, "--generation", "1", "--schema-file", schemaPath)
	message := runHubE2EJSON[hubstore.Message](t, fixture, fixture.publisher, "message", "post", topic.ID, "--actor", actor.ID, "--text", "Ready to verify")
	event := runHubE2EJSON[hubstore.Event](t, fixture, fixture.publisher, "event", "publish", topic.ID, "--actor", actor.ID, "--type", "release.ready", "--version", "1", "--payload-file", payloadPath)
	discovered := runHubE2EJSON[[]hubstore.Topic](t, fixture, fixture.consumer, "topic", "list", "--query", "release")
	consumer := runHubE2EJSON[hubstore.Consumer](t, fixture, fixture.consumer, "consumer", "enroll", "release-reader")
	sub := runHubE2EJSON[hubstore.Subscription](t, fixture, fixture.consumer, "subscribe", "add", "--consumer", consumer.ID, "--topic", topic.ID, "--from-beginning")
	page := runHubE2EJSON[hubstore.PullPage](t, fixture, fixture.consumer, "pull", sub.ID, "--consumer", consumer.ID)
	ack := runHubE2EJSON[struct {
		SubscriptionID       string `json:"subscription_id"`
		AcknowledgedSequence int64  `json:"acknowledged_sequence"`
	}](t, fixture, fixture.consumer, "ack", sub.ID, "--consumer", consumer.ID, "--prior", strconv.FormatInt(page.PriorAcknowledged, 10), "--next", strconv.FormatInt(page.NextScanPosition, 10), "--token", page.AckToken)
	restarted := runHubE2EJSON[hubstore.PullPage](t, fixture, fixture.consumer, "pull", sub.ID, "--consumer", consumer.ID)

	// Assert
	assert.Equal(t, "publisher", actor.ID)
	assert.Equal(t, "active", topic.State)
	assert.NotEmpty(t, topic.ScopeID)
	assert.Equal(t, "release.ready", schema.TypeKey)
	assert.Equal(t, topic.ID, schema.TopicID)
	assert.Equal(t, "message", message.Kind)
	assert.Equal(t, int64(1), message.Sequence)
	assert.Equal(t, "event", event.Kind)
	assert.Equal(t, int64(2), event.Sequence)
	assert.JSONEq(t, `{"status":"ready"}`, string(event.Payload))
	require.Len(t, discovered, 1)
	assert.Equal(t, topic.ID, discovered[0].ID)
	assert.Equal(t, topic.ScopeID, discovered[0].ScopeID)
	assert.Equal(t, topic.ScopeID, sub.ScopeID)
	assert.Equal(t, int64(0), sub.AcknowledgedSequence)
	assert.Equal(t, int64(0), page.PriorAcknowledged)
	assert.Equal(t, int64(2), page.NextScanPosition)
	assert.NotEmpty(t, page.AckToken)
	require.Len(t, page.Entries, 2)
	require.NotNil(t, page.Entries[0].Message)
	assert.Equal(t, message.EntryID, page.Entries[0].Message.EntryID)
	assert.Equal(t, "Ready to verify", page.Entries[0].Message.Text)
	require.NotNil(t, page.Entries[1].Event)
	assert.Equal(t, event.EntryID, page.Entries[1].Event.EntryID)
	assert.JSONEq(t, `{"status":"ready"}`, string(page.Entries[1].Event.Payload))
	assert.Equal(t, sub.ID, ack.SubscriptionID)
	assert.Equal(t, int64(2), ack.AcknowledgedSequence)
	assert.Equal(t, int64(2), restarted.PriorAcknowledged)
	assert.Equal(t, int64(2), restarted.NextScanPosition)
	assert.Empty(t, restarted.Entries)
	assert.FileExists(t, filepath.Join(fixture.home, "hub.sqlite"))
	assert.NoFileExists(t, filepath.Join(fixture.home, "devspecs.db"), "hub must not require the rebuildable source index")
}

func TestHubE2EIdempotentReplayAcrossProcesses(t *testing.T) {
	// Arrange
	fixture := newHubE2EFixture(t)
	runHubE2EJSON[hubstore.Actor](t, fixture, fixture.publisher, "actor", "enroll", "publisher")
	topic := runHubE2EJSON[hubstore.Topic](t, fixture, fixture.publisher, "topic", "create", "replay", "--actor", "publisher", "--name", "Replay", "--description", "Retry verification")

	// Act
	first := runHubE2EJSON[hubstore.Message](t, fixture, fixture.publisher, "message", "post", topic.ID, "--actor", "publisher", "--text", "One publication", "--key", "retry-1")
	replayed := runHubE2EJSON[hubstore.Message](t, fixture, fixture.consumer, "message", "post", topic.ID, "--actor", "publisher", "--text", "One publication", "--key", "retry-1")
	messages := runHubE2EJSON[[]hubstore.Message](t, fixture, fixture.consumer, "message", "list", topic.ID)

	// Assert
	assert.False(t, first.Replayed)
	assert.True(t, replayed.Replayed)
	assert.Equal(t, first.EntryID, replayed.EntryID)
	assert.Equal(t, first.MessageID, replayed.MessageID)
	assert.Equal(t, first.Sequence, replayed.Sequence)
	require.Len(t, messages, 1)
	assert.Equal(t, first.EntryID, messages[0].EntryID)
	assert.NoFileExists(t, filepath.Join(fixture.home, "devspecs.db"))
}

func TestHubE2EConcurrentProcessPublicationsSerialize(t *testing.T) {
	// Arrange
	fixture := newHubE2EFixture(t)
	runHubE2EJSON[hubstore.Actor](t, fixture, fixture.publisher, "actor", "enroll", "publisher")
	topic := runHubE2EJSON[hubstore.Topic](t, fixture, fixture.publisher, "topic", "create", "parallel", "--actor", "publisher", "--name", "Parallel", "--description", "Concurrent writers")
	binary, err := os.Executable()
	require.NoError(t, err)
	firstCmd := exec.Command(binary, "-test.run=^TestHubE2EProcess$", "--", "--repo", ".", "--json", "message", "post", topic.ID, "--actor", "publisher", "--text", "first")
	firstCmd.Dir = fixture.publisher
	firstCmd.Env = hubE2EEnv(fixture.home)
	secondCmd := exec.Command(binary, "-test.run=^TestHubE2EProcess$", "--", "--repo", ".", "--json", "message", "post", topic.ID, "--actor", "publisher", "--text", "second")
	secondCmd.Dir = fixture.consumer
	secondCmd.Env = hubE2EEnv(fixture.home)
	type processResult struct {
		output []byte
		err    error
	}
	start := make(chan struct{})
	firstDone := make(chan processResult, 1)
	secondDone := make(chan processResult, 1)
	go func() {
		<-start
		output, runErr := firstCmd.CombinedOutput()
		firstDone <- processResult{output: output, err: runErr}
	}()
	go func() {
		<-start
		output, runErr := secondCmd.CombinedOutput()
		secondDone <- processResult{output: output, err: runErr}
	}()

	// Act
	close(start)
	firstResult := <-firstDone
	secondResult := <-secondDone
	messages := runHubE2EJSON[[]hubstore.Message](t, fixture, fixture.consumer, "message", "list", topic.ID)

	// Assert
	require.NoError(t, firstResult.err, string(firstResult.output))
	require.NoError(t, secondResult.err, string(secondResult.output))
	var first, second hubJSONResponse[hubstore.Message]
	require.NoError(t, json.Unmarshal(firstResult.output, &first))
	require.NoError(t, json.Unmarshal(secondResult.output, &second))
	assert.NotEqual(t, first.Result.EntryID, second.Result.EntryID)
	assert.NotEqual(t, first.Result.Sequence, second.Result.Sequence)
	assert.Equal(t, int64(3), first.Result.Sequence+second.Result.Sequence)
	require.Len(t, messages, 2)
	assert.Equal(t, int64(2), messages[0].Sequence)
	assert.Equal(t, int64(1), messages[1].Sequence)
	assert.NoFileExists(t, filepath.Join(fixture.home, "devspecs.db"))
}
