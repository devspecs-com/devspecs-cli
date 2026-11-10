package orchestration

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

var (
	errHTTPDispatchResult = errors.New("HTTP dispatch v2 result could not be verified locally")
	errHTTPDispatchPaths  = errors.New("HTTP dispatch v2 invalid path evidence")
	errHTTPDispatchTask   = errors.New("HTTP dispatch v2 result edits protected lifecycle paths")

	httpDispatchUsageLabel = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*(?:[/:][A-Za-z0-9][A-Za-z0-9._-]*)*$`)
	httpDispatchDOSAlias   = regexp.MustCompile(`~[0-9]+(?:\.|$)`)
)

// This is evidence, not a checkpoint decision. Report and verification states
// are provider claims; artifact byte checks do not authenticate those claims.
type httpDispatchAdmittedReceipt struct {
	DispatchID                   string                    `json:"dispatch_id"`
	WireSHA256                   string                    `json:"wire_sha256"`
	RequestSHA256                string                    `json:"request_sha256"`
	ReceiptID                    string                    `json:"receipt_id"`
	SourceCommit                 string                    `json:"source_commit"`
	ResultCommit                 *string                   `json:"result_commit"`
	State                        string                    `json:"state"`
	Attempts                     int                       `json:"attempts"`
	StartedAt                    *time.Time                `json:"started_at"`
	EndedAt                      time.Time                 `json:"ended_at"`
	Failure                      *httpDispatchFailure      `json:"failure,omitempty"`
	ProviderPathsComplete        bool                      `json:"provider_paths_complete"`
	PathClaimsDisposition        string                    `json:"path_claims_disposition"`
	PathsManifest                *httpDispatchArtifactFact `json:"paths_manifest,omitempty"`
	ReportRequired               bool                      `json:"report_required"`
	ReportState                  string                    `json:"report_state"`
	ReportArtifact               *httpDispatchArtifactFact `json:"report_artifact,omitempty"`
	RequiredVerificationStrength string                    `json:"required_verification_strength,omitempty"`
	VerificationState            string                    `json:"verification_state"`
	VerificationStrength         string                    `json:"verification_strength,omitempty"`
	TestOutput                   *httpDispatchArtifactFact `json:"test_output,omitempty"`
	CancelRequested              bool                      `json:"cancel_requested"`
	Usage                        *httpDispatchUsage        `json:"usage,omitempty"`
	RequestedModel               string                    `json:"requested_model,omitempty"`
	EffectiveModel               string                    `json:"effective_model,omitempty"`
	ModelSubstituted             bool                      `json:"model_substituted"`
}

type httpDispatchFailure struct {
	Class             string `json:"class"`
	DetailDisposition string `json:"detail_disposition"`
}

type httpDispatchArtifactFact struct {
	SHA256        string `json:"sha256"`
	Size          int64  `json:"size_bytes"`
	BytesVerified bool   `json:"bytes_verified"`
}

// Terminal usage is cumulative for this dispatch. A nil field means unknown.
type httpDispatchUsage struct {
	Provider          string            `json:"provider,omitempty"`
	Model             string            `json:"model,omitempty"`
	Effort            string            `json:"effort,omitempty"`
	InputTokens       *int64            `json:"input_tokens,omitempty"`
	CachedInputTokens *int64            `json:"cached_input_tokens,omitempty"`
	OutputTokens      *int64            `json:"output_tokens,omitempty"`
	WallTimeMS        *int64            `json:"wall_time_ms,omitempty"`
	Cost              *httpDispatchCost `json:"cost,omitempty"`
}

type httpDispatchCost struct {
	Amount   string `json:"amount"`
	Currency string `json:"currency"`
}

type httpDispatchReceiptAdmissionWire struct {
	httpDispatchCorrelation
	ReceiptID string `json:"receipt_id"`
	Execution struct {
		State     string     `json:"state"`
		Attempts  int        `json:"attempts"`
		StartedAt *time.Time `json:"started_at"`
		EndedAt   time.Time  `json:"ended_at"`
		Failure   *struct {
			Class string `json:"class"`
		} `json:"failure"`
	} `json:"execution"`
	Repository struct {
		httpDispatchRepository
		ResultCommit *string `json:"result_commit"`
		ResultRef    string  `json:"result_ref"`
	} `json:"repository"`
	Evidence struct {
		Read     []string              `json:"files_read"`
		Edited   []string              `json:"files_edited"`
		Complete bool                  `json:"paths_complete"`
		Manifest *httpDispatchArtifact `json:"paths_manifest"`
	} `json:"evidence"`
	Report struct {
		State    string                `json:"state"`
		Artifact *httpDispatchArtifact `json:"artifact"`
	} `json:"report"`
	Verification struct {
		State      string                `json:"state"`
		Strength   string                `json:"strength"`
		TestOutput *httpDispatchArtifact `json:"test_output"`
	} `json:"verification"`
	Cancellation struct {
		Supported bool `json:"supported"`
		Requested bool `json:"requested"`
	} `json:"cancellation"`
	Usage        *httpDispatchUsage `json:"usage"`
	Substitution *struct {
		Requested string `json:"requested_model"`
		Effective string `json:"effective_model"`
	} `json:"model_substitution"`
}

// The caller supplies the accepted handle and terminal observation from the
// same authorized endpoint, a trusted local checkout, and repo-relative roots.
func admitHTTPDispatchReceipt(ctx context.Context, runner CommandRunner, checkout string, frozen []byte, handle *httpDispatchHandle, status *httpDispatchStatus, raw []byte, protectedPaths []string) (*httpDispatchAdmittedReceipt, error) {
	if len(frozen) > httpDispatchRequestBody || len(raw) > httpDispatchReceiptBody {
		return nil, errHTTPDispatchLimit
	}
	var request struct {
		httpDispatchRequest
		CreatedAt time.Time                `json:"created_at"`
		Options   httpDispatchAgentOptions `json:"options"`
	}
	if !decodeHTTPDispatch(frozen, "Request", &request) {
		return nil, errHTTPDispatchRequest
	}
	inline, external, valid := request.validateInput()
	if !valid || inline > httpDispatchMaxInline || external > 32<<20 {
		return nil, errHTTPDispatchRequest
	}
	var wire httpDispatchReceiptAdmissionWire
	if !decodeHTTPDispatch(raw, "Receipt", &wire) {
		return nil, errHTTPDispatchResponse
	}
	if handle == nil || status == nil || handle.DispatchID != request.DispatchID ||
		handle.RequestSHA256 != httpDispatchDigest(frozen) || wire.Repository.httpDispatchRepository != request.Repository ||
		wire.httpDispatchCorrelation != handle.httpDispatchCorrelation || status.httpDispatchCorrelation != handle.httpDispatchCorrelation {
		return nil, errHTTPDispatchCorrelation
	}
	execution := wire.Execution
	if request.CreatedAt.IsZero() || handle.AcceptedAt.IsZero() || handle.AcceptedAt.Before(request.CreatedAt) ||
		!terminalHTTPDispatchState(status.State) || status.State != execution.State ||
		status.CancelRequested != wire.Cancellation.Requested ||
		execution.EndedAt.Before(handle.AcceptedAt) || execution.EndedAt.After(status.ObservedAt) ||
		(execution.State == "succeeded" && execution.EndedAt.Sub(handle.AcceptedAt) > time.Duration(request.Requirements.TimeoutSeconds)*time.Second) ||
		execution.Attempts > request.Requirements.MaxAttempts || (execution.StartedAt == nil) != (execution.Attempts == 0) ||
		(execution.StartedAt != nil && (execution.StartedAt.Before(handle.AcceptedAt) || execution.StartedAt.After(execution.EndedAt))) {
		return nil, errHTTPDispatchCorrelation
	}
	if (wire.Cancellation.Requested || request.Requirements.Cancellation == "required") && !wire.Cancellation.Supported {
		return nil, errHTTPDispatchResponse
	}
	if !wire.Evidence.Manifest.validInline() || !wire.Report.Artifact.validInline() || !wire.Verification.TestOutput.validInline() {
		return nil, errHTTPDispatchResponse
	}
	if !validHTTPDispatchStatusUsage(status) {
		return nil, errHTTPDispatchResponse
	}
	effective, err := wire.normalizeUsage(request.Options.Model)
	if err != nil {
		return nil, err
	}
	if err := wire.verifyPaths(ctx, runner, checkout, protectedPaths); err != nil {
		return nil, err
	}
	admitted := &httpDispatchAdmittedReceipt{
		DispatchID: wire.DispatchID, WireSHA256: httpDispatchDigest(raw), RequestSHA256: wire.RequestSHA256,
		ReceiptID: wire.ReceiptID, SourceCommit: wire.Repository.SourceCommit, ResultCommit: wire.Repository.ResultCommit,
		State: execution.State, Attempts: execution.Attempts, EndedAt: execution.EndedAt.UTC(),
		ProviderPathsComplete: wire.Evidence.Complete, PathClaimsDisposition: "omitted",
		PathsManifest:  projectHTTPDispatchArtifact(wire.Evidence.Manifest),
		ReportRequired: request.Requirements.Report.Required, ReportState: wire.Report.State, ReportArtifact: projectHTTPDispatchArtifact(wire.Report.Artifact),
		RequiredVerificationStrength: request.Requirements.Verification.Strength,
		VerificationState:            wire.Verification.State, VerificationStrength: wire.Verification.Strength,
		TestOutput: projectHTTPDispatchArtifact(wire.Verification.TestOutput), CancelRequested: wire.Cancellation.Requested,
		Usage: wire.Usage, RequestedModel: safeHTTPDispatchUsageLabel(request.Options.Model),
		EffectiveModel: safeHTTPDispatchUsageLabel(effective), ModelSubstituted: wire.Substitution != nil,
	}
	if execution.StartedAt != nil {
		started := execution.StartedAt.UTC()
		admitted.StartedAt = &started
	}
	if execution.Failure != nil {
		// The draft accepts free-form classes. Retain only known labels.
		class := execution.Failure.Class
		switch class {
		case "cancelled", "timeout", "execution_failed", "verification_failed", "provider_error":
		default:
			class = "other"
		}
		admitted.Failure = &httpDispatchFailure{Class: class, DetailDisposition: "omitted"}
	}
	return admitted, nil
}

func projectHTTPDispatchArtifact(a *httpDispatchArtifact) *httpDispatchArtifactFact {
	if a == nil {
		return nil
	}
	return &httpDispatchArtifactFact{SHA256: a.SHA256, Size: a.Size, BytesVerified: a.Kind == "inline"}
}

// The terminal status can carry its own cumulative snapshot. Validate it
// separately; it is neither a source of defaults nor an increment to usage.
func validHTTPDispatchStatusUsage(status *httpDispatchStatus) bool {
	if len(status.Usage) == 0 {
		return true
	}
	if len(status.Usage) > httpDispatchSmallBody {
		return false
	}
	raw, err := json.Marshal(struct {
		Schema        string `json:"schema"`
		SchemaVersion int    `json:"schema_version"`
		*httpDispatchStatus
	}{"devspecs.dispatch.status", 2, status})
	var observation struct {
		Usage *httpDispatchUsage `json:"usage"`
	}
	return err == nil && len(raw) <= httpDispatchSmallBody && decodeHTTPDispatch(raw, "Status", &observation) && observation.Usage.validTokenTotals()
}

func (u *httpDispatchUsage) validTokenTotals() bool {
	return u == nil || u.InputTokens == nil || u.CachedInputTokens == nil || *u.CachedInputTokens <= *u.InputTokens
}

func (wire *httpDispatchReceiptAdmissionWire) normalizeUsage(requested string) (string, error) {
	var effective string
	if u := wire.Usage; u != nil {
		if !u.validTokenTotals() {
			return "", errHTTPDispatchResponse
		}
		effective = u.Model
		u.Provider, u.Model, u.Effort = safeHTTPDispatchUsageLabel(u.Provider), safeHTTPDispatchUsageLabel(u.Model), safeHTTPDispatchUsageLabel(u.Effort)
	}
	if sub := wire.Substitution; sub != nil {
		if sub.Requested != requested || sub.Effective == requested || (effective != "" && effective != sub.Effective) {
			return "", errHTTPDispatchCorrelation
		}
		effective = sub.Effective
	} else if requested != "" && effective != "" && effective != requested {
		return "", errHTTPDispatchCorrelation
	}
	return effective, nil
}

// Project only identifier-shaped values. This is not a secret detector.
func safeHTTPDispatchUsageLabel(label string) string {
	if !httpDispatchUsageLabel.MatchString(label) {
		return ""
	}
	return label
}

func (wire *httpDispatchReceiptAdmissionWire) verifyPaths(ctx context.Context, runner CommandRunner, checkout string, protected []string) error {
	roots := []string{"devspecs", ".devspecs"}
	for _, root := range protected {
		if !validHTTPDispatchRepoPath(root) {
			return errHTTPDispatchPaths
		}
		roots = append(roots, strings.ToLower(root))
	}
	edited := make(map[string]bool, len(wire.Evidence.Edited))
	paths := make(map[string]bool)
	for _, name := range wire.Evidence.Edited {
		if !validHTTPDispatchRepoPath(name) {
			return errHTTPDispatchPaths
		}
		if protectedHTTPDispatchPath(name, roots) {
			return errHTTPDispatchTask
		}
		edited[name], paths[name] = true, true
	}
	for _, name := range wire.Evidence.Read {
		if !validHTTPDispatchRepoPath(name) {
			return errHTTPDispatchPaths
		}
		paths[name] = true
	}
	repo := wire.Repository
	if repo.ResultRef != "" {
		if runner == nil || checkout == "" {
			return errHTTPDispatchResult
		}
		// Syntax only. Never resolve, compare against, or persist the label.
		if _, err := runGitBytes(ctx, runner, checkout, "check-ref-format", repo.ResultRef); err != nil {
			return errHTTPDispatchResponse
		}
	}
	if repo.ResultCommit == nil {
		return nil // No committed result is claimed; paths remain provider evidence.
	}
	return verifyHTTPDispatchResult(ctx, runner, checkout, repo.SourceCommit, *repo.ResultCommit, roots, edited, paths, wire.Evidence.Complete)
}

func validHTTPDispatchRepoPath(name string) bool {
	if name == "" || len(name) > 4096 || !utf8.ValidString(name) || strings.ContainsAny(name, `\:<>"|?*`) ||
		strings.ContainsFunc(name, func(r rune) bool { return unicode.IsControl(r) || unicode.Is(unicode.Cf, r) }) {
		return false
	}
	for _, part := range strings.Split(name, "/") {
		if part == "" || strings.HasSuffix(part, ".") || strings.HasSuffix(part, " ") ||
			strings.EqualFold(part, ".git") || httpDispatchDOSAlias.MatchString(part) {
			return false
		}
	}
	return true
}

