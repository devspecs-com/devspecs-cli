package commands

import (
	"fmt"
	"io"
	"strings"

	"github.com/devspecs-com/devspecs-cli/internal/retrieval"
)

type FindPackOutput struct {
	Query            string                `json:"query"`
	Retriever        string                `json:"retriever"`
	Mode             string                `json:"mode"`
	Summary          retrieval.PackSummary `json:"summary,omitempty"`
	LocalLanguage    []string              `json:"local_language,omitempty"`
	Groups           []retrieval.PackGroup `json:"groups"`
	ExcludedNoise    []retrieval.PackItem  `json:"excluded_noise,omitempty"`
	Counts           map[string]int        `json:"counts,omitempty"`
	RankedResults    []FindResult          `json:"ranked_results"`
	GitTrust         *FindGitTrustContext  `json:"git_trust,omitempty"`
	GraphContext     *FindGraphPackContext `json:"graph_context,omitempty"`
	GraphDiagnostics *FindGraphDiagnostics `json:"graph_diagnostics,omitempty"`
}

func findPackOutput(query, retrieverName string, candidates []retrieval.Candidate, reasons map[string][]string, rolePack retrieval.RoleGroupedPack) FindPackOutput {
	return FindPackOutput{
		Query:         query,
		Retriever:     retrieverName,
		Mode:          rolePack.Mode,
		Summary:       rolePack.Summary,
		LocalLanguage: retrieval.LocalLanguageReceipts(rolePack),
		Groups:        rolePack.Groups,
		ExcludedNoise: rolePack.ExcludedNoise,
		Counts:        rolePack.Counts,
		RankedResults: findResults(candidates, reasons, retrieverName),
	}
}

func writeFindPackText(out io.Writer, query, retrieverName string, rolePack retrieval.RoleGroupedPack, gitTrust *FindGitTrustContext, verbose bool) error {
	fmt.Fprintf(out, "Working set: %s\n", query)
	if verbose {
		fmt.Fprintf(out, "Retriever: %s\n", retrieverName)
		fmt.Fprintf(out, "Mode: %s\n", rolePack.Mode)
	}
	writePackSummary(out, rolePack.Summary)
	boundaryPrimary := retrieval.IsBoundaryPrimaryPack(rolePack) && !verbose

	if len(rolePack.Groups) == 0 && len(rolePack.ExcludedNoise) == 0 {
		fmt.Fprintln(out)
		fmt.Fprintln(out, "No matching artifacts found.")
		return nil
	}
	writePackLocalLanguage(out, rolePack)

	for _, group := range rolePack.Groups {
		if len(group.Items) == 0 {
			continue
		}
		items := group.Items
		relatedCount := 0
		if boundaryPrimary {
			items = make([]retrieval.PackItem, 0, len(group.Items))
			for _, item := range group.Items {
				if retrieval.PackItemIsRelated(item) {
					relatedCount++
					continue
				}
				items = append(items, item)
			}
			if len(items) == 0 {
				continue
			}
		}
		title := group.Title
		if title == "" {
			title = retrieval.PackRoleTitle(group.Role)
		}
		if boundaryPrimary && relatedCount > 0 {
			fmt.Fprintf(out, "\n%s (%d primary, %d related)\n", title, len(items), relatedCount)
		} else {
			fmt.Fprintf(out, "\n%s (%d)\n", title, len(items))
		}
		if group.OverflowCount > 0 && !boundaryPrimary {
			fmt.Fprintf(out, "  Note: %d item(s) over the recommended budget of %d.\n", group.OverflowCount, group.Budget)
		}
		for _, item := range items {
			writePackItem(out, item, false, verbose)
		}
	}

	if boundaryPrimary {
		writeBoundaryPrimarySummary(out, rolePack)
	}

	if len(rolePack.ExcludedNoise) > 0 {
		title := retrieval.PackRoleTitle(retrieval.PackRoleExcludedNoise)
		if !verbose {
			title = "Downgraded as likely noise"
		}
		fmt.Fprintf(out, "\n%s (%d)\n", title, len(rolePack.ExcludedNoise))
		for _, item := range rolePack.ExcludedNoise {
			writePackItem(out, item, true, verbose)
		}
	}
	writeGitTrustText(out, gitTrust)
	return nil
}

