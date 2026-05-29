// Package sections extracts deterministic document sections for indexing and retrieval.
package sections

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// Section is a markdown section with stable line-range evidence.
type Section struct {
	ID                 string
	ArtifactID         string
	RevisionID         string
	SourcePath         string
	HeadingPath        string
	HeadingDepth       int
	StartLine          int
	EndLine            int
	Title              string
	Body               string
	Frontmatter        map[string]string
	Tasks              []string
	AcceptanceCriteria []string
	Links              []string
	TokenEstimate      int
	Kind               string
	Metadata           map[string]string
}

// ExtractMarkdown returns heading-delimited markdown sections.
func ExtractMarkdown(body string) []Section {
	body = strings.ReplaceAll(body, "\r\n", "\n")
	body = strings.ReplaceAll(body, "\r", "\n")
	lines := strings.Split(body, "\n")
	frontmatter, startIndex := parseFrontmatter(lines)
	type headingState struct {
		depth int
		title string
	}
	var stack []headingState
	var out []Section
	var current *Section
	var contentStart int
	inCode := false
	finalize := func(endLine int) {
		if current == nil {
			return
		}
		if endLine < contentStart {
			current.EndLine = current.StartLine
			current.Body = ""
		} else {
			current.EndLine = endLine
			current.Body = strings.TrimRight(strings.Join(lines[contentStart-1:endLine], "\n"), "\r\n")
		}
		current.Tasks = extractTaskLines(current.Body)
		current.AcceptanceCriteria = extractAcceptanceLines(current.HeadingPath, current.Body)
		current.Links = extractMarkdownLinks(current.Body)
		current.TokenEstimate = ApproxTokenCount(current.Body)
		out = append(out, *current)
		current = nil
	}
	for i := startIndex; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)
		if isMarkdownFence(trimmed) {
			inCode = !inCode
		}
		if inCode {
			continue
		}
		depth, title, ok := parseMarkdownHeading(line)
		if !ok {
			continue
		}
		lineNo := i + 1
		finalize(lineNo - 1)
		for len(stack) > 0 && stack[len(stack)-1].depth >= depth {
			stack = stack[:len(stack)-1]
		}
		stack = append(stack, headingState{depth: depth, title: title})
		pathParts := make([]string, len(stack))
		for j, item := range stack {
			pathParts[j] = item.title
		}
		current = &Section{
			HeadingPath:  strings.Join(pathParts, " > "),
			HeadingDepth: depth,
			StartLine:    lineNo,
			Title:        title,
			Frontmatter:  copyStringMap(frontmatter),
		}
		contentStart = lineNo + 1
	}
	finalize(len(lines))
	return out
}

// AssignStableIDs fills section ids and parent/source metadata.
func AssignStableIDs(sections []Section, artifactID, revisionID, sourcePath string) []Section {
	for i := range sections {
		sections[i].ArtifactID = artifactID
		sections[i].RevisionID = revisionID
		sections[i].SourcePath = sourcePath
		sections[i].ID = StableID(artifactID, revisionID, sections[i].HeadingPath, sections[i].StartLine, sections[i].EndLine)
	}
	return sections
}

// StableID creates a deterministic section id within one artifact revision.
func StableID(artifactID, revisionID, headingPath string, startLine, endLine int) string {
	key := strings.Join([]string{
		artifactID,
		revisionID,
		headingPath,
		intString(startLine),
		intString(endLine),
	}, "\x00")
	sum := sha256.Sum256([]byte(key))
	return "sec_" + hex.EncodeToString(sum[:8])
}

// EnclosingSectionID returns the section id containing line, if any.
func EnclosingSectionID(sections []Section, line int) string {
	if line <= 0 {
		return ""
	}
	for _, section := range sections {
		if section.ID != "" && line >= section.StartLine && line <= section.EndLine {
			return section.ID
		}
	}
	return ""
}

// ApproxTokenCount uses the existing project approximation.
func ApproxTokenCount(text string) int {
	if text == "" {
		return 0
	}
	return (len(text) + 3) / 4
}

func parseFrontmatter(lines []string) (map[string]string, int) {
	if len(lines) == 0 {
		return nil, 0
	}
	delimiter := strings.TrimSpace(lines[0])
	if delimiter != "---" && delimiter != "+++" {
		return nil, 0
	}
	frontmatter := map[string]string{}
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == delimiter {
			if len(frontmatter) == 0 {
				frontmatter = nil
			}
			return frontmatter, i + 1
		}
		key, value, ok := parseFrontmatterLine(lines[i])
		if ok {
			frontmatter[key] = value
		}
	}
	return nil, 0
}

func parseFrontmatterLine(line string) (string, string, bool) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return "", "", false
	}
	separator := strings.Index(line, ":")
	if separator < 0 {
		separator = strings.Index(line, "=")
	}
	if separator <= 0 {
		return "", "", false
	}
	key := strings.TrimSpace(line[:separator])
	value := strings.Trim(strings.TrimSpace(line[separator+1:]), `"'`)
	if key == "" || value == "" {
		return "", "", false
	}
	return key, value, true
}

func parseMarkdownHeading(line string) (int, string, bool) {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "#") {
		return 0, "", false
	}
	depth := 0
	for depth < len(trimmed) && trimmed[depth] == '#' {
		depth++
	}
	if depth == 0 || depth > 6 || depth >= len(trimmed) || trimmed[depth] != ' ' {
		return 0, "", false
	}
	title := strings.TrimSpace(trimmed[depth:])
	title = strings.TrimSpace(strings.TrimRight(title, "#"))
	if title == "" {
		return 0, "", false
	}
	return depth, title, true
}

func isMarkdownFence(trimmed string) bool {
	return strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~")
}

func extractTaskLines(body string) []string {
	var out []string
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		lower := strings.ToLower(trimmed)
		if strings.HasPrefix(lower, "- [ ]") || strings.HasPrefix(lower, "- [x]") ||
			strings.HasPrefix(lower, "* [ ]") || strings.HasPrefix(lower, "* [x]") {
			out = append(out, trimmed)
		}
	}
	return out
}

func extractAcceptanceLines(headingPath, body string) []string {
	if !containsAny(strings.ToLower(headingPath), "acceptance", "criteria", "requirement") &&
		!containsAny(strings.ToLower(body), "acceptance criteria", "acceptance criterion") {
		return nil
	}
	var out []string
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") || strings.Contains(strings.ToLower(trimmed), "acceptance") {
			out = append(out, trimmed)
		}
	}
	return out
}

func extractMarkdownLinks(body string) []string {
	var out []string
	rest := body
	for {
		start := strings.Index(rest, "](")
		if start < 0 {
			break
		}
		after := rest[start+2:]
		end := strings.Index(after, ")")
		if end < 0 {
			break
		}
		target := strings.TrimSpace(after[:end])
		if target != "" {
			out = append(out, target)
		}
		rest = after[end+1:]
	}
	return uniqueStrings(out)
}

func containsAny(s string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(s, needle) {
			return true
		}
	}
	return false
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

func copyStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func intString(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
