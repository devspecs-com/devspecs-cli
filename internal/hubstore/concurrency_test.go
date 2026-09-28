package hubstore

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIndependentProcessesRaceOneTopicKey(t *testing.T) {
	// Arrange
	ctx := context.Background()
	repo := testRepo(t)
	home := filepath.Join(t.TempDir(), "home")
	d := testDB(t, home, nil)
	_, err := d.EnrollRepo(ctx, repo)
	require.NoError(t, err)
	_, err = d.EnrollActor(ctx, "owner")
	require.NoError(t, err)
	first := exec.Command(os.Args[0], "-test.run=^TestHubProcessCreateTopic$")
	first.Env = append(os.Environ(), "HUB_CHILD=1", "HUB_HOME="+home, "HUB_REPO="+repo)
	second := exec.Command(os.Args[0], "-test.run=^TestHubProcessCreateTopic$")
	second.Env = append(os.Environ(), "HUB_CHILD=1", "HUB_HOME="+home, "HUB_REPO="+repo)
	var firstOut, secondOut bytes.Buffer
	first.Stdout, first.Stderr = &firstOut, &firstOut
	second.Stdout, second.Stderr = &secondOut, &secondOut

	// Act
	require.NoError(t, first.Start())
	require.NoError(t, second.Start())
	firstErr := first.Wait()
	secondErr := second.Wait()

	// Assert
	require.NoError(t, firstErr, firstOut.String())
	require.NoError(t, secondErr, secondOut.String())
	a, b := firstOut.String(), secondOut.String()
	assert.Equal(t, 1, strings.Count(a, "created")+strings.Count(b, "created"))
	assert.Equal(t, 1, strings.Count(a, "conflict")+strings.Count(b, "conflict"))
	var count int
	require.NoError(t, d.sql.QueryRowContext(ctx, "SELECT COUNT(*) FROM topics WHERE key='race'").Scan(&count))
	assert.Equal(t, 1, count)
}

func TestConcurrentFirstOpenReturnsCompleteAuthorityToBothCallers(t *testing.T) {
	// Arrange
	home := filepath.Join(t.TempDir(), "home")
	ctx := context.Background()
	start := make(chan struct{})
	type result struct {
		db  *DB
		err error
	}
	results := make(chan result, 2)
	var ready sync.WaitGroup
	ready.Add(2)
	openOnce := func() {
		ready.Done()
		<-start
		db, err := Open(ctx, Options{Home: home})
		results <- result{db: db, err: err}
	}

	// Act
	go openOnce()
	go openOnce()
	ready.Wait()
	close(start)
	first := <-results
	second := <-results

	// Assert
	require.NoError(t, first.err)
	require.NoError(t, second.err)
	require.NotNil(t, first.db)
	require.NotNil(t, second.db)
	defer first.db.Close()
	defer second.db.Close()
	assert.NotEmpty(t, first.db.AuthorityID())
	assert.Equal(t, first.db.AuthorityID(), second.db.AuthorityID())
}

func TestConcurrentArchiveAndSchemaRegistrationCommitOnePolicyGeneration(t *testing.T) {
	// Arrange
	ctx := context.Background()
	f := newMessageFixture(t)
	other := testDB(t, filepath.Dir(f.d.Path()), nil)
	start := make(chan struct{})
	var ready sync.WaitGroup
	ready.Add(2)
	type archiveResult struct {
		topic Topic
		err   error
	}
	archiveResults := make(chan archiveResult, 1)
	schemaResults := make(chan error, 1)

	// Act
	go func() {
		ready.Done()
		<-start
		topic, err := f.d.ArchiveTopic(ctx, f.repo, f.topic.ID, "owner", f.topic.PolicyGeneration, "Completed")
		archiveResults <- archiveResult{topic: topic, err: err}
	}()
	go func() {
		ready.Done()
		<-start
		_, err := other.RegisterEventSchema(ctx, f.repo, f.topic.ID, "owner", f.topic.PolicyGeneration, "job.finished", 1, []byte(jobSchema))
		schemaResults <- err
	}()
	ready.Wait()
	close(start)
	archive := <-archiveResults
	schemaErr := <-schemaResults
	topic, topicErr := f.d.ShowTopic(ctx, f.repo, f.topic.ID)
	_, schemaReadErr := f.d.ShowEventSchema(ctx, f.repo, f.topic.ID, "job.finished", 1)
	audit, auditErr := f.d.ListTopicAudit(ctx, f.repo, f.topic.ID, 10, 0)

	// Assert
	require.NoError(t, topicErr)
	require.NoError(t, auditErr)
	require.Len(t, audit, 2)
	assert.Equal(t, f.topic.PolicyGeneration+1, topic.PolicyGeneration)
	if archive.err == nil {
		assert.ErrorIs(t, schemaErr, ErrConflict)
		assert.Equal(t, "archived", archive.topic.State)
		assert.Equal(t, "archived", topic.State)
		assert.ErrorIs(t, schemaReadErr, ErrVersionUnknown)
		assert.Equal(t, "archive", audit[1].Action)
	} else {
		assert.ErrorIs(t, archive.err, ErrConflict)
		assert.NoError(t, schemaErr)
		assert.Equal(t, "active", topic.State)
		assert.NoError(t, schemaReadErr)
		assert.Equal(t, "register_event_schema", audit[1].Action)
	}
}

func TestHubProcessCreateTopic(t *testing.T) {
	if os.Getenv("HUB_CHILD") != "1" {
		t.Skip("child process only")
	}
	ctx := context.Background()
	d, err := Open(ctx, Options{Home: os.Getenv("HUB_HOME")})
	require.NoError(t, err)
	defer d.Close()
	_, err = d.CreateTopic(ctx, os.Getenv("HUB_REPO"), "owner", TopicInput{Key: "race", Name: "Race", Description: "Concurrent creation"})
	if err == nil {
		fmt.Println("created")
		return
	}
	if errors.Is(err, ErrConflict) {
		fmt.Println("conflict")
		return
	}
	require.NoError(t, err)
}
