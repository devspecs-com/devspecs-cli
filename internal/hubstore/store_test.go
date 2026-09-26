package hubstore

import (
	"context"
	"database/sql"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testRepo(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "repo")
	require.NoError(t, os.Mkdir(path, 0o700))
	cmd := exec.Command("git", "init", "-q", path)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
	return path
}

func testDB(t *testing.T, home string, now func() time.Time) *DB {
	t.Helper()
	d, err := Open(context.Background(), Options{Home: home, Now: now})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, d.Close()) })
	return d
}

func insertDanglingBinding(t *testing.T, d *DB) {
	t.Helper()
	_, err := d.sql.Exec("PRAGMA foreign_keys=OFF")
	require.NoError(t, err)
	_, err = d.sql.Exec("INSERT INTO scope_bindings (scope_id, common_dir, marker, file_identity) VALUES ('missing', 'missing-dir', 'missing-marker', 'missing-file')")
	require.NoError(t, err)
	_, err = d.sql.Exec("PRAGMA foreign_keys=ON")
	require.NoError(t, err)
	var enabled int
	require.NoError(t, d.sql.QueryRow("PRAGMA foreign_keys").Scan(&enabled))
	require.Equal(t, 1, enabled)
}

func TestOpenCreatesDedicatedAuthority(t *testing.T) {
	// Arrange
	home := filepath.Join(t.TempDir(), "home")

	// Act
	d, err := Open(context.Background(), Options{Home: home})
	require.NoError(t, err)
	defer d.Close()

	// Assert
	assert.Equal(t, filepath.Join(home, "hub.sqlite"), d.Path())
	assert.NotEmpty(t, d.AuthorityID())
	_, err = os.Stat(filepath.Join(home, "devspecs.db"))
	assert.ErrorIs(t, err, os.ErrNotExist)
	var journal, syncMode string
	var fk int
	require.NoError(t, d.sql.QueryRow("PRAGMA journal_mode").Scan(&journal))
	require.NoError(t, d.sql.QueryRow("PRAGMA synchronous").Scan(&syncMode))
	require.NoError(t, d.sql.QueryRow("PRAGMA foreign_keys").Scan(&fk))
	assert.Equal(t, "wal", journal)
	assert.Equal(t, "2", syncMode)
	assert.Equal(t, 1, fk)
}

func TestOpenExistingAuthorityPreservesIdentity(t *testing.T) {
	// Arrange
	home := filepath.Join(t.TempDir(), "home")
	initial := testDB(t, home, nil)
	id := initial.AuthorityID()

	// Act
	reopened, err := Open(context.Background(), Options{Home: home})
	require.NoError(t, err)
	defer reopened.Close()

	// Assert
	assert.Equal(t, id, reopened.AuthorityID())
}

func TestOpenExistingAuthoritySkipsFullForeignKeyScan(t *testing.T) {
	// Arrange
	home := filepath.Join(t.TempDir(), "home")
	d := testDB(t, home, nil)
	insertDanglingBinding(t, d)

	// Act
	reopened, err := Open(context.Background(), Options{Home: home})
	require.NoError(t, err)
	defer reopened.Close()

	// Assert
	assert.Equal(t, d.AuthorityID(), reopened.AuthorityID())
}

func TestCheckIntegrityRejectsDanglingForeignKey(t *testing.T) {
	// Arrange
	d := testDB(t, filepath.Join(t.TempDir(), "home"), nil)
	insertDanglingBinding(t, d)

	// Act
	err := d.CheckIntegrity(context.Background())

	// Assert
	assert.ErrorIs(t, err, ErrUnsupportedFormat)
}

func TestSQLiteDSNEncodesQuestionAndFragmentInPath(t *testing.T) {
	// Arrange
	path := filepath.Join(t.TempDir(), "home?#", "hub.sqlite")

	// Act
	dsn := sqliteDSN(path, "rw")

	// Assert
	parsed, err := url.Parse(dsn)
	require.NoError(t, err)
	expectedPath := filepath.ToSlash(path)
	if runtime.GOOS == "windows" {
		expectedPath = "/" + expectedPath
	}
	assert.Equal(t, expectedPath, parsed.Path)
	assert.Equal(t, "rw", parsed.Query().Get("mode"))
	assert.Contains(t, dsn, "%3F")
	assert.Contains(t, dsn, "%23")
}

func TestOpenHomeContainingFragment(t *testing.T) {
	// Arrange
	home := filepath.Join(t.TempDir(), "home#fragment")

	// Act
	d, err := Open(context.Background(), Options{Home: home})
	require.NoError(t, err)
	defer d.Close()

	// Assert
	assert.Equal(t, filepath.Join(home, "hub.sqlite"), d.Path())
	_, err = os.Stat(d.Path())
	assert.NoError(t, err)
}

func TestOpenReadOnlyHomeContainingFragment(t *testing.T) {
	// Arrange
	home := filepath.Join(t.TempDir(), "home#fragment")
	d := testDB(t, home, nil)

	// Act
	reader, err := OpenReadOnly(context.Background(), Options{Home: home})
	require.NoError(t, err)
	defer reader.Close()

	// Assert
	assert.Equal(t, d.AuthorityID(), reader.AuthorityID())
}

