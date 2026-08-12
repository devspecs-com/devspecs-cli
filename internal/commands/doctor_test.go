package commands

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/devspecs-com/devspecs-cli/internal/store"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewDoctorCmd_ReturnsReadOnlyDiagnosticSurface(t *testing.T) {
	cmd := NewDoctorCmd()

	assert.Equal(t, "doctor", cmd.Use)
	assert.Contains(t, cmd.Short, "Diagnose")
	assert.NotNil(t, cmd.Flags().Lookup("repo"))
	assert.NotNil(t, cmd.Flags().Lookup("json"))
	assert.NotNil(t, cmd.Flags().Lookup("redact"))
}

func TestDoctorCommand_WhenHomeIsMissing_DoesNotCreateLocalState(t *testing.T) {
	home := filepath.Join(t.TempDir(), "missing-home")
	repoRoot := t.TempDir()
	t.Setenv("DEVSPECS_HOME", home)
	cmd := NewDoctorCmd()
	output := &bytes.Buffer{}
	cmd.SetOut(output)
	cmd.SetArgs([]string{"--repo", repoRoot, "--json"})

	err := cmd.Execute()

	require.NoError(t, err)
	var report doctorReport
	require.NoError(t, json.Unmarshal(output.Bytes(), &report))
	assert.Equal(t, "environment", report.Home.Source)
	assert.Equal(t, home, report.Home.Path)
	assert.False(t, report.Home.Exists)
	assert.False(t, report.Index.Exists)
	assert.Equal(t, store.IndexCompatibilityAbsent, report.Index.Compatibility)
	_, statErr := os.Stat(home)
	assert.ErrorIs(t, statErr, os.ErrNotExist)
}

func TestDoctorCommand_WhenIndexIsMissingInGitRepository_RecommendsInitialScan(t *testing.T) {
	home := filepath.Join(t.TempDir(), "missing-home")
	repoRoot := initDoctorGitRepository(t)
	t.Setenv("DEVSPECS_HOME", home)
	cmd := NewDoctorCmd()
	output := &bytes.Buffer{}
	cmd.SetOut(output)
	cmd.SetArgs([]string{"--repo", repoRoot, "--json"})

	err := cmd.Execute()

	require.NoError(t, err)
	var report doctorReport
	require.NoError(t, json.Unmarshal(output.Bytes(), &report))
	assert.Equal(t, doctorStatusWarning, report.Status)
	assert.True(t, report.Repository.IsGit)
	assert.Equal(t, doctorStatusWarning, report.Index.Status)
	assert.Contains(t, output.String(), `"id": "index.missing"`)
	assert.Contains(t, output.String(), `"safety": "mutating"`)
	assert.Contains(t, output.String(), "ds scan --path")
}

func TestDoctorCommand_WithCurrentSchema_ReportsHealthyIndex(t *testing.T) {
	home := t.TempDir()
	repoRoot := t.TempDir()
	t.Setenv("DEVSPECS_HOME", home)
	database, err := store.Open(filepath.Join(home, "devspecs.db"))
	require.NoError(t, err)
	require.NoError(t, database.Close())
	cmd := NewDoctorCmd()
	output := &bytes.Buffer{}
	cmd.SetOut(output)
	cmd.SetArgs([]string{"--repo", repoRoot, "--json"})

	err = cmd.Execute()

	require.NoError(t, err)
	var report doctorReport
	require.NoError(t, json.Unmarshal(output.Bytes(), &report))
	assert.Equal(t, doctorStatusOK, report.Index.Status)
	assert.Equal(t, store.SchemaVersion, report.Index.DatabaseSchema)
	assert.Equal(t, store.IndexCompatibilityCurrent, report.Index.Compatibility)
}

func TestDoctorCommand_WithOlderSchema_RecommendsForwardMigration(t *testing.T) {
	home := t.TempDir()
	repoRoot := t.TempDir()
	t.Setenv("DEVSPECS_HOME", home)
	writeDoctorSchemaDatabase(t, filepath.Join(home, "devspecs.db"), store.SchemaVersion-1)
	cmd := NewDoctorCmd()
	output := &bytes.Buffer{}
	cmd.SetOut(output)
	cmd.SetArgs([]string{"--repo", repoRoot, "--json"})

	err := cmd.Execute()

	require.NoError(t, err)
	var report doctorReport
	require.NoError(t, json.Unmarshal(output.Bytes(), &report))
	assert.Equal(t, doctorStatusWarning, report.Index.Status)
	assert.Equal(t, store.IndexCompatibilityOlder, report.Index.Compatibility)
	assert.Contains(t, output.String(), `"id": "index.schema_older"`)
	assert.NotContains(t, output.String(), "scan --rebuild")
}

