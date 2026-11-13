//go:build windows

package orchestration

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHTTPDriverServiceDispatch_WithJunctionStateHomeInsideCheckout_RefusesBeforeWritingState(t *testing.T) {
	f := newHTTPDriverFixture(t)
	target := filepath.Join(f.checkout, ".git", "http-state")
	require.NoError(t, os.Mkdir(target, 0o700))
	alias := filepath.Join(t.TempDir(), "outside-state")
	output, err := exec.Command("cmd.exe", "/c", "mklink", "/J", alias, target).CombinedOutput()
	require.NoError(t, err, string(output))
	f.service = NewService(alias)
	f.service.Runner = f.runner

	_, _, err = f.service.Dispatch(t.Context(), f.options())

	require.ErrorIs(t, err, errHTTPDriverBinding)
	assert.Zero(t, f.calls.Load())
	assert.NoDirExists(t, filepath.Join(target, "dispatches"))
}

func TestHTTPDriverServicePreflight_WithJunctionProfileInsideCheckout_RefusesBeforeAuthentication(t *testing.T) {
	f := newHTTPDriverFixture(t)
	target := filepath.Join(f.checkout, ".git", "trusted-profile")
	require.NoError(t, os.Mkdir(target, 0o700))
	f.writeProfile(t, filepath.Join(target, "profile.json"))
	alias := filepath.Join(t.TempDir(), "external-profile")
	output, err := exec.Command("cmd.exe", "/c", "mklink", "/J", alias, target).CombinedOutput()
	require.NoError(t, err, string(output))
	t.Setenv("DEVSPECS_HTTP_DISPATCH_CONFIG", filepath.Join(alias, "profile.json"))

	_, err = f.service.Preflight(t.Context(), f.checkout)

	require.ErrorIs(t, err, errHTTPDriverBinding)
	assert.Zero(t, f.calls.Load())
}

func TestHTTPDriverOutside_WithDifferentExistingVolumes_AcceptsExternalCandidate(t *testing.T) {
	name := t.TempDir()
	root := os.Getenv("SystemDrive") + `\`
	if filepath.VolumeName(name) == filepath.VolumeName(root) {
		t.Skip("cross-volume temporary storage is unavailable")
	}

	outside := httpDriverOutside(name, root)

	assert.True(t, outside)
}
