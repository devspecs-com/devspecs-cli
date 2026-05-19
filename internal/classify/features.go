package classify

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"gopkg.in/yaml.v3"
)

const (
	MarkerGenerated  = "generated"
	MarkerChangelog  = "changelog"
	MarkerStale      = "stale"
	MarkerDeprecated = "deprecated"
	MarkerSuperseded = "superseded"
	MarkerScratch    = "scratch"
	MarkerVendored   = "vendored"
)

var (
	markdownLinkRE = regexp.MustCompile(`\[[^\]]+\]\(([^)\s]+)(?:\s+"[^"]*")?\)`)
	bareURLRE      = regexp.MustCompile(`https?://[^\s<>)]+`)
	dateTokenRE    = regexp.MustCompile(`(?i)(?:^|[^0-9])((?:19|20)[0-9]{2}[-_]?[01][0-9][-_]?[0-3][0-9]|[0-9]{14}|[0-9]{6})(?:[^0-9]|$)`)
	pathLikeRE     = regexp.MustCompile(`(?:[A-Za-z0-9_.-]+/){1,}[A-Za-z0-9_.-]+`)
)

// ExtractFeatures extracts deterministic, stack-neutral document evidence.
func ExtractFeatures(path, body string) Features {
	path = filepath.ToSlash(path)
	normalized := normalizeNewlines(body)
	lines := strings.Split(normalized, "\n")
	frontmatter, contentStartLine := extractFrontmatter(lines)
	contentLines := lines
	if contentStartLine > 0 && contentStartLine <= len(lines) {
		contentLines = lines[contentStartLine-1:]
	}

	headings := extractHeadings(lines, contentStartLine)
	return Features{
		PathTokens:         pathTokens(path),
		FilenameTokens:     filenameTokens(path),
		DateTokens:         dateTokens(path + "\n" + normalized),
		Frontmatter:        frontmatter,
		Title:              documentTitle(frontmatter, headings),
		Headings:           headings,
		Sections:           sectionsFromHeadings(headings, len(lines)),
		ChecklistItems:     checklistCount(lines),
		StatusPhrases:      statusPhrases(frontmatter, lines),
		LifecyclePhrases:   lifecyclePhrases(frontmatter, lines, path),
		Identifiers:        identifierTerms(path + "\n" + normalized),
		PathReferences:     pathReferences(path, normalized),
		LinkTargets:        linkTargets(normalized),
		CodeFenceLanguages: codeFenceLanguages(contentLines),
		LocalTerms:         localTerms(normalized),
		Markers:            documentMarkers(path, normalized),
	}
}

// EnrichCandidate returns a copy of c with Features populated from c.Path/body.
func EnrichCandidate(c Candidate) Candidate {
	c.Features = ExtractFeatures(c.Path, c.Body)
	return c
}

func normalizeNewlines(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.ReplaceAll(s, "\r", "\n")
}

func extractFrontmatter(lines []string) (map[string]string, int) {
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return nil, 0
	}
	closeLine := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			closeLine = i
			break
		}
	}
	if closeLine < 0 {
		return nil, 0
	}
	raw := strings.Join(lines[1:closeLine], "\n")
	var decoded map[string]any
	if err := yaml.Unmarshal([]byte(raw), &decoded); err != nil {
		return nil, closeLine + 2
	}
	out := map[string]string{}
	for key, value := range decoded {
		if key == "" {
			continue
		}
		out[key] = stringifyYAMLValue(value)
	}
	if len(out) == 0 {
		return nil, closeLine + 2
	}
	return out, closeLine + 2
}

