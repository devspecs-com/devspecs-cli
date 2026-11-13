package orchestration

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHTTPDriverBundleFinalize_WithVerifiedNodeResultAndReport_RetainsTransferredEvidence(t *testing.T) {
	f := newHTTPDriverBundleFixture(t)
	f.requireNodeEvidence(t)
	f.base.arrangePending(t, true)

	receipt, path, err := f.base.service.Finalize(t.Context(), testHTTPDispatchID)

	require.NoError(t, err)
	assert.FileExists(t, path)
	require.NotNil(t, receipt.Repository.ResultCommit)
	assert.Equal(t, f.result, *receipt.Repository.ResultCommit)
	assert.Equal(t, "passed", receipt.Verification.State)
	assert.Equal(t, "suite", receipt.Verification.Strength)
	assert.Equal(t, "produced", receipt.Report.State)
	require.NotNil(t, receipt.Report.Artifact)
	require.NotNil(t, receipt.Verification.Artifact)
	assert.FileExists(t, receipt.Report.Artifact.Value)
	assert.FileExists(t, receipt.Verification.Artifact.Value)
	require.Len(t, receipt.Artifacts, 1)
	raw, readErr := os.ReadFile(receipt.Artifacts[0].Value)
	require.NoError(t, readErr)
	assert.Equal(t, string(f.bundle), string(raw))
	assert.EqualValues(t, 1, f.downloads.Load())
}

func TestHTTPDriverBundleFinalize_WithEditedResult_DoesNotMutateCallerObjectsRefsIndexOrWorktree(t *testing.T) {
	f := newHTTPDriverBundleFixture(t)
	f.base.arrangePending(t, true)
	refs := bundleFixtureGit(t, f.base.checkout, "show-ref")
	objects := bundleFixtureGit(t, f.base.checkout, "count-objects", "-v")
	index := bundleFixtureGit(t, f.base.checkout, "hash-object", "--no-filters", ".git/index")

	_, _, err := f.base.service.Finalize(t.Context(), testHTTPDispatchID)

	require.NoError(t, err)
	assert.Equal(t, refs, bundleFixtureGit(t, f.base.checkout, "show-ref"))
	assert.Equal(t, objects, bundleFixtureGit(t, f.base.checkout, "count-objects", "-v"))
	assert.Equal(t, index, bundleFixtureGit(t, f.base.checkout, "hash-object", "--no-filters", ".git/index"))
	assert.Equal(t, f.base.profile.SourceCommit, bundleFixtureGit(t, f.base.checkout, "rev-parse", "HEAD"))
	assert.Empty(t, bundleFixtureGit(t, f.base.checkout, "status", "--porcelain=v1", "--untracked-files=all"))
	missing, gitErr := (ExecRunner{}).Run(t.Context(), f.base.checkout, "git", "cat-file", "-e", f.result)
	require.NoError(t, gitErr)
	assert.NotZero(t, missing.ExitCode)
}

func TestHTTPDriverBundleFinalize_AfterLostPostCrashAndCredentialRotation_ReplaysOriginalBytes(t *testing.T) {
	f := newHTTPDriverBundleFixture(t)
	original := f.base.arrangePending(t, false)
	f.base.profile.CredentialID = "credential_rotated"
	f.base.writeProfile(t, f.base.profilePath)
	t.Setenv("HTTP_DRIVER_FIXTURE_TOKEN", "rotated-synthetic-token")

	_, _, err := f.base.service.Finalize(t.Context(), testHTTPDispatchID)

	require.NoError(t, err)
	require.Len(t, f.base.posts, 1)
	assert.Equal(t, string(original), string(<-f.base.posts))
	assert.Equal(t, "Bearer rotated-synthetic-token", f.base.lastAuthorization())
	assert.EqualValues(t, 1, f.downloads.Load())
}

func TestHTTPDriverBundleFinalize_WithInvalidResultTree_RefusesReceipt(t *testing.T) {
	f := newHTTPDriverBundleFixture(t)
	f.reportedTree = strings.Repeat("a", 40)
	f.base.arrangePending(t, true)

	_, _, err := f.base.service.Finalize(t.Context(), testHTTPDispatchID)

	require.ErrorIs(t, err, errHTTPDriverBundle)
	assert.NoFileExists(t, f.base.service.Store.ReceiptPath(testHTTPDispatchID))
}

