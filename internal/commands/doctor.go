package commands

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/devspecs-com/devspecs-cli/internal/config"
	"github.com/devspecs-com/devspecs-cli/internal/repo"
	"github.com/devspecs-com/devspecs-cli/internal/store"
	"github.com/devspecs-com/devspecs-cli/internal/version"
	"github.com/spf13/cobra"
)

const (
	doctorReportSchemaVersion = 1
	doctorRepoTimeout         = 5 * time.Second
	doctorPathCandidateLimit  = 8

	doctorStatusOK            = "ok"
	doctorStatusWarning       = "warning"
	doctorStatusError         = "error"
	doctorStatusUnknown       = "unknown"
	doctorStatusNotApplicable = "not_applicable"
)

type doctorOptions struct {
	RepoPath string
	AsJSON   bool
	Redact   bool
}

type doctorReport struct {
	SchemaVersion int                    `json:"schema_version"`
	Status        string                 `json:"status"`
	Redacted      bool                   `json:"redacted"`
	Runtime       doctorRuntimeReport    `json:"runtime"`
	Home          doctorHomeReport       `json:"home"`
	Index         doctorIndexReport      `json:"index"`
	Repository    doctorRepositoryReport `json:"repository"`
	Findings      []doctorFinding        `json:"findings"`
}

type doctorRuntimeReport struct {
	Status             string   `json:"status"`
	OS                 string   `json:"os"`
	Arch               string   `json:"arch"`
	Version            string   `json:"version"`
	Commit             string   `json:"commit"`
	Built              string   `json:"built"`
	Executable         string   `json:"executable,omitempty"`
	ResolvedExecutable string   `json:"resolved_executable,omitempty"`
	PathFirst          string   `json:"path_first,omitempty"`
	ActiveIsPathFirst  bool     `json:"active_is_path_first"`
	PathCandidateCount int      `json:"path_candidate_count"`
	PathCandidates     []string `json:"path_candidates,omitempty"`
	InstallSource      string   `json:"install_source"`
	InstallConfidence  string   `json:"install_confidence"`
}

type doctorHomeReport struct {
	Status string `json:"status"`
	Source string `json:"source"`
	Path   string `json:"path,omitempty"`
	Exists bool   `json:"exists"`
}

type doctorIndexReport struct {
	Status            string `json:"status"`
	Path              string `json:"path,omitempty"`
	Exists            bool   `json:"exists"`
	DatabaseBytes     int64  `json:"database_bytes"`
	WALBytes          int64  `json:"wal_bytes"`
	DatabaseSchema    int    `json:"database_schema"`
	SupportedSchema   int    `json:"supported_schema"`
	Compatibility     string `json:"compatibility"`
	WriterState       string `json:"writer_state"`
	WriterObservedAt  string `json:"writer_observed_at,omitempty"`
	WriterObservation string `json:"writer_observation,omitempty"`
}

type doctorRepositoryReport struct {
	Status            string `json:"status"`
	RequestedPath     string `json:"requested_path,omitempty"`
	RootPath          string `json:"root_path,omitempty"`
	IsGit             bool   `json:"is_git"`
	Branch            string `json:"branch,omitempty"`
	IdentityMode      string `json:"identity_mode,omitempty"`
	GitIdentity       string `json:"git_identity,omitempty"`
	RootCommitPresent bool   `json:"root_commit_present"`
}

type doctorFinding struct {
	ID          string              `json:"id"`
	Severity    string              `json:"severity"`
	Summary     string              `json:"summary"`
	Evidence    string              `json:"evidence,omitempty"`
	Remediation []doctorRemediation `json:"remediation,omitempty"`
}

type doctorRemediation struct {
	Command string `json:"command,omitempty"`
	Safety  string `json:"safety"`
	Reason  string `json:"reason"`
}

type doctorHealthError struct{}

func (doctorHealthError) Error() string {
	return "DevSpecs doctor found one or more errors"
}