func stringifyYAMLValue(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(v)
	case []any:
		var parts []string
		for _, item := range v {
			if s := stringifyYAMLValue(item); s != "" {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, ", ")
	case map[string]any:
		var keys []string
		for key := range v {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		var parts []string
		for _, key := range keys {
			if s := stringifyYAMLValue(v[key]); s != "" {
				parts = append(parts, key+"="+s)
			}
		}
		return strings.Join(parts, ", ")
	default:
		return strings.TrimSpace(fmt.Sprint(v))
	}
}

func pathTokens(path string) []string {
	var out []string
	for _, part := range strings.Split(filepath.ToSlash(path), "/") {
		if part == "" || part == "." {
			continue
		}
		out = append(out, tokenVariants(strings.TrimSuffix(part, filepath.Ext(part)))...)
	}
	return uniqueSorted(out)
}

func filenameTokens(path string) []string {
	base := filepath.Base(filepath.ToSlash(path))
	name := strings.TrimSuffix(base, filepath.Ext(base))
	return uniqueSorted(tokenVariants(name))
}

func tokenVariants(s string) []string {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return nil
	}
	var out []string
	add := func(token string) {
		token = strings.Trim(token, " _-.")
		if len(token) >= 2 {
			out = append(out, token)
		}
	}
	add(s)
	for _, part := range strings.FieldsFunc(s, func(r rune) bool {
		return r == '_' || r == '-' || r == '.' || unicode.IsSpace(r)
	}) {
		add(part)
	}
	return out
}

func dateTokens(text string) []string {
	var out []string
	for _, match := range dateTokenRE.FindAllStringSubmatch(text, -1) {
		if len(match) < 2 {
			continue
		}
		token := strings.ReplaceAll(match[1], "_", "-")
		out = append(out, token)
		out = append(out, strings.ReplaceAll(token, "-", ""))
	}
	return uniqueSorted(out)
}

func extractHeadings(lines []string, contentStartLine int) []Heading {
	if contentStartLine <= 0 {
		contentStartLine = 1
	}
	var headings []Heading
	for i := contentStartLine - 1; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if !strings.HasPrefix(line, "#") {
			continue
		}
		level := 0
		for level < len(line) && line[level] == '#' {
			level++
		}
		if level < 1 || level > 6 || level >= len(line) || !unicode.IsSpace(rune(line[level])) {
			continue
		}
		text := strings.TrimSpace(line[level:])
		text = strings.TrimSpace(strings.TrimRight(text, "#"))
		if text == "" {
			continue
		}
		headings = append(headings, Heading{Level: level, Text: text, Line: i + 1})
	}
	return headings
}

func documentTitle(frontmatter map[string]string, headings []Heading) string {
	if frontmatter != nil {
		if title := strings.TrimSpace(frontmatter["title"]); title != "" {
			return title
		}
	}
	for _, h := range headings {
		if h.Level == 1 {
			return h.Text
		}
	}
	if len(headings) > 0 {
		return headings[0].Text
	}
	return ""
}

func sectionsFromHeadings(headings []Heading, totalLines int) []Section {
	if len(headings) == 0 {
		return nil
	}
	sections := make([]Section, 0, len(headings))
	for i, h := range headings {
		start := headings[i].Line
		end := totalLines
		if i+1 < len(headings) {
			end = headings[i+1].Line - 1
		}
		sections = append(sections, Section{
			Heading:   h.Text,
			Role:      inferSectionRole(h.Text),
			StartLine: start,
			EndLine:   end,
		})
	}
	return sections
}

func inferSectionRole(heading string) string {
	h := strings.ToLower(heading)
	switch {
	case containsAny(h, "decision outcome", "decision"):
		return "decision"
	case containsAny(h, "context", "background", "problem"):
		return "context"
	case containsAny(h, "consequence", "consequences", "impact"):
		return "consequences"
	case containsAny(h, "goal", "non-goal", "user outcome", "users", "personas"):
		return "product"
	case containsAny(h, "requirement", "acceptance criteria", "success metric"):
		return "requirements"
	case containsAny(h, "risk", "drawback"):
		return "risk"
	case containsAny(h, "open question", "unresolved"):
		return "open_questions"
	case containsAny(h, "task", "implementation", "plan", "todo"):
		return "tasks"
	case containsAny(h, "rollout", "migration"):
		return "rollout"
	case containsAny(h, "deferred", "out of scope", "non-goals"):
		return "deferred"
	case containsAny(h, "option", "alternative"):
		return "alternatives"
	case containsAny(h, "motivation", "proposal", "design", "summary", "abstract"):
		return "proposal"
	default:
		return ""
	}
}

