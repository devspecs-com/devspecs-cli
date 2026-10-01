package orchestration

import (
	"bytes"
	"context"
	"fmt"
	"net/url"
	"os"
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
	untracked, err := runGitBytes(ctx, runner, path, "ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return nil, "", err
	}
	for _, name := range bytes.Split(untracked, []byte{0}) {
		if len(name) == 0 {
			continue
		}
		filePath := filepath.Join(path, filepath.FromSlash(string(name)))
		info, err := os.Lstat(filePath)
		if err != nil {
			return nil, "", fmt.Errorf("inspect untracked result: %w", err)
		}
		var contentHash string
		if info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(filePath)
			if err != nil {
				return nil, "", fmt.Errorf("read untracked result link: %w", err)
			}
			contentHash = sha256Bytes([]byte(target))
		} else {
			contentHash, err = runGitText(ctx, runner, path, "hash-object", "--no-filters", "--", string(name))
			if err != nil {
				return nil, "", fmt.Errorf("hash untracked result: %w", err)
			}
		}
		state = append(state, name...)
		state = append(state, 0)
		state = append(state, []byte(info.Mode().String()+":"+contentHash)...)
		state = append(state, 0)
	}
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
	if !strings.Contains(value, "://") {
		if colon := strings.IndexByte(value, ':'); colon > 1 {
			host := value[:colon]
			if at := strings.LastIndexByte(host, '@'); at >= 0 {
				host = host[at+1:]
			}
			value = "https://" + host + "/" + value[colon+1:]
		} else if strings.Contains(value, "@") {
			return "root"
		} else {
			return strings.TrimSuffix(value, ".git")
		}
	}
	remote, err := url.Parse(value)
	if err != nil || remote.Host == "" {
		return "root"
	}
	remote.User = nil
	remote.RawQuery = ""
	remote.Fragment = ""
	remote.Path = strings.TrimSuffix(remote.Path, ".git")
	remote.RawPath = ""
	if remote.Scheme == "ssh" || remote.Scheme == "git" {
		remote.Scheme = "https"
	}
	return remote.String()
}

func boundedMessage(data []byte) string {
	value := strings.TrimSpace(string(data))
	if len(value) > 1000 {
		return value[:1000] + "..."
	}
	return value
}