func writePackLocalLanguage(out io.Writer, rolePack retrieval.RoleGroupedPack) {
	receipts := retrieval.LocalLanguageReceipts(rolePack)
	if len(receipts) == 0 {
		return
	}
	fmt.Fprintln(out)
	if len(receipts) == 1 {
		fmt.Fprintf(out, "Local language: %s\n", receipts[0])
		return
	}
	fmt.Fprintln(out, "Local language:")
	for _, receipt := range firstStrings(receipts, 3) {
		fmt.Fprintf(out, "- %s\n", receipt)
	}
}

func writeBoundaryPrimarySummary(out io.Writer, rolePack retrieval.RoleGroupedPack) {
	related := retrieval.BoundaryRelatedSummaries(rolePack)
	if len(related) == 0 {
		return
	}
	total := 0
	for _, summary := range related {
		total += summary.Count
	}
	fmt.Fprintf(out, "Related context kept for verbose/JSON: %d artifact(s)\n", total)
	for _, summary := range firstBoundarySummaries(related, 4) {
		label := summary.Title
		if label == "" {
			label = retrieval.PackRoleTitle(summary.Role)
		}
		fmt.Fprintf(out, "  - %s: %d from %s", label, summary.Count, summary.Boundary)
		if len(summary.Examples) > 0 {
			fmt.Fprintf(out, " (%s)", strings.Join(firstStrings(summary.Examples, 2), "; "))
		}
		fmt.Fprintln(out)
	}
	if len(related) > 4 {
		fmt.Fprintf(out, "  - +%d more related group(s)\n", len(related)-4)
	}
}

func firstBoundarySummaries(values []retrieval.PackBoundarySummary, limit int) []retrieval.PackBoundarySummary {
	if limit <= 0 || len(values) <= limit {
		return values
	}
	return values[:limit]
}

func writeGitTrustText(out io.Writer, gitTrust *FindGitTrustContext) {
	if gitTrust == nil || len(gitTrust.Receipts) == 0 {
		return
	}
	fmt.Fprintf(out, "\nRelevant commits (%d)\n", len(gitTrust.Receipts))
	for _, receipt := range gitTrust.Receipts {
		label := receipt.ShortSHA
		if label == "" {
			label = receipt.SHA
		}
		if receipt.CommittedAt != "" {
			fmt.Fprintf(out, "  - %s  %s  %s\n", label, receipt.CommittedAt, receipt.Subject)
		} else {
			fmt.Fprintf(out, "  - %s  %s\n", label, receipt.Subject)
		}
		if receipt.Detail != "" && !strings.EqualFold(receipt.Detail, receipt.Subject) {
			fmt.Fprintf(out, "      %s\n", receipt.Detail)
		}
		if len(receipt.MatchedPaths) > 0 {
			fmt.Fprintf(out, "      touched: %s\n", strings.Join(firstStrings(receipt.MatchedPaths, 3), "; "))
		}
		if len(receipt.MatchedTerms) > 0 {
			fmt.Fprintf(out, "      matched: %s\n", strings.Join(firstStrings(receipt.MatchedTerms, 4), ", "))
		}
		if len(receipt.Signals) > 0 {
			fmt.Fprintf(out, "      signals: %s\n", strings.Join(firstStrings(receipt.Signals, 3), "; "))
		}
	}
}

func writePackSummary(out io.Writer, summary retrieval.PackSummary) {
	if summary.IncludedCount == 0 && summary.ExcludedNoiseCount == 0 && summary.GroupCount == 0 {
		return
	}
	fmt.Fprintf(out, "Summary: %d artifact(s) in %d role(s)", summary.IncludedCount, summary.RoleDiversity)
	if summary.ExcludedNoiseCount > 0 {
		fmt.Fprintf(out, "; %d downgraded as likely noise", summary.ExcludedNoiseCount)
	}
	fmt.Fprintln(out)
	fmt.Fprintf(out, "Coverage: %s\n", packCoverageText(summary))
	if len(summary.Notes) > 0 {
		fmt.Fprintf(out, "Notes: %s\n", strings.Join(limitStrings(summary.Notes, 2), "; "))
	}
}