func protectedHTTPDispatchPath(name string, roots []string) bool {
	name = strings.ToLower(name)
	for _, root := range roots {
		if name == root || strings.HasPrefix(name, root+"/") || strings.HasPrefix(root, name+"/") {
			return true
		}
	}
	return false
}

func verifyHTTPDispatchResult(ctx context.Context, runner CommandRunner, checkout, source, result string, roots []string, edited, paths map[string]bool, complete bool) error {
	if runner == nil || checkout == "" {
		return errHTTPDispatchResult
	}
	// Older Git can lazy-fetch missing objects with no opt-out flag. Refuse
	// promisor config before issuing any object lookup in the trusted checkout.
	config, err := runner.Run(ctx, checkout, "git", "config", "--get-regexp", `^(extensions\.partialclone|remote\..*\.promisor)$`)
	if err != nil || config.ExitCode != 1 || len(config.Stdout) != 0 || len(config.Stderr) != 0 {
		return errHTTPDispatchResult
	}
	git := func(args ...string) ([]byte, error) {
		return runGitBytes(ctx, runner, checkout, append([]string{"--no-replace-objects", "--literal-pathspecs"}, args...)...)
	}
	format, err := git("rev-parse", "--show-object-format")
	width := map[string]int{"sha1": 40, "sha256": 64}[strings.TrimSpace(string(format))]
	if err != nil || width == 0 || len(source) != width || len(result) != width {
		return errHTTPDispatchResult
	}
	grafts, err := git("rev-parse", "--path-format=absolute", "--git-path", "info/grafts")
	graftPath := strings.TrimRight(string(grafts), "\r\n")
	if err != nil || !filepath.IsAbs(graftPath) || os.Getenv("GIT_GRAFT_FILE") != "" {
		return errHTTPDispatchResult
	}
	if _, err := os.Lstat(graftPath); !os.IsNotExist(err) {
		return errHTTPDispatchResult
	}
	for _, commit := range []string{source, result} {
		resolved, err := git("rev-parse", "--verify", "--end-of-options", commit+"^{commit}")
		if err != nil || strings.TrimSpace(string(resolved)) != commit {
			return errHTTPDispatchResult
		}
	}
	if _, err := git("merge-base", "--is-ancestor", source, result); err != nil {
		return errHTTPDispatchResult
	}
	if source != result {
		// A final-tree diff cannot see a protected edit that was later reverted.
		args := []string{"--no-replace-objects", "rev-list", "--full-history", "--max-count=1", source + ".." + result, "--"}
		for _, root := range roots {
			args = append(args, ":(icase,literal)"+root)
		}
		history, err := runGitBytes(ctx, runner, checkout, args...)
		if err != nil {
			return errHTTPDispatchResult
		}
		if len(history) != 0 {
			return errHTTPDispatchTask
		}
	}
	diff, err := git("diff-tree", "-r", "--no-commit-id", "--no-renames", "--no-ext-diff", "--no-textconv", "--no-relative", "--ignore-submodules=none", "--raw", "-z", source, result, "--")
	if err != nil {
		return errHTTPDispatchResult
	}
	missing := false
	for len(diff) != 0 {
		header, rest, headerOK := bytes.Cut(diff, []byte{0})
		file, rest, fileOK := bytes.Cut(rest, []byte{0})
		diff = rest
		fields, name := strings.Fields(string(header)), string(file)
		if !headerOK || !fileOK || len(fields) != 5 || !validHTTPDispatchRepoPath(name) {
			return errHTTPDispatchPaths
		}
		if protectedHTTPDispatchPath(name, roots) {
			return errHTTPDispatchTask
		}
		if unsafeHTTPDispatchMode(strings.TrimPrefix(fields[0], ":")) || unsafeHTTPDispatchMode(fields[1]) {
			return errHTTPDispatchPaths
		}
		missing = missing || (complete && !edited[name])
	}
	if missing {
		return errHTTPDispatchPaths
	}
	if len(paths) == 0 {
		return nil
	}
	// Inspect only immutable tree entries at the claimed paths or their
	// ancestors. Do not open claimed paths or follow working-tree symlinks.
	var names []string
	seen := make(map[string]bool)
	for name := range paths {
		for name != "." && !seen[name] {
			seen[name] = true
			names = append(names, name)
			name = path.Dir(name)
		}
	}
	for _, commit := range []string{source, result} {
		tree, err := git(append([]string{"ls-tree", "--full-tree", "-t", "-z", commit, "--"}, names...)...)
		if err != nil {
			return errHTTPDispatchResult
		}
		for len(tree) != 0 {
			entry, rest, ok := bytes.Cut(tree, []byte{0})
			tree = rest
			info, name, tab := bytes.Cut(entry, []byte{'\t'})
			mode, _, space := bytes.Cut(info, []byte{' '})
			if !ok || !tab || !space || (seen[string(name)] && unsafeHTTPDispatchMode(string(mode))) {
				return errHTTPDispatchPaths
			}
		}
	}
	return nil
}

func unsafeHTTPDispatchMode(mode string) bool {
	return mode == "120000" || mode == "160000"
}