func TestOpenInitializesRetentionCountersAtZero(t *testing.T) {
	// Arrange
	home := filepath.Join(t.TempDir(), "home")

	// Act
	d, err := Open(context.Background(), Options{Home: home})
	require.NoError(t, err)
	defer d.Close()

	// Assert
	var epoch int64
	require.NoError(t, d.sql.QueryRow("SELECT retention_epoch FROM hub_meta WHERE singleton=1").Scan(&epoch))
	assert.Zero(t, epoch)
}

func TestOpenUsesConfiguredHome(t *testing.T) {
	// Arrange
	home := filepath.Join(t.TempDir(), "configured")
	t.Setenv("DEVSPECS_HOME", home)

	// Act
	d, err := Open(context.Background(), Options{})
	require.NoError(t, err)
	defer d.Close()

	// Assert
	assert.Equal(t, filepath.Join(home, "hub.sqlite"), d.Path())
}

func TestOpenRejectsNewerFormatBeforeMutation(t *testing.T) {
	// Arrange
	home := filepath.Join(t.TempDir(), "home")
	d := testDB(t, home, nil)
	path := d.Path()
	raw, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	_, err = raw.Exec("PRAGMA user_version=3")
	require.NoError(t, err)
	require.NoError(t, raw.Close())
	before, err := os.ReadFile(path)
	require.NoError(t, err)

	// Act
	_, err = Open(context.Background(), Options{Home: home})

	// Assert
	assert.ErrorIs(t, err, ErrUnsupportedFormat)
	after, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	assert.Equal(t, before, after)
}

func TestOpenRejectsUnknownApplicationBeforeMutation(t *testing.T) {
	// Arrange
	home := filepath.Join(t.TempDir(), "home")
	require.NoError(t, os.MkdirAll(home, 0o700))
	path := filepath.Join(home, "hub.sqlite")
	raw, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	_, err = raw.Exec("CREATE TABLE unrelated (id INTEGER)")
	require.NoError(t, err)
	require.NoError(t, raw.Close())
	before, err := os.ReadFile(path)
	require.NoError(t, err)

	// Act
	_, err = Open(context.Background(), Options{Home: home})

	// Assert
	assert.ErrorIs(t, err, ErrUnsupportedFormat)
	after, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	assert.Equal(t, before, after)
}

func TestWriteAdmissionHasBoundedBusyError(t *testing.T) {
	// Arrange
	ctx := context.Background()
	d := testDB(t, filepath.Join(t.TempDir(), "home"), nil)
	other, err := sql.Open("sqlite", d.Path())
	require.NoError(t, err)
	defer other.Close()
	conn, err := other.Conn(ctx)
	require.NoError(t, err)
	defer conn.Close()
	_, err = conn.ExecContext(ctx, "BEGIN IMMEDIATE")
	require.NoError(t, err)
	defer conn.ExecContext(ctx, "ROLLBACK")

	// Act
	_, err = d.EnrollActor(ctx, "contender")

	// Assert
	assert.ErrorIs(t, err, ErrBusyRetryable)
}

func TestReadOnlyMissingAuthorityCreatesNothing(t *testing.T) {
	// Arrange
	home := filepath.Join(t.TempDir(), "absent")

	// Act
	_, err := OpenReadOnly(context.Background(), Options{Home: home})

	// Assert
	assert.ErrorIs(t, err, ErrNotFound)
	_, err = os.Stat(home)
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestReadOnlyOpenInspectsExistingAuthority(t *testing.T) {
	// Arrange
	home := filepath.Join(t.TempDir(), "home")
	d := testDB(t, home, nil)

	// Act
	reader, err := OpenReadOnly(context.Background(), Options{Home: home})
	require.NoError(t, err)
	defer reader.Close()

	// Assert
	assert.Equal(t, d.AuthorityID(), reader.AuthorityID())
}

func TestReadOnlyOpenRejectsWrites(t *testing.T) {
	// Arrange
	home := filepath.Join(t.TempDir(), "home")
	_ = testDB(t, home, nil)
	reader, err := OpenReadOnly(context.Background(), Options{Home: home})
	require.NoError(t, err)
	defer reader.Close()

	// Act
	_, err = reader.EnrollActor(context.Background(), "blocked")

	// Assert
	assert.ErrorIs(t, err, ErrUnauthorized)
}

func TestLookupRepoDoesNotEnroll(t *testing.T) {
	// Arrange
	repo := testRepo(t)
	d := testDB(t, filepath.Join(t.TempDir(), "home"), nil)
	common, err := exec.Command("git", "-C", repo, "rev-parse", "--path-format=absolute", "--git-common-dir").Output()
	require.NoError(t, err)

	// Act
	_, err = d.LookupRepo(context.Background(), repo)

	// Assert
	assert.ErrorIs(t, err, ErrNotFound)
	_, err = os.Stat(filepath.Join(strings.TrimSpace(string(common)), markerFile))
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestEnrollmentIsolatedAcrossHomes(t *testing.T) {
	// Arrange
	ctx := context.Background()
	repo := testRepo(t)
	d1 := testDB(t, filepath.Join(t.TempDir(), "first"), nil)
	d2 := testDB(t, filepath.Join(t.TempDir(), "second"), nil)
	first, err := d1.EnrollRepo(ctx, repo)
	require.NoError(t, err)

	// Act
	second, err := d2.EnrollRepo(ctx, repo)
	require.NoError(t, err)

	// Assert
	assert.NotEqual(t, first.ID, second.ID)
	assert.Equal(t, first.CommonDir, second.CommonDir)
}

func TestLookupRefusesChangedIncarnation(t *testing.T) {
	// Arrange
	ctx := context.Background()
	repo := testRepo(t)
	d := testDB(t, filepath.Join(t.TempDir(), "home"), nil)
	scope, err := d.EnrollRepo(ctx, repo)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(scope.CommonDir, markerFile), []byte("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\n"), 0o600))

	// Act
	_, err = d.LookupRepo(ctx, repo)

	// Assert
	assert.ErrorIs(t, err, ErrBindingConflict)
}