func TestHTTPDriverBundleFinalize_WithNonDirectParent_RefusesReceipt(t *testing.T) {
	f := newHTTPDriverBundleFixture(t)
	f.commitFile(t, "extra.txt", "second result commit\n")
	f.rebundle(t)
	f.edited = []string{"extra.txt", "src/add.cjs"}
	f.base.arrangePending(t, true)

	_, _, err := f.base.service.Finalize(t.Context(), testHTTPDispatchID)

	require.ErrorIs(t, err, errHTTPDriverBundle)
	assert.NoFileExists(t, f.base.service.Store.ReceiptPath(testHTTPDispatchID))
}

func TestHTTPDriverBundleFinalize_WithExtraHead_RefusesBeforeGitImport(t *testing.T) {
	f := newHTTPDriverBundleFixture(t)
	end := bytes.Index(f.bundle, []byte("\n\n"))
	require.Greater(t, end, 0)
	f.bundle = append(append(append([]byte(nil), f.bundle[:end]...), []byte("\n"+f.result+" refs/heads/extra")...), f.bundle[end:]...)
	f.base.arrangePending(t, true)

	_, _, err := f.base.service.Finalize(t.Context(), testHTTPDispatchID)

	require.ErrorIs(t, err, errHTTPDriverBundle)
	assert.NoFileExists(t, f.base.service.Store.ReceiptPath(testHTTPDispatchID))
}

func TestHTTPDriverBundleFinalize_WithExtraPrerequisite_RefusesBeforeGitImport(t *testing.T) {
	f := newHTTPDriverBundleFixture(t)
	first := bytes.IndexByte(f.bundle, '\n') + 1
	f.bundle = append(append(append([]byte(nil), f.bundle[:first]...), []byte("-"+f.base.profile.SourceCommit+" duplicate\n")...), f.bundle[first:]...)
	f.base.arrangePending(t, true)

	_, _, err := f.base.service.Finalize(t.Context(), testHTTPDispatchID)

	require.ErrorIs(t, err, errHTTPDriverBundle)
	assert.NoFileExists(t, f.base.service.Store.ReceiptPath(testHTTPDispatchID))
}

func TestHTTPDriverBundleFinalize_WithWrongPrerequisite_RefusesBeforeGitImport(t *testing.T) {
	f := newHTTPDriverBundleFixture(t)
	f.bundle = bytes.Replace(f.bundle, []byte("-"+f.base.profile.SourceCommit+" "), []byte("-"+strings.Repeat("b", 40)+" "), 1)
	f.base.arrangePending(t, true)

	_, _, err := f.base.service.Finalize(t.Context(), testHTTPDispatchID)

	require.ErrorIs(t, err, errHTTPDriverBundle)
	assert.NoFileExists(t, f.base.service.Store.ReceiptPath(testHTTPDispatchID))
}

func TestHTTPDriverBundleFinalize_WithDigestMismatch_RefusesBeforeGitImport(t *testing.T) {
	f := newHTTPDriverBundleFixture(t)
	f.digestOverride = "sha256:" + strings.Repeat("0", 64)
	f.base.arrangePending(t, true)

	_, _, err := f.base.service.Finalize(t.Context(), testHTTPDispatchID)

	require.ErrorIs(t, err, errHTTPDriverBundle)
	assert.NoFileExists(t, f.base.service.Store.ReceiptPath(testHTTPDispatchID))
}

func TestHTTPDriverBundleFinalize_WithSizeMismatch_RefusesBeforeGitImport(t *testing.T) {
	f := newHTTPDriverBundleFixture(t)
	f.sizeOverride = int64(len(f.bundle) + 1)
	f.base.arrangePending(t, true)

	_, _, err := f.base.service.Finalize(t.Context(), testHTTPDispatchID)

	require.ErrorIs(t, err, errHTTPDriverBundle)
	assert.NoFileExists(t, f.base.service.Store.ReceiptPath(testHTTPDispatchID))
}