func writePackItem(out io.Writer, item retrieval.PackItem, excluded, verbose bool) {
	label := item.ShortID
	if label == "" {
		label = item.ID
	}
	if label == "" {
		label = item.Path
	}
	title := strings.TrimSpace(item.Title)
	if title == "" {
		title = item.Path
	}
	if item.PackTier != "" {
		fmt.Fprintf(out, "  %2d. %s  %s [%s]\n", item.OriginalRank, label, title, item.PackTier)
	} else {
		fmt.Fprintf(out, "  %2d. %s  %s\n", item.OriginalRank, label, title)
	}
	if item.Path != "" && shouldShowPackSource(item, verbose) {
		fmt.Fprintf(out, "      Source: %s\n", item.Path)
	}
	if verbose && item.SourcePath != "" && item.SourcePath != item.Path {
		fmt.Fprintf(out, "      From: %s\n", item.SourcePath)
	}
	if verbose && (item.Kind != "" || item.Subtype != "") {
		fmt.Fprintf(out, "      Type: %s\n", compactKindSubtype(item.Kind, item.Subtype))
	}
	if verbose && item.RoleReason != "" {
		prefix := "Why"
		if excluded {
			prefix = "Because"
		}
		fmt.Fprintf(out, "      %s: %s\n", prefix, displayPackRoleReason(item.RoleReason))
	}
	if verbose && len(item.AuthorityCues) > 0 {
		signals, cautions := splitPackCues(item.AuthorityCues)
		if len(signals) > 0 {
			fmt.Fprintf(out, "      Signals: %s\n", strings.Join(limitStrings(signals, 3), "; "))
		}
		if excluded && len(cautions) > 0 {
			fmt.Fprintf(out, "      Caution: %s\n", strings.Join(limitStrings(cautions, 3), "; "))
		}
	}
	if !verbose && excluded && item.RoleReason != "" {
		fmt.Fprintf(out, "      Because: %s\n", displayPackRoleReason(item.RoleReason))
	}
	if !verbose && excluded {
		_, cautions := splitPackCues(item.AuthorityCues)
		if len(cautions) > 0 {
			fmt.Fprintf(out, "      Caution: %s\n", strings.Join(firstStrings(cautions, 2), "; "))
		}
	}
	if evidence := packItemEvidence(item.Reasons, excluded, verbose); len(evidence) > 0 {
		reasonLabel := "Evidence"
		if excluded {
			reasonLabel = "Weak evidence"
		}
		if !verbose && excluded {
			return
		}
		fmt.Fprintf(out, "      %s: %s\n", reasonLabel, strings.Join(evidence, "; "))
	}
}

func shouldShowPackSource(item retrieval.PackItem, verbose bool) bool {
	if verbose || item.Path == "" {
		return verbose && item.Path != ""
	}
	title := strings.ToLower(strings.TrimSpace(item.Title))
	path := strings.ToLower(strings.TrimSpace(item.Path))
	if path == "" {
		return false
	}
	return title == "" || (title != path && !strings.Contains(title, path))
}

func packItemEvidence(reasons []string, excluded, verbose bool) []string {
	if verbose {
		return limitStrings(displayPackReasons(reasons, excluded), 4)
	}
	return concisePackReasons(reasons)
}

func concisePackReasons(reasons []string) []string {
	evidence := concisePackEvidence{}
	for _, reason := range reasons {
		evidence.addReason(reason)
	}
	return evidence.render()
}

type concisePackEvidence struct {
	anchors    []string
	sections   []string
	pathTerms  []string
	titleTerms []string
	bodyTerms  []string
}