func TestLinkedWorktreeUsesSameScope(t *testing.T) {
	// Arrange
	ctx := context.Background()
	repo := testRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repo, "README"), []byte("hello\n"), 0o600))
	addOut, err := exec.Command("git", "-C", repo, "add", "README").CombinedOutput()
	require.NoError(t, err, string(addOut))
	commitOut, err := exec.Command("git", "-C", repo, "-c", "user.name=Hub Test", "-c", "user.email=hub@example.invalid", "commit", "-qm", "initial").CombinedOutput()
	require.NoError(t, err, string(commitOut))
	worktree := filepath.Join(t.TempDir(), "linked")
	out, err := exec.Command("git", "-C", repo, "worktree", "add", "--detach", worktree).CombinedOutput()
	require.NoError(t, err, string(out))
	d := testDB(t, filepath.Join(t.TempDir(), "home"), nil)
	original, err := d.EnrollRepo(ctx, repo)
	require.NoError(t, err)

	// Act
	linked, err := d.EnrollRepo(ctx, worktree)
	require.NoError(t, err)

	// Assert
	assert.Equal(t, original.ID, linked.ID)
	assert.Equal(t, original.CommonDir, linked.CommonDir)
}

func TestActorEnrollmentReusesExplicitID(t *testing.T) {
	// Arrange
	ctx := context.Background()
	d := testDB(t, filepath.Join(t.TempDir(), "home"), nil)
	first, err := d.EnrollActor(ctx, "agent.one")
	require.NoError(t, err)

	// Act
	second, err := d.EnrollActor(ctx, "agent.one")
	require.NoError(t, err)

	// Assert
	assert.Equal(t, first, second)
}

func TestEnrollRepoWithDeletedMarkerRejectsBindingWithoutRecreation(t *testing.T) {
	// Arrange
	ctx := context.Background()
	repo := testRepo(t)
	d := testDB(t, filepath.Join(t.TempDir(), "home"), nil)
	scope, err := d.EnrollRepo(ctx, repo)
	require.NoError(t, err)
	markerPath := filepath.Join(scope.CommonDir, markerFile)
	require.NoError(t, os.Remove(markerPath))

	// Act
	_, err = d.EnrollRepo(ctx, repo)

	// Assert
	assert.ErrorIs(t, err, ErrBindingConflict)
	_, statErr := os.Stat(markerPath)
	assert.ErrorIs(t, statErr, os.ErrNotExist)
}

func TestEnrollRepoInitializesLowWaterSequenceAtZero(t *testing.T) {
	// Arrange
	ctx := context.Background()
	repo := testRepo(t)
	d := testDB(t, filepath.Join(t.TempDir(), "home"), nil)

	// Act
	scope, err := d.EnrollRepo(ctx, repo)
	require.NoError(t, err)

	// Assert
	var lowWater int64
	require.NoError(t, d.sql.QueryRow("SELECT low_water_sequence FROM repo_scopes WHERE scope_id=?", scope.ID).Scan(&lowWater))
	assert.Zero(t, lowWater)
}

func TestResolveGitPreservesCanonicalCommonDirForFilesystem(t *testing.T) {
	// Arrange
	repo := testRepo(t)
	common, err := exec.Command("git", "-C", repo, "rev-parse", "--path-format=absolute", "--git-common-dir").Output()
	require.NoError(t, err)
	canonical, err := filepath.EvalSymlinks(strings.TrimSpace(string(common)))
	require.NoError(t, err)
	canonical, err = filepath.Abs(canonical)
	require.NoError(t, err)

	// Act
	ev, err := resolveGit(context.Background(), repo, true)
	require.NoError(t, err)

	// Assert
	assert.Equal(t, filepath.Clean(canonical), ev.path)
	if runtime.GOOS == "windows" {
		assert.Equal(t, strings.ToLower(ev.path), ev.key)
	} else {
		assert.Equal(t, ev.path, ev.key)
	}
	_, err = os.Stat(filepath.Join(ev.path, markerFile))
	assert.NoError(t, err)
}