func TestDoctorCommand_WithNewerSchema_RendersJSONBeforeReturningError(t *testing.T) {
	home := t.TempDir()
	repoRoot := t.TempDir()
	t.Setenv("DEVSPECS_HOME", home)
	writeDoctorSchemaDatabase(t, filepath.Join(home, "devspecs.db"), store.SchemaVersion+1)
	cmd := NewDoctorCmd()
	output := &bytes.Buffer{}
	cmd.SetOut(output)
	cmd.SetArgs([]string{"--repo", repoRoot, "--json"})

	err := cmd.Execute()

	require.Error(t, err)
	assert.ErrorContains(t, err, "doctor found one or more errors")
	var report doctorReport
	require.NoError(t, json.Unmarshal(output.Bytes(), &report))
	assert.Equal(t, doctorStatusError, report.Status)
	assert.Equal(t, doctorStatusError, report.Index.Status)
	assert.Equal(t, store.SchemaVersion+1, report.Index.DatabaseSchema)
	assert.Equal(t, store.IndexCompatibilityNewer, report.Index.Compatibility)
	assert.Contains(t, output.String(), `"id": "index.schema_newer"`)
	assert.NotContains(t, output.String(), "scan --rebuild")
}

func TestDoctorCommand_WithMalformedIndex_PreservesIndependentRuntimeEvidence(t *testing.T) {
	home := t.TempDir()
	repoRoot := t.TempDir()
	dbPath := filepath.Join(home, "devspecs.db")
	t.Setenv("DEVSPECS_HOME", home)
	require.NoError(t, os.WriteFile(dbPath, []byte("not sqlite"), 0o600))
	cmd := NewDoctorCmd()
	output := &bytes.Buffer{}
	cmd.SetOut(output)
	cmd.SetArgs([]string{"--repo", repoRoot, "--json"})

	err := cmd.Execute()

	require.Error(t, err)
	var report doctorReport
	require.NoError(t, json.Unmarshal(output.Bytes(), &report))
	assert.Equal(t, doctorStatusError, report.Index.Status)
	assert.Equal(t, "unreadable", report.Index.Compatibility)
	assert.NotEmpty(t, report.Runtime.Executable)
	assert.NotEmpty(t, report.Runtime.Version)
	assert.Contains(t, output.String(), `"id": "index.unreadable"`)
}

func TestDoctorCommand_WithInterruptedRecoveryJournal_ReportsExactRestoreAction(t *testing.T) {
	home := t.TempDir()
	repoRoot := t.TempDir()
	t.Setenv("DEVSPECS_HOME", home)
	activePath := filepath.Join(home, "devspecs.db")
	database, err := store.Open(activePath)
	require.NoError(t, err)
	require.NoError(t, database.Close())
	backupPath := filepath.Join(home, "backups", "index", "rollback.db")
	journal := store.IndexRecoveryJournal{
		Version:     1,
		OperationID: "interrupted",
		Operation:   store.IndexBackupReasonRestore,
		ActivePath:  activePath,
		BackupPath:  backupPath,
		Phase:       "active_preserved",
	}
	body, err := json.Marshal(journal)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(store.IndexRecoveryJournalPath(activePath), body, 0o600))
	cmd := NewDoctorCmd()
	output := &bytes.Buffer{}
	cmd.SetOut(output)
	cmd.SetArgs([]string{"--repo", repoRoot, "--json"})

	err = cmd.Execute()

	require.Error(t, err)
	var report doctorReport
	require.NoError(t, json.Unmarshal(output.Bytes(), &report))
	assert.True(t, report.Index.RecoveryPending)
	assert.Equal(t, "active_preserved", report.Index.RecoveryPhase)
	assert.Equal(t, store.IndexRecoveryJournalPath(activePath), report.Index.RecoveryJournal)
	assert.Equal(t, backupPath, report.Index.RecoveryBackup)
	assert.Contains(t, output.String(), `"id": "index.recovery_incomplete"`)
	assert.Contains(t, output.String(), "ds index restore")
}