func TestHTTPDriverBundleFinalize_WithCorruptedPackAndMatchingDigest_RefusesGitImport(t *testing.T) {
	f := newHTTPDriverBundleFixture(t)
	f.bundle[len(f.bundle)-1] ^= 0xff
	f.base.arrangePending(t, true)

	_, _, err := f.base.service.Finalize(t.Context(), testHTTPDispatchID)

	require.ErrorIs(t, err, errHTTPDriverBundle)
	assert.NoFileExists(t, f.base.service.Store.ReceiptPath(testHTTPDispatchID))
}

func TestHTTPDriverBundleFinalize_WithOutsideArtifactURL_DoesNotForwardCredentials(t *testing.T) {
	f := newHTTPDriverBundleFixture(t)
	f.urlOverride = "https://outside.invalid/result.bundle"
	f.base.arrangePending(t, true)

	_, _, err := f.base.service.Finalize(t.Context(), testHTTPDispatchID)

	require.ErrorIs(t, err, errHTTPDriverBundle)
	assert.Zero(t, f.downloads.Load())
	assert.NotContains(t, err.Error(), f.urlOverride)
}

func TestHTTPDriverBundleFinalize_WithRedirect_DoesNotFollowArtifactLocation(t *testing.T) {
	f := newHTTPDriverBundleFixture(t)
	f.redirect = "https://outside.invalid/private-" + testHTTPDispatchToken
	f.base.arrangePending(t, true)

	_, _, err := f.base.service.Finalize(t.Context(), testHTTPDispatchID)

	var statusErr *httpDispatchError
	require.ErrorAs(t, err, &statusErr)
	assert.Equal(t, http.StatusTemporaryRedirect, statusErr.Status)
	assert.EqualValues(t, 1, f.downloads.Load())
	assert.NotContains(t, err.Error(), testHTTPDispatchToken)
	assert.NotContains(t, err.Error(), "outside.invalid")
}

func TestHTTPDriverBundleFinalize_WithExpiredArtifact_PreservesAcceptedStateWithoutPost(t *testing.T) {
	f := newHTTPDriverBundleFixture(t)
	f.status = http.StatusGone
	f.base.arrangePending(t, true)

	_, _, err := f.base.service.Finalize(t.Context(), testHTTPDispatchID)

	var statusErr *httpDispatchError
	require.ErrorAs(t, err, &statusErr)
	assert.Equal(t, http.StatusGone, statusErr.Status)
	assert.Empty(t, f.base.posts)
	assert.FileExists(t, filepath.Join(f.base.service.Store.Root, httpDriverStateDir, testHTTPDispatchID+".accepted.json"))
	assert.NoFileExists(t, f.base.service.Store.ReceiptPath(testHTTPDispatchID))
}

func TestHTTPDriverBundleFinalize_WithProtectedEdit_RefusesReceipt(t *testing.T) {
	f := newHTTPDriverBundleFixture(t)
	f.amendFile(t, "devspecs/tasks/forbidden.md", "protected\n")
	f.rebundle(t)
	f.edited = []string{"devspecs/tasks/forbidden.md", "src/add.cjs"}
	f.base.arrangePending(t, true)

	_, _, err := f.base.service.Finalize(t.Context(), testHTTPDispatchID)

	require.ErrorIs(t, err, errHTTPDispatchTask)
	assert.NoFileExists(t, f.base.service.Store.ReceiptPath(testHTTPDispatchID))
}

func TestHTTPDriverBundleFinalize_WithMissingEditedPath_RefusesReceipt(t *testing.T) {
	f := newHTTPDriverBundleFixture(t)
	f.edited = []string{}
	f.base.arrangePending(t, true)

	_, _, err := f.base.service.Finalize(t.Context(), testHTTPDispatchID)

	require.ErrorIs(t, err, errHTTPDispatchPaths)
	assert.NoFileExists(t, f.base.service.Store.ReceiptPath(testHTTPDispatchID))
}

