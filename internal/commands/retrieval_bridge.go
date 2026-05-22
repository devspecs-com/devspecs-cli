package commands

import (
	"encoding/json"
	"fmt"
	"math"
	"path/filepath"
	"strings"

	"github.com/devspecs-com/devspecs-cli/internal/retrieval"
	"github.com/devspecs-com/devspecs-cli/internal/store"
)

const commandTokenCounterName = "approx_chars_div_4"

func loadRetrievalCandidates(db *store.DB, fp store.FilterParams) ([]retrieval.Candidate, error) {
	artifacts, err := db.ListArtifacts(fp)
	if err != nil {
		return nil, err
	}
	candidates := make([]retrieval.Candidate, 0, len(artifacts))
	for _, art := range artifacts {
		sources, _ := db.GetSourcesForArtifact(art.ID)
		links, _ := db.GetLinksForArtifact(art.ID)
		todos, _ := db.GetTodosForArtifact(art.ID)
		var body string
		var extractedJSON string
		if art.CurrentRevID != "" {
			if rev, err := db.GetRevision(art.CurrentRevID); err == nil && rev != nil {
				body = rev.Body
				extractedJSON = rev.ExtractedJSON
			}
		}
		candidates = append(candidates, artifactCandidateWithLinks(art, sources, links, todos, body, extractedJSON))
	}
	return candidates, nil
}

func artifactCandidate(art store.ArtifactRow, sources []store.SourceRow, todos []store.TodoRow, body, extractedJSON string) retrieval.Candidate {
	return artifactCandidateWithLinks(art, sources, nil, todos, body, extractedJSON)
}

func artifactCandidateWithLinks(art store.ArtifactRow, sources []store.SourceRow, links []store.LinkRow, todos []store.TodoRow, body, extractedJSON string) retrieval.Candidate {
	sourcePath := firstSourcePath(sources)
	path := candidatePathFromSources(sources)
	if path == "" {
		path = art.Title
	}
	if path == "" {
		path = art.ID
	}
	metadata := map[string]string{
		"repo_id":              art.RepoID,
		"short_id":             art.ShortID,
		"current_revision_id":  art.CurrentRevID,
		"created_at":           art.CreatedAt,
		"updated_at":           art.UpdatedAt,
		"last_observed_at":     art.LastObservedAt,
		"token_counter":        commandTokenCounterName,
		"retrieval_candidate":  "sqlite_artifact",
		"source_context_scope": "indexed_artifacts",
	}
	for key, value := range classifierCandidateMetadata(extractedJSON) {
		metadata[key] = value
	}
	for key, value := range artifactExtractedCandidateMetadata(extractedJSON) {
		metadata[key] = value
	}
	for key, value := range linkCandidateMetadata(links) {
		metadata[key] = value
	}
	return retrieval.Candidate{
		ID:       art.ID,
		Path:     filepath.ToSlash(path),
		Kind:     art.Kind,
		Subtype:  art.Subtype,
		Title:    art.Title,
		Status:   art.Status,
		Source:   filepath.ToSlash(sourcePath),
		Body:     renderRetrievalCandidateBody(art, sources, todos, body),
		Metadata: metadata,
	}
}

func candidatePathFromSources(sources []store.SourceRow) string {
	for _, src := range sources {
		if strings.TrimSpace(src.Path) == "" {
			continue
		}
		path := filepath.ToSlash(src.Path)
		if src.SourceType != "test_case" {
			return path
		}
		parts := strings.Split(src.SourceIdentity, "|")
		if len(parts) >= 3 && strings.TrimSpace(parts[2]) != "" {
			return path + "#L" + strings.TrimSpace(parts[2])
		}
		return path + "#test-case"
	}
	return ""
}

