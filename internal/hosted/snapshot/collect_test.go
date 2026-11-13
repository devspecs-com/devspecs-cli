package snapshot

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const fixturePolicyID = "55555555-5555-4555-8555-555555555555"

func fixtureGit(t *testing.T, dir string, stdin []byte, args ...string) []byte {
	t.Helper()
	options := []string{"-C", dir, "-c", "core.autocrlf=false", "-c", "core.hooksPath=" + os.DevNull, "-c", "user.name=Synthetic Y03", "-c", "user.email=synthetic@example.invalid", "-c", "commit.gpgsign=false"}
	cmd := exec.Command("git", append(options, args...)...)
	cmd.Env = append(gitEnvironment(), "GIT_AUTHOR_DATE=2026-11-13T12:00:00Z", "GIT_COMMITTER_DATE=2026-11-13T12:00:00Z")
	cmd.Stdin = bytes.NewReader(stdin)
	output, err := cmd.Output()
	require.NoError(t, err, "isolated synthetic Git fixture setup failed")
	return output
}

func fixtureRepository(t *testing.T, name string) (string, string) {
	t.Helper()
	dir := t.TempDir()
	fixtureGit(t, dir, nil, "init", "--initial-branch=main", "--template=")
	root, err := filepath.Abs(filepath.Join("testdata", name))
	require.NoError(t, err)
	fixtureGit(t, dir, nil, "--work-tree="+root, "add", "--all", "--", ".")
	fixtureGit(t, dir, nil, "commit", "--quiet", "-m", "synthetic real CLI records")
	fixtureGit(t, dir, nil, "restore", "--source=HEAD", "--worktree", "--", ".")
	return dir, strings.TrimSpace(string(fixtureGit(t, dir, nil, "rev-parse", "HEAD")))
}

func fixturePolicy() Policy {
	return Policy{Schema: "devspecs.hosted.snapshot.policy", SchemaVersion: 1, PolicyID: fixturePolicyID, Roots: []Root{{Label: "planning", Kind: "task_parent", Path: "custom/planning"}, {Label: "decisions", Kind: "adr", Path: "architecture/decisions"}}}
}

func TestCollect_WithRealCLIAndBothApprovedTaskRoots_MatchesServiceSemanticRecords(t *testing.T) {
	dir, commit := fixtureRepository(t, "real-cli")
	policy := fixturePolicy()
	policy.Roots = append(policy.Roots, Root{Label: "normal", Kind: "task_parent", Path: "devspecs/tasks"})

	manifest, err := Collect(t.Context(), dir, "repo-one", policy, commit, "refs/heads/main")

	require.NoError(t, err)
	require.Len(t, manifest.Records, 5)
	assert.Equal(t, "task", manifest.Records[0].Kind)
	assert.Equal(t, "y03-real-fixture", manifest.Records[0].RecordID)
	require.Len(t, manifest.Records[0].Files, 6)
	assert.Equal(t, "1844a167476939bb52b103499893a2bffd45816a", gitObjectID("blob", manifest.Records[0].Files[0].Content, 40))
	assert.Equal(t, "thread", manifest.Records[1].Kind)
	assert.Equal(t, "checkpoint", manifest.Records[2].Kind)
	require.Len(t, manifest.Records[2].Files, 2)
	assert.Equal(t, 3, manifest.Records[2].SchemaVersion)
	assert.Equal(t, "adr", manifest.Records[3].Kind)
	assert.Equal(t, "decisions/0001-verify-committed-record-provenance.md", manifest.Records[3].RecordID)
	assert.Equal(t, "http-normal-fixture", manifest.Records[4].RecordID)
	require.Len(t, manifest.Records[4].Files, 4)
	assertSemanticGolden(t, "real-cli", manifest, "8a1425ea608e6a5aee2da41ac4b5afce50f135b62c0ec542df5dd8e946cdf818")
}

