package scan

import (
	"os"
	"path/filepath"
	"sort"

	"github.com/devspecs-com/devspecs-cli/internal/adapters"
	"github.com/devspecs-com/devspecs-cli/internal/classify"
)

const classifierExtractKey = "classifier"

func attachClassifierMetadata(repoRoot string, c adapters.Candidate, art adapters.Artifact) adapters.Artifact {
	cfg := classify.DefaultPipelineConfig()
	body := readCandidateBody(c.PrimaryPath, art.Body)
	relPath := filepath.ToSlash(c.RelPath)
	resolution := classify.ClassifyCandidate(classify.Candidate{
		Path:  relPath,
		Scope: classify.ScopeDocument,
		Body:  body,
	}, cfg)

	if art.Extracted == nil {
		art.Extracted = map[string]any{}
	}
	payload := map[string]any{
		"evaluator":       classify.EvaluatorDeclarativeDocumentModelsV0,
		"profile":         cfg.Profile,
		"config_version":  cfg.Version,
		"discovery_score": c.DiscoveryScore,
		"discovery_reasons": append([]string(nil),
			c.DiscoveryReasons...),
		"input_scope":      string(classify.ScopeDocument),
		"input_path":       relPath,
		"winner":           classificationPayload(resolution.Winner),
		"alternatives":     classificationAlternativesPayload(resolution.Alternatives),
		"ambiguous":        resolution.Ambiguous,
		"fallback_generic": resolution.FallbackGeneric,
	}
	if c.AdapterName == "openspec" && art.LayoutGroup != "" {
		container := classify.ClassifyCandidate(classify.Candidate{
			Path:            filepath.ToSlash(art.LayoutGroup),
			Scope:           classify.ScopeContainer,
			ChildCandidates: openspecContainerChildren(repoRoot, art.LayoutGroup),
		}, cfg)
		payload["container"] = map[string]any{
			"input_scope":      string(classify.ScopeContainer),
			"input_path":       filepath.ToSlash(art.LayoutGroup),
			"winner":           classificationPayload(container.Winner),
			"alternatives":     classificationAlternativesPayload(container.Alternatives),
			"ambiguous":        container.Ambiguous,
			"fallback_generic": container.FallbackGeneric,
		}
	}
	art.Extracted[classifierExtractKey] = payload
	return art
}

func readCandidateBody(path, fallback string) string {
	if path == "" {
		return fallback
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fallback
	}
	return string(data)
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func classificationPayload(cl classify.Classification) map[string]any {
	return map[string]any{
		"classifier":       cl.Classifier,
		"scope":            cl.Scope,
		"subformat":        cl.Subformat,
		"family":           cl.Family,
		"accepted":         cl.Accepted,
		"confidence":       cl.Confidence,
		"kind":             cl.Kind,
		"subtype":          cl.Subtype,
		"status":           cl.Status,
		"lifecycle":        cl.Lifecycle,
		"authority":        cl.Authority,
		"format_profile":   cl.FormatProfile,
		"layout_group":     cl.LayoutGroup,
		"positive_reasons": cl.PositiveReasons,
		"negative_reasons": cl.NegativeReasons,
		"child_candidates": classifyCandidatePayloads(cl.ChildCandidates),
	}
}

func classificationAlternativesPayload(alternatives []classify.Classification) []map[string]any {
	out := make([]map[string]any, 0, len(alternatives))
	for _, alternative := range alternatives {
		out = append(out, map[string]any{
			"classifier": alternative.Classifier,
			"confidence": alternative.Confidence,
			"accepted":   alternative.Accepted,
			"subformat":  alternative.Subformat,
			"family":     alternative.Family,
			"authority":  alternative.Authority,
		})
	}
	return out
}

func classifyCandidatePayloads(candidates []classify.Candidate) []map[string]any {
	out := make([]map[string]any, 0, len(candidates))
	for _, candidate := range candidates {
		out = append(out, map[string]any{
			"path": candidate.Path,
			"role": candidate.Role,
		})
	}
	return out
}

func openspecContainerChildren(repoRoot, layoutGroup string) []classify.Candidate {
	layoutGroup = filepath.ToSlash(layoutGroup)
	childSpecs := []struct {
		path string
		role string
	}{
		{filepath.ToSlash(filepath.Join(layoutGroup, "proposal.md")), "proposal"},
		{filepath.ToSlash(filepath.Join(layoutGroup, "design.md")), "design"},
		{filepath.ToSlash(filepath.Join(layoutGroup, "tasks.md")), "tasks"},
	}
	var out []classify.Candidate
	for _, spec := range childSpecs {
		if fileExists(filepath.Join(repoRoot, filepath.FromSlash(spec.path))) {
			out = append(out, classify.Candidate{Path: spec.path, Scope: classify.ScopeDocument, Role: spec.role})
		}
	}
	specsDir := filepath.Join(repoRoot, filepath.FromSlash(layoutGroup), "specs")
	_ = filepath.WalkDir(specsDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || d.Name() != "spec.md" {
			return nil
		}
		rel, relErr := filepath.Rel(repoRoot, path)
		if relErr != nil {
			return nil
		}
		out = append(out, classify.Candidate{
			Path:  filepath.ToSlash(rel),
			Scope: classify.ScopeDocument,
			Role:  "spec_delta",
		})
		return nil
	})
	sort.Slice(out, func(i, j int) bool {
		if out[i].Role == out[j].Role {
			return out[i].Path < out[j].Path
		}
		return out[i].Role < out[j].Role
	})
	return out
}
