package sourcecontext

import (
	"fmt"
	"path/filepath"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

const (
	maxShellSymbols  = 120
	maxShellTests    = 80
	maxShellImports  = 120
	maxShellDispatch = 48
)

// ShellSemanticAnchor is one bounded, source-positioned shell semantic.
type ShellSemanticAnchor struct {
	Name    string
	Kind    string
	Parent  string
	Line    int
	EndLine int
}

// ShellSemantics contains parser-backed shell facts for the compact source manifest.
type ShellSemantics struct {
	Symbols []ShellSemanticAnchor
	Tests   []ShellSemanticAnchor
	Imports []ShellSemanticAnchor
}

// ExtractShellSemantics parses one shell source file without evaluating expansions.
// A test-file anchor is retained when parsing fails so callers can preserve the
// whole-file behavior-test relationship.
func ExtractShellSemantics(rel, role, body string) (ShellSemantics, error) {
	semantics := ShellSemantics{}
	variant := shellLanguageVariant(rel)
	parser := syntax.NewParser(syntax.Variant(variant))
	file, err := parser.Parse(strings.NewReader(body), filepath.ToSlash(rel))
	if err != nil {
		semantics.Tests = shellFileTestFallback(rel, role, body)
		return semantics, fmt.Errorf("parse shell source: %w", err)
	}

	functionNames := shellFunctionNames(file)
	if shellRoleAllowsSymbols(role) {
		semantics.Symbols = shellDispatchAnchors(file, functionNames)
		semantics.Symbols = append(semantics.Symbols, shellFunctionAnchors(file)...)
		semantics.Symbols = compactShellAnchors(semantics.Symbols, maxShellSymbols)
	}
	semantics.Tests = shellTestAnchors(file, rel, role, body)
	semantics.Imports = shellImportAnchors(file, body)
	return semantics, nil
}

func shellLanguageVariant(rel string) syntax.LangVariant {
	switch strings.ToLower(filepath.Ext(filepath.ToSlash(rel))) {
	case ".bats":
		return syntax.LangBats
	case ".zsh":
		return syntax.LangZsh
	default:
		return syntax.LangBash
	}
}

func shellRoleAllowsSymbols(role string) bool {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "test", "test_doc_example", "fixture":
		return false
	default:
		return true
	}
}

func shellFunctionNames(file *syntax.File) map[string]bool {
	names := map[string]bool{}
	syntax.Walk(file, func(node syntax.Node) bool {
		function, ok := node.(*syntax.FuncDecl)
		if !ok {
			return true
		}
		for _, name := range shellFunctionDeclarationNames(function) {
			names[name] = true
		}
		return true
	})
	return names
}

func shellFunctionAnchors(file *syntax.File) []ShellSemanticAnchor {
	var anchors []ShellSemanticAnchor
	syntax.Walk(file, func(node syntax.Node) bool {
		function, ok := node.(*syntax.FuncDecl)
		if !ok {
			return true
		}
		for _, name := range shellFunctionDeclarationNames(function) {
			anchors = append(anchors, shellNodeAnchor(name, "function", "", function))
		}
		return len(anchors) < maxShellSymbols
	})
	return compactShellAnchors(anchors, maxShellSymbols)
}

func shellFunctionDeclarationNames(function *syntax.FuncDecl) []string {
	if function == nil {
		return nil
	}
	if function.Name != nil && function.Name.Value != "" {
		return []string{function.Name.Value}
	}
	var names []string
	for _, name := range function.Names {
		if name != nil && name.Value != "" {
			names = append(names, name.Value)
		}
	}
	return names
}

func shellDispatchAnchors(file *syntax.File, functionNames map[string]bool) []ShellSemanticAnchor {
	var anchors []ShellSemanticAnchor
	syntax.Walk(file, func(node syntax.Node) bool {
		clause, ok := node.(*syntax.CaseClause)
		if !ok {
			return true
		}
		for _, item := range clause.Items {
			target := shellCaseItemFunctionTarget(item, functionNames)
			if target == "" {
				continue
			}
			for _, pattern := range item.Patterns {
				name, ok := shellStaticWord(pattern)
				if !ok || !shellUsefulDispatchPattern(name) {
					continue
				}
				anchors = append(anchors, shellNodeAnchor(name, "dispatch", target, item))
				if len(anchors) >= maxShellDispatch {
					return false
				}
			}
		}
		return true
	})
	return compactShellAnchors(anchors, maxShellDispatch)
}

