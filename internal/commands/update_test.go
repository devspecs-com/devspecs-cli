package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
)

func TestDetectInstallSourceHomebrew(t *testing.T) {
	for _, path := range []string{"/opt/homebrew/bin/ds", "/home/linuxbrew/.linuxbrew/bin/ds"} {
		source, confidence, command, alternatives := detectInstallSource(path)
		if source != "homebrew" {
			t.Fatalf("%s source = %q", path, source)
		}
		if confidence != "medium" {
			t.Fatalf("%s confidence = %q", path, confidence)
		}
		if command != "brew update && brew upgrade devspecs-com/tap/devspecs" {
			t.Fatalf("%s command = %q", path, command)
		}
		if len(alternatives) != 0 {
			t.Fatalf("%s alternatives = %#v", path, alternatives)
		}
	}
}

func TestDetectInstallSourceScoop(t *testing.T) {
	for _, path := range []string{
		`C:\Users\alice\scoop\apps\devspecs\current\ds.exe`,
		`C:\Users\alice\scoop\shims\ds.exe`,
	} {
		source, _, command, _ := detectInstallSource(path)
		if source != "scoop" {
			t.Fatalf("%s source = %q", path, source)
		}
		if command != "scoop update devspecs" {
			t.Fatalf("%s command = %q", path, command)
		}
	}
}

func TestDetectInstallSourceGoInstall(t *testing.T) {
	source, _, command, _ := detectInstallSource(`/home/dev/go/bin/ds`)
	if source != "go install" {
		t.Fatalf("source = %q", source)
	}
	if command != "go install github.com/devspecs-com/devspecs-cli/cmd/ds@latest" {
		t.Fatalf("command = %q", command)
	}
}

func TestUpdateReportManualIncludesSupportedChannels(t *testing.T) {
	report := buildUpdateReport(`/tmp/ds`)
	if report.InstallSource != "manual or unknown" {
		t.Fatalf("install source = %q", report.InstallSource)
	}
	if report.UpdateCommand == "" || !strings.Contains(report.UpdateCommand, "install.sh") {
		t.Fatalf("update command = %q", report.UpdateCommand)
	}
	if !containsUpdateString(report.Alternatives, "scoop update devspecs") {
		t.Fatalf("expected scoop alternative, got %#v", report.Alternatives)
	}
	if report.CanApply {
		t.Fatal("expected guidance-only update report")
	}
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
	if err := outputUpdateReport(cmd, report, false); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	for _, want := range []string{
		"DevSpecs update",
		"Installed version: v1.1.0",
		"Install source: homebrew",
		"brew update && brew upgrade devspecs-com/tap/devspecs",
		"guidance-only",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q:\n%s", want, got)
		}
	}
}

func TestOutputUpdateReportJSON(t *testing.T) {
	report := buildUpdateReport(`/home/dev/go/bin/ds`)
	cmd := &cobra.Command{}
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	if err := outputUpdateReport(cmd, report, true); err != nil {
		t.Fatal(err)
	}
	var got updateReport
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, buf.String())
	}
	if got.InstallSource != "go install" {
		t.Fatalf("install source = %q", got.InstallSource)
	}
	if got.UpdateCommand != "go install github.com/devspecs-com/devspecs-cli/cmd/ds@latest" {
		t.Fatalf("update command = %q", got.UpdateCommand)
	}
}

func TestClassifyVersionStatus(t *testing.T) {
	cases := []struct {
		current string
		latest  string
		want    string
	}{
		{current: "v1.0.0", latest: "v1.0.1", want: "stale"},
		{current: "v1.0.1", latest: "v1.0.1", want: "current"},
		{current: "v1.1.0", latest: "v1.0.1", want: "current"},
		{current: "dev", latest: "v1.0.1", want: "development"},
		{current: "v1.1.0-dev", latest: "v1.0.1", want: "development"},
		{current: "not-a-version", latest: "v1.0.1", want: "unknown"},
		{current: "v1.0.0", latest: "", want: "unknown"},
	}
	for _, tc := range cases {
		if got := classifyVersionStatus(tc.current, tc.latest); got != tc.want {
			t.Fatalf("classifyVersionStatus(%q, %q) = %q, want %q", tc.current, tc.latest, got, tc.want)
		}
	}
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
	enrichUpdateReportWithLatest(context.Background(), &report, updateCheckOptions{
		Enabled: true,
		Now:     now,
		TTL:     updateCacheTTL,
		Fetcher: func(context.Context) (string, error) {
			t.Fatal("fetcher should not run when cache is fresh")
			return "", nil
		},
	})

	if report.Latest != "v1.0.1" {
		t.Fatalf("latest = %q", report.Latest)
	}
	if report.LatestSource != "cache" {
		t.Fatalf("latest source = %q", report.LatestSource)
	}
	if report.VersionStatus != "stale" || !report.UpdateAvailable {
		t.Fatalf("status = %q update_available=%v", report.VersionStatus, report.UpdateAvailable)
	}
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

	if report.Latest != "v1.0.1" || report.LatestSource != "github" {
		t.Fatalf("latest/source = %q/%q", report.Latest, report.LatestSource)
	}
	cached, ok := readUpdateCheckCache(filepath.Join(home, updateCacheFileName))
	if !ok {
		t.Fatal("expected cache to be written")
	}
	if cached.Latest != "v1.0.1" || cached.CheckedAt != now.Format(time.RFC3339) {
		t.Fatalf("cache = %#v", cached)
	}
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

	if report.Latest != "unknown" {
		t.Fatalf("latest = %q", report.Latest)
	}
	if report.VersionStatus != "unknown" {
		t.Fatalf("status = %q", report.VersionStatus)
	}
	if !strings.Contains(report.CheckError, "network unavailable") {
		t.Fatalf("check error = %q", report.CheckError)
	}
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

	if report.Latest != "v1.0.1" {
		t.Fatalf("latest = %q", report.Latest)
	}
	if report.LatestSource != "stale cache" {
		t.Fatalf("source = %q", report.LatestSource)
	}
	if report.CheckError == "" {
		t.Fatal("expected check error to explain stale cache fallback")
	}
	if report.VersionStatus != "stale" {
		t.Fatalf("status = %q", report.VersionStatus)
	}
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
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}