func TestHTTPDriverBundleFinalize_WithExtraEditedPath_RefusesReceipt(t *testing.T) {
	f := newHTTPDriverBundleFixture(t)
	f.edited = []string{"src/add.cjs", "not-changed.txt"}
	f.base.arrangePending(t, true)

	_, _, err := f.base.service.Finalize(t.Context(), testHTTPDispatchID)

	require.ErrorIs(t, err, errHTTPDispatchPaths)
	assert.NoFileExists(t, f.base.service.Store.ReceiptPath(testHTTPDispatchID))
}

func TestHTTPDriverBundleFinalize_WithReadOnlyFrozenMode_RefusesEditedTransfer(t *testing.T) {
	f := newHTTPDriverBundleFixture(t)
	f.base.profile.AllowEdits = false
	f.base.profile.ReadOnly = true
	f.base.writeProfile(t, f.base.profilePath)
	f.base.arrangePending(t, true)

	_, _, err := f.base.service.Finalize(t.Context(), testHTTPDispatchID)

	require.ErrorIs(t, err, errHTTPDriverEdited)
	assert.Zero(t, f.downloads.Load())
}

func TestHTTPDriverBundleFinalize_WithRequiredReportMissing_RefusesBeforeDownload(t *testing.T) {
	f := newHTTPDriverBundleFixture(t)
	f.base.profile.ReportRequired = true
	f.base.writeProfile(t, f.base.profilePath)
	f.base.arrangePending(t, true)

	_, _, err := f.base.service.Finalize(t.Context(), testHTTPDispatchID)

	require.ErrorIs(t, err, errHTTPDriverEvidence)
	assert.Zero(t, f.downloads.Load())
}

func TestHTTPDriverBundleFinalize_WithRequiredVerificationMissing_RefusesBeforeDownload(t *testing.T) {
	f := newHTTPDriverBundleFixture(t)
	f.base.profile.Verification = &httpDispatchVerificationProfile{Profile: "node-unit", Strength: "suite"}
	f.base.writeProfile(t, f.base.profilePath)
	f.base.arrangePending(t, true)

	_, _, err := f.base.service.Finalize(t.Context(), testHTTPDispatchID)

	require.ErrorIs(t, err, errHTTPDriverEvidence)
	assert.Zero(t, f.downloads.Load())
}

func TestHTTPDriverBundleFinalize_WithSymlinkChange_RefusesUnsafeMode(t *testing.T) {
	f := newHTTPDriverBundleFixture(t)
	blob := bundleFixtureGit(t, f.provider, "rev-parse", f.result+":README.md")
	bundleFixtureGit(t, f.provider, "update-index", "--add", "--cacheinfo", "120000", blob, "unsafe-link")
	bundleFixtureGit(t, f.provider, "-c", "core.hooksPath=.git/hooks", "-c", "commit.gpgsign=false", "commit", "--amend", "--no-edit")
	f.rebundle(t)
	f.edited = []string{"src/add.cjs", "unsafe-link"}
	f.base.arrangePending(t, true)

	_, _, err := f.base.service.Finalize(t.Context(), testHTTPDispatchID)

	require.ErrorIs(t, err, errHTTPDispatchPaths)
	assert.NoFileExists(t, f.base.service.Store.ReceiptPath(testHTTPDispatchID))
}

func TestHTTPDriverBundleFinalize_WithGitlinkChange_RefusesUnsafeMode(t *testing.T) {
	f := newHTTPDriverBundleFixture(t)
	bundleFixtureGit(t, f.provider, "update-index", "--add", "--cacheinfo", "160000", f.base.profile.SourceCommit, "unsafe-module")
	bundleFixtureGit(t, f.provider, "-c", "core.hooksPath=.git/hooks", "-c", "commit.gpgsign=false", "commit", "--amend", "--no-edit")
	f.rebundle(t)
	f.edited = []string{"src/add.cjs", "unsafe-module"}
	f.base.arrangePending(t, true)

	_, _, err := f.base.service.Finalize(t.Context(), testHTTPDispatchID)

	require.ErrorIs(t, err, errHTTPDispatchPaths)
	assert.NoFileExists(t, f.base.service.Store.ReceiptPath(testHTTPDispatchID))
}

