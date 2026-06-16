package commands

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

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
	source, _, command, _ := detectInstallSource(`C:\Users\brenn\scoop\apps\devspecs\current\ds.exe`)
	if source != "scoop" {
		t.Fatalf("source = %q", source)
	}
	if command != "scoop update devspecs" {
		t.Fatalf("command = %q", command)
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

func containsUpdateString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
