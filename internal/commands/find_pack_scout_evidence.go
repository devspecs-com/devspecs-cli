package commands

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/devspecs-com/devspecs-cli/internal/retrieval"
)

const (
	findPackScoutBodyEvidenceMaxBytes = 64 * 1024
	findPackScoutBodyEvidencePrefix   = "bounded body evidence:"
)

var findPackScoutTokenRE = regexp.MustCompile(`[A-Za-z0-9_]{3,}`)

func addFindPackScoutBodyEvidence(repoRoot, query string, rolePack retrieval.RoleGroupedPack) retrieval.RoleGroupedPack {
	repoRoot = strings.TrimSpace(repoRoot)
	if repoRoot == "" {
		return rolePack
	}
	queryTerms := findPackScoutBodyEvidenceTerms(query)
	if len(queryTerms) == 0 {
		return rolePack
	}
	totalBytes := 0
	evidenceCount := 0
	for groupIdx := range rolePack.Groups {
		for itemIdx := range rolePack.Groups[groupIdx].Items {
			item := &rolePack.Groups[groupIdx].Items[itemIdx]
			if !packItemHasReasonPrefix(*item, "scout source rescue:") || item.Path == "" {
				continue
			}
			evidence, bytesRead := findPackScoutBodyEvidence(repoRoot, item.Path, queryTerms)
			totalBytes += bytesRead
			if evidence == "" {
				continue
			}
			item.Reasons = appendUniqueString(item.Reasons, evidence)
			evidenceCount++
		}
	}
	if evidenceCount == 0 && totalBytes == 0 {
		return rolePack
	}
	if rolePack.Metadata == nil {
		rolePack.Metadata = map[string]string{}
	}
	rolePack.Metadata["pack_scout_body_evidence_count"] = strconv.Itoa(evidenceCount)
	rolePack.Metadata["pack_scout_body_evidence_bytes"] = strconv.Itoa(totalBytes)
	return rolePack
}

func findPackScoutBodyEvidence(repoRoot, itemPath string, queryTerms []string) (string, int) {
	cleanPath := filepath.FromSlash(strings.SplitN(itemPath, "#", 2)[0])
	fullPath := filepath.Clean(filepath.Join(repoRoot, cleanPath))
	repoClean := filepath.Clean(repoRoot)
	rel, err := filepath.Rel(repoClean, fullPath)
	if err != nil || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
		return "", 0
	}
	file, err := os.Open(fullPath)
	if err != nil {
		return "", 0
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, findPackScoutBodyEvidenceMaxBytes))
	if err != nil {
		return "", 0
	}
	text := strings.ToLower(string(data))
	hits := map[string]int{}
	for _, term := range queryTerms {
		if count := strings.Count(text, term); count > 0 {
			hits[term] = count
		}
	}
	if len(hits) == 0 {
		return "", len(data)
	}
	keys := make([]string, 0, len(hits))
	for key := range hits {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var parts []string
	for _, key := range firstStrings(keys, 4) {
		parts = append(parts, fmt.Sprintf("%s=%d", key, hits[key]))
	}
	return fmt.Sprintf("%s %s; bytes=%d", findPackScoutBodyEvidencePrefix, strings.Join(parts, ", "), len(data)), len(data)
}

func findPackScoutBodyEvidenceTerms(query string) []string {
	seen := map[string]bool{}
	var out []string
	for _, raw := range findPackScoutTokenRE.FindAllString(strings.ToLower(query), -1) {
		term := strings.Trim(raw, "_")
		if len(term) < 3 || isGenericPackReceiptTerm(term) || seen[term] {
			continue
		}
		seen[term] = true
		out = append(out, term)
	}
	return out
}

func packItemHasReasonPrefix(item retrieval.PackItem, prefix string) bool {
	prefix = strings.ToLower(prefix)
	for _, reason := range item.Reasons {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(reason)), prefix) {
			return true
		}
	}
	return false
}
