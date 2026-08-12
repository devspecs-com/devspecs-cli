//go:build !windows

package commands

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFindDoctorExecutableCandidates_OnUnix_SkipsNonExecutableFile(t *testing.T) {
	directory := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(directory, "ds"), []byte("binary"), 0o600))

	candidates := findDoctorExecutableCandidates(directory, "", "linux")

	assert.Empty(t, candidates)
}

func TestFindDoctorExecutableCandidates_OnUnix_IncludesExecutableFile(t *testing.T) {
	directory := t.TempDir()
	executablePath := filepath.Join(directory, "ds")
	require.NoError(t, os.WriteFile(executablePath, []byte("binary"), 0o700))

	candidates := findDoctorExecutableCandidates(directory, "", "linux")

	require.Len(t, candidates, 1)
	assert.Equal(t, executablePath, candidates[0])
}

func TestSameDoctorExecutable_OnUnix_ResolvesSymbolicLinks(t *testing.T) {
	directory := t.TempDir()
	executablePath := filepath.Join(directory, "ds-real")
	linkPath := filepath.Join(directory, "ds")
	require.NoError(t, os.WriteFile(executablePath, []byte("binary"), 0o700))
	require.NoError(t, os.Symlink(executablePath, linkPath))

	same := sameDoctorExecutable(executablePath, linkPath, "linux")

	assert.True(t, same)
}
