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