func TestDoctorCommand_WithHomePathThatIsFile_ReturnsCompleteJSONError(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home-file")
	repoRoot := t.TempDir()
	require.NoError(t, os.WriteFile(home, []byte("not a directory"), 0o600))
	t.Setenv("DEVSPECS_HOME", home)
	cmd := NewDoctorCmd()
	output := &bytes.Buffer{}
	cmd.SetOut(output)
	cmd.SetArgs([]string{"--repo", repoRoot, "--json"})

	err := cmd.Execute()

	require.Error(t, err)
	var report doctorReport
	require.NoError(t, json.Unmarshal(output.Bytes(), &report))
	assert.Equal(t, doctorStatusError, report.Status)
	assert.Equal(t, doctorStatusError, report.Home.Status)
	assert.Equal(t, doctorStatusNotApplicable, report.Index.Status)
	assert.Equal(t, store.IndexCompatibilityAbsent, report.Index.Compatibility)
	assert.Contains(t, output.String(), `"id": "home.not_directory"`)
}

func TestDoctorCommand_WhenWriterLeaseIsHeld_ReportsWarningWithoutFailure(t *testing.T) {
	home := t.TempDir()
	repoRoot := t.TempDir()
	dbPath := filepath.Join(home, "devspecs.db")
	t.Setenv("DEVSPECS_HOME", home)
	database, err := store.Open(dbPath)
	require.NoError(t, err)
	require.NoError(t, database.Close())
	lease, err := store.AcquireIndexWriter(context.Background(), dbPath, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = lease.Release() })
	cmd := NewDoctorCmd()
	output := &bytes.Buffer{}
	cmd.SetOut(output)
	cmd.SetArgs([]string{"--repo", repoRoot, "--json"})

	err = cmd.Execute()

	require.NoError(t, err)
	var report doctorReport
	require.NoError(t, json.Unmarshal(output.Bytes(), &report))
	assert.Equal(t, doctorStatusWarning, report.Status)
	assert.Equal(t, doctorStatusWarning, report.Index.Status)
	assert.Equal(t, store.IndexWriterStateHeld, report.Index.WriterState)
	assert.Contains(t, output.String(), `"id": "index.writer_held"`)
}

func TestDoctorCommand_WithRedaction_RemovesRepositoryAndHomePaths(t *testing.T) {
	home := filepath.Join(t.TempDir(), "private-home")
	repoRoot := initDoctorGitRepository(t)
	t.Setenv("DEVSPECS_HOME", home)
	cmd := NewDoctorCmd()
	output := &bytes.Buffer{}
	cmd.SetOut(output)
	cmd.SetArgs([]string{"--repo", repoRoot, "--redact", "--json"})

	err := cmd.Execute()

	require.NoError(t, err)
	var report doctorReport
	require.NoError(t, json.Unmarshal(output.Bytes(), &report))
	assert.True(t, report.Redacted)
	assert.Equal(t, "<home>", report.Home.Path)
	assert.Equal(t, "<repo>", report.Repository.RootPath)
	assert.NotContains(t, output.String(), home)
	assert.NotContains(t, output.String(), repoRoot)
	assert.Contains(t, output.String(), "<home>")
	assert.Contains(t, output.String(), "<repo>")
}

