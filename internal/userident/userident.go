// Package userident detects the current user identity for scan attribution.
package userident

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
)

// Detect returns the user identity using: git config user.name > OS username > generated fallback.
func Detect(repoRoot string) string {
	return DetectContext(context.Background(), repoRoot)
}

// DetectContext is Detect with cancellation for Git configuration lookup.
func DetectContext(ctx context.Context, repoRoot string) string {
	if name := gitUserNameContext(ctx, repoRoot); name != "" {
		return name
	}
	if name := osUserName(); name != "" {
		return name
	}
	return generatedFallback()
}

func gitUserName(repoRoot string) string {
	return gitUserNameContext(context.Background(), repoRoot)
}

func gitUserNameContext(ctx context.Context, repoRoot string) string {
	cmd := exec.CommandContext(ctx, "git", "config", "user.name")
	cmd.Dir = repoRoot
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func osUserName() string {
	u, err := user.Current()
	if err != nil {
		return ""
	}
	return u.Username
}

func generatedFallback() string {
	home := devspecsHome()
	idFile := filepath.Join(home, "identity")

	data, err := os.ReadFile(idFile)
	if err == nil {
		if id := strings.TrimSpace(string(data)); id != "" {
			return id
		}
	}

	b := make([]byte, 4)
	rand.Read(b)
	id := hex.EncodeToString(b)

	os.MkdirAll(home, 0o755)
	os.WriteFile(idFile, []byte(id+"\n"), 0o644)
	return id
}

func devspecsHome() string {
	if env := os.Getenv("DEVSPECS_HOME"); env != "" {
		return env
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".devspecs")
}