func (e *concisePackEvidence) addReason(reason string) {
	reason = strings.TrimSpace(reason)
	lower := strings.ToLower(reason)
	switch {
	case strings.HasPrefix(lower, "anchor-first ranking:"):
		for _, term := range anchorTermsFromReason(reason) {
			e.anchors = appendUniqueString(e.anchors, term)
		}
	case strings.HasPrefix(lower, "query term match in "):
		field, term, ok := queryTermFromReason(reason)
		if !ok {
			return
		}
		switch field {
		case "path":
			e.pathTerms = appendUniqueString(e.pathTerms, term)
		case "title":
			e.titleTerms = appendUniqueString(e.titleTerms, term)
		case "body":
			e.bodyTerms = appendUniqueString(e.bodyTerms, term)
		default:
			e.bodyTerms = appendUniqueString(e.bodyTerms, term)
		}
	case strings.HasPrefix(lower, "section-packed context:"):
		e.addSections(strings.TrimSpace(strings.TrimPrefix(reason, "section-packed context:")))
	case strings.HasPrefix(lower, "indexed section match:"):
		e.addSections(strings.TrimSpace(strings.TrimPrefix(reason, "indexed section match:")))
	case lower == "test-case behavior signal":
		e.anchors = appendUniqueString(e.anchors, "test behavior")
	case strings.HasPrefix(lower, "matched test behavior:"):
		body := strings.TrimSpace(strings.TrimPrefix(reason, "matched test behavior:"))
		if body != "" {
			e.anchors = appendUniqueString(e.anchors, body)
		}
	}
}

func (e *concisePackEvidence) addSections(body string) {
	for _, part := range strings.Split(body, ";") {
		section := conciseSectionName(part)
		if section == "" || isLowValuePackSection(section) {
			continue
		}
		e.sections = appendUniqueString(e.sections, section)
	}
}

func (e concisePackEvidence) render() []string {
	var out []string
	if len(e.anchors) > 0 {
		out = append(out, "matched: "+strings.Join(firstStrings(e.anchors, 5), ", "))
	}
	if len(e.sections) > 0 {
		out = append(out, "sections: "+strings.Join(firstStrings(e.sections, 2), "; "))
	}
	if len(out) >= 2 {
		return out
	}
	if field := conciseFieldEvidence("path", e.pathTerms); field != "" {
		out = append(out, field)
	}
	if len(out) >= 2 {
		return out
	}
	if field := conciseFieldEvidence("title", e.titleTerms); field != "" {
		out = append(out, field)
	}
	if len(out) >= 2 {
		return out
	}
	if field := conciseFieldEvidence("body", e.bodyTerms); field != "" {
		out = append(out, field)
	}
	return out
}

func anchorTermsFromReason(reason string) []string {
	parts := strings.Split(strings.TrimSpace(strings.TrimPrefix(reason, "anchor-first ranking:")), ";")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		lower := strings.ToLower(part)
		if !strings.HasPrefix(lower, "matches ") {
			continue
		}
		var terms []string
		for _, term := range strings.Split(strings.TrimSpace(part[len("matches "):]), ",") {
			term = strings.TrimSpace(term)
			if term != "" && !isGenericPackReceiptTerm(term) {
				terms = append(terms, term)
			}
		}
		return terms
	}
	return nil
}

func queryTermFromReason(reason string) (string, string, bool) {
	trimmed := strings.TrimSpace(strings.TrimPrefix(reason, "query term match in "))
	field, term, ok := strings.Cut(trimmed, ":")
	if !ok {
		return "", "", false
	}
	field = strings.ToLower(strings.TrimSpace(field))
	term = strings.TrimSpace(term)
	if term == "" || isGenericPackReceiptTerm(term) {
		return "", "", false
	}
	return field, term, true
}

func conciseSectionName(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	parts := strings.Split(value, ">")
	leaf := strings.TrimSpace(parts[len(parts)-1])
	if idx := strings.Index(strings.ToLower(leaf), " lines "); idx >= 0 {
		leaf = strings.TrimSpace(leaf[:idx])
	}
	leaf = strings.Trim(leaf, "` ")
	return limitRunes(leaf, 96)
}

func isLowValuePackSection(section string) bool {
	lower := strings.ToLower(strings.TrimSpace(section))
	switch lower {
	case "", "table of contents", "overview", "summary", "quick start":
		return true
	default:
		return false
	}
}

func conciseFieldEvidence(field string, terms []string) string {
	terms = firstStrings(terms, 3)
	if len(terms) == 0 {
		return ""
	}
	return field + ": " + strings.Join(terms, ", ")
}

func appendUniqueString(values []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return values
	}
	for _, existing := range values {
		if strings.EqualFold(existing, value) {
			return values
		}
	}
	return append(values, value)
}

