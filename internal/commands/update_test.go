package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetectInstallSource_WithAppleHomebrewPath_ReturnsHomebrewGuidance(t *testing.T) {
	source, confidence, command, alternatives := detectInstallSource("/opt/homebrew/bin/ds")

	assert.Equal(t, "homebrew", source)
	assert.Equal(t, "medium", confidence)
	assert.Equal(t, "brew update && brew upgrade devspecs-com/tap/devspecs", command)
	assert.Empty(t, alternatives)
}

func TestDetectInstallSource_WithLinuxbrewPath_ReturnsHomebrewGuidance(t *testing.T) {
	source, confidence, command, alternatives := detectInstallSource("/home/linuxbrew/.linuxbrew/bin/ds")

	assert.Equal(t, "homebrew", source)
	assert.Equal(t, "medium", confidence)
	assert.Equal(t, "brew update && brew upgrade devspecs-com/tap/devspecs", command)
	assert.Empty(t, alternatives)
}

func TestDetectInstallSourceUsrLocalBinaryIsManual(t *testing.T) {
	source, confidence, command, _ := detectInstallSource("/usr/local/bin/ds")
	assert.Equal(t, "manual or unknown", source, "source=%q confidence=%q", source, confidence)
	assert.Equal(t, "low", confidence, "source=%q confidence=%q", source, confidence)
	assert.NotContains(t, command, "brew", "unexpected manual update guidance: %q", command)
	assert.Contains(t, command, "install.sh", "unexpected manual update guidance: %q", command)

}

func TestDetectInstallSource_WithScoopAppPath_ReturnsScoopGuidance(t *testing.T) {
	source, _, command, _ := detectInstallSource(`C:\Users\alice\scoop\apps\devspecs\current\ds.exe`)

	assert.Equal(t, "scoop", source)
	assert.Equal(t, "scoop update devspecs", command)
}

func TestDetectInstallSource_WithScoopShimPath_ReturnsScoopGuidance(t *testing.T) {
	source, _, command, _ := detectInstallSource(`C:\Users\alice\scoop\shims\ds.exe`)

	assert.Equal(t, "scoop", source)
	assert.Equal(t, "scoop update devspecs", command)
}

func TestDetectInstallSourceGoInstall(t *testing.T) {
	source, _, command, _ := detectInstallSource(`/home/dev/go/bin/ds`)
	assert.Equal(t, "go install", source,
		"source = %q", source)
	assert.Equal(t, "go install github.com/devspecs-com/devspecs-cli/cmd/ds@latest", command,
		"command = %q", command)

}

func TestUpdateReportManualIncludesSupportedChannels(t *testing.T) {
	report := buildUpdateReport(`/tmp/ds`)
	assert.Equal(t, "manual or unknown", report.InstallSource,
		"install source = %q", report.InstallSource)
	assert.NotEqual(t, "", report.UpdateCommand, "update command = %q", report.UpdateCommand)
	assert.Contains(t, report.UpdateCommand, "install.sh", "update command = %q", report.UpdateCommand)
	assert.True(t, containsUpdateString(report.Alternatives, "scoop update devspecs"),
		"expected scoop alternative, got %#v", report.Alternatives)
	assert.False(t, report.CanApply,
		"expected guidance-only update report")

}

func TestOutputUpdateReportText(t *testing.T) {
	report := updateReport{
		Version:        "v1.1.0",
		Commit:         "abc123",
		Built:          "2026-06-16T00:00:00Z",
		Executable:     "/opt/homebrew/bin/ds",
		InstallSource:  "homebrew",
		Confidence:     "medium",
		Latest:         "not checked",
		UpdateCommand:  "brew update && brew upgrade devspecs-com/tap/devspecs",
		RestartMessage: "restart message",
	}
	cmd := &cobra.Command{}
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	{
		err := outputUpdateReport(cmd, report, false)
		require.NoError(t, err)
	}

	got := buf.String()
	assert.Contains(t, got, "DevSpecs update",
		"output missing %q:\n%s", "DevSpecs update", got)
	assert.Contains(t, got, "Installed version: v1.1.0",
		"output missing %q:\n%s", "Installed version: v1.1.0", got)
	assert.Contains(t, got, "Install source: homebrew",
		"output missing %q:\n%s", "Install source: homebrew", got)
	assert.Contains(t, got, "brew update && brew upgrade devspecs-com/tap/devspecs",
		"output missing %q:\n%s", "brew update && brew upgrade devspecs-com/tap/devspecs", got)
	assert.Contains(t, got, "guidance-only",
		"output missing %q:\n%s", "guidance-only", got)

}