func TestCollect_WithDirtyCRLFAndUntrackedFiles_PreservesOnlyCommittedBytes(t *testing.T) {
	dir, commit := fixtureRepository(t, "real-cli")
	name := "custom/planning/y03-real-fixture/task.json"
	original := fixtureGit(t, dir, nil, "cat-file", "blob", commit+":"+name)
	require.NoError(t, os.WriteFile(filepath.Join(dir, filepath.FromSlash(name)), append(bytes.ReplaceAll(original, []byte("\n"), []byte("\r\n")), []byte("dirty")...), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "custom", "planning", "y03-real-fixture", "untracked.json"), []byte("untracked"), 0o600))

	manifest, err := Collect(t.Context(), dir, "repo-one", fixturePolicy(), commit, "refs/heads/main")

	require.NoError(t, err)
	require.Len(t, manifest.Records, 4)
	require.Len(t, manifest.Records[0].Files, 6)
	assert.Equal(t, string(original), string(manifest.Records[0].Files[0].Content))
	assert.Equal(t, Hash(original), manifest.Records[0].Files[0].SHA256)
}

func TestCollect_WithStartedContinueCheckpoint_MatchesActualCLIProjection(t *testing.T) {
	dir, commit := fixtureRepository(t, "real-continue")
	policy := fixturePolicy()
	policy.Roots = []Root{{Label: "tasks", Kind: "task_parent", Path: "devspecs/tasks"}}

	manifest, err := Collect(t.Context(), dir, "repo-one", policy, commit, "refs/heads/main")

	require.NoError(t, err)
	require.Len(t, manifest.Records, 2)
	assert.Equal(t, "checkpoint", manifest.Records[1].Kind)
	var checkpoint checkpointProjection
	require.NoError(t, json.Unmarshal(manifest.Records[1].Files[0].Content, &checkpoint))
	assert.Equal(t, "started", checkpoint.Stage)
	assert.Equal(t, "continue", checkpoint.Decision)
	assert.Equal(t, "cp_20261113T123704Z_e01_started", checkpoint.CheckpointID)
	assertSemanticGolden(t, "real-continue", manifest, "593a4f956942e54ba96ebafbe30b44c7695489c51f2ad19d9721478972659b57")
}

func TestCollect_WithRealWorkspaceAndChange_DoesNotFollowRepositoryPaths(t *testing.T) {
	dir, commit := fixtureRepository(t, "real-workspace")
	policy := fixturePolicy()
	policy.Roots = []Root{{Label: "workspace", Kind: "workspace_manifest", Path: "devspecs/workspace.yaml"}}

	manifest, err := Collect(t.Context(), dir, "repo-one", policy, commit, "refs/heads/main")

	require.NoError(t, err)
	require.Len(t, manifest.Records, 2)
	assert.Equal(t, "workspace", manifest.Records[0].Kind)
	assert.Equal(t, "y03-cli-workspace-continue-fixture", manifest.Records[0].RecordID)
	require.Len(t, manifest.Records[0].Files, 2)
	assert.Equal(t, "change", manifest.Records[1].Kind)
	assert.Equal(t, "y03-c001", manifest.Records[1].RecordID)
	assert.Equal(t, "workspace", manifest.Records[1].OwnerKind)
	assert.NoDirExists(t, filepath.Join(dir, "app"))
	assertSemanticGolden(t, "real-workspace", manifest, "957147bd256771d0164224180e57a31aff58bb324a73ac30168360797baf0eea")
}

func TestCollect_WithExternalWorkspaceArtifactDir_RefusesWholeSnapshot(t *testing.T) {
	dir, _ := fixtureRepository(t, "real-workspace")
	name := filepath.Join(dir, "devspecs", "workspace.yaml")
	raw, err := os.ReadFile(name)
	require.NoError(t, err)
	require.Contains(t, string(raw), "artifact_dir: devspecs")
	require.NoError(t, os.WriteFile(name, bytes.Replace(raw, []byte("artifact_dir: devspecs"), []byte("artifact_dir: planning"), 1), 0o600))
	fixtureGit(t, dir, nil, "add", "devspecs/workspace.yaml")
	fixtureGit(t, dir, nil, "commit", "--quiet", "-m", "unsupported external workspace root")
	commit := strings.TrimSpace(string(fixtureGit(t, dir, nil, "rev-parse", "HEAD")))
	policy := fixturePolicy()
	policy.Roots = []Root{{Label: "workspace", Kind: "workspace_manifest", Path: "devspecs/workspace.yaml"}}

	_, err = Collect(t.Context(), dir, "repo-one", policy, commit, "refs/heads/main")

	require.ErrorIs(t, err, ErrUnsupported)
}

