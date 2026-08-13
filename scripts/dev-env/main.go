package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/devspecs-com/devspecs-cli/scripts/internal/devhome"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	os.Exit(run(ctx, os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("dev-env", flag.ContinueOnError)
	flags.SetOutput(stderr)
	channel := flags.String("channel", "", "persistent development channel key")
	worktree := flags.Bool("worktree", false, "use a home scoped to the DevSpecs CLI source worktree (default when --channel is omitted)")
	sourceRoot := flags.String("source-root", "", "DevSpecs CLI source worktree root")
	childDir := flags.String("child-dir", "", "working directory for the child command")
	asJSON := flags.Bool("json", false, "print the resolved selection as JSON without running a child command")
	quiet := flags.Bool("quiet", false, "print only the selected home when no command is supplied")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	childArgs := flags.Args()
	if *asJSON && len(childArgs) > 0 {
		fmt.Fprintln(stderr, "--json cannot be combined with a child command")
		return 2
	}

	selection, err := devhome.Resolve(devhome.Options{
		Channel:    *channel,
		Worktree:   *worktree,
		SourceRoot: *sourceRoot,
		DevRoot:    os.Getenv("DEVSPECS_DEV_HOME_ROOT"),
	})
	if err != nil {
		fmt.Fprintf(stderr, "resolve development home: %v\n", err)
		return 1
	}
	if _, err := devhome.Prepare(selection, time.Now()); err != nil {
		fmt.Fprintf(stderr, "prepare development home: %v\n", err)
		return 1
	}

	if *asJSON {
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(selection); err != nil {
			fmt.Fprintf(stderr, "write development home selection: %v\n", err)
			return 1
		}
		return 0
	}
	if len(childArgs) == 0 {
		if *quiet {
			fmt.Fprintln(stdout, selection.Home)
		} else {
			fmt.Fprintf(stdout, "DevSpecs development home\nScope: %s\nKey: %s\nHome: %s\nTelemetry default: disabled\n", selection.Scope, selection.Key, selection.Home)
		}
		return 0
	}

	if !*quiet {
		fmt.Fprintf(stderr, "DevSpecs development home: %s (%s)\n", selection.Home, selection.Scope)
	}
	command := exec.CommandContext(ctx, childArgs[0], childArgs[1:]...)
	command.Stdin = stdin
	command.Stdout = stdout
	command.Stderr = stderr
	command.Env = devhome.ChildEnvironment(os.Environ(), selection.Home)
	if *childDir != "" {
		absoluteChildDir, err := filepath.Abs(*childDir)
		if err != nil {
			fmt.Fprintf(stderr, "resolve child working directory: %v\n", err)
			return 1
		}
		command.Dir = absoluteChildDir
	}
	if err := command.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return exitErr.ExitCode()
		}
		if errors.Is(err, exec.ErrNotFound) {
			fmt.Fprintf(stderr, "start child command: %v\n", err)
			return 127
		}
		fmt.Fprintf(stderr, "run child command: %v\n", err)
		return 1
	}
	return 0
}