func checklistCount(lines []string) int {
	count := 0
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "- [ ]") ||
			strings.HasPrefix(strings.ToLower(trimmed), "- [x]") ||
			strings.HasPrefix(trimmed, "* [ ]") ||
			strings.HasPrefix(strings.ToLower(trimmed), "* [x]") {
			count++
		}
	}
	return count
}

func statusPhrases(frontmatter map[string]string, lines []string) []string {
	var out []string
	if frontmatter != nil {
		if status := strings.TrimSpace(frontmatter["status"]); status != "" {
			out = append(out, normalizePhrase("status:"+status))
		}
	}
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		lower := strings.ToLower(trimmed)
		if strings.HasPrefix(lower, "status:") || strings.HasPrefix(lower, "* status:") || strings.HasPrefix(lower, "- status:") {
			out = append(out, normalizePhrase(trimmed))
		}
	}
	return uniqueSorted(out)
}

func lifecyclePhrases(frontmatter map[string]string, lines []string, path string) []string {
	haystack := strings.ToLower(path + "\n" + strings.Join(lines, "\n"))
	for key, value := range frontmatter {
		haystack += "\n" + strings.ToLower(key+":"+value)
	}
	var out []string
	for _, phrase := range []string{
		"active", "accepted", "proposed", "draft", "implementing", "completed",
		"superseded", "deprecated", "stale", "archived", "obsolete", "rejected",
		"legacy", "old",
	} {
		if strings.Contains(haystack, phrase) {
			out = append(out, phrase)
		}
	}
	return uniqueSorted(out)
}

func identifierTerms(text string) []string {
	var out []string
	for _, token := range roughTokens(text) {
		if len(token) > 160 {
			continue
		}
		if strings.Contains(token, "/") {
			for _, segment := range strings.Split(token, "/") {
				if isIdentifierLike(segment) {
					out = append(out, identifierVariants(segment)...)
				}
			}
			continue
		}
		if isIdentifierLike(token) {
			out = append(out, identifierVariants(token)...)
		}
	}
	return uniqueSorted(out)
}

func identifierVariants(token string) []string {
	cleaned := strings.ToLower(strings.Trim(token, "`'\".,:;()[]{}<>"))
	if cleaned == "" {
		return nil
	}
	withoutExt := strings.TrimSuffix(cleaned, filepath.Ext(cleaned))
	if withoutExt == "" {
		withoutExt = cleaned
	}
	var out []string
	out = append(out, cleaned)
	if withoutExt != cleaned {
		out = append(out, withoutExt)
	}
	fields := strings.FieldsFunc(withoutExt, func(r rune) bool {
		return r == '_' || r == '-' || r == '.'
	})
	if len(fields) > 1 {
		for i := 0; i < len(fields)-1; i++ {
			suffix := strings.Join(fields[i:], "_")
			if len(suffix) >= 3 && !allDigits(strings.ReplaceAll(suffix, "_", "")) {
				out = append(out, suffix)
			}
		}
	}
	return out
}

func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

func isIdentifierLike(token string) bool {
	token = strings.Trim(token, "`'\".,:;()[]{}<>")
	if len(token) < 3 {
		return false
	}
	if strings.ContainsAny(token, "_.-") {
		return true
	}
	var sawLower bool
	for _, r := range token {
		if unicode.IsLower(r) {
			sawLower = true
			continue
		}
		if sawLower && unicode.IsUpper(r) {
			return true
		}
	}
	return false
}

