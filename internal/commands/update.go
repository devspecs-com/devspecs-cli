package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/devspecs-com/devspecs-cli/internal/config"
	"github.com/devspecs-com/devspecs-cli/internal/version"
	"github.com/spf13/cobra"
)

const (
	updateCacheFileName = "update-check.json"
	updateCacheTTL      = 24 * time.Hour
	updateCheckTimeout  = 2 * time.Second
	latestReleaseURL    = "https://api.github.com/repos/devspecs-com/devspecs-cli/releases/latest"
)

type updateReport struct {
	Version         string   `json:"version"`
	Commit          string   `json:"commit"`
	Built           string   `json:"built"`
	Executable      string   `json:"executable"`
	InstallSource   string   `json:"install_source"`
	Confidence      string   `json:"confidence"`
	Latest          string   `json:"latest"`
	LatestSource    string   `json:"latest_source,omitempty"`
	LatestChecked   string   `json:"latest_checked_at,omitempty"`
	VersionStatus   string   `json:"version_status"`
	CheckError      string   `json:"check_error,omitempty"`
	CachePath       string   `json:"cache_path,omitempty"`
	CacheTTL        string   `json:"cache_ttl,omitempty"`
	UpdateCommand   string   `json:"update_command,omitempty"`
	Alternatives    []string `json:"alternatives,omitempty"`
	RestartMessage  string   `json:"restart_message"`
	CanApply        bool     `json:"can_apply"`
	UpdateAvailable bool     `json:"update_available"`
}

type updateCheckOptions struct {
	Enabled bool
	Refresh bool
	Now     time.Time
	TTL     time.Duration
	Fetcher latestVersionFetcher
}

type latestVersionFetcher func(context.Context) (string, error)

type updateCheckCache struct {
	Latest    string `json:"latest"`
	CheckedAt string `json:"checked_at"`
	Source    string `json:"source"`
}

// NewUpdateCmd creates the ds update command.
func NewUpdateCmd() *cobra.Command {
	var asJSON bool
	var noCheck bool
	var refresh bool

	cmd := &cobra.Command{
		Use:   "update",
		Short: "Show how to update this DevSpecs install",
		Long: `Show package-manager-aware update guidance for the active ds binary.

This command is safe by default: it does not run package manager commands,
download binaries, or modify your system. It detects the likely install source
from the active executable path and prints the recommended command to run.

By default, ds update checks the latest GitHub release through a small local
cache under DEVSPECS_HOME or ~/.devspecs. Use --no-check for fully offline
guidance or --refresh to bypass the cache.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			exe, err := os.Executable()
			if err != nil {
				exe = ""
			}
			report := buildUpdateReport(exe)
			opts := updateCheckOptions{
				Enabled: !noCheck,
				Refresh: refresh,
				Now:     time.Now().UTC(),
				TTL:     updateCacheTTL,
				Fetcher: fetchLatestReleaseVersion,
			}
			enrichUpdateReportWithLatest(cmd.Context(), &report, opts)
			return outputUpdateReport(cmd, report, asJSON)
		},
	}

	cmd.Flags().BoolVar(&asJSON, "json", false, "Output as JSON")
	cmd.Flags().BoolVar(&noCheck, "no-check", false, "Do not check the latest release; print local guidance only")
	cmd.Flags().BoolVar(&refresh, "refresh", false, "Bypass the local latest-version cache")
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
		VersionStatus:  "unknown",
		UpdateCommand:  command,
		Alternatives:   alternatives,
		RestartMessage: "After updating, restart your shell or IDE terminal if `ds` still points to the old binary.",
		CanApply:       false,
	}
}

func enrichUpdateReportWithLatest(ctx context.Context, report *updateReport, opts updateCheckOptions) {
	if !opts.Enabled {
		report.Latest = "not checked"
		report.VersionStatus = classifyVersionStatus(report.Version, "")
		return
	}
	if opts.Now.IsZero() {
		opts.Now = time.Now().UTC()
	}
	if opts.TTL <= 0 {
		opts.TTL = updateCacheTTL
	}
	if opts.Fetcher == nil {
		opts.Fetcher = fetchLatestReleaseVersion
	}
	cachePath, err := updateCheckCachePath()
	if err == nil {
		report.CachePath = cachePath
		report.CacheTTL = opts.TTL.String()
		if !opts.Refresh {
			if cached, ok := readFreshUpdateCheckCache(cachePath, opts.Now, opts.TTL); ok {
				applyLatestVersion(report, cached.Latest, "cache", cached.CheckedAt)
				return
			}
		}
	}

	checkCtx, cancel := context.WithTimeout(ctx, updateCheckTimeout)
	defer cancel()
	latest, err := opts.Fetcher(checkCtx)
	if err == nil && strings.TrimSpace(latest) != "" {
		checkedAt := opts.Now.Format(time.RFC3339)
		applyLatestVersion(report, latest, "github", checkedAt)
		if cachePath != "" {
			_ = writeUpdateCheckCache(cachePath, updateCheckCache{
				Latest:    latest,
				CheckedAt: checkedAt,
				Source:    "github",
			})
		}
		return
	}

	report.Latest = "unknown"
	report.LatestSource = "github"
	report.VersionStatus = classifyVersionStatus(report.Version, "")
	report.CheckError = friendlyUpdateCheckError(err)
	if cachePath != "" {
		if cached, ok := readUpdateCheckCache(cachePath); ok {
			applyLatestVersion(report, cached.Latest, "stale cache", cached.CheckedAt)
			report.CheckError = friendlyUpdateCheckError(err)
		}
	}
}

func updateCheckCachePath() (string, error) {
	home, err := config.HomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, updateCacheFileName), nil
}

func readFreshUpdateCheckCache(path string, now time.Time, ttl time.Duration) (updateCheckCache, bool) {
	cached, ok := readUpdateCheckCache(path)
	if !ok {
		return updateCheckCache{}, false
	}
	checkedAt, err := time.Parse(time.RFC3339, cached.CheckedAt)
	if err != nil {
		return updateCheckCache{}, false
	}
	return cached, now.Sub(checkedAt) >= 0 && now.Sub(checkedAt) < ttl
}

func readUpdateCheckCache(path string) (updateCheckCache, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return updateCheckCache{}, false
	}
	var cached updateCheckCache
	if err := json.Unmarshal(data, &cached); err != nil {
		return updateCheckCache{}, false
	}
	if strings.TrimSpace(cached.Latest) == "" || strings.TrimSpace(cached.CheckedAt) == "" {
		return updateCheckCache{}, false
	}
	return cached, true
}

func writeUpdateCheckCache(path string, cached updateCheckCache) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cached, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

func fetchLatestReleaseVersion(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, latestReleaseURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("accept", "application/vnd.github+json")
	req.Header.Set("user-agent", "devspecs-cli/"+version.Version)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("GitHub release check returned %s", resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	var payload struct {
		TagName string `json:"tag_name"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", err
	}
	if strings.TrimSpace(payload.TagName) == "" {
		return "", fmt.Errorf("GitHub release response did not include tag_name")
	}
	return strings.TrimSpace(payload.TagName), nil
}