func TestCollect_WithUnreachableCommit_RefusesProvenance(t *testing.T) {
	dir, _ := fixtureRepository(t, "real-cli")
	tree := strings.TrimSpace(string(fixtureGit(t, dir, nil, "rev-parse", "HEAD^{tree}")))
	orphan := strings.TrimSpace(string(fixtureGit(t, dir, []byte("unreachable\n"), "commit-tree", tree)))

	_, err := Collect(t.Context(), dir, "repo-one", fixturePolicy(), orphan, "refs/heads/main")

	require.ErrorIs(t, err, ErrProvenance)
}

func TestCollect_WithUnknownCommittedTaskMember_RefusesRatherThanDrops(t *testing.T) {
	dir, _ := fixtureRepository(t, "real-cli")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "custom", "planning", "y03-real-fixture", "unknown.json"), []byte("{}\n"), 0o600))
	fixtureGit(t, dir, nil, "add", "custom/planning/y03-real-fixture/unknown.json")
	fixtureGit(t, dir, nil, "commit", "--quiet", "-m", "unsupported committed member")
	commit := strings.TrimSpace(string(fixtureGit(t, dir, nil, "rev-parse", "HEAD")))

	_, err := Collect(t.Context(), dir, "repo-one", fixturePolicy(), commit, "refs/heads/main")

	require.ErrorIs(t, err, ErrUnsupported)
}

func TestCollect_WithChangedSameKeyCheckpointMarkdown_RefusesImmutableConflict(t *testing.T) {
	dir, _ := fixtureRepository(t, "real-cli")
	name := "custom/planning/y03-real-fixture/checkpoints/20261113-115453-validated.md"
	require.NoError(t, os.WriteFile(filepath.Join(dir, filepath.FromSlash(name)), []byte("# Altered immutable evidence\n"), 0o600))
	fixtureGit(t, dir, nil, "add", name)
	fixtureGit(t, dir, nil, "commit", "--quiet", "-m", "changed immutable checkpoint bytes")
	commit := strings.TrimSpace(string(fixtureGit(t, dir, nil, "rev-parse", "HEAD")))

	_, err := Collect(t.Context(), dir, "repo-one", fixturePolicy(), commit, "refs/heads/main")

	require.ErrorIs(t, err, ErrConflict)
}

func TestCollect_WithAmbientGitContext_UsesSelectedPhysicalRepository(t *testing.T) {
	dir, commit := fixtureRepository(t, "real-cli")
	other, _ := fixtureRepository(t, "real-workspace")
	t.Setenv("GIT_DIR", filepath.Join(other, ".git"))
	t.Setenv("GIT_WORK_TREE", other)
	t.Setenv("DEVSPECS_REMOTE_TOKEN", "must-not-enter-git")

	manifest, err := Collect(t.Context(), dir, "repo-one", fixturePolicy(), commit, "refs/heads/main")

	require.NoError(t, err)
	assert.Equal(t, commit, manifest.Commit)
	require.Len(t, manifest.Records, 4)
}

func TestCollect_WithSymlinkInsideApprovedRoot_RefusesProvenance(t *testing.T) {
	dir, _ := fixtureRepository(t, "real-cli")
	blob := strings.TrimSpace(string(fixtureGit(t, dir, []byte("task.json"), "hash-object", "-w", "--stdin")))
	fixtureGit(t, dir, nil, "update-index", "--add", "--cacheinfo", "120000", blob, "custom/planning/y03-real-fixture/link.md")
	fixtureGit(t, dir, nil, "commit", "--quiet", "-m", "unsafe symlink")
	commit := strings.TrimSpace(string(fixtureGit(t, dir, nil, "rev-parse", "HEAD")))

	_, err := Collect(t.Context(), dir, "repo-one", fixturePolicy(), commit, "refs/heads/main")

	require.ErrorIs(t, err, ErrProvenance)
}