func pathReferences(path, body string) []string {
	var out []string
	out = append(out, filepath.ToSlash(path))
	for _, match := range pathLikeRE.FindAllString(body, -1) {
		cleaned := strings.Trim(match, "`'\".,:;()[]{}<>")
		if cleaned != "" && len(cleaned) <= 220 {
			out = append(out, filepath.ToSlash(cleaned))
		}
	}
	return uniqueSorted(out)
}

func linkTargets(body string) []string {
	var out []string
	for _, match := range markdownLinkRE.FindAllStringSubmatch(body, -1) {
		if len(match) > 1 {
			out = append(out, strings.TrimSpace(match[1]))
		}
	}
	for _, match := range bareURLRE.FindAllString(body, -1) {
		out = append(out, strings.TrimRight(match, ".,)"))
	}
	return uniqueSorted(out)
}

func codeFenceLanguages(lines []string) []string {
	var out []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "```") && !strings.HasPrefix(trimmed, "~~~") {
			continue
		}
		lang := strings.TrimSpace(trimmed[3:])
		if lang == "" {
			continue
		}
		lang = strings.Fields(lang)[0]
		out = append(out, strings.ToLower(lang))
	}
	return uniqueSorted(out)
}

func localTerms(body string) []string {
	counts := map[string]int{}
	for _, token := range roughTokens(body) {
		token = strings.ToLower(strings.Trim(token, "`'\".,:;()[]{}<>"))
		if len(token) < 4 || commonToken(token) || strings.ContainsAny(token, "/") {
			continue
		}
		counts[token]++
	}
	type termCount struct {
		term  string
		count int
	}
	var ranked []termCount
	for term, count := range counts {
		if count >= 2 {
			ranked = append(ranked, termCount{term: term, count: count})
		}
	}
	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].count == ranked[j].count {
			return ranked[i].term < ranked[j].term
		}
		return ranked[i].count > ranked[j].count
	})
	if len(ranked) > 30 {
		ranked = ranked[:30]
	}
	out := make([]string, len(ranked))
	for i, item := range ranked {
		out[i] = item.term
	}
	sort.Strings(out)
	return out
}

func documentMarkers(path, body string) []string {
	haystack := strings.ToLower(path + "\n" + body)
	var out []string
	addIf := func(marker string, needles ...string) {
		for _, needle := range needles {
			if strings.Contains(haystack, needle) {
				out = append(out, marker)
				return
			}
		}
	}
	addIf(MarkerGenerated, "do not edit", "auto-generated", "autogenerated", "generated by", "this file is generated", "code generated")
	addIf(MarkerChangelog, "changelog", "generated release notes", "release notes. do not edit", "release notes do not edit", "release-note")
	addIf(MarkerStale, "stale", "legacy", "old-", "old_", "obsolete")
	addIf(MarkerDeprecated, "deprecated")
	addIf(MarkerSuperseded, "superseded", "supersedes")
	addIf(MarkerScratch, "scratch/")
	addIf(MarkerVendored, "vendor/", "node_modules/")
	return uniqueSorted(out)
}

func roughTokens(text string) []string {
	var out []string
	var b strings.Builder
	flush := func() {
		if b.Len() > 0 {
			out = append(out, b.String())
			b.Reset()
		}
	}
	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' || r == '.' || r == '/' {
			b.WriteRune(r)
			continue
		}
		flush()
	}
	flush()
	return out
}

func commonToken(token string) bool {
	switch token {
	case "about", "after", "also", "before", "from", "have", "into", "keep", "must", "should", "that", "their", "then", "there", "this", "with", "without", "will":
		return true
	default:
		return false
	}
}

func containsAny(s string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(s, needle) {
			return true
		}
	}
	return false
}

func normalizePhrase(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(s))), " ")
}

func uniqueSorted(items []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" || seen[item] {
			continue
		}
		seen[item] = true
		out = append(out, item)
	}
	sort.Strings(out)
	return out
}
