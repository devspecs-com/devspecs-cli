// Package todoparse extracts markdown checklist items from text content.
// It recognizes `- [ ]`, `- [x]`, and `- [X]` syntax.
// Under acceptance/success/OKR headings, checklists are classified as criteria;
// otherwise they are actionable todos.
package todoparse

import (
	"bufio"
	"strings"
)

// CriteriaKind values stored for checklist lines under matching headings.
const (
	KindAcceptance = "acceptance"
	KindSuccess    = "success"
	KindOKR        = "okr"
)

// Todo represents a single extracted actionable checklist item.
type Todo struct {
	Ordinal    int
	Text       string
	Done       bool
	SourceFile string
	SourceLine int // 1-indexed
}

// Criterion represents a checklist line under an acceptance/success/OKR heading.
type Criterion struct {
	Ordinal      int
	Text         string
	Done         bool
	SourceFile   string
	SourceLine   int    // 1-indexed
	CriteriaKind string // KindAcceptance, KindSuccess, or KindOKR
}

// ParseResult holds todos and criteria extracted from one markdown document.
type ParseResult struct {
	Todos    []Todo
	Criteria []Criterion
}

// Parse extracts actionable todos and section-classified criteria from markdown.
// sourceFile is stored on each item for provenance tracking.
func Parse(content, sourceFile string) ParseResult {
	var out ParseResult
	scanner := bufio.NewScanner(strings.NewReader(content))
	lineNum := 0
	todoOrd := 0
	critOrd := 0
	inFencedBlock := false
	var sectionKind string // criteria Kind*, or "" for actionable todos

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inFencedBlock = !inFencedBlock
			continue
		}
		if inFencedBlock {
			continue
		}

		if strings.HasPrefix(trimmed, ">") {
			continue
		}

		if title, ok := parseMarkdownHeading(trimmed); ok {
			sectionKind = classifySectionTitle(title)
			continue
		}

		item, ok := parseChecklistLine(trimmed)
		if !ok {
			continue
		}

		if ck := sectionKind; ck != "" {
			itemCrit := Criterion{
				Ordinal:      critOrd,
				Text:         item.Text,
				Done:         item.Done,
				SourceFile:   sourceFile,
				SourceLine:   lineNum,
				CriteriaKind: ck,
			}
			out.Criteria = append(out.Criteria, itemCrit)
			critOrd++
			continue
		}

		itemTodo := Todo{
			Ordinal:    todoOrd,
			Text:       item.Text,
			Done:       item.Done,
			SourceFile: sourceFile,
			SourceLine: lineNum,
		}
		out.Todos = append(out.Todos, itemTodo)
		todoOrd++
	}
	return out
}

func parseMarkdownHeading(trimmed string) (title string, ok bool) {
	if !strings.HasPrefix(trimmed, "#") {
		return "", false
	}
	n := 0
	for n < len(trimmed) && trimmed[n] == '#' {
		n++
	}
	if n == 0 || n > 6 {
		return "", false
	}
	if n >= len(trimmed) {
		return "", false
	}
	rest := strings.TrimSpace(trimmed[n:])
	if rest == "" {
		return "", false
	}
	return rest, true
}

// classifySectionTitle returns KindAcceptance, KindSuccess, KindOKR, or "" for ordinary sections.
// Match order matters (okr → acceptance → success).
func classifySectionTitle(title string) string {
	h := strings.ToLower(strings.Join(strings.Fields(title), " "))
	switch {
	case strings.Contains(h, "okr") || strings.Contains(h, "objectives and key results"):
		return KindOKR
	case strings.Contains(h, "acceptance criteria") || strings.Contains(h, "acceptance criterion") ||
		strings.Contains(h, "definition of done"):
		return KindAcceptance
	case strings.Contains(h, "success criteria") || strings.Contains(h, "success criterion") ||
		strings.Contains(h, "auditable success"):
		return KindSuccess
	default:
		return ""
	}
}

type checklistItem struct {
	Text string
	Done bool
}

func parseChecklistLine(trimmed string) (checklistItem, bool) {
	if !strings.HasPrefix(trimmed, "- [") {
		return checklistItem{}, false
	}
	if len(trimmed) < 6 {
		return checklistItem{}, false
	}
	marker := trimmed[3]
	if trimmed[4] != ']' {
		return checklistItem{}, false
	}
	var done bool
	switch marker {
	case ' ':
		done = false
	case 'x', 'X':
		done = true
	default:
		return checklistItem{}, false
	}
	if trimmed[5] != ' ' {
		return checklistItem{}, false
	}
	text := trimmed[6:]
	return checklistItem{Text: text, Done: done}, true
}