func TestCollect_WithOversizedCommittedFile_RefusesWholeSnapshot(t *testing.T) {
	dir, _ := fixtureRepository(t, "real-cli")
	name := "custom/planning/y03-real-fixture/Q01-verify-approved-committed-record-bytes-plan.md"
	require.NoError(t, os.WriteFile(filepath.Join(dir, filepath.FromSlash(name)), bytes.Repeat([]byte("x"), MaxFileBytes+1), 0o600))
	fixtureGit(t, dir, nil, "add", name)
	fixtureGit(t, dir, nil, "commit", "--quiet", "-m", "oversized committed plan")
	commit := strings.TrimSpace(string(fixtureGit(t, dir, nil, "rev-parse", "HEAD")))

	_, err := Collect(t.Context(), dir, "repo-one", fixturePolicy(), commit, "refs/heads/main")

	require.ErrorIs(t, err, ErrProvenance)
}

func TestCollect_WithCancelledContext_ReturnsCancellation(t *testing.T) {
	dir, commit := fixtureRepository(t, "real-cli")
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := Collect(ctx, dir, "repo-one", fixturePolicy(), commit, "refs/heads/main")

	require.ErrorIs(t, err, context.Canceled)
}

func TestGitEnvironment_WithAmbientCredentialsAndGitOverrides_ExcludesThem(t *testing.T) {
	t.Setenv("DEVSPECS_REMOTE_TOKEN", "caller-secret-canary")
	t.Setenv("GH_TOKEN", "github-secret-canary")
	t.Setenv("GIT_DIR", "untrusted-git-context")
	t.Setenv("GIT_CONFIG_COUNT", "1")
	t.Setenv("GIT_CONFIG_KEY_0", "core.fsmonitor")
	t.Setenv("GIT_CONFIG_VALUE_0", "untrusted-monitor")

	environment := strings.Join(gitEnvironment(), "\n")

	assert.NotContains(t, environment, "caller-secret-canary")
	assert.NotContains(t, environment, "github-secret-canary")
	assert.NotContains(t, environment, "untrusted-git-context")
	assert.NotContains(t, environment, "untrusted-monitor")
	assert.Contains(t, environment, "GIT_CONFIG_COUNT=0")
	assert.Contains(t, environment, "GIT_NO_LAZY_FETCH=1")
}

func TestCollect_WithLocalMonitorAndSmudgeCommands_DoesNotRunThemOrMutateSource(t *testing.T) {
	dir, commit := fixtureRepository(t, "real-cli")
	marker := filepath.Join(dir, ".git", "executed-canary")
	script := filepath.Join(dir, ".git", "unsafe-command.sh")
	require.NoError(t, os.WriteFile(script, []byte("#!/bin/sh\nprintf '%s' \"$DEVSPECS_REMOTE_TOKEN\" > '"+filepath.ToSlash(marker)+"'\n"), 0o600))
	command := "sh '" + filepath.ToSlash(script) + "'"
	fixtureGit(t, dir, nil, "config", "core.fsmonitor", command)
	fixtureGit(t, dir, nil, "config", "filter.canary.smudge", command)
	fixtureGit(t, dir, nil, "config", "filter.canary.required", "true")
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".git", "info"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".git", "info", "attributes"), []byte("*.md filter=canary\n"), 0o600))
	indexBefore, err := os.ReadFile(filepath.Join(dir, ".git", "index"))
	require.NoError(t, err)
	refBefore, err := os.ReadFile(filepath.Join(dir, ".git", "refs", "heads", "main"))
	require.NoError(t, err)
	t.Setenv("DEVSPECS_REMOTE_TOKEN", "must-not-enter-source-process")

	manifest, err := Collect(t.Context(), dir, "repo-one", fixturePolicy(), commit, "refs/heads/main")

	require.NoError(t, err)
	assert.Equal(t, commit, manifest.Commit)
	assert.NoFileExists(t, marker)
	indexAfter, readErr := os.ReadFile(filepath.Join(dir, ".git", "index"))
	require.NoError(t, readErr)
	assert.Equal(t, Hash(indexBefore), Hash(indexAfter))
	refAfter, readErr := os.ReadFile(filepath.Join(dir, ".git", "refs", "heads", "main"))
	require.NoError(t, readErr)
	assert.Equal(t, string(refBefore), string(refAfter))
}

