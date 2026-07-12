package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

const repoTargetFlagName = "repo"

func addRepoTargetPersistentFlag(cmd *cobra.Command) {
	cmd.PersistentFlags().String(repoTargetFlagName, "", "Target repository path for repo-local DevSpecs artifacts and context")
}

func commandRepoTarget(cmd *cobra.Command) string {
	if cmd == nil {
		return ""
	}
	flag := cmd.Flag(repoTargetFlagName)
	if flag == nil {
		return ""
	}
	return strings.TrimSpace(flag.Value.String())
}

func resolveTargetRepoRoot(repoPath string) (string, error) {
	repoPath = strings.TrimSpace(repoPath)
	if repoPath == "" {
		wd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		return resolveRepoRootForPath(wd), nil
	}
	abs, err := filepath.Abs(repoPath)
	if err != nil {
		return "", fmt.Errorf("resolve target repo %q: %w", repoPath, err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("resolve target repo %q: %w", repoPath, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("target repo is not a directory: %s", abs)
	}
	return resolveRepoRootForPath(abs), nil
}

func resolveRepoRootForPath(path string) string {
	repoRoot := canonicalRepoRoot(resolveRepoRootFromWd(path))
	if repoRoot == "" {
		repoRoot = canonicalRepoRoot(path)
	}
	return repoRoot
}

func commandArg(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return `""`
	}
	if strings.ContainsAny(value, " \t\r\n\"") {
		return strconv.Quote(value)
	}
	return value
}
