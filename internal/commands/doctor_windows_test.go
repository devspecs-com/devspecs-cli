//go:build windows

package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFindDoctorExecutableCandidates_OnWindows_UsesPATHEXTOrder(t *testing.T) {
	directory := t.TempDir()
	cmdPath := filepath.Join(directory, "ds.cmd")
	exePath := filepath.Join(directory, "ds.exe")
	require.NoError(t, os.WriteFile(cmdPath, []byte("@echo off\n"), 0o600))
	require.NoError(t, os.WriteFile(exePath, []byte("binary"), 0o600))

	candidates := findDoctorExecutableCandidates(directory, ".CMD;.EXE", "windows")

	require.Len(t, candidates, 2)
	assert.Equal(t, cmdPath, candidates[0])
	assert.Equal(t, exePath, candidates[1])
}

func TestFindDoctorExecutableCandidates_OnWindows_IncludesFilesWithoutUnixExecuteBits(t *testing.T) {
	directory := t.TempDir()
	executablePath := filepath.Join(directory, "ds.exe")
	require.NoError(t, os.WriteFile(executablePath, []byte("binary"), 0o600))

	candidates := findDoctorExecutableCandidates(directory, ".EXE", "windows")

	require.Len(t, candidates, 1)
	assert.Equal(t, executablePath, candidates[0])
}

func TestSameDoctorExecutable_OnWindows_IgnoresPathCase(t *testing.T) {
	executablePath := filepath.Join(t.TempDir(), "ds.exe")
	require.NoError(t, os.WriteFile(executablePath, []byte("binary"), 0o600))

	same := sameDoctorExecutable(executablePath, strings.ToUpper(executablePath), "windows")

	assert.True(t, same)
}
