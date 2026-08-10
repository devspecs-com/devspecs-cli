package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type activationManifest struct {
	Repos []activationRepo `json:"repos"`
}

type activationRepo struct {
	ID        string `json:"id"`
	URL       string `json:"url"`
	CommitSHA string `json:"commit_sha"`
}

func main() {
	if len(os.Args) != 2 {
		fatalf("usage: go run ./scripts/ci/prepare_activation_smoke.go <manifest.json>")
	}
	root := strings.TrimSpace(os.Getenv("DEVSPECS_SMOKE_ROOT"))
	if root == "" {
		fatalf("DEVSPECS_SMOKE_ROOT must be set")
	}

	data, err := os.ReadFile(os.Args[1])
	if err != nil {
		fatalf("read manifest: %v", err)
	}
	var manifest activationManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		fatalf("parse manifest: %v", err)
	}
	if len(manifest.Repos) == 0 {
		fatalf("manifest has no repositories")
	}

	root, err = filepath.Abs(root)
	if err != nil {
		fatalf("resolve smoke root: %v", err)
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		fatalf("create smoke root: %v", err)
	}

	for _, repo := range manifest.Repos {
		prepareRepo(root, repo)
	}
}

func prepareRepo(root string, repo activationRepo) {
	if repo.ID == "" || filepath.Base(repo.ID) != repo.ID || repo.ID == "." {
		fatalf("invalid repository id %q", repo.ID)
	}
	if strings.TrimSpace(repo.URL) == "" || strings.TrimSpace(repo.CommitSHA) == "" {
		fatalf("repository %q requires url and commit_sha", repo.ID)
	}

	target := filepath.Join(root, repo.ID)
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		fatalf("repository target already exists: %s", target)
	}
	runGit("clone", "--no-tags", "--quiet", repo.URL, target)
	runGit("-C", target, "checkout", "--detach", "--quiet", repo.CommitSHA)

	actual := gitOutput("-C", target, "rev-parse", "HEAD")
	if actual != repo.CommitSHA {
		fatalf("repository %q resolved to %s, expected %s", repo.ID, actual, repo.CommitSHA)
	}
	if shallow := gitOutput("-C", target, "rev-parse", "--is-shallow-repository"); shallow != "false" {
		fatalf("repository %q is shallow; smoke-5 requires full history", repo.ID)
	}
	fmt.Printf("prepared %s at %s\n", repo.ID, shortSHA(repo.CommitSHA))
}

func shortSHA(sha string) string {
	if len(sha) <= 12 {
		return sha
	}
	return sha[:12]
}

func runGit(args ...string) {
	cmd := exec.Command("git", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fatalf("git %s: %v", strings.Join(args, " "), err)
	}
}

func gitOutput(args ...string) string {
	out, err := exec.Command("git", args...).Output()
	if err != nil {
		fatalf("git %s: %v", strings.Join(args, " "), err)
	}
	return strings.TrimSpace(string(out))
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