func TestHTTPDriverBundleFinalize_WithOrdinaryRename_RequiresBothRawChangedPaths(t *testing.T) {
	f := newHTTPDriverBundleFixture(t)
	bundleFixtureGit(t, f.provider, "mv", "README.md", "OVERVIEW.md")
	bundleFixtureGit(t, f.provider, "-c", "core.hooksPath=.git/hooks", "-c", "commit.gpgsign=false", "commit", "--amend", "--no-edit")
	f.rebundle(t)
	f.edited = []string{"README.md", "OVERVIEW.md", "src/add.cjs"}
	f.base.arrangePending(t, true)

	_, _, err := f.base.service.Finalize(t.Context(), testHTTPDispatchID)

	require.NoError(t, err)
}

func TestHTTPDriverBundleFinalize_WithCollapsedRenameClaim_RefusesMissingOldPath(t *testing.T) {
	f := newHTTPDriverBundleFixture(t)
	bundleFixtureGit(t, f.provider, "mv", "README.md", "OVERVIEW.md")
	bundleFixtureGit(t, f.provider, "-c", "core.hooksPath=.git/hooks", "-c", "commit.gpgsign=false", "commit", "--amend", "--no-edit")
	f.rebundle(t)
	f.edited = []string{"OVERVIEW.md", "src/add.cjs"}
	f.base.arrangePending(t, true)

	_, _, err := f.base.service.Finalize(t.Context(), testHTTPDispatchID)

	require.ErrorIs(t, err, errHTTPDispatchPaths)
}

func TestHTTPDriverBundleFinalize_WithTrailingPackGarbage_RefusesReceipt(t *testing.T) {
	f := newHTTPDriverBundleFixture(t)
	f.bundle = append(f.bundle, []byte("trailing unverified bytes")...)
	f.base.arrangePending(t, true)

	_, _, err := f.base.service.Finalize(t.Context(), testHTTPDispatchID)

	require.ErrorIs(t, err, errHTTPDriverBundle)
	assert.NoFileExists(t, f.base.service.Store.ReceiptPath(testHTTPDispatchID))
}

func TestHTTPDriverBundleFinalize_WithOversizedDeclaredArtifact_RefusesBeforeDownload(t *testing.T) {
	f := newHTTPDriverBundleFixture(t)
	f.sizeOverride = httpDriverBundleLimit + 1
	f.base.arrangePending(t, true)

	_, _, err := f.base.service.Finalize(t.Context(), testHTTPDispatchID)

	require.ErrorIs(t, err, errHTTPDispatchResponse)
	assert.Zero(t, f.downloads.Load())
}

func TestHTTPDriverBundleFinalize_WithTooManyPackedObjects_RefusesBeforeGitImport(t *testing.T) {
	f := newHTTPDriverBundleFixture(t)
	pack := bytes.Index(f.bundle, []byte("\n\n")) + 2
	require.Greater(t, pack, 1)
	binary.BigEndian.PutUint32(f.bundle[pack+8:pack+12], httpDriverPackObjectLimit+1)
	f.base.arrangePending(t, true)

	_, _, err := f.base.service.Finalize(t.Context(), testHTTPDispatchID)

	require.ErrorIs(t, err, errHTTPDriverBundle)
	assert.NoFileExists(t, f.base.service.Store.ReceiptPath(testHTTPDispatchID))
}

func TestHTTPDriverBundleFinalize_WithChangedRetainedReceipt_RefusesBeforeDownload(t *testing.T) {
	f := newHTTPDriverBundleFixture(t)
	f.base.arrangePending(t, true)
	d := &httpDriver{store: f.base.service.Store}
	var p httpDriverPending
	require.NoError(t, d.readRecord(testHTTPDispatchID, ".pending.json", httpDriverPendingLimit, &p))
	require.NoError(t, d.writeRecord(testHTTPDispatchID, ".receipt-binding.json", struct {
		RequestSHA256 string `json:"request_sha256"`
		WireSHA256    string `json:"wire_sha256"`
	}{p.Digest, httpDispatchDigest([]byte("different retained receipt"))}))

	_, _, err := f.base.service.Finalize(t.Context(), testHTTPDispatchID)

	require.ErrorIs(t, err, errHTTPDriverState)
	assert.Zero(t, f.downloads.Load())
}