func TestOutputUpdateReportJSON(t *testing.T) {
	report := buildUpdateReport(`/home/dev/go/bin/ds`)
	cmd := &cobra.Command{}
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	{
		err := outputUpdateReport(cmd, report, true)
		require.NoError(t, err)
	}

	var got updateReport
	{
		err := json.Unmarshal(buf.Bytes(), &got)
		require.NoError(t, err,
			"invalid JSON: %v\n%s", err, buf.String())
	}
	assert.Equal(t, "go install", got.InstallSource,
		"install source = %q", got.InstallSource)
	assert.Equal(t, "go install github.com/devspecs-com/devspecs-cli/cmd/ds@latest", got.UpdateCommand,
		"update command = %q", got.UpdateCommand)

}

func TestClassifyVersionStatus_WhenLatestIsNewer_ReturnsStale(t *testing.T) {
	got := classifyVersionStatus("v1.0.0", "v1.0.1")

	assert.Equal(t, "stale", got)
}

func TestClassifyVersionStatus_WhenVersionsMatch_ReturnsCurrent(t *testing.T) {
	got := classifyVersionStatus("v1.0.1", "v1.0.1")

	assert.Equal(t, "current", got)
}

func TestClassifyVersionStatus_WhenCurrentIsNewer_ReturnsCurrent(t *testing.T) {
	got := classifyVersionStatus("v1.1.0", "v1.0.1")

	assert.Equal(t, "current", got)
}

func TestClassifyVersionStatus_WithDevelopmentVersion_ReturnsDevelopment(t *testing.T) {
	got := classifyVersionStatus("dev", "v1.0.1")

	assert.Equal(t, "development", got)
}

func TestClassifyVersionStatus_WithDevelopmentSuffix_ReturnsDevelopment(t *testing.T) {
	got := classifyVersionStatus("v1.1.0-dev", "v1.0.1")

	assert.Equal(t, "development", got)
}

func TestClassifyVersionStatus_WithInvalidCurrentVersion_ReturnsUnknown(t *testing.T) {
	got := classifyVersionStatus("not-a-version", "v1.0.1")

	assert.Equal(t, "unknown", got)
}

func TestClassifyVersionStatus_WithoutLatestVersion_ReturnsUnknown(t *testing.T) {
	got := classifyVersionStatus("v1.0.0", "")

	assert.Equal(t, "unknown", got)
}

func TestEnrichUpdateReportUsesFreshCacheWithoutFetcher(t *testing.T) {
	home := t.TempDir()
	t.Setenv("DEVSPECS_HOME", home)
	now := time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC)
	mustWriteUpdateCheckCache(t, filepath.Join(home, updateCacheFileName), updateCheckCache{
		Latest:    "v1.0.1",
		CheckedAt: now.Add(-time.Hour).Format(time.RFC3339),
		Source:    "github",
	})

	report := buildUpdateReport("/opt/homebrew/bin/ds")
	report.Version = "v1.0.0"
	fetchCalled := false

	enrichUpdateReportWithLatest(context.Background(), &report, updateCheckOptions{
		Enabled: true,
		Now:     now,
		TTL:     updateCacheTTL,
		Fetcher: func(context.Context) (string, error) {
			fetchCalled = true
			return "", nil
		},
	})

	assert.False(t, fetchCalled)
	assert.Equal(t, "v1.0.1", report.Latest)
	assert.Equal(t, "cache", report.LatestSource)
	assert.Equal(t, "stale", report.VersionStatus)
	assert.True(t, report.UpdateAvailable)
}

