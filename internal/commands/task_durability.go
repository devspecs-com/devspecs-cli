package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var unresolvedComposePlaceholderRE = regexp.MustCompile(`(?i)<(?:describe|state|list|link|name|what|option|driver|goal|context|concern|subject|intended|outcome|tradeoff|system|stakeholder|question|risk|required|observable|primary|explicitly|metric)[^>\n]*>`)

func validateTaskDurabilityCheckpoint(repoRoot, workspace string, manifest taskManifest, target taskSliceArtifact, opts taskCheckpointOptions) (taskCheckpointOptions, error) {
	isCloseout := target.Kind == "closeout" && isTaskSeriesTarget(manifest, target.ID)
	if !isCloseout {
		if opts.DurableRecord != "" || len(opts.DurableArtifacts) > 0 {
			return opts, fmt.Errorf("--durable-record and --durable-artifact apply only to the task-series closeout target %s00", defaultTaskSeries(manifest.Series))
		}
		return opts, nil
	}
	for _, slice := range taskSyncSlices(manifest) {
		if !taskTargetTerminal(slice) {
			return opts, fmt.Errorf("cannot close task track while implementation target %s is non-terminal", slice.ID)
		}
	}
	decision := strings.ToLower(strings.TrimSpace(opts.Decision))
	if decision != "complete" && decision != "completed" {
		return opts, fmt.Errorf("task-series closeout requires --decision complete")
	}
	if !strings.EqualFold(strings.TrimSpace(opts.Stage), "completed") {
		return opts, fmt.Errorf("task-series closeout requires --stage completed")
	}
	switch opts.DurableRecord {
	case "none":
		if len(opts.DurableArtifacts) > 0 {
			return opts, fmt.Errorf("durable record disposition none does not accept --durable-artifact")
		}
		if opts.NextTarget != "" {
			return opts, fmt.Errorf("durable record disposition none does not accept --next-target")
		}
	case "recorded":
		if len(opts.DurableArtifacts) == 0 {
			return opts, fmt.Errorf("durable record disposition recorded requires at least one --durable-artifact")
		}
		if opts.NextTarget != "" {
			return opts, fmt.Errorf("durable record disposition recorded does not accept --next-target")
		}
		artifacts, err := validateDurableArtifacts(repoRoot, workspace, opts.DurableArtifacts)
		if err != nil {
			return opts, err
		}
		opts.DurableArtifacts = artifacts
	case "deferred":
		if len(opts.DurableArtifacts) > 0 {
			return opts, fmt.Errorf("durable record disposition deferred does not accept --durable-artifact")
		}
		if opts.NextTarget == "" {
			return opts, fmt.Errorf("durable record disposition deferred requires --next-target")
		}
	default:
		return opts, fmt.Errorf("task-series closeout requires --durable-record none, recorded, or deferred")
	}
	return opts, nil
}

func validateDurableArtifacts(repoRoot, workspace string, artifacts []string) ([]string, error) {
	var validated []string
	for _, artifact := range artifacts {
		path := strings.TrimSpace(artifact)
		if !filepath.IsAbs(path) {
			path = filepath.Join(repoRoot, filepath.FromSlash(path))
		}
		path = filepath.Clean(path)
		rel, err := filepath.Rel(repoRoot, path)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return nil, fmt.Errorf("durable artifact must stay inside repository %s: %s", repoRoot, artifact)
		}
		if relWorkspace, err := filepath.Rel(workspace, path); err == nil && relWorkspace != ".." && !strings.HasPrefix(relWorkspace, ".."+string(filepath.Separator)) {
			return nil, fmt.Errorf("durable artifact must stay outside the DevSpecs task corpus: %s", filepath.ToSlash(rel))
		}
		info, err := os.Stat(path)
		if err != nil {
			return nil, fmt.Errorf("durable artifact %s: %w", filepath.ToSlash(rel), err)
		}
		if info.IsDir() || !strings.EqualFold(filepath.Ext(path), ".md") {
			return nil, fmt.Errorf("durable artifact must be a Markdown file: %s", filepath.ToSlash(rel))
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read durable artifact %s: %w", filepath.ToSlash(rel), err)
		}
		if unresolvedComposePlaceholderRE.Match(body) {
			return nil, fmt.Errorf("durable artifact still contains unresolved compose placeholders: %s", filepath.ToSlash(rel))
		}
		rel = filepath.ToSlash(rel)
		if !durableArtifactRecognized(rel, string(body)) {
			return nil, fmt.Errorf("durable artifact is not recognized as an ADR, RFC, or PRD: %s", rel)
		}
		validated = appendUniqueString(validated, rel)
	}
	return validated, nil
}

func durableArtifactRecognized(path, body string) bool {
	if composePathMatchesType(path, composeTypeADR) || composePathMatchesType(path, composeTypeRFC) || composePathMatchesType(path, composeTypePRD) {
		return true
	}
	firstHeading := strings.ToLower(strings.TrimSpace(firstMarkdownH1(body)))
	return strings.HasPrefix(firstHeading, "adr-") ||
		strings.HasPrefix(firstHeading, "adr:") ||
		strings.HasPrefix(firstHeading, "rfc:") ||
		strings.HasPrefix(firstHeading, "prd:")
}

func firstMarkdownH1(body string) string {
	for _, line := range strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "# ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "# "))
		}
	}
	return ""
}

func applyTaskDurabilityReview(manifest *taskManifest, target taskSliceArtifact, opts taskCheckpointOptions, now time.Time) {
	if target.Kind != "closeout" || opts.DurableRecord == "" {
		return
	}
	manifest.Durability.Disposition = opts.DurableRecord
	manifest.Durability.Artifacts = append([]string(nil), opts.DurableArtifacts...)
	manifest.Durability.DeferredTarget = ""
	if opts.DurableRecord == "deferred" {
		manifest.Durability.DeferredTarget = opts.NextTarget
	}
	manifest.Durability.UpdatedAt = now.Format(time.RFC3339)
}