func TestHTTPDriverBundleFinalize_WithChangedModeOnResume_RefusesBeforeAuthentication(t *testing.T) {
	f := newHTTPDriverBundleFixture(t)
	f.base.arrangePending(t, true)
	f.base.profile.ReadOnly, f.base.profile.AllowEdits = true, false
	f.base.writeProfile(t, f.base.profilePath)

	_, _, err := f.base.service.Finalize(t.Context(), testHTTPDispatchID)

	require.ErrorIs(t, err, errHTTPDriverBinding)
	assert.Zero(t, f.base.calls.Load())
}

func TestHTTPDriverBundleFinalize_WithOtherPrincipal_RefusesBeforeAuthentication(t *testing.T) {
	f := newHTTPDriverBundleFixture(t)
	f.base.arrangePending(t, true)
	f.base.profile.PrincipalID = "principal_other"
	f.base.writeProfile(t, f.base.profilePath)

	_, _, err := f.base.service.Finalize(t.Context(), testHTTPDispatchID)

	require.ErrorIs(t, err, errHTTPDriverBinding)
	assert.Zero(t, f.base.calls.Load())
}

func TestHTTPDriverBundleFinalize_WithSourceAlternates_RefusesSourceObjectExport(t *testing.T) {
	f := newHTTPDriverBundleFixture(t)
	f.base.arrangePending(t, true)
	require.NoError(t, os.WriteFile(filepath.Join(f.base.checkout, ".git", "objects", "info", "alternates"), []byte(t.TempDir()+"\n"), 0o600))

	_, _, err := f.base.service.Finalize(t.Context(), testHTTPDispatchID)

	require.ErrorIs(t, err, errHTTPDriverBundle)
	assert.NoFileExists(t, f.base.service.Store.ReceiptPath(testHTTPDispatchID))
}

func TestHTTPDriverQuarantine_WithHostileAmbientGitConfig_ExportsOnlyPinnedSource(t *testing.T) {
	f := newHTTPDriverBundleFixture(t)
	stage := t.TempDir()
	t.Setenv("GIT_DIR", filepath.Join(stage, "not-the-repository"))
	t.Setenv("GIT_CONFIG_COUNT", "1")
	t.Setenv("GIT_CONFIG_KEY_0", "core.hooksPath")
	t.Setenv("GIT_CONFIG_VALUE_0", f.provider)
	t.Setenv("GIT_OBJECT_DIRECTORY", filepath.Join(stage, "not-the-objects"))
	t.Setenv("GIT_ALTERNATE_OBJECT_DIRECTORIES", filepath.Join(f.provider, ".git", "objects"))
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(stage, "missing-global"))

	runner, quarantine, err := newHTTPDriverQuarantine(t.Context(), stage, f.base.checkout, f.base.profile.SourceCommit)

	require.NoError(t, err)
	actual, gitErr := runner.checked(t.Context(), quarantine, "rev-parse", "refs/heads/source")
	require.NoError(t, gitErr)
	assert.Equal(t, f.base.profile.SourceCommit, strings.TrimSpace(string(actual)))
	missing, gitErr := runner.Run(t.Context(), quarantine, "git", "cat-file", "-e", f.result)
	require.NoError(t, gitErr)
	assert.NotZero(t, missing.ExitCode)
}