func shellCaseItemFunctionTarget(item *syntax.CaseItem, functionNames map[string]bool) string {
	if item == nil {
		return ""
	}
	for _, statement := range item.Stmts {
		call, ok := statement.Cmd.(*syntax.CallExpr)
		if !ok || len(call.Args) == 0 {
			continue
		}
		name, ok := shellStaticWord(call.Args[0])
		if ok && functionNames[name] {
			return name
		}
	}
	return ""
}

func shellUsefulDispatchPattern(value string) bool {
	value = strings.TrimSpace(value)
	return value != "" && value != "*" && !strings.ContainsAny(value, "?[")
}

func shellTestAnchors(file *syntax.File, rel, role, body string) []ShellSemanticAnchor {
	var anchors []ShellSemanticAnchor
	syntax.Walk(file, func(node syntax.Node) bool {
		switch value := node.(type) {
		case *syntax.TestDecl:
			name, ok := shellStaticWord(value.Description)
			if ok && name != "" {
				anchors = append(anchors, shellNodeAnchor(name, "bats_test", "", value))
			}
		case *syntax.CallExpr:
			if anchor, ok := shellDynamicBatsTestAnchor(value); ok {
				anchors = append(anchors, anchor)
			}
		}
		return len(anchors) < maxShellTests
	})
	anchors = compactShellAnchors(anchors, maxShellTests)
	if len(anchors) == 0 {
		anchors = shellBehaviorSectionAnchors(role, body)
	}
	if len(anchors) == 0 {
		return shellFileTestFallback(rel, role, body)
	}
	return anchors
}

func shellBehaviorSectionAnchors(role, body string) []ShellSemanticAnchor {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "test", "test_doc_example":
	default:
		return nil
	}
	lines := strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n")
	type section struct {
		line int
		name string
	}
	var sections []section
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		if !shellBehaviorHeadingLine(line) || (i > 0 && shellBehaviorHeadingLine(lines[i-1])) {
			continue
		}
		name := strings.TrimSpace(strings.TrimPrefix(line, "#"))
		for j := i + 1; j < len(lines) && strings.HasPrefix(lines[j], "# "); j++ {
			continuation := strings.TrimSpace(strings.TrimPrefix(lines[j], "#"))
			if continuation == "" || len(name)+1+len(continuation) > 180 {
				break
			}
			name += " " + continuation
		}
		sections = append(sections, section{line: i + 1, name: name})
	}
	anchors := make([]ShellSemanticAnchor, 0, len(sections))
	for i, section := range sections {
		endLine := len(lines)
		if i+1 < len(sections) {
			endLine = sections[i+1].line - 1
		}
		anchors = append(anchors, ShellSemanticAnchor{
			Name:    section.name,
			Kind:    "shell_behavior",
			Line:    section.line,
			EndLine: endLine,
		})
	}
	return compactShellAnchors(anchors, maxShellTests)
}

func shellBehaviorHeadingLine(line string) bool {
	if !strings.HasPrefix(line, "# ") || len(line) < 3 {
		return false
	}
	first := rune(line[2])
	return first >= 'A' && first <= 'Z' || first >= '0' && first <= '9'
}

func shellDynamicBatsTestAnchor(call *syntax.CallExpr) (ShellSemanticAnchor, bool) {
	if call == nil || len(call.Args) < 2 {
		return ShellSemanticAnchor{}, false
	}
	command, ok := shellStaticWord(call.Args[0])
	if !ok || command != "bats_test_function" {
		return ShellSemanticAnchor{}, false
	}
	var description string
	var target string
	for i := 1; i < len(call.Args); i++ {
		arg, static := shellStaticWord(call.Args[i])
		if !static {
			continue
		}
		if arg == "--description" && i+1 < len(call.Args) {
			description, _ = shellStaticWord(call.Args[i+1])
			i++
			continue
		}
		if arg == "--" && i+1 < len(call.Args) {
			target, _ = shellStaticWord(call.Args[i+1])
			break
		}
		if !strings.HasPrefix(arg, "-") {
			target = arg
		}
	}
	if target == "" {
		return ShellSemanticAnchor{}, false
	}
	if description == "" {
		description = target
	}
	return shellNodeAnchor(description, "bats_test", target, call), true
}