func TestEnrichUpdateReportFetchesAndCachesLatest(t *testing.T) {
	home := t.TempDir()
	t.Setenv("DEVSPECS_HOME", home)
	now := time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC)

	report := buildUpdateReport("/opt/homebrew/bin/ds")
	report.Version = "v1.0.0"
	enrichUpdateReportWithLatest(context.Background(), &report, updateCheckOptions{
		Enabled: true,
		Now:     now,
		TTL:     updateCacheTTL,
		Fetcher: func(context.Context) (string, error) {
			return "v1.0.1", nil
		},
	})
	assert.Equal(t, "v1.0.1", report.Latest, "latest/source = %q/%q", report.Latest, report.LatestSource)
	assert.Equal(t, "github", report.LatestSource, "latest/source = %q/%q", report.Latest, report.LatestSource)

	cached, ok := readUpdateCheckCache(filepath.Join(home, updateCacheFileName))
	require.True(t, ok,
		"expected cache to be written")
	assert.Equal(t, "v1.0.1", cached.Latest, "cache = %#v", cached)
	assert.Equal(t, now.Format(time.RFC3339), cached.CheckedAt, "cache = %#v", cached)

}

func TestEnrichUpdateReportOfflineGracefulWithoutCache(t *testing.T) {
	t.Setenv("DEVSPECS_HOME", t.TempDir())
	report := buildUpdateReport("/opt/homebrew/bin/ds")
	report.Version = "v1.0.0"
	enrichUpdateReportWithLatest(context.Background(), &report, updateCheckOptions{
		Enabled: true,
		Now:     time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC),
		TTL:     updateCacheTTL,
		Fetcher: func(context.Context) (string, error) {
			return "", errors.New("network unavailable")
		},
	})
	assert.Equal(t, "unknown", report.Latest,
		"latest = %q", report.Latest)
	assert.Equal(t, "unknown", report.VersionStatus,
		"status = %q", report.VersionStatus)
	assert.Contains(t, report.CheckError, "network unavailable",
		"check error = %q", report.CheckError)

}

func TestEnrichUpdateReportUsesStaleCacheAfterFetchFailure(t *testing.T) {
	home := t.TempDir()
	t.Setenv("DEVSPECS_HOME", home)
	now := time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC)
	mustWriteUpdateCheckCache(t, filepath.Join(home, updateCacheFileName), updateCheckCache{
		Latest:    "v1.0.1",
		CheckedAt: now.Add(-48 * time.Hour).Format(time.RFC3339),
		Source:    "github",
	})

	report := buildUpdateReport("/opt/homebrew/bin/ds")
	report.Version = "v1.0.0"
	enrichUpdateReportWithLatest(context.Background(), &report, updateCheckOptions{
		Enabled: true,
		Now:     now,
		TTL:     updateCacheTTL,
		Fetcher: func(context.Context) (string, error) {
			return "", errors.New("offline")
		},
	})
	assert.Equal(t, "v1.0.1", report.Latest,
		"latest = %q", report.Latest)
	assert.Equal(t, "stale cache", report.LatestSource,
		"source = %q", report.LatestSource)
	assert.NotEqual(t, "", report.CheckError,
		"expected check error to explain stale cache fallback")
	assert.Equal(t, "stale", report.VersionStatus,
		"status = %q", report.VersionStatus)

}

func containsUpdateString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func mustWriteUpdateCheckCache(t *testing.T, path string, cached updateCheckCache) {
	t.Helper()
	data, err := json.Marshal(cached)
	require.NoError(t, err)
	{

		err := os.MkdirAll(filepath.Dir(path), 0o755)
		require.NoError(t, err)
	}
	{

		err := os.WriteFile(path, data, 0o644)
		require.NoError(t, err)
	}

}