func TestHTTPDriverBundleDispatch_WithEditedGoal_FreezesProviderOwnedRequirements(t *testing.T) {
	f := newHTTPDriverBundleFixture(t)
	f.requireNodeEvidence(t)

	handle, _, err := f.base.service.Dispatch(t.Context(), f.base.options())

	require.NoError(t, err)
	assert.Equal(t, "http-v2-bundle-1", handle.Provider.AdapterVersion)
	require.Len(t, f.base.posts, 1)
	var request httpDispatchPortableRequest
	require.NoError(t, json.Unmarshal(<-f.base.posts, &request))
	assert.True(t, request.Requirements.Report.Required)
	assert.True(t, request.Requirements.Verification.Required)
	assert.Equal(t, "node-unit", request.Requirements.Verification.Profile)
	assert.Equal(t, "suite", request.Requirements.Verification.Strength)
	assert.NotContains(t, request.Input.ApplyPayload, "HTTP experimental read-only goal")
}

type httpDriverBundleFixture struct {
	base                                  *httpDriverFixture
	provider, result, tree, reportedTree  string
	bundle                                []byte
	edited                                []string
	downloads                             atomic.Int32
	urlOverride, digestOverride, redirect string
	sizeOverride                          int64
	status                                int
	report, testOutput                    []byte
}