func shellFileTestFallback(rel, role, body string) []ShellSemanticAnchor {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "test", "test_doc_example":
	default:
		return nil
	}
	base := filepath.Base(filepath.ToSlash(rel))
	name := strings.TrimSuffix(base, filepath.Ext(base))
	name = strings.TrimSpace(name)
	if name == "" {
		return nil
	}
	endLine := 1
	if body != "" {
		endLine = strings.Count(body, "\n") + 1
	}
	return []ShellSemanticAnchor{{Name: name, Kind: "shell_test", Line: 1, EndLine: endLine}}
}

func shellImportAnchors(file *syntax.File, body string) []ShellSemanticAnchor {
	var anchors []ShellSemanticAnchor
	syntax.Walk(file, func(node syntax.Node) bool {
		call, ok := node.(*syntax.CallExpr)
		if !ok || len(call.Args) < 2 {
			return true
		}
		command, ok := shellStaticWord(call.Args[0])
		if command == `\.` {
			command = "."
		}
		if !ok || (command != "source" && command != "." && command != "load") {
			return true
		}
		ref := shellSourceWord(body, call.Args[1])
		if ref == "" {
			return true
		}
		anchors = append(anchors, shellNodeAnchor(ref, "source", "", call.Args[1]))
		return len(anchors) < maxShellImports
	})
	return compactShellAnchors(anchors, maxShellImports)
}

func shellStaticWord(word *syntax.Word) (string, bool) {
	if word == nil || len(word.Parts) == 0 {
		return "", false
	}
	var out strings.Builder
	for _, part := range word.Parts {
		switch value := part.(type) {
		case *syntax.Lit:
			out.WriteString(value.Value)
		case *syntax.SglQuoted:
			out.WriteString(value.Value)
		case *syntax.DblQuoted:
			for _, quotedPart := range value.Parts {
				literal, ok := quotedPart.(*syntax.Lit)
				if !ok {
					return "", false
				}
				out.WriteString(literal.Value)
			}
		default:
			return "", false
		}
	}
	value := strings.TrimSpace(out.String())
	return value, value != ""
}

func shellSourceWord(body string, word *syntax.Word) string {
	if word == nil || !word.Pos().IsValid() || !word.End().IsValid() {
		return ""
	}
	start := int(word.Pos().Offset())
	end := int(word.End().Offset())
	if start < 0 || end <= start || end > len(body) {
		return ""
	}
	value := strings.TrimSpace(body[start:end])
	if len(value) >= 2 && ((value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'')) {
		value = value[1 : len(value)-1]
	}
	if value == "" || len(value) > 180 || strings.ContainsAny(value, "\r\n\t ") {
		return ""
	}
	return value
}

func shellNodeAnchor(name, kind, parent string, node syntax.Node) ShellSemanticAnchor {
	return ShellSemanticAnchor{
		Name:    strings.TrimSpace(name),
		Kind:    kind,
		Parent:  strings.TrimSpace(parent),
		Line:    int(node.Pos().Line()),
		EndLine: int(node.End().Line()),
	}
}

func compactShellAnchors(values []ShellSemanticAnchor, limit int) []ShellSemanticAnchor {
	seen := map[string]bool{}
	out := make([]ShellSemanticAnchor, 0, len(values))
	for _, value := range values {
		value.Name = strings.TrimSpace(value.Name)
		if value.Name == "" || len(value.Name) > 180 || value.Line <= 0 {
			continue
		}
		if value.EndLine < value.Line {
			value.EndLine = value.Line
		}
		key := value.Kind + "\x00" + value.Name + "\x00" + value.Parent
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, value)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}
