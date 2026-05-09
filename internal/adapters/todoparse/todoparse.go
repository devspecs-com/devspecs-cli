// Package todoparse extracts markdown checklist items from text content.
// It recognizes `- [ ]`, `- [x]`, and `- [X]` syntax.
package todoparse

import (
	"bufio"
	"strings"
)

// Todo represents a single extracted checklist item.
type Todo struct {
	Ordinal    int
	Text       string
	Done       bool
	SourceFile string
	SourceLine int // 1-indexed
}

// Parse extracts checklist todos from markdown content.
// sourceFile is stored on each Todo for provenance tracking.
func Parse(content, sourceFile string) []Todo {
	var todos []Todo
	scanner := bufio.NewScanner(strings.NewReader(content))
	lineNum := 0
	ordinal := 0
	inFencedBlock := false

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		// Track fenced code blocks — ignore checklists inside them
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inFencedBlock = !inFencedBlock
			continue
		}
		if inFencedBlock {
			continue
		}

		// Skip blockquote lines
		if strings.HasPrefix(trimmed, ">") {
			continue
		}

		todo, ok := parseLine(trimmed)
		if !ok {
			continue
		}

		todo.Ordinal = ordinal
		todo.SourceFile = sourceFile
		todo.SourceLine = lineNum
		todos = append(todos, todo)
		ordinal++
	}
	return todos
}

func parseLine(trimmed string) (Todo, bool) {
	// Match: optional leading whitespace already trimmed, then "- [ ] " or "- [x] " or "- [X] "
	if !strings.HasPrefix(trimmed, "- [") {
		return Todo{}, false
	}

	// Need at least "- [x] a" = 7 chars
	if len(trimmed) < 6 {
		return Todo{}, false
	}

	marker := trimmed[3]
	if trimmed[4] != ']' {
		return Todo{}, false
	}

	var done bool
	switch marker {
	case ' ':
		done = false
	case 'x', 'X':
		done = true
	default:
		return Todo{}, false
	}

	// Must have space after ] (len >= 6 guaranteed by check above)
	if trimmed[5] != ' ' {
		return Todo{}, false
	}

	text := trimmed[6:]
	return Todo{Text: text, Done: done}, true
}
