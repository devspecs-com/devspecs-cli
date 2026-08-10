package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const defaultBaselinePath = "testdata/testingassert/direct-testing-baseline.json"

var directAssertionMethods = map[string]bool{
	"Error":   true,
	"Errorf":  true,
	"Fail":    true,
	"FailNow": true,
	"Fatal":   true,
	"Fatalf":  true,
}

type assertionBaseline struct {
	Version int            `json:"version"`
	Counts  map[string]int `json:"counts"`
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("check-test-assertions", flag.ContinueOnError)
	flags.SetOutput(stderr)
	root := flags.String("root", ".", "repository root")
	baselinePath := flags.String("baseline", defaultBaselinePath, "baseline path relative to root")
	update := flags.Bool("update", false, "replace the baseline with current direct assertion counts")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 {
		fmt.Fprintln(stderr, "unexpected positional arguments")
		return 2
	}

	rootAbs, err := filepath.Abs(*root)
	if err != nil {
		fmt.Fprintf(stderr, "resolve root: %v\n", err)
		return 1
	}
	counts, err := collectDirectAssertions(rootAbs)
	if err != nil {
		fmt.Fprintf(stderr, "collect direct test assertions: %v\n", err)
		return 1
	}
	baselineAbs := *baselinePath
	if !filepath.IsAbs(baselineAbs) {
		baselineAbs = filepath.Join(rootAbs, baselineAbs)
	}

	if *update {
		if err := writeBaseline(baselineAbs, counts); err != nil {
			fmt.Fprintf(stderr, "write assertion baseline: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "updated direct assertion baseline: %d call(s) across %d file(s)\n", sumCounts(counts), len(counts))
		return 0
	}

	baseline, err := loadBaseline(baselineAbs)
	if err != nil {
		fmt.Fprintf(stderr, "load assertion baseline: %v\n", err)
		return 1
	}
	problems := compareCounts(counts, baseline.Counts)
	if len(problems) > 0 {
		for _, problem := range problems {
			fmt.Fprintln(stderr, problem)
		}
		fmt.Fprintln(stderr, "run go run ./scripts/ci/check-test-assertions --update after reviewing intentional reductions")
		return 1
	}

	fmt.Fprintf(stdout, "direct testing assertion baseline unchanged: %d call(s) across %d file(s)\n", sumCounts(counts), len(counts))
	return 0
}

func collectDirectAssertions(root string) (map[string]int, error) {
	counts := map[string]int{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if path != root && ignoredAssertionDir(entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(entry.Name(), "_test.go") {
			return nil
		}

		parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}
		count := countDirectAssertions(parsed)
		if count == 0 {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return fmt.Errorf("resolve relative path for %s: %w", path, err)
		}
		counts[filepath.ToSlash(rel)] = count
		return nil
	})
	return counts, err
}

func ignoredAssertionDir(name string) bool {
	switch name {
	case ".git", ".devspecs", "dist", "node_modules", "vendor", "_ignore":
		return true
	default:
		return false
	}
}

func countDirectAssertions(file *ast.File) int {
	count := 0
	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || !directAssertionMethods[selector.Sel.Name] {
			return true
		}
		receiver, ok := selector.X.(*ast.Ident)
		if ok && isTestingHandle(receiver) {
			count++
		}
		return true
	})
	return count
}

func isTestingHandle(identifier *ast.Ident) bool {
	if identifier.Obj == nil {
		return false
	}
	field, ok := identifier.Obj.Decl.(*ast.Field)
	if !ok {
		return false
	}
	return isTestingType(field.Type)
}

func isTestingType(expr ast.Expr) bool {
	if pointer, ok := expr.(*ast.StarExpr); ok {
		expr = pointer.X
	}
	selector, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkg, ok := selector.X.(*ast.Ident)
	if !ok || pkg.Name != "testing" {
		return false
	}
	switch selector.Sel.Name {
	case "T", "B", "F", "TB":
		return true
	default:
		return false
	}
}

func loadBaseline(path string) (assertionBaseline, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return assertionBaseline{}, err
	}
	var baseline assertionBaseline
	if err := json.Unmarshal(data, &baseline); err != nil {
		return assertionBaseline{}, err
	}
	if baseline.Version != 1 {
		return assertionBaseline{}, fmt.Errorf("unsupported version %d", baseline.Version)
	}
	if baseline.Counts == nil {
		baseline.Counts = map[string]int{}
	}
	return baseline, nil
}

func writeBaseline(path string, counts map[string]int) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(assertionBaseline{Version: 1, Counts: counts}, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

func compareCounts(current, baseline map[string]int) []string {
	paths := make(map[string]bool, len(current)+len(baseline))
	for path := range current {
		paths[path] = true
	}
	for path := range baseline {
		paths[path] = true
	}
	ordered := make([]string, 0, len(paths))
	for path := range paths {
		ordered = append(ordered, path)
	}
	sort.Strings(ordered)

	var problems []string
	for _, path := range ordered {
		got, want := current[path], baseline[path]
		switch {
		case got > want:
			problems = append(problems, fmt.Sprintf("%s: direct testing assertions increased from %d to %d", path, want, got))
		case got < want:
			problems = append(problems, fmt.Sprintf("%s: direct testing assertions fell from %d to %d; ratchet the baseline", path, want, got))
		}
	}
	return problems
}

func sumCounts(counts map[string]int) int {
	total := 0
	for _, count := range counts {
		total += count
	}
	return total
}
