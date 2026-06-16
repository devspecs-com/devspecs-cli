package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/devspecs-com/devspecs-cli/internal/version"
	"github.com/spf13/cobra"
)

type updateReport struct {
	Version        string   `json:"version"`
	Commit         string   `json:"commit"`
	Built          string   `json:"built"`
	Executable     string   `json:"executable"`
	InstallSource  string   `json:"install_source"`
	Confidence     string   `json:"confidence"`
	Latest         string   `json:"latest"`
	UpdateCommand  string   `json:"update_command,omitempty"`
	Alternatives   []string `json:"alternatives,omitempty"`
	RestartMessage string   `json:"restart_message"`
	CanApply       bool     `json:"can_apply"`
}

// NewUpdateCmd creates the ds update command.
func NewUpdateCmd() *cobra.Command {
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "update",
		Short: "Show how to update this DevSpecs install",
		Long: `Show package-manager-aware update guidance for the active ds binary.

This command is safe by default: it does not run package manager commands,
download binaries, or modify your system. It detects the likely install source
from the active executable path and prints the recommended command to run.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			exe, err := os.Executable()
			if err != nil {
				exe = ""
			}
			report := buildUpdateReport(exe)
			return outputUpdateReport(cmd, report, asJSON)
		},
	}

	cmd.Flags().BoolVar(&asJSON, "json", false, "Output as JSON")
	return cmd
}

func buildUpdateReport(executable string) updateReport {
	source, confidence, command, alternatives := detectInstallSource(executable)
	return updateReport{
		Version:        version.Version,
		Commit:         version.Commit,
		Built:          version.Date,
		Executable:     executable,
		InstallSource:  source,
		Confidence:     confidence,
		Latest:         "not checked",
		UpdateCommand:  command,
		Alternatives:   alternatives,
		RestartMessage: "After updating, restart your shell or IDE terminal if `ds` still points to the old binary.",
		CanApply:       false,
	}
}

func detectInstallSource(executable string) (source, confidence, command string, alternatives []string) {
	normalized := normalizeExecutablePath(executable)

	switch {
	case strings.Contains(normalized, "/scoop/apps/devspecs/") || strings.Contains(normalized, "/scoop/shims/ds"):
		return "scoop", "high", "scoop update devspecs", nil
	case strings.Contains(normalized, "/cellar/devspecs/") ||
		strings.Contains(normalized, "/homebrew/bin/ds") ||
		strings.Contains(normalized, "/linuxbrew/bin/ds") ||
		strings.Contains(normalized, "/linuxbrew/.linuxbrew/bin/ds") ||
		strings.Contains(normalized, "/usr/local/bin/ds") ||
		strings.Contains(normalized, "/opt/homebrew/bin/ds"):
		return "homebrew", "medium", "brew update && brew upgrade devspecs-com/tap/devspecs", nil
	case looksLikeGoInstall(normalized):
		return "go install", "medium", "go install github.com/devspecs-com/devspecs-cli/cmd/ds@latest", nil
	case strings.Contains(normalized, "/devspecs-cli/.devspecs/bin/") ||
		strings.Contains(normalized, "/devspecs-cli/cmd/ds/") ||
		strings.Contains(normalized, "/devspecs-cli/"):
		return "local development build", "medium", "", []string{
			"go install github.com/devspecs-com/devspecs-cli/cmd/ds@latest",
			"brew update && brew upgrade devspecs-com/tap/devspecs",
			"scoop update devspecs",
		}
	default:
		return "manual or unknown", "low", "curl -fsSL https://raw.githubusercontent.com/devspecs-com/devspecs-cli/main/install.sh | sh", []string{
			"irm https://raw.githubusercontent.com/devspecs-com/devspecs-cli/main/install.ps1 | iex",
			"go install github.com/devspecs-com/devspecs-cli/cmd/ds@latest",
			"brew update && brew upgrade devspecs-com/tap/devspecs",
			"scoop update devspecs",
		}
	}
}

func normalizeExecutablePath(path string) string {
	path = filepath.ToSlash(strings.TrimSpace(path))
	return strings.ToLower(path)
}

func looksLikeGoInstall(normalized string) bool {
	if strings.HasSuffix(normalized, "/go/bin/ds") || strings.HasSuffix(normalized, "/go/bin/ds.exe") {
		return true
	}
	return strings.Contains(normalized, "/go/bin/ds@") || strings.Contains(normalized, "/gopath/bin/ds")
}

func outputUpdateReport(cmd *cobra.Command, report updateReport, asJSON bool) error {
	if asJSON {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(report)
	}

	out := cmd.OutOrStdout()
	fmt.Fprintln(out, "DevSpecs update")
	fmt.Fprintf(out, "Installed version: %s (commit: %s, built: %s)\n", report.Version, report.Commit, report.Built)
	if report.Executable != "" {
		fmt.Fprintf(out, "Active binary: %s\n", report.Executable)
	}
	fmt.Fprintf(out, "Install source: %s (%s confidence)\n", report.InstallSource, report.Confidence)
	fmt.Fprintf(out, "Latest version: %s\n", report.Latest)
	fmt.Fprintln(out)
	if report.UpdateCommand != "" {
		fmt.Fprintln(out, "Recommended update command:")
		fmt.Fprintf(out, "  %s\n", report.UpdateCommand)
	} else {
		fmt.Fprintln(out, "Recommended update command:")
		fmt.Fprintln(out, "  Not automatic for this install source. Choose one of the supported install channels below.")
	}
	if len(report.Alternatives) > 0 {
		fmt.Fprintln(out)
		fmt.Fprintln(out, "Other supported install channels:")
		for _, alt := range report.Alternatives {
			fmt.Fprintf(out, "  %s\n", alt)
		}
	}
	fmt.Fprintln(out)
	fmt.Fprintln(out, report.RestartMessage)
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Note: ds update is guidance-only for now; it does not modify your system.")
	return nil
}
