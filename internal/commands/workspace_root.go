package commands

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/devspecs-com/devspecs-cli/internal/scan"
	"github.com/spf13/cobra"
)

const workspaceRootCandidateLimit = 6

var workspaceRootMarkerFiles = []string{
	".git",
	".devspecs",
	"go.mod",
	"package.json",
	"pyproject.toml",
	"Cargo.toml",
	"pom.xml",
	"build.gradle",
	"settings.gradle",
	"composer.json",
	"Gemfile",
	"mix.exs",
	"deno.json",
}

var workspaceRootContainerDirs = map[string]bool{
	"apps":       true,
	"crates":     true,
	"libs":       true,
	"modules":    true,
	"packages":   true,
	"projects":   true,
	"repos":      true,
	"services":   true,
	"workspaces": true,
}

var workspaceRootSkipDirs = map[string]bool{
	".git":         true,
	".hg":          true,
	".svn":         true,
	".devspecs":    true,
	".next":        true,
	".turbo":       true,
	".venv":        true,
	"build":        true,
	"coverage":     true,
	"dist":         true,
	"examples":     true,
	"fixtures":     true,
	"node_modules": true,
	"target":       true,
	"testdata":     true,
	"vendor":       true,
}

func maybeWarnWorkspaceRoot(cmd *cobra.Command, repoRoot string) *scan.RootSelectionWarning {
	warning := detectWorkspaceRootWarning(repoRoot, workspaceCommandName(cmd))
	if warning == nil {
		return nil
	}
	writeWorkspaceRootWarning(cmd.ErrOrStderr(), warning)
	return warning
}

func detectWorkspaceRootWarning(repoRoot, commandName string) *scan.RootSelectionWarning {
	repoRoot = canonicalRepoRoot(repoRoot)
	if repoRoot == "" {
		return nil
	}
	candidates := workspaceRootCandidates(repoRoot)
	if len(candidates) < 2 {
		return nil
	}
	if len(candidates) > workspaceRootCandidateLimit {
		candidates = candidates[:workspaceRootCandidateLimit]
	}
	suggested := make([]string, 0, len(candidates))
	for _, rel := range candidates {
		suggested = append(suggested, workspaceSuggestedCommand(rel, commandName))
	}
	return &scan.RootSelectionWarning{
		Kind:              "workspace_root",
		Path:              repoRoot,
		Message:           "This looks like a workspace or monorepo root. Scanning from here can be slow or mix unrelated projects; consider running DevSpecs from one focused project root.",
		CandidateRoots:    candidates,
		SuggestedCommands: suggested,
	}
}

func workspaceRootCandidates(repoRoot string) []string {
	seen := map[string]bool{}
	add := func(rel string) {
		rel = filepath.ToSlash(filepath.Clean(rel))
		if rel == "." || rel == "" {
			return
		}
		seen[rel] = true
	}
	for _, entry := range readWorkspaceRootDirs(repoRoot) {
		name := entry.Name()
		if workspaceRootSkipDirs[strings.ToLower(name)] {
			continue
		}
		abs := filepath.Join(repoRoot, name)
		if hasWorkspaceRootMarker(abs) {
			add(name)
		}
		if !workspaceRootContainerDirs[strings.ToLower(name)] {
			continue
		}
		for _, child := range readWorkspaceRootDirs(abs) {
			childName := child.Name()
			if workspaceRootSkipDirs[strings.ToLower(childName)] {
				continue
			}
			childAbs := filepath.Join(abs, childName)
			if hasWorkspaceRootMarker(childAbs) {
				add(filepath.Join(name, childName))
			}
		}
	}
	out := make([]string, 0, len(seen))
	for rel := range seen {
		out = append(out, rel)
	}
	sort.Strings(out)
	return out
}

func readWorkspaceRootDirs(dir string) []os.DirEntry {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var dirs []os.DirEntry
	for _, entry := range entries {
		if entry.IsDir() {
			dirs = append(dirs, entry)
		}
	}
	return dirs
}

func hasWorkspaceRootMarker(dir string) bool {
	for _, marker := range workspaceRootMarkerFiles {
		if _, err := os.Stat(filepath.Join(dir, marker)); err == nil {
			return true
		}
	}
	return false
}

func workspaceCommandName(cmd *cobra.Command) string {
	if cmd == nil {
		return "scan"
	}
	name := cmd.CommandPath()
	fields := strings.Fields(name)
	if len(fields) == 0 {
		return "scan"
	}
	return fields[len(fields)-1]
}

func workspaceSuggestedCommand(rel, commandName string) string {
	commandName = strings.TrimSpace(commandName)
	if commandName == "" {
		commandName = "scan"
	}
	switch commandName {
	case "find", "task":
		return fmt.Sprintf("cd %s && ds %s ...", filepath.ToSlash(rel), commandName)
	default:
		return fmt.Sprintf("cd %s && ds %s", filepath.ToSlash(rel), commandName)
	}
}

func writeWorkspaceRootWarning(out io.Writer, warning *scan.RootSelectionWarning) {
	if warning == nil || out == nil {
		return
	}
	fmt.Fprintf(out, "Workspace root warning: %s\n", warning.Message)
	fmt.Fprintf(out, "Root: %s\n", warning.Path)
	if len(warning.CandidateRoots) > 0 {
		fmt.Fprintln(out, "Candidate project roots:")
		for i, rel := range warning.CandidateRoots {
			fmt.Fprintf(out, "  - %s", rel)
			if i < len(warning.SuggestedCommands) && warning.SuggestedCommands[i] != "" {
				fmt.Fprintf(out, "  (%s)", warning.SuggestedCommands[i])
			}
			fmt.Fprintln(out)
		}
	}
	fmt.Fprintln(out, "Continuing with the current root.")
}