func applyLatestVersion(report *updateReport, latest, source, checkedAt string) {
	report.Latest = strings.TrimSpace(latest)
	report.LatestSource = source
	report.LatestChecked = checkedAt
	report.VersionStatus = classifyVersionStatus(report.Version, report.Latest)
	report.UpdateAvailable = report.VersionStatus == "stale"
}

func classifyVersionStatus(current, latest string) string {
	current = strings.TrimSpace(current)
	latest = strings.TrimSpace(latest)
	if isDevelopmentVersion(current) {
		return "development"
	}
	if latest == "" {
		return "unknown"
	}
	cmp, ok := compareVersionTags(current, latest)
	if !ok {
		return "unknown"
	}
	if cmp < 0 {
		return "stale"
	}
	return "current"
}

func isDevelopmentVersion(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	return value == "" ||
		value == "dev" ||
		value == "unknown" ||
		value == "none" ||
		strings.Contains(value, "dev") ||
		strings.Contains(value, "dirty")
}

func compareVersionTags(current, latest string) (int, bool) {
	currentParts, ok := parseVersionTag(current)
	if !ok {
		return 0, false
	}
	latestParts, ok := parseVersionTag(latest)
	if !ok {
		return 0, false
	}
	for i := 0; i < 3; i++ {
		if currentParts[i] < latestParts[i] {
			return -1, true
		}
		if currentParts[i] > latestParts[i] {
			return 1, true
		}
	}
	return 0, true
}

func parseVersionTag(value string) ([3]int, bool) {
	var parts [3]int
	value = strings.TrimSpace(strings.TrimPrefix(value, "v"))
	if value == "" {
		return parts, false
	}
	if cut := strings.IndexAny(value, "-+"); cut >= 0 {
		value = value[:cut]
	}
	raw := strings.Split(value, ".")
	if len(raw) < 2 || len(raw) > 3 {
		return parts, false
	}
	for i, part := range raw {
		n, err := strconv.Atoi(part)
		if err != nil {
			return parts, false
		}
		parts[i] = n
	}
	return parts, true
}

func friendlyUpdateCheckError(err error) string {
	if err == nil {
		return ""
	}
	msg := strings.TrimSpace(err.Error())
	if msg == "" {
		return "latest release check failed"
	}
	if len(msg) > 180 {
		msg = msg[:180] + "..."
	}
	return msg
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
	path = strings.ReplaceAll(path, `\`, `/`)
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
	fmt.Fprintf(out, "Latest version: %s", report.Latest)
	if report.LatestSource != "" {
		fmt.Fprintf(out, " (%s", report.LatestSource)
		if report.LatestChecked != "" {
			fmt.Fprintf(out, ", checked %s", report.LatestChecked)
		}
		fmt.Fprint(out, ")")
	}
	fmt.Fprintln(out)
	fmt.Fprintf(out, "Version status: %s", report.VersionStatus)
	if report.UpdateAvailable {
		fmt.Fprint(out, " (update available)")
	}
	fmt.Fprintln(out)
	if report.CheckError != "" {
		fmt.Fprintf(out, "Latest check warning: %s\n", report.CheckError)
	}
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
