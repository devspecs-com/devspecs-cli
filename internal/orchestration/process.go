package orchestration

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
)

type CommandResult struct {
	Stdout   []byte
	Stderr   []byte
	ExitCode int
}

type CommandRunner interface {
	Run(ctx context.Context, dir, executable string, args ...string) (CommandResult, error)
}

type ExecRunner struct{}

func (ExecRunner) Run(ctx context.Context, dir, executable string, args ...string) (CommandResult, error) {
	command := exec.CommandContext(ctx, executable, args...)
	command.Dir = dir
	command.Env = os.Environ()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()
	result := CommandResult{Stdout: stdout.Bytes(), Stderr: stderr.Bytes()}
	if ctx.Err() != nil {
		return result, ctx.Err()
	}
	if err == nil {
		return result, nil
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		return result, fmt.Errorf("start %s: %w", executable, err)
	}
	result.ExitCode = exitErr.ExitCode()
	return result, nil
}