func artifactExtractedCandidateMetadata(extractedJSON string) map[string]string {
	if strings.TrimSpace(extractedJSON) == "" {
		return nil
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(extractedJSON), &payload); err != nil {
		return nil
	}
	keys := []string{
		"mode",
		"role",
		"artifact_scope",
		"source_standard",
		"openspec_role",
		"openspec_base_path",
		"openspec_change_id",
		"openspec_capability",
		"layout_group",
		"language",
		"framework",
		"source_line_range",
		"test_name",
		"parent_title",
	}
	out := map[string]string{}
	for _, key := range keys {
		value, ok := payload[key]
		if !ok {
			continue
		}
		text := strings.TrimSpace(fmt.Sprint(value))
		if text != "" {
			out[key] = text
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func linkCandidateMetadata(links []store.LinkRow) map[string]string {
	if len(links) == 0 {
		return nil
	}
	grouped := map[string][]string{}
	for _, link := range links {
		linkType := strings.TrimSpace(link.LinkType)
		target := strings.TrimSpace(link.Target)
		if linkType == "" || target == "" {
			continue
		}
		grouped[linkType] = append(grouped[linkType], target)
	}
	if len(grouped) == 0 {
		return nil
	}
	out := map[string]string{}
	for linkType, targets := range grouped {
		out["link_"+strings.ReplaceAll(linkType, "-", "_")] = strings.Join(uniqueStrings(targets), "\n")
	}
	return out
}

func classifierCandidateMetadata(extractedJSON string) map[string]string {
	if strings.TrimSpace(extractedJSON) == "" {
		return nil
	}
	var payload struct {
		Classifier struct {
			Evaluator       string `json:"evaluator"`
			Profile         string `json:"profile"`
			ConfigVersion   int    `json:"config_version"`
			Ambiguous       bool   `json:"ambiguous"`
			FallbackGeneric bool   `json:"fallback_generic"`
			Winner          struct {
				Classifier    string  `json:"classifier"`
				Subformat     string  `json:"subformat"`
				Family        string  `json:"family"`
				Confidence    float64 `json:"confidence"`
				Mode          string  `json:"mode"`
				Kind          string  `json:"kind"`
				Subtype       string  `json:"subtype"`
				Status        string  `json:"status"`
				Lifecycle     string  `json:"lifecycle"`
				Authority     string  `json:"authority"`
				FormatProfile string  `json:"format_profile"`
			} `json:"winner"`
		} `json:"classifier"`
	}
	if err := json.Unmarshal([]byte(extractedJSON), &payload); err != nil {
		return nil
	}
	if payload.Classifier.Winner.Classifier == "" {
		return nil
	}
	out := map[string]string{
		"classifier_evaluator":        payload.Classifier.Evaluator,
		"classifier_profile":          payload.Classifier.Profile,
		"classifier_config_version":   fmt.Sprintf("%d", payload.Classifier.ConfigVersion),
		"classifier_model":            payload.Classifier.Winner.Classifier,
		"classifier_confidence":       fmt.Sprintf("%.3f", payload.Classifier.Winner.Confidence),
		"classifier_mode":             payload.Classifier.Winner.Mode,
		"classifier_ambiguous":        fmt.Sprintf("%t", payload.Classifier.Ambiguous),
		"classifier_fallback_generic": fmt.Sprintf("%t", payload.Classifier.FallbackGeneric),
		"classifier_kind":             payload.Classifier.Winner.Kind,
		"classifier_subtype":          payload.Classifier.Winner.Subtype,
		"classifier_status":           payload.Classifier.Winner.Status,
		"classifier_subformat":        payload.Classifier.Winner.Subformat,
		"classifier_family":           payload.Classifier.Winner.Family,
		"classifier_lifecycle":        payload.Classifier.Winner.Lifecycle,
		"classifier_authority":        payload.Classifier.Winner.Authority,
		"classifier_format_profile":   payload.Classifier.Winner.FormatProfile,
	}
	for key, value := range out {
		if value == "" {
			delete(out, key)
		}
	}
	return out
}

func renderRetrievalCandidateBody(art store.ArtifactRow, sources []store.SourceRow, todos []store.TodoRow, body string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Title: %s\n", art.Title)
	fmt.Fprintf(&b, "Kind: %s\n", art.Kind)
	if art.Subtype != "" {
		fmt.Fprintf(&b, "Subtype: %s\n", art.Subtype)
	}
	fmt.Fprintf(&b, "Status: %s\n", art.Status)
	for _, src := range sources {
		if src.Path != "" {
			fmt.Fprintf(&b, "Source: %s\n", filepath.ToSlash(src.Path))
		}
		if src.FormatProfile != "" {
			fmt.Fprintf(&b, "Format profile: %s\n", src.FormatProfile)
		}
		if src.LayoutGroup != "" {
			fmt.Fprintf(&b, "Layout group: %s\n", src.LayoutGroup)
		}
	}
	if len(todos) > 0 {
		fmt.Fprintln(&b, "\nTasks:")
		for _, td := range todos {
			marker := "[ ]"
			if td.Done {
				marker = "[x]"
			}
			fmt.Fprintf(&b, "- %s %s\n", marker, td.Text)
		}
	}
	if strings.TrimSpace(body) != "" {
		fmt.Fprintf(&b, "\n%s", strings.TrimRight(body, "\r\n"))
	}
	return b.String()
}

func firstSourcePath(sources []store.SourceRow) string {
	for _, src := range sources {
		if strings.TrimSpace(src.Path) != "" {
			return filepath.ToSlash(src.Path)
		}
	}
	return ""
}

func approximateTokenCount(text string) int {
	if text == "" {
		return 0
	}
	return int(math.Ceil(float64(len(text)) / 4.0))
}

func capCandidates(candidates []retrieval.Candidate, limit int) []retrieval.Candidate {
	if limit <= 0 || len(candidates) <= limit {
		return candidates
	}
	return candidates[:limit]
}

func shortCandidateID(c retrieval.Candidate) string {
	if c.Metadata != nil && c.Metadata["short_id"] != "" {
		return c.Metadata["short_id"]
	}
	if len(c.ID) > 8 {
		return c.ID[:8]
	}
	return c.ID
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}
