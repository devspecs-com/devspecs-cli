package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
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
	type packageFiles struct {
		files []*ast.File
		paths []string
	}

	fileSet := token.NewFileSet()
	packages := map[string]*packageFiles{}
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
		if !strings.HasSuffix(entry.Name(), ".go") {
			return nil
		}

		parsed, err := parser.ParseFile(fileSet, path, nil, 0)
		if err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return fmt.Errorf("resolve relative path for %s: %w", path, err)
		}
		packageKey := filepath.Dir(rel) + "\x00" + parsed.Name.Name
		group := packages[packageKey]
		if group == nil {
			group = &packageFiles{}
			packages[packageKey] = group
		}
		group.files = append(group.files, parsed)
		group.paths = append(group.paths, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return nil, err
	}

	counts := map[string]int{}
	for _, group := range packages {
		info := resolveTestingMethods(fileSet, group.files)
		for index, file := range group.files {
			count := countResolvedDirectAssertions(file, info)
			if count > 0 {
				counts[group.paths[index]] = count
			}
		}
	}
	return counts, nil
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
	fileSet := token.NewFileSet()
	fileSet.AddFile("fixture.go", -1, int(file.End())+1)
	return countDirectAssertionsWithFileSet(fileSet, file)
}

func countDirectAssertionsWithFileSet(fileSet *token.FileSet, file *ast.File) int {
	info := resolveTestingMethods(fileSet, []*ast.File{file})
	return countResolvedDirectAssertions(file, info)
}

func resolveTestingMethods(fileSet *token.FileSet, files []*ast.File) *types.Info {
	info := &types.Info{Uses: map[*ast.Ident]types.Object{}}
	checker := types.Config{
		Importer: importer.Default(),
		Error:    func(error) {},
	}
	_, _ = checker.Check(files[0].Name.Name, fileSet, files, info)
	return info
}

func countResolvedDirectAssertions(file *ast.File, info *types.Info) int {
	count := 0
	ast.Inspect(file, func(node ast.Node) bool {
		selector, ok := node.(*ast.SelectorExpr)
		if !ok || !directAssertionMethods[selector.Sel.Name] {
			return true
		}
		method := info.Uses[selector.Sel]
		if method != nil && method.Pkg() != nil && method.Pkg().Path() == "testing" {
			count++
		}
		return true
	})
	return count
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
