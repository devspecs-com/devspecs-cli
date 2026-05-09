package todoparse

import (
	"testing"
)

func TestTodoParser(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []Todo
	}{
		{
			name:     "empty input",
			input:    "",
			expected: nil,
		},
		{
			name:  "single incomplete",
			input: "- [ ] Write tests",
			expected: []Todo{
				{Ordinal: 0, Text: "Write tests", Done: false, SourceFile: "test.md", SourceLine: 1},
			},
		},
		{
			name:  "single complete lowercase",
			input: "- [x] Write tests",
			expected: []Todo{
				{Ordinal: 0, Text: "Write tests", Done: true, SourceFile: "test.md", SourceLine: 1},
			},
		},
		{
			name:  "single complete uppercase",
			input: "- [X] Write tests",
			expected: []Todo{
				{Ordinal: 0, Text: "Write tests", Done: true, SourceFile: "test.md", SourceLine: 1},
			},
		},
		{
			name:  "mixed items",
			input: "- [ ] First\n- [x] Second\n- [ ] Third",
			expected: []Todo{
				{Ordinal: 0, Text: "First", Done: false, SourceFile: "test.md", SourceLine: 1},
				{Ordinal: 1, Text: "Second", Done: true, SourceFile: "test.md", SourceLine: 2},
				{Ordinal: 2, Text: "Third", Done: false, SourceFile: "test.md", SourceLine: 3},
			},
		},
		{
			name:  "indented bullet",
			input: "  - [ ] Indented task",
			expected: []Todo{
				{Ordinal: 0, Text: "Indented task", Done: false, SourceFile: "test.md", SourceLine: 1},
			},
		},
		{
			name:     "non-dash bullet ignored",
			input:    "* [ ] Star bullet\n+ [ ] Plus bullet",
			expected: nil,
		},
		{
			name:     "inside fenced code block ignored",
			input:    "```\n- [ ] Not a todo\n```",
			expected: nil,
		},
		{
			name:     "inside tilde fence ignored",
			input:    "~~~\n- [ ] Not a todo\n~~~",
			expected: nil,
		},
		{
			name:     "blockquote ignored",
			input:    "> - [ ] Quoted task",
			expected: nil,
		},
		{
			name:     "malformed marker ignored",
			input:    "- [y] Not valid\n- [?] Also not valid",
			expected: nil,
		},
		{
			name:     "empty text after marker ignored",
			input:    "- [ ] ",
			expected: nil,
		},
		{
			name:     "no space after bracket ignored",
			input:    "- [ ]nospace",
			expected: nil,
		},
		{
			name:     "truncated marker too short",
			input:    "- [x",
			expected: nil,
		},
		{
			name:     "missing closing bracket",
			input:    "- [x  some text",
			expected: nil,
		},
		{
			name:     "only marker no text",
			input:    "- [x]",
			expected: nil,
		},
		{
			name:     "marker with only whitespace after",
			input:    "- [ ]    ",
			expected: nil,
		},
		{
			name:  "line number accuracy with mixed content",
			input: "# Title\n\nSome text\n\n- [ ] Task one\n\n- [x] Task two\n",
			expected: []Todo{
				{Ordinal: 0, Text: "Task one", Done: false, SourceFile: "test.md", SourceLine: 5},
				{Ordinal: 1, Text: "Task two", Done: true, SourceFile: "test.md", SourceLine: 7},
			},
		},
		{
			name:  "ordinal monotonicity",
			input: "- [ ] A\n- [ ] B\n- [x] C\n- [ ] D",
			expected: []Todo{
				{Ordinal: 0, Text: "A", Done: false, SourceFile: "test.md", SourceLine: 1},
				{Ordinal: 1, Text: "B", Done: false, SourceFile: "test.md", SourceLine: 2},
				{Ordinal: 2, Text: "C", Done: true, SourceFile: "test.md", SourceLine: 3},
				{Ordinal: 3, Text: "D", Done: false, SourceFile: "test.md", SourceLine: 4},
			},
		},
		{
			name:  "code block before real todos",
			input: "```md\n- [ ] Fake\n```\n- [ ] Real",
			expected: []Todo{
				{Ordinal: 0, Text: "Real", Done: false, SourceFile: "test.md", SourceLine: 4},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Parse(tt.input, "test.md")
			if len(got) != len(tt.expected) {
				t.Fatalf("expected %d todos, got %d: %+v", len(tt.expected), len(got), got)
			}
			for i, want := range tt.expected {
				g := got[i]
				if g.Ordinal != want.Ordinal {
					t.Errorf("[%d] ordinal: want %d, got %d", i, want.Ordinal, g.Ordinal)
				}
				if g.Text != want.Text {
					t.Errorf("[%d] text: want %q, got %q", i, want.Text, g.Text)
				}
				if g.Done != want.Done {
					t.Errorf("[%d] done: want %v, got %v", i, want.Done, g.Done)
				}
				if g.SourceFile != want.SourceFile {
					t.Errorf("[%d] sourceFile: want %q, got %q", i, want.SourceFile, g.SourceFile)
				}
				if g.SourceLine != want.SourceLine {
					t.Errorf("[%d] sourceLine: want %d, got %d", i, want.SourceLine, g.SourceLine)
				}
			}
		})
	}
}