func TestRedactDoctorReport_WithLocalPathsAndIdentity_RemovesShareSensitiveValues(t *testing.T) {
	home := filepath.Join("C:\\Users", "alice", ".devspecs")
	repoRoot := filepath.Join("C:\\Users", "alice", "src", "private-repo")
	report := doctorReport{
		Runtime: doctorRuntimeReport{
			Executable:         filepath.Join(home, "bin", "ds.exe"),
			ResolvedExecutable: filepath.Join(home, "bin", "ds.exe"),
			PathFirst:          filepath.Join(repoRoot, "bin", "ds.exe"),
			PathCandidates: []string{
				filepath.Join(home, "bin", "ds.exe"),
				filepath.Join(repoRoot, "bin", "ds.exe"),
			},
		},
		Home:  doctorHomeReport{Path: home},
		Index: doctorIndexReport{Path: filepath.Join(home, "devspecs.db")},
		Repository: doctorRepositoryReport{
			RequestedPath: repoRoot,
			RootPath:      repoRoot,
			Branch:        "secret-feature",
			GitIdentity:   "git_private",
		},
		Findings: []doctorFinding{{
			ID:       "index.missing",
			Evidence: "active " + filepath.Join(home, "bin", "ds.exe") + "; first " + filepath.Join(repoRoot, "bin", "ds.exe"),
			Remediation: []doctorRemediation{{
				Command: "ds scan --path " + repoRoot,
				Safety:  "mutating",
				Reason:  "scan " + repoRoot,
			}},
		}},
	}

	redacted := redactDoctorReport(report)

	assert.True(t, redacted.Redacted)
	assert.Contains(t, redacted.Runtime.Executable, "<home>")
	assert.Contains(t, redacted.Runtime.PathFirst, "<repo>")
	assert.Empty(t, redacted.Runtime.PathCandidates)
	assert.Equal(t, "<redacted>", redacted.Repository.Branch)
	assert.Equal(t, "<redacted>", redacted.Repository.GitIdentity)
	require.Len(t, redacted.Findings, 1)
	assert.NotContains(t, redacted.Findings[0].Evidence, home)
	assert.NotContains(t, redacted.Findings[0].Evidence, repoRoot)
	assert.Contains(t, redacted.Findings[0].Evidence, "<home>")
	assert.Contains(t, redacted.Findings[0].Evidence, "<repo>")
	require.Len(t, redacted.Findings[0].Remediation, 1)
	assert.NotContains(t, redacted.Findings[0].Remediation[0].Command, repoRoot)
	assert.Contains(t, redacted.Findings[0].Remediation[0].Command, "<repo>")
}

func TestOutputDoctorReport_WithHumanOutput_ExplainsSharingBoundary(t *testing.T) {
	report := doctorReport{
		Status: doctorStatusOK,
		Runtime: doctorRuntimeReport{
			Version:           "v1.4.0",
			Commit:            "abc123",
			Built:             "2026-08-12",
			OS:                "linux",
			Arch:              "amd64",
			Executable:        "/usr/local/bin/ds",
			InstallSource:     "manual or unknown",
			InstallConfidence: "low",
		},
		Home:       doctorHomeReport{Path: "/home/alice/.devspecs", Source: "default"},
		Index:      doctorIndexReport{Path: "/home/alice/.devspecs/devspecs.db", SupportedSchema: store.SchemaVersion},
		Repository: doctorRepositoryReport{RootPath: "/work/repo", IdentityMode: "git"},
		Findings:   []doctorFinding{},
	}
	cmd := &cobra.Command{}
	output := &bytes.Buffer{}
	cmd.SetOut(output)

	err := outputDoctorReport(cmd, report, false)

	require.NoError(t, err)
	assert.Contains(t, output.String(), "DevSpecs doctor")
	assert.Contains(t, output.String(), "Runtime")
	assert.Contains(t, output.String(), "Local state")
	assert.Contains(t, output.String(), "Repository")
	assert.Contains(t, output.String(), "ds doctor --redact")
}

func writeDoctorSchemaDatabase(t *testing.T, dbPath string, schemaVersion int) {
	t.Helper()
	database, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)
	_, err = database.Exec("CREATE TABLE schema_migrations (version INTEGER NOT NULL)")
	require.NoError(t, err)
	_, err = database.Exec("INSERT INTO schema_migrations (version) VALUES (?)", schemaVersion)
	require.NoError(t, err)
	require.NoError(t, database.Close())
}

func initDoctorGitRepository(t *testing.T) string {
	t.Helper()
	repoRoot := t.TempDir()
	command := exec.Command("git", "init", "--quiet")
	command.Dir = repoRoot
	require.NoError(t, command.Run())
	command = exec.Command("git", "config", "user.email", "doctor@example.test")
	command.Dir = repoRoot
	require.NoError(t, command.Run())
	command = exec.Command("git", "config", "user.name", "Doctor Test")
	command.Dir = repoRoot
	require.NoError(t, command.Run())
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "README.md"), []byte("# fixture\n"), 0o600))
	command = exec.Command("git", "add", "README.md")
	command.Dir = repoRoot
	require.NoError(t, command.Run())
	command = exec.Command("git", "commit", "--quiet", "-m", "fixture")
	command.Dir = repoRoot
	require.NoError(t, command.Run())
	return repoRoot
}
