package scan

import (
	"encoding/json"
	"path/filepath"
	"sort"
	"strings"
)

const (
	linkContains          = "contains"
	linkContainedBy       = "contained_by"
	linkOpenSpecCompanion = "openspec_companion"
	linkUpdates           = "updates"
)

type openSpecLinkArtifact struct {
	id             string
	path           string
	sourceIdentity string
	layoutGroup    string
	basePath       string
	scope          string
	role           string
	changeID       string
	capability     string
}

func (s *Scanner) syncOpenSpecLinks(repoID, now string) error {
	artifacts, err := s.openSpecLinkArtifacts(repoID)
	if err != nil {
		return err
	}
	if len(artifacts) == 0 {
		return nil
	}
	if _, err := s.db.Exec(
		`DELETE FROM links
		 WHERE artifact_id IN (SELECT id FROM artifacts WHERE repo_id = ?)
		   AND link_type IN (?, ?, ?, ?)`,
		repoID, linkContains, linkContainedBy, linkOpenSpecCompanion, linkUpdates,
	); err != nil {
		return err
	}

	collections := map[string]openSpecLinkArtifact{}
	bundles := map[string]openSpecLinkArtifact{}
	childrenByLayout := map[string][]openSpecLinkArtifact{}
	capabilities := map[string]openSpecLinkArtifact{}

	for i := range artifacts {
		artifact := artifacts[i]
		switch artifact.scope {
		case "collection":
			if artifact.basePath != "" {
				collections[artifact.basePath] = artifact
			}
		case "bundle":
			if artifact.layoutGroup != "" {
				bundles[artifact.layoutGroup] = artifact
			}
		default:
			if artifact.role == "capability_spec" {
				if artifact.capability != "" {
					capabilities[openSpecCapabilityKey(artifact)] = artifact
				}
				continue
			}
			if artifact.layoutGroup != "" {
				childrenByLayout[artifact.layoutGroup] = append(childrenByLayout[artifact.layoutGroup], artifact)
			}
		}
	}

	seen := map[string]bool{}
	insert := func(artifactID, linkType, target string) error {
		if artifactID == "" || target == "" {
			return nil
		}
		key := artifactID + "\x00" + linkType + "\x00" + target
		if seen[key] {
			return nil
		}
		seen[key] = true
		return s.db.InsertLink(s.ids.NewWithPrefix("link_"), artifactID, linkType, target, now)
	}

	for _, bundle := range sortedOpenSpecArtifacts(bundles) {
		collection, ok := collections[bundle.basePath]
		if !ok {
			continue
		}
		if err := insert(collection.id, linkContains, artifactTarget(bundle.id)); err != nil {
			return err
		}
		if err := insert(bundle.id, linkContainedBy, artifactTarget(collection.id)); err != nil {
			return err
		}
	}
	for _, capability := range sortedOpenSpecArtifacts(capabilities) {
		collection, ok := collections[capability.basePath]
		if !ok {
			continue
		}
		if err := insert(collection.id, linkContains, artifactTarget(capability.id)); err != nil {
			return err
		}
		if err := insert(capability.id, linkContainedBy, artifactTarget(collection.id)); err != nil {
			return err
		}
	}

	for layoutGroup, bundle := range bundles {
		children := childrenByLayout[layoutGroup]
		for _, child := range children {
			if err := insert(bundle.id, linkContains, artifactTarget(child.id)); err != nil {
				return err
			}
			if err := insert(child.id, linkContainedBy, artifactTarget(bundle.id)); err != nil {
				return err
			}
			if child.role == "spec_delta" && child.capability != "" {
				if capability, ok := capabilities[openSpecCapabilityKey(child)]; ok {
					if err := insert(child.id, linkUpdates, artifactTarget(capability.id)); err != nil {
						return err
					}
				}
			}
		}
		for _, child := range children {
			for _, peer := range children {
				if child.id == peer.id {
					continue
				}
				if !isCoreOpenSpecChild(child.role) && !isCoreOpenSpecChild(peer.role) {
					continue
				}
				if err := insert(child.id, linkOpenSpecCompanion, artifactTarget(peer.id)); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (s *Scanner) openSpecLinkArtifacts(repoID string) ([]openSpecLinkArtifact, error) {
	rows, err := s.db.Query(
		`SELECT a.id, COALESCE(s.path, ''), s.source_identity, COALESCE(s.layout_group, ''), COALESCE(rv.extracted_json, '')
		 FROM artifacts a
		 JOIN sources s ON s.artifact_id = a.id
		 LEFT JOIN artifact_revisions rv ON rv.id = a.current_revision_id
		 WHERE a.repo_id = ? AND s.source_type = 'openspec'
		 ORDER BY a.id, s.path`,
		repoID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	byID := map[string]openSpecLinkArtifact{}
	for rows.Next() {
		var row openSpecLinkArtifact
		var extractedJSON string
		if err := rows.Scan(&row.id, &row.path, &row.sourceIdentity, &row.layoutGroup, &extractedJSON); err != nil {
			return nil, err
		}
		row.applyExtractedJSON(extractedJSON)
		row.inferMissingFields()
		if existing, ok := byID[row.id]; ok {
			if preferOpenSpecSource(row.sourceIdentity, existing.sourceIdentity) {
				byID[row.id] = row
			}
			continue
		}
		byID[row.id] = row
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return sortedOpenSpecArtifacts(byID), nil
}

func (a *openSpecLinkArtifact) applyExtractedJSON(extractedJSON string) {
	if strings.TrimSpace(extractedJSON) == "" {
		return
	}
	var payload struct {
		ArtifactScope      string `json:"artifact_scope"`
		OpenSpecRole       string `json:"openspec_role"`
		OpenSpecChangeID   string `json:"openspec_change_id"`
		OpenSpecCapability string `json:"openspec_capability"`
		OpenSpecBasePath   string `json:"openspec_base_path"`
		LayoutGroup        string `json:"layout_group"`
	}
	if err := json.Unmarshal([]byte(extractedJSON), &payload); err != nil {
		return
	}
	a.scope = payload.ArtifactScope
	a.role = payload.OpenSpecRole
	a.changeID = payload.OpenSpecChangeID
	a.capability = payload.OpenSpecCapability
	a.basePath = filepath.ToSlash(payload.OpenSpecBasePath)
	if a.layoutGroup == "" {
		a.layoutGroup = payload.LayoutGroup
	}
}

func (a *openSpecLinkArtifact) inferMissingFields() {
	path := strings.Trim(filepath.ToSlash(a.path), "/")
	identity := strings.TrimSpace(a.sourceIdentity)
	if a.scope == "" {
		switch {
		case strings.Contains(identity, "|openspec_collection") || path == "openspec" || strings.HasSuffix(path, "/openspec"):
			a.scope = "collection"
		case strings.Contains(identity, "|openspec_bundle") || (strings.Contains(path, "/changes/") && !strings.HasSuffix(path, ".md")):
			a.scope = "bundle"
		default:
			a.scope = "file"
		}
	}
	if a.role == "" {
		switch {
		case a.scope == "collection":
			a.role = "collection"
		case a.scope == "bundle":
			a.role = "change_bundle"
		case strings.HasSuffix(path, "/proposal.md"):
			a.role = "proposal"
		case strings.HasSuffix(path, "/design.md"):
			a.role = "design"
		case strings.HasSuffix(path, "/tasks.md"):
			a.role = "tasks"
		case strings.HasSuffix(path, "/spec.md") && strings.Contains(path, "/changes/"):
			a.role = "spec_delta"
		case strings.HasSuffix(path, "/spec.md"):
			a.role = "capability_spec"
		}
	}
	if a.layoutGroup == "" {
		a.layoutGroup = inferOpenSpecLayoutGroup(path, a.scope, a.role)
	}
	if a.basePath == "" {
		a.basePath = inferOpenSpecBasePath(path, a.scope)
	}
	if a.capability == "" && (a.role == "spec_delta" || a.role == "capability_spec") {
		a.capability = inferOpenSpecCapability(path)
	}
	if a.changeID == "" && strings.Contains(path, "/changes/") {
		a.changeID = inferOpenSpecChangeID(path)
	}
}

func preferOpenSpecSource(candidate, current string) bool {
	candidateBundleSource := strings.Contains(candidate, "|openspec_bundle_source")
	currentBundleSource := strings.Contains(current, "|openspec_bundle_source")
	if candidateBundleSource != currentBundleSource {
		return !candidateBundleSource
	}
	return candidate < current
}

func artifactTarget(id string) string {
	return "artifact:" + id
}

func openSpecCapabilityKey(artifact openSpecLinkArtifact) string {
	return artifact.basePath + "\x00" + artifact.capability
}

func isCoreOpenSpecChild(role string) bool {
	switch role {
	case "proposal", "design", "tasks":
		return true
	default:
		return false
	}
}

func inferOpenSpecLayoutGroup(path, scope, role string) string {
	if path == "" {
		return ""
	}
	switch scope {
	case "collection", "bundle":
		return path
	}
	if role == "capability_spec" {
		return strings.TrimSuffix(path, "/spec.md")
	}
	if idx := strings.Index(path, "/changes/archive/"); idx >= 0 {
		rest := path[idx+len("/changes/archive/"):]
		parts := strings.Split(rest, "/")
		if len(parts) > 0 {
			return path[:idx] + "/changes/archive/" + parts[0]
		}
	}
	if idx := strings.Index(path, "/changes/"); idx >= 0 {
		rest := path[idx+len("/changes/"):]
		parts := strings.Split(rest, "/")
		if len(parts) > 0 {
			return path[:idx] + "/changes/" + parts[0]
		}
	}
	return ""
}

func inferOpenSpecCapability(path string) string {
	idx := strings.Index(path, "/specs/")
	if idx < 0 {
		return ""
	}
	value := path[idx+len("/specs/"):]
	value = strings.TrimSuffix(value, "/spec.md")
	value = strings.TrimSuffix(value, "spec.md")
	return strings.Trim(value, "/")
}

func inferOpenSpecBasePath(path, scope string) string {
	path = strings.Trim(filepath.ToSlash(path), "/")
	if path == "" {
		return ""
	}
	if scope == "collection" {
		return path
	}
	parts := strings.Split(path, "/")
	for i, part := range parts {
		if (part == "changes" || part == "specs") && i > 0 {
			return strings.Join(parts[:i], "/")
		}
	}
	if len(parts) > 0 {
		return parts[0]
	}
	return ""
}

func inferOpenSpecChangeID(path string) string {
	path = filepath.ToSlash(path)
	for _, marker := range []string{"/changes/archive/", "/changes/"} {
		idx := strings.Index(path, marker)
		if idx < 0 {
			continue
		}
		rest := path[idx+len(marker):]
		parts := strings.Split(rest, "/")
		if len(parts) > 0 {
			return parts[0]
		}
	}
	return ""
}

func sortedOpenSpecArtifacts(values map[string]openSpecLinkArtifact) []openSpecLinkArtifact {
	out := make([]openSpecLinkArtifact, 0, len(values))
	for _, value := range values {
		out = append(out, value)
	}
	sortOpenSpecArtifacts(out)
	return out
}

func sortOpenSpecArtifacts(values []openSpecLinkArtifact) {
	sort.Slice(values, func(i, j int) bool {
		if values[i].path == values[j].path {
			return values[i].id < values[j].id
		}
		return values[i].path < values[j].path
	})
}