func firstStrings(values []string, limit int) []string {
	if limit <= 0 || len(values) <= limit {
		return values
	}
	out := make([]string, limit)
	copy(out, values[:limit])
	return out
}

func packCoverageText(summary retrieval.PackSummary) string {
	var parts []string
	if summary.HasBackgroundDecisions {
		parts = append(parts, "background")
	}
	if summary.HasImplementation {
		parts = append(parts, "implementation")
	}
	if summary.HasBehaviorTests {
		parts = append(parts, "tests")
	}
	if summary.HasConfigSchema {
		parts = append(parts, "config/schema")
	}
	if summary.HasOpenWork {
		parts = append(parts, "open work")
	}
	if summary.HasSupportingContext {
		parts = append(parts, "supporting docs")
	}
	if len(parts) == 0 {
		return "none"
	}
	return strings.Join(parts, " + ")
}

func displayPackRoleReason(reason string) string {
	switch strings.TrimSpace(reason) {
	case "source or implementation-adjacent artifact":
		return "source or implementation-adjacent entry point"
	case "test artifact captures expected behavior":
		return "behavior test or fixture captures expected behavior"
	case "requested process or instruction artifact":
		return "process or instruction artifact relevant to the request"
	case "supporting matched artifact":
		return "supporting artifact matched the request"
	default:
		return reason
	}
}

func splitPackCues(cues []string) ([]string, []string) {
	var signals []string
	var cautions []string
	for _, cue := range cues {
		cue = strings.TrimSpace(cue)
		if cue == "" {
			continue
		}
		if isPackCautionCue(cue) {
			cautions = append(cautions, cue)
		} else {
			signals = append(signals, cue)
		}
	}
	return signals, cautions
}

func isPackCautionCue(cue string) bool {
	lower := strings.ToLower(strings.TrimSpace(cue))
	return lower == "archived" ||
		lower == "deprecated" ||
		lower == "generated" ||
		lower == "stale" ||
		strings.Contains(lower, "archive") ||
		strings.Contains(lower, "deprecated") ||
		strings.Contains(lower, "generated") ||
		strings.Contains(lower, "stale") ||
		strings.Contains(lower, "superseded")
}

func displayPackReasons(reasons []string, excluded bool) []string {
	out := make([]string, 0, len(reasons))
	seen := map[string]bool{}
	for _, reason := range reasons {
		for _, display := range displayPackReason(reason, excluded) {
			display = strings.TrimSpace(display)
			if display == "" || seen[display] {
				continue
			}
			seen[display] = true
			out = append(out, display)
		}
	}
	return out
}

func displayPackReason(reason string, excluded bool) []string {
	reason = strings.TrimSpace(reason)
	lower := strings.ToLower(reason)
	switch {
	case strings.HasPrefix(lower, "anchor-first ranking:"):
		if display := displayAnchorFirstReason(reason); display != "" {
			return []string{display}
		}
		return nil
	case strings.HasPrefix(lower, "query term match in "):
		if display := displayQueryTermReason(reason); display != "" {
			return []string{display}
		}
		return nil
	case strings.HasPrefix(lower, "section-packed context:"):
		if display := displaySectionListReason(reason, "section focus", "section-packed context:"); display != "" {
			return []string{display}
		}
		return nil
	case strings.HasPrefix(lower, "indexed section match:"):
		if display := displaySectionListReason(reason, "section evidence", "indexed section match:"); display != "" {
			return []string{display}
		}
		return nil
	case strings.HasPrefix(lower, "authority prior:"):
		if display := displayAuthorityReason(reason, excluded); display != "" {
			return []string{display}
		}
		return nil
	case lower == "test-case behavior signal":
		return []string{"test behavior signal"}
	case strings.HasPrefix(lower, "matched test behavior:"):
		return []string{strings.TrimSpace(strings.TrimPrefix(reason, "matched test behavior:"))}
	}
	return []string{reason}
}

