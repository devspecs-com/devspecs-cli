package commands

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// A shared phase budget lets a slow lookup finish without multiplying the
// deadline by the number of displayed areas. Exhaustion must not erase evidence.
const mapGitEvidenceTimeout = 10 * time.Second

func mapGitHistoryAvailable(ctx context.Context, repoRoot string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if _, err := os.Stat(filepath.Join(repoRoot, ".git")); errors.Is(err, os.ErrNotExist) {
		return false, nil
	} else if err != nil {
		return false, fmt.Errorf("inspect map git directory: %w", err)
	}
	if err := exec.CommandContext(ctx, "git", "-C", repoRoot, "rev-parse", "--is-inside-work-tree").Run(); err != nil {
		if ctx.Err() != nil {
			return false, ctx.Err()
		}
		return false, fmt.Errorf("inspect map git repository: %w", err)
	}
	err := exec.CommandContext(ctx, "git", "-C", repoRoot, "rev-parse", "--verify", "--quiet", "HEAD^{commit}").Run()
	if err != nil {
		if ctx.Err() != nil {
			return false, ctx.Err()
		}
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			// Git status succeeds for an unborn branch, but rejects a corrupt HEAD.
			if statusErr := exec.CommandContext(ctx, "git", "--no-optional-locks", "-C", repoRoot, "status", "--porcelain=v1", "--untracked-files=no").Run(); statusErr == nil {
				return false, nil
			}
			if ctx.Err() != nil {
				return false, ctx.Err()
			}
		}
		return false, fmt.Errorf("inspect map git HEAD: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	return true, nil
}