func newHTTPDriverBundleFixture(t *testing.T) *httpDriverBundleFixture {
	t.Helper()
	f := &httpDriverBundleFixture{base: newHTTPDriverFixture(t), edited: []string{"src/add.cjs"}}
	writeOrchestrationConfig(t, f.base.checkout, "http", map[string]any{"goal": "edited"})
	require.NoError(t, os.MkdirAll(filepath.Join(f.base.checkout, "src"), 0o700))
	require.NoError(t, os.MkdirAll(filepath.Join(f.base.checkout, "test"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(f.base.checkout, "src", "add.cjs"), []byte("module.exports = (a, b) => a - b;\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(f.base.checkout, "test", "add.test.cjs"), []byte("const test = require('node:test'); const assert = require('node:assert/strict'); const add = require('../src/add.cjs'); test('adds', () => assert.equal(add(2, 3), 5));\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(f.base.checkout, "README.md"), []byte("Synthetic bundle fixture.\n"), 0o600))
	bundleFixtureGit(t, f.base.checkout, "add", "-f", ".")
	bundleFixtureGit(t, f.base.checkout, "-c", "core.hooksPath=.git/hooks", "-c", "commit.gpgsign=false", "commit", "-m", "Edited source fixture")
	f.base.profile.SourceCommit = bundleFixtureGit(t, f.base.checkout, "rev-parse", "HEAD")
	f.base.profile.ReadOnly, f.base.profile.AllowEdits = false, true
	f.base.writeProfile(t, f.base.profilePath)
	f.provider = filepath.Join(t.TempDir(), "provider")
	bundleFixtureGit(t, t.TempDir(), "clone", "--no-hardlinks", "--template=", f.base.checkout, f.provider)
	f.commitFile(t, "src/add.cjs", "module.exports = (a, b) => a + b;\n")
	f.rebundle(t)
	f.base.mode = "succeeded"
	f.base.observation = f.observe
	f.base.artifactHandler = f.serveArtifact
	return f
}

func bundleFixtureGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	raw, err := runGitBytes(t.Context(), ExecRunner{}, dir, args...)
	require.NoError(t, err)
	return strings.TrimSpace(string(raw))
}

func (f *httpDriverBundleFixture) commitFile(t *testing.T, name, content string) {
	t.Helper()
	path := filepath.Join(f.provider, filepath.FromSlash(name))
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	bundleFixtureGit(t, f.provider, "add", "-f", "--", name)
	bundleFixtureGit(t, f.provider, "-c", "core.hooksPath=.git/hooks", "-c", "commit.gpgsign=false", "commit", "-m", "Synthetic implementation result")
}

func (f *httpDriverBundleFixture) amendFile(t *testing.T, name, content string) {
	t.Helper()
	path := filepath.Join(f.provider, filepath.FromSlash(name))
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	bundleFixtureGit(t, f.provider, "add", "-f", "--", name)
	bundleFixtureGit(t, f.provider, "-c", "core.hooksPath=.git/hooks", "-c", "commit.gpgsign=false", "commit", "--amend", "--no-edit")
}

func (f *httpDriverBundleFixture) rebundle(t *testing.T) {
	t.Helper()
	f.result = bundleFixtureGit(t, f.provider, "rev-parse", "HEAD")
	f.tree = bundleFixtureGit(t, f.provider, "rev-parse", "HEAD^{tree}")
	f.reportedTree = f.tree
	ref := "refs/dispatch/" + testHTTPDispatchID
	bundleFixtureGit(t, f.provider, "update-ref", ref, f.result)
	path := filepath.Join(t.TempDir(), "result.bundle")
	bundleFixtureGit(t, f.provider, "bundle", "create", "--version=2", path, f.base.profile.SourceCommit+".."+ref)
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	f.bundle = raw
}

func (f *httpDriverBundleFixture) requireNodeEvidence(t *testing.T) {
	t.Helper()
	result, err := (ExecRunner{}).Run(t.Context(), f.provider, "node", "--test", "test/add.test.cjs")
	require.NoError(t, err)
	require.Zero(t, result.ExitCode, string(result.Stderr))
	f.testOutput = result.Stdout
	f.report = []byte("Implemented addition and passed node-unit.\n")
	f.base.profile.ReportRequired = true
	f.base.profile.Verification = &httpDispatchVerificationProfile{Profile: "node-unit", Strength: "suite"}
	f.base.capabilities = strings.ReplaceAll(testHTTPDispatchCaps, "go-unit", "node-unit")
	f.base.writeProfile(t, f.base.profilePath)
}

func inlineBundleFixtureArtifact(id string, content []byte) map[string]any {
	return map[string]any{"artifact_id": id, "kind": "inline", "media_type": "text/plain", "size_bytes": len(content), "sha256": httpDispatchDigest(content), "content_base64": base64.StdEncoding.EncodeToString(content)}
}

func (f *httpDriverBundleFixture) observe(kind string, object map[string]any) {
	if kind != "receipt" {
		return
	}
	repository := object["repository"].(map[string]any)
	repository["result_commit"], repository["result_tree"] = f.result, f.reportedTree
	url := f.base.server.URL + "/v2/dispatches/" + testHTTPDispatchID + "/artifacts/result-bundle"
	if f.urlOverride != "" {
		url = f.urlOverride
	}
	digest := httpDispatchDigest(f.bundle)
	if f.digestOverride != "" {
		digest = f.digestOverride
	}
	size := int64(len(f.bundle))
	if f.sizeOverride != 0 {
		size = f.sizeOverride
	}
	repository["result_bundle"] = httpDriverBundleArtifact{ID: "result-bundle", Kind: "https", MediaType: "application/x-git-bundle", Size: size, SHA256: digest, URL: url}
	object["evidence"].(map[string]any)["files_edited"] = f.edited
	if f.report != nil {
		object["report"] = map[string]any{"state": "produced", "artifact": inlineBundleFixtureArtifact("report", f.report)}
	}
	if f.testOutput != nil {
		object["verification"] = map[string]any{"state": "passed", "strength": "suite", "test_output": inlineBundleFixtureArtifact("tests", f.testOutput)}
	}
}

func (f *httpDriverBundleFixture) serveArtifact(w http.ResponseWriter, r *http.Request) {
	f.downloads.Add(1)
	if f.status != 0 {
		w.WriteHeader(f.status)
		return
	}
	if f.redirect != "" {
		w.Header().Set("Location", f.redirect)
		w.WriteHeader(http.StatusTemporaryRedirect)
		return
	}
	w.Header().Set("Content-Type", "application/x-git-bundle")
	_, _ = w.Write(f.bundle)
}

func TestDecodeHTTPDriverReceipt_WithNoEditExtension_RefusesNewFields(t *testing.T) {
	f := newHTTPDriverBundleFixture(t)
	f.result = f.base.profile.SourceCommit
	f.base.arrangePending(t, true)
	response := newBundleReceiptRecorder(f)

	_, err := decodeHTTPDriverReceipt(response)

	require.ErrorIs(t, err, errHTTPDispatchResponse)
}

func newBundleReceiptRecorder(f *httpDriverBundleFixture) []byte {
	response := httptest.NewRecorder()
	f.base.writeObservation(response, "receipt")
	return response.Body.Bytes()
}