// NewDoctorCmd creates the read-only local diagnostics command.
func NewDoctorCmd() *cobra.Command {
	opts := doctorOptions{}
	cmd := &cobra.Command{
		Use:           "doctor",
		Short:         "Diagnose the active CLI and local index without changing them",
		SilenceErrors: true,
		SilenceUsage:  true,
		Long: `Inspect the active DevSpecs binary, PATH precedence, installation
channel, DEVSPECS_HOME, local index compatibility, writer state, and repository
identity.

Doctor is offline and read-only. It does not create or migrate an index, run a
repair, or contact an update service. Use --redact before sharing the report.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDoctor(cmd, opts)
		},
	}
	cmd.Flags().StringVar(&opts.RepoPath, repoTargetFlagName, "", "Repository path for identity and index guidance")
	cmd.Flags().BoolVar(&opts.AsJSON, "json", false, "Output as JSON")
	cmd.Flags().BoolVar(&opts.Redact, "redact", false, "Redact local paths and repository identity for sharing")
	return cmd
}

func runDoctor(cmd *cobra.Command, opts doctorOptions) error {
	report := collectDoctorReport(cmd.Context(), opts.RepoPath)
	if opts.Redact {
		report = redactDoctorReport(report)
	}
	if err := outputDoctorReport(cmd, report, opts.AsJSON); err != nil {
		return err
	}
	if report.Status == doctorStatusError {
		return doctorHealthError{}
	}
	return nil
}

func collectDoctorReport(ctx context.Context, repoPath string) doctorReport {
	report := doctorReport{
		SchemaVersion: doctorReportSchemaVersion,
		Status:        doctorStatusOK,
		Findings:      []doctorFinding{},
	}
	collectDoctorRuntime(&report)
	collectDoctorHome(&report)
	collectDoctorRepository(ctx, repoPath, &report)
	collectDoctorIndex(ctx, &report)
	report.Status = doctorOverallStatus(report.Findings)
	return report
}

func collectDoctorRuntime(report *doctorReport) {
	runtimeReport := doctorRuntimeReport{
		Status:  doctorStatusOK,
		OS:      runtime.GOOS,
		Arch:    runtime.GOARCH,
		Version: version.Version,
		Commit:  version.Commit,
		Built:   version.Date,
	}
	executable, err := os.Executable()
	if err != nil {
		runtimeReport.Status = doctorStatusError
		report.addFinding("runtime.executable_unavailable", doctorStatusError,
			"The active DevSpecs executable could not be resolved.", err.Error())
		report.Runtime = runtimeReport
		return
	}
	runtimeReport.Executable = executable
	runtimeReport.ResolvedExecutable = resolvedExecutablePath(executable)
	runtimeReport.InstallSource, runtimeReport.InstallConfidence, _, _ = detectInstallSource(executable)
	candidates := findDoctorExecutableCandidates(os.Getenv("PATH"), os.Getenv("PATHEXT"), runtime.GOOS)
	runtimeReport.PathCandidateCount = len(candidates)
	if len(candidates) > doctorPathCandidateLimit {
		runtimeReport.PathCandidates = append([]string(nil), candidates[:doctorPathCandidateLimit]...)
	} else {
		runtimeReport.PathCandidates = append([]string(nil), candidates...)
	}
	if len(candidates) == 0 {
		runtimeReport.Status = doctorStatusWarning
		report.addFinding("runtime.not_on_path", doctorStatusWarning,
			"No ds executable was found on PATH.",
			fmt.Sprintf("active executable: %s", executable),
			doctorRemediation{Safety: "read_only", Reason: "Check PATH and restart the shell or IDE terminal after correcting it."})
		report.Runtime = runtimeReport
		return
	}
	runtimeReport.PathFirst = candidates[0]
	runtimeReport.ActiveIsPathFirst = sameDoctorExecutable(executable, candidates[0], runtime.GOOS)
	if !runtimeReport.ActiveIsPathFirst {
		runtimeReport.Status = doctorStatusWarning
		report.addFinding("runtime.path_mismatch", doctorStatusWarning,
			"The active DevSpecs binary is not the first ds executable on PATH.",
			fmt.Sprintf("active executable: %s; PATH first: %s", executable, candidates[0]),
			doctorRemediation{Safety: "read_only", Reason: "Correct PATH precedence, then restart the shell or IDE terminal."})
	}
	report.Runtime = runtimeReport
}

func collectDoctorHome(report *doctorReport) {
	homeReport := doctorHomeReport{Status: doctorStatusOK, Source: "default"}
	if strings.TrimSpace(os.Getenv("DEVSPECS_HOME")) != "" {
		homeReport.Source = "environment"
	}
	home, err := config.HomeDir()
	if err != nil {
		homeReport.Status = doctorStatusError
		report.addFinding("home.unavailable", doctorStatusError,
			"The DevSpecs home directory could not be resolved.", err.Error())
		report.Home = homeReport
		return
	}
	homeReport.Path = home
	info, err := os.Stat(home)
	switch {
	case errors.Is(err, os.ErrNotExist):
		homeReport.Status = doctorStatusNotApplicable
	case err != nil:
		homeReport.Status = doctorStatusError
		report.addFinding("home.unreadable", doctorStatusError,
			"The DevSpecs home path could not be inspected.", err.Error())
	case !info.IsDir():
		homeReport.Exists = true
		homeReport.Status = doctorStatusError
		report.addFinding("home.not_directory", doctorStatusError,
			"The DevSpecs home path is not a directory.", home)
	default:
		homeReport.Exists = true
	}
	report.Home = homeReport
}

func collectDoctorRepository(ctx context.Context, repoPath string, report *doctorReport) {
	repositoryReport := doctorRepositoryReport{Status: doctorStatusNotApplicable}
	requested := strings.TrimSpace(repoPath)
	if requested == "" {
		requested = "."
	}
	absolute, err := filepath.Abs(requested)
	if err == nil {
		repositoryReport.RequestedPath = absolute
	} else {
		repositoryReport.RequestedPath = requested
	}

	repoCtx, cancel := context.WithTimeout(ctx, doctorRepoTimeout)
	defer cancel()
	root, err := resolveTargetRepoRootContext(repoCtx, requested)
	if err != nil {
		repositoryReport.Status = doctorStatusError
		report.addFinding("repository.unavailable", doctorStatusError,
			"The requested repository path could not be resolved.", err.Error())
		report.Repository = repositoryReport
		return
	}
	repositoryReport.RootPath = root
	identity := repo.DetectIdentityContext(repoCtx, root)
	repositoryReport.IsGit = identity.IsGit
	repositoryReport.RootPath = identity.RootPath
	if !identity.IsGit {
		repositoryReport.IdentityMode = "path"
		report.Repository = repositoryReport
		return
	}
	repositoryReport.Status = doctorStatusOK
	repositoryReport.Branch = identity.CurrentBranch
	repositoryReport.GitIdentity = identity.GitIdentity
	repositoryReport.RootCommitPresent = strings.TrimSpace(identity.RootCommit) != ""
	repositoryReport.IdentityMode = "path"
	if strings.TrimSpace(identity.GitIdentity) != "" {
		repositoryReport.IdentityMode = "git"
	}
	if repoCtx.Err() != nil {
		repositoryReport.Status = doctorStatusWarning
		report.addFinding("repository.identity_timeout", doctorStatusWarning,
			"Repository identity discovery exceeded its diagnostic deadline.",
			"The repository remains usable with path-scoped identity for this report.")
	}
	report.Repository = repositoryReport
}

func collectDoctorIndex(ctx context.Context, report *doctorReport) {
	indexReport := doctorIndexReport{
		Status:          doctorStatusUnknown,
		Compatibility:   doctorStatusUnknown,
		WriterState:     store.IndexWriterStateUnknown,
		SupportedSchema: store.SchemaVersion,
	}
	dbPath, err := config.DBPath()
	if err != nil {
		indexReport.Status = doctorStatusError
		report.addFinding("index.path_unavailable", doctorStatusError,
			"The DevSpecs index path could not be resolved.", err.Error())
		report.Index = indexReport
		return
	}
	indexReport.Path = dbPath
	inspection, inspectErr := store.InspectIndex(ctx, dbPath)
	indexReport.Exists = inspection.Exists
	indexReport.DatabaseBytes = inspection.DatabaseBytes
	indexReport.WALBytes = inspection.WALBytes
	indexReport.DatabaseSchema = inspection.DatabaseSchema
	indexReport.SupportedSchema = inspection.SupportedSchema
	indexReport.Compatibility = inspection.Compatibility
	if inspectErr != nil {
		indexReport.Status = doctorStatusError
		indexReport.Compatibility = "unreadable"
		report.addFinding("index.unreadable", doctorStatusError,
			"The local DevSpecs index could not be read safely.", inspectErr.Error(),
			doctorRemediation{Safety: "read_only", Reason: "Preserve the index and inspect update options.", Command: "ds update --no-check"})
	} else {
		classifyDoctorIndex(&indexReport, report)
	}

	indexReport.WriterObservedAt = time.Now().UTC().Format(time.RFC3339Nano)
	indexReport.WriterObservation = "instantaneous; ownership may change after this report"
	writerState, writerErr := store.InspectIndexWriter(dbPath)
	indexReport.WriterState = writerState
	if writerErr != nil {
		report.addFinding("index.writer_unknown", doctorStatusWarning,
			"The index writer state could not be determined.", writerErr.Error())
		if indexReport.Status != doctorStatusError {
			indexReport.Status = doctorStatusWarning
		}
	} else if writerState == store.IndexWriterStateHeld {
		report.addFinding("index.writer_held", doctorStatusWarning,
			"Another DevSpecs operation currently holds the index writer slot.",
			"This is an instantaneous observation; wait for the active operation and retry.",
			doctorRemediation{Safety: "read_only", Reason: "Recheck writer state after waiting.", Command: "ds doctor"})
		if indexReport.Status != doctorStatusError {
			indexReport.Status = doctorStatusWarning
		}
	}
	report.Index = indexReport
}

func classifyDoctorIndex(indexReport *doctorIndexReport, report *doctorReport) {
	switch indexReport.Compatibility {
	case store.IndexCompatibilityAbsent:
		indexReport.Status = doctorStatusNotApplicable
		if report.Repository.IsGit {
			indexReport.Status = doctorStatusWarning
			report.addFinding("index.missing", doctorStatusWarning,
				"No local DevSpecs index exists for this installation.",
				"The repository remains authoritative; scanning creates rebuildable local index data.",
				doctorRemediation{Safety: "mutating", Reason: "Create the local index from repository evidence.", Command: doctorScanCommand(report.Repository.RootPath)})
		}
	case store.IndexCompatibilityCurrent:
		indexReport.Status = doctorStatusOK
	case store.IndexCompatibilityOlder:
		indexReport.Status = doctorStatusWarning
		report.addFinding("index.schema_older", doctorStatusWarning,
			"The local index schema is older than this CLI supports.",
			fmt.Sprintf("database schema v%d; CLI supports v%d", indexReport.DatabaseSchema, indexReport.SupportedSchema),
			doctorRemediation{Safety: "mutating", Reason: "Apply supported forward migrations while scanning repository evidence.", Command: doctorScanCommand(report.Repository.RootPath)})
	case store.IndexCompatibilityNewer:
		indexReport.Status = doctorStatusError
		report.addFinding("index.schema_newer", doctorStatusError,
			"The local index schema is newer than this CLI supports.",
			fmt.Sprintf("database schema v%d; CLI supports v%d", indexReport.DatabaseSchema, indexReport.SupportedSchema),
			doctorRemediation{Safety: "read_only", Reason: "Inspect installation-specific update guidance before using the index.", Command: "ds update --no-check"})
	default:
		indexReport.Status = doctorStatusUnknown
	}
}

func (report *doctorReport) addFinding(id, severity, summary, evidence string, remediation ...doctorRemediation) {
	report.Findings = append(report.Findings, doctorFinding{
		ID:          id,
		Severity:    severity,
		Summary:     summary,
		Evidence:    evidence,
		Remediation: append([]doctorRemediation(nil), remediation...),
	})
}

func doctorOverallStatus(findings []doctorFinding) string {
	status := doctorStatusOK
	for _, finding := range findings {
		if finding.Severity == doctorStatusError {
			return doctorStatusError
		}
		if finding.Severity == doctorStatusWarning {
			status = doctorStatusWarning
		}
	}
	return status
}

func findDoctorExecutableCandidates(pathValue, pathExt, goos string) []string {
	names := []string{"ds"}
	if goos == "windows" {
		names = doctorWindowsExecutableNames(pathExt)
	}
	seen := map[string]bool{}
	var candidates []string
	for _, directory := range filepath.SplitList(pathValue) {
		if strings.TrimSpace(directory) == "" {
			directory = "."
		}
		for _, name := range names {
			candidate, err := filepath.Abs(filepath.Join(directory, name))
			if err != nil {
				continue
			}
			info, err := os.Stat(candidate)
			if err != nil || !info.Mode().IsRegular() {
				continue
			}
			if goos != "windows" && info.Mode().Perm()&0o111 == 0 {
				continue
			}
			key := doctorPathKey(resolvedExecutablePath(candidate), goos)
			if seen[key] {
				continue
			}
			seen[key] = true
			candidates = append(candidates, candidate)
		}
	}
	return candidates
}

func doctorWindowsExecutableNames(pathExt string) []string {
	extensions := filepath.SplitList(pathExt)
	if len(extensions) == 0 {
		extensions = []string{".COM", ".EXE", ".BAT", ".CMD"}
	}
	seen := map[string]bool{}
	var names []string
	for _, extension := range extensions {
		extension = strings.TrimSpace(extension)
		if extension == "" {
			continue
		}
		if !strings.HasPrefix(extension, ".") {
			extension = "." + extension
		}
		name := "ds" + strings.ToLower(extension)
		if seen[name] {
			continue
		}
		seen[name] = true
		names = append(names, name)
	}
	return names
}

func resolvedExecutablePath(path string) string {
	resolved, err := filepath.EvalSymlinks(path)
	if err == nil {
		path = resolved
	}
	absolute, err := filepath.Abs(path)
	if err == nil {
		path = absolute
	}
	return filepath.Clean(path)
}

func sameDoctorExecutable(left, right, goos string) bool {
	left = doctorPathKey(resolvedExecutablePath(left), goos)
	right = doctorPathKey(resolvedExecutablePath(right), goos)
	return left == right
}

func doctorPathKey(path, goos string) string {
	path = filepath.Clean(path)
	if goos == "windows" {
		return strings.ToLower(path)
	}
	return path
}

func doctorScanCommand(repoRoot string) string {
	if strings.TrimSpace(repoRoot) == "" {
		return "ds scan"
	}
	return "ds scan --path " + commandArg(repoRoot)
}

func redactDoctorReport(report doctorReport) doctorReport {
	homePath := report.Home.Path
	repoPath := report.Repository.RootPath
	activePath := report.Runtime.Executable
	resolvedPath := report.Runtime.ResolvedExecutable
	pathFirst := report.Runtime.PathFirst
	report.Redacted = true
	report.Runtime.Executable = redactDoctorPathWithLabel(report.Runtime.Executable, homePath, repoPath, "active")
	report.Runtime.ResolvedExecutable = redactDoctorPathWithLabel(report.Runtime.ResolvedExecutable, homePath, repoPath, "active")
	report.Runtime.PathFirst = redactDoctorPathWithLabel(report.Runtime.PathFirst, homePath, repoPath, "path-first")
	report.Runtime.PathCandidates = nil
	report.Home.Path = redactDoctorPath(report.Home.Path, homePath, repoPath)
	report.Index.Path = redactDoctorPath(report.Index.Path, homePath, repoPath)
	report.Repository.RequestedPath = redactDoctorPath(report.Repository.RequestedPath, homePath, repoPath)
	report.Repository.RootPath = redactDoctorPath(report.Repository.RootPath, homePath, repoPath)
	if report.Repository.Branch != "" {
		report.Repository.Branch = "<redacted>"
	}
	if report.Repository.GitIdentity != "" {
		report.Repository.GitIdentity = "<redacted>"
	}

	findings := make([]doctorFinding, len(report.Findings))
	for index, finding := range report.Findings {
		finding.Evidence = redactDoctorRuntimeText(finding.Evidence, homePath, repoPath, activePath, resolvedPath, pathFirst)
		remediation := make([]doctorRemediation, len(finding.Remediation))
		for remediationIndex, item := range finding.Remediation {
			item.Command = redactDoctorRuntimeText(item.Command, homePath, repoPath, activePath, resolvedPath, pathFirst)
			item.Reason = redactDoctorRuntimeText(item.Reason, homePath, repoPath, activePath, resolvedPath, pathFirst)
			remediation[remediationIndex] = item
		}
		finding.Remediation = remediation
		findings[index] = finding
	}
	report.Findings = findings
	return report
}

func redactDoctorPath(path, homePath, repoPath string) string {
	return redactDoctorPathWithLabel(path, homePath, repoPath, "external")
}

func redactDoctorPathWithLabel(path, homePath, repoPath, label string) string {
	if strings.TrimSpace(path) == "" {
		return ""
	}
	if redacted, ok := replaceDoctorPathPrefix(path, repoPath, "<repo>"); ok {
		return redacted
	}
	if redacted, ok := replaceDoctorPathPrefix(path, homePath, "<home>"); ok {
		return redacted
	}
	if filepath.IsAbs(path) {
		return filepath.Join("<path:"+label+">", filepath.Base(path))
	}
	return path
}

func replaceDoctorPathPrefix(path, prefix, replacement string) (string, bool) {
	if strings.TrimSpace(prefix) == "" {
		return "", false
	}
	relative, err := filepath.Rel(prefix, path)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", false
	}
	if relative == "." {
		return replacement, true
	}
	return filepath.Join(replacement, relative), true
}

func redactDoctorText(value, homePath, repoPath string) string {
	replacements := []struct {
		value string
		token string
	}{
		{value: repoPath, token: "<repo>"},
		{value: homePath, token: "<home>"},
	}
	sort.SliceStable(replacements, func(left, right int) bool {
		return len(replacements[left].value) > len(replacements[right].value)
	})
	for _, replacement := range replacements {
		if strings.TrimSpace(replacement.value) == "" {
			continue
		}
		value = strings.ReplaceAll(value, replacement.value, replacement.token)
		value = strings.ReplaceAll(value, filepath.ToSlash(replacement.value), replacement.token)
	}
	return value
}

func redactDoctorRuntimeText(value, homePath, repoPath, activePath, resolvedPath, pathFirst string) string {
	value = redactDoctorText(value, homePath, repoPath)
	replacements := []struct {
		value string
		label string
	}{
		{value: activePath, label: "active"},
		{value: resolvedPath, label: "active"},
		{value: pathFirst, label: "path-first"},
	}
	sort.SliceStable(replacements, func(left, right int) bool {
		return len(replacements[left].value) > len(replacements[right].value)
	})
	for _, replacement := range replacements {
		if strings.TrimSpace(replacement.value) == "" {
			continue
		}
		redacted := redactDoctorPathWithLabel(replacement.value, homePath, repoPath, replacement.label)
		value = strings.ReplaceAll(value, replacement.value, redacted)
		value = strings.ReplaceAll(value, filepath.ToSlash(replacement.value), filepath.ToSlash(redacted))
	}
	return value
}

func outputDoctorReport(cmd *cobra.Command, report doctorReport, asJSON bool) error {
	if asJSON {
		encoder := json.NewEncoder(cmd.OutOrStdout())
		encoder.SetEscapeHTML(false)
		encoder.SetIndent("", "  ")
		return encoder.Encode(report)
	}
	out := cmd.OutOrStdout()
	fmt.Fprintln(out, "DevSpecs doctor")
	fmt.Fprintf(out, "Overall: %s\n", report.Status)
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Runtime")
	fmt.Fprintf(out, "  CLI: %s (commit: %s, built: %s)\n", report.Runtime.Version, report.Runtime.Commit, report.Runtime.Built)
	fmt.Fprintf(out, "  Platform: %s/%s\n", report.Runtime.OS, report.Runtime.Arch)
	fmt.Fprintf(out, "  Active binary: %s\n", doctorDisplayValue(report.Runtime.Executable))
	fmt.Fprintf(out, "  PATH first: %s\n", doctorDisplayValue(report.Runtime.PathFirst))
	fmt.Fprintf(out, "  PATH candidates: %d\n", report.Runtime.PathCandidateCount)
	fmt.Fprintf(out, "  Install source: %s (%s confidence)\n", report.Runtime.InstallSource, report.Runtime.InstallConfidence)
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Local state")
	fmt.Fprintf(out, "  DEVSPECS_HOME: %s (%s)\n", doctorDisplayValue(report.Home.Path), report.Home.Source)
	fmt.Fprintf(out, "  Database: %s\n", doctorDisplayValue(report.Index.Path))
	fmt.Fprintf(out, "  Database size: %s", formatByteSize(report.Index.DatabaseBytes))
	if report.Index.WALBytes > 0 {
		fmt.Fprintf(out, " (+ %s WAL)", formatByteSize(report.Index.WALBytes))
	}
	fmt.Fprintln(out)
	fmt.Fprintf(out, "  Schema: %d (CLI supports %d; %s)\n", report.Index.DatabaseSchema, report.Index.SupportedSchema, report.Index.Compatibility)
	fmt.Fprintf(out, "  Writer: %s (%s)\n", report.Index.WriterState, report.Index.WriterObservation)
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Repository")
	fmt.Fprintf(out, "  Root: %s\n", doctorDisplayValue(report.Repository.RootPath))
	fmt.Fprintf(out, "  Identity: %s\n", doctorDisplayValue(report.Repository.IdentityMode))
	if report.Repository.Branch != "" {
		fmt.Fprintf(out, "  Branch: %s\n", report.Repository.Branch)
	}
	if report.Repository.GitIdentity != "" {
		fmt.Fprintf(out, "  Git identity: %s\n", report.Repository.GitIdentity)
	}
	if len(report.Findings) > 0 {
		fmt.Fprintln(out)
		fmt.Fprintln(out, "Findings")
		for _, finding := range report.Findings {
			fmt.Fprintf(out, "  [%s] %s: %s\n", finding.Severity, finding.ID, finding.Summary)
			if finding.Evidence != "" {
				fmt.Fprintf(out, "    Evidence: %s\n", finding.Evidence)
			}
			for _, remediation := range finding.Remediation {
				fmt.Fprintf(out, "    Remediation (%s): %s", remediation.Safety, remediation.Reason)
				if remediation.Command != "" {
					fmt.Fprintf(out, " Run `%s`.", remediation.Command)
				}
				fmt.Fprintln(out)
			}
		}
	}
	if !report.Redacted {
		fmt.Fprintln(out)
		fmt.Fprintln(out, "Paths and repository identity are local-sensitive. Rerun with `ds doctor --redact` before sharing.")
	}
	return nil
}

func doctorDisplayValue(value string) string {
	if strings.TrimSpace(value) == "" {
		return "not available"
	}
	return value
}