func displayAnchorFirstReason(reason string) string {
	parts := strings.Split(strings.TrimSpace(strings.TrimPrefix(reason, "anchor-first ranking:")), ";")
	var terms []string
	var fields []string
	for _, part := range parts {
		part = strings.TrimSpace(part)
		lower := strings.ToLower(part)
		switch {
		case strings.HasPrefix(lower, "matches "):
			for _, term := range strings.Split(strings.TrimSpace(part[len("matches "):]), ",") {
				term = strings.TrimSpace(term)
				if term != "" && !isGenericPackReceiptTerm(term) {
					terms = append(terms, term)
				}
			}
		case strings.HasPrefix(lower, "fields "):
			for _, field := range strings.Split(strings.TrimSpace(part[len("fields "):]), ",") {
				field = strings.TrimSpace(field)
				if field != "" {
					fields = append(fields, field)
				}
			}
		}
	}
	if len(terms) == 0 {
		return ""
	}
	display := "matched anchors: " + strings.Join(limitStrings(terms, 5), ", ")
	if len(fields) > 0 {
		display += " (" + strings.Join(limitStrings(fields, 4), ", ") + ")"
	}
	return display
}

func displayQueryTermReason(reason string) string {
	trimmed := strings.TrimSpace(strings.TrimPrefix(reason, "query term match in "))
	field, term, ok := strings.Cut(trimmed, ":")
	if !ok {
		return reason
	}
	field = strings.TrimSpace(field)
	term = strings.TrimSpace(term)
	if term == "" || isGenericPackReceiptTerm(term) {
		return ""
	}
	return field + " matched: " + term
}

func displaySectionListReason(reason, label, prefix string) string {
	body := strings.TrimSpace(strings.TrimPrefix(reason, prefix))
	if body == "" {
		return ""
	}
	parts := compactSectionReasonParts(strings.Split(body, ";"))
	if len(parts) == 0 {
		return ""
	}
	return label + ": " + strings.Join(limitStrings(parts, 2), "; ")
}

func compactSectionReasonParts(parts []string) []string {
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		out = append(out, limitRunes(part, 140))
	}
	return out
}

func displayAuthorityReason(reason string, excluded bool) string {
	body := strings.TrimSpace(strings.TrimPrefix(reason, "authority prior:"))
	lower := strings.ToLower(body)
	switch {
	case strings.HasPrefix(lower, "recognized artifact role "):
		role := strings.TrimSpace(body[len("recognized artifact role "):])
		if role != "" {
			return "recognized " + role + " artifact"
		}
	case lower == "behavioral test signal":
		return "behavior test signal"
	case lower == "stale or superseded":
		if !excluded {
			return ""
		}
		return "stale or superseded signal"
	case lower == "archive path":
		if !excluded {
			return ""
		}
		return "archive-path signal"
	case lower == "archived lifecycle":
		if !excluded {
			return ""
		}
		return "archived lifecycle signal"
	case lower == "generated/reference path":
		if !excluded {
			return ""
		}
		return "generated/reference signal"
	case strings.HasPrefix(lower, "classifier "):
		return ""
	case lower == "classifier confidence" || lower == "high classifier confidence":
		return ""
	}
	if body == "" {
		return ""
	}
	return body
}

func isGenericPackReceiptTerm(term string) bool {
	switch strings.ToLower(strings.TrimSpace(term)) {
	case "a", "an", "and", "are", "as", "be", "before", "change", "changed",
		"changing", "context", "do", "does", "first", "follow", "for", "from",
		"get", "give", "how", "in", "into", "is", "it", "load", "of", "on",
		"or", "repo", "repository", "show", "task", "the", "this", "to",
		"trace", "understand", "update", "use", "what", "when", "where",
		"which", "why", "with", "work":
		return true
	default:
		return false
	}
}

func limitRunes(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	if limit <= 3 {
		return string(runes[:limit])
	}
	return strings.TrimSpace(string(runes[:limit-3])) + "..."
}

func compactKindSubtype(kind, subtype string) string {
	kind = strings.TrimSpace(kind)
	subtype = strings.TrimSpace(subtype)
	switch {
	case kind == "" && subtype == "":
		return "-"
	case subtype == "":
		return kind
	case kind == "":
		return subtype
	default:
		return kind + "/" + subtype
	}
}

func limitStrings(values []string, limit int) []string {
	if limit <= 0 || len(values) <= limit {
		return values
	}
	out := make([]string, limit, limit+1)
	copy(out, values[:limit])
	out = append(out, fmt.Sprintf("+%d more", len(values)-limit))
	return out
}
