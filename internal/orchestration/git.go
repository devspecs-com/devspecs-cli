package orchestration

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

func inspectRepository(ctx context.Context, runner CommandRunner, path string) (Repository, error) {
	root, err := runGitText(ctx, runner, path, "rev-parse", "--show-toplevel")
	if err != nil {
		return Repository{}, fmt.Errorf("resolve execution repository: %w", err)
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return Repository{}, err
	}
	status, err := runGitText(ctx, runner, root, "status", "--porcelain=v1", "--untracked-files=all")
	if err != nil {
		return Repository{}, fmt.Errorf("inspect execution repository state: %w", err)
	}
	if status != "" {
		return Repository{}, fmt.Errorf("execution repository is dirty; commit or stash changes before orchestration")
	}
	commit, err := runGitText(ctx, runner, root, "rev-parse", "HEAD")
	if err != nil {
		return Repository{}, err
	}
	tree, err := runGitText(ctx, runner, root, "rev-parse", "HEAD^{tree}")
	if err != nil {
		return Repository{}, err
	}
	roots, err := runGitText(ctx, runner, root, "rev-list", "--max-parents=0", "HEAD")
	if err != nil {
		return Repository{}, err
	}
	rootCommits := strings.Fields(roots)
	sort.Strings(rootCommits)
	if len(rootCommits) == 0 {
		return Repository{}, fmt.Errorf("execution repository has no root commit")
	}
	remote, _ := runGitText(ctx, runner, root, "remote", "get-url", "origin")
	identity := "root"
	if remote != "" {
		identity = normalizeRemote(remote)
	}
	return Repository{
		LogicalID:         "git:" + identity + "@" + rootCommits[0],
		RequestedRoot:     filepath.Clean(root),
		SourceCommit:      commit,
		SourceState:       "clean",
		SourceStateDigest: sha256Bytes([]byte(tree)),
	}, nil
}

func resultRepositoryState(ctx context.Context, runner CommandRunner, path string) (*string, string, error) {
	commit, err := runGitText(ctx, runner, path, "rev-parse", "HEAD")
	if err != nil {
		return nil, "", err
	}
	status, err := runGitBytes(ctx, runner, path, "status", "--porcelain=v1", "--untracked-files=all")
	if err != nil {
		return nil, "", err
	}
	diff, err := runGitBytes(ctx, runner, path, "diff", "--binary")
	if err != nil {
		return nil, "", err
	}
	cached, err := runGitBytes(ctx, runner, path, "diff", "--cached", "--binary")
	if err != nil {
		return nil, "", err
	}
	state := append([]byte(commit+"\n"), status...)
	state = append(state, diff...)
	state = append(state, cached...)
	return &commit, sha256Bytes(state), nil
}

func runGitText(ctx context.Context, runner CommandRunner, dir string, args ...string) (string, error) {
	data, err := runGitBytes(ctx, runner, dir, args...)
	return strings.TrimSpace(string(data)), err
}

func runGitBytes(ctx context.Context, runner CommandRunner, dir string, args ...string) ([]byte, error) {
	result, err := runner.Run(ctx, dir, "git", args...)
	if err != nil {
		return nil, err
	}
	if result.ExitCode != 0 {
		return nil, fmt.Errorf("git %s failed: %s", strings.Join(args, " "), boundedMessage(result.Stderr))
	}
	return result.Stdout, nil
}

func normalizeRemote(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimSuffix(value, ".git")
	if strings.HasPrefix(value, "git@") {
		value = strings.TrimPrefix(value, "git@")
		value = strings.Replace(value, ":", "/", 1)
		return "https://" + value
	}
	return value
}

func boundedMessage(data []byte) string {
	value := strings.TrimSpace(string(data))
	if len(value) > 1000 {
		return value[:1000] + "..."
	}
	return value
}