func TestCollect_WithExplicitCustomWorkspaceRoot_MatchesIndependentServiceGolden(t *testing.T) {
	dir, commit := fixtureRepository(t, "real-workspace-custom")
	policy := fixturePolicy()
	policy.Roots = []Root{{Label: "coordination", Kind: "workspace_manifest", Path: "devspecs/workspace.yaml", ArtifactRoot: "planning"}}

	manifest, err := Collect(t.Context(), dir, "repo-one", policy, commit, "refs/heads/main")

	require.NoError(t, err)
	require.Len(t, manifest.Records, 2)
	assert.Equal(t, "workspace", manifest.Records[0].Kind)
	assert.Equal(t, "y03-cli-workspace-custom-fixture", manifest.Records[0].RecordID)
	assert.Equal(t, "change", manifest.Records[1].Kind)
	assertSemanticGolden(t, "real-workspace-custom", manifest, "cf3f89c91ec4cf2a316ae01e764ce5c663efed3cb8670f95bf8b91a1497d6a62")
}

func TestCollect_WithMismatchedExplicitArtifactRoot_RefusesRatherThanExpands(t *testing.T) {
	dir, commit := fixtureRepository(t, "real-workspace-custom")
	policy := fixturePolicy()
	policy.Roots = []Root{{Label: "coordination", Kind: "workspace_manifest", Path: "devspecs/workspace.yaml", ArtifactRoot: "unapproved"}}

	_, err := Collect(t.Context(), dir, "repo-one", policy, commit, "refs/heads/main")

	require.ErrorIs(t, err, ErrProvenance)
}

func TestCollect_WithZeroByteDeclaredPlan_PreservesEmptyCommittedBytes(t *testing.T) {
	dir, _ := fixtureRepository(t, "real-cli")
	name := "custom/planning/y03-real-fixture/Q01-verify-approved-committed-record-bytes-plan.md"
	require.NoError(t, os.WriteFile(filepath.Join(dir, filepath.FromSlash(name)), []byte{}, 0o600))
	fixtureGit(t, dir, nil, "add", name)
	fixtureGit(t, dir, nil, "commit", "--quiet", "-m", "legitimate empty declared plan")
	commit := strings.TrimSpace(string(fixtureGit(t, dir, nil, "rev-parse", "HEAD")))

	manifest, err := Collect(t.Context(), dir, "repo-one", fixturePolicy(), commit, "refs/heads/main")

	require.NoError(t, err)
	file := observedFile(t, manifest, name)
	require.NotNil(t, file.Content)
	assert.Empty(t, file.Content)
	assert.Equal(t, "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", file.SHA256)
	raw, encodeErr := Canonical(manifest)
	require.NoError(t, encodeErr)
	assert.Contains(t, string(raw), `"content_base64":""`)
}

func observedFile(t *testing.T, manifest Snapshot, name string) File {
	t.Helper()
	for _, record := range manifest.Records {
		for _, file := range record.Files {
			if file.Path == name {
				return file
			}
		}
	}
	require.FailNow(t, "expected committed file was not collected")
	return File{}
}

func assertSemanticGolden(t *testing.T, name string, manifest Snapshot, expectedSHA string) {
	t.Helper()
	manifest.Commit = strings.Repeat("1", 40)
	actual, err := Canonical(manifest)
	require.NoError(t, err)
	golden, err := os.ReadFile(filepath.Join("testdata", "semantic-goldens", name+".snapshot.json"))
	require.NoError(t, err)
	assert.Equal(t, expectedSHA, Hash(actual))
	assert.Equal(t, strings.TrimSuffix(string(golden), "\n"), string(actual))
}
