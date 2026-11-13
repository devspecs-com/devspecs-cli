package orchestration

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type httpDriverGit struct{ executable, home, objectDirectory string }

func newHTTPDriverLocalGit() (*httpDriverGit, error) {
	executable, err := exec.LookPath("git")
	if err != nil {
		return nil, errHTTPDriverBinding
	}
	executable, err = filepath.Abs(executable)
	if err != nil {
		return nil, errHTTPDriverBinding
	}
	return &httpDriverGit{executable: executable, home: os.DevNull}, nil
}

func httpDriverRepositoryRoot(ctx context.Context, runner *httpDriverGit, name string) (string, error) {
	physical, err := httpDriverTaskRoot(name)
	if err != nil {
		return "", err
	}
	raw, err := runner.checked(ctx, name, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", err
	}
	reported, err := httpDriverResolvedPath(strings.TrimSpace(string(raw)))
	if err != nil {
		return "", errHTTPDriverBinding
	}
	physicalInfo, err := os.Stat(physical)
	if err != nil {
		return "", errHTTPDriverBinding
	}
	reportedInfo, err := os.Stat(reported)
	if err != nil || !os.SameFile(physicalInfo, reportedInfo) {
		return "", errHTTPDriverBinding
	}
	return reported, nil
}

func checkHTTPDriverGitPolicy(ctx context.Context, runner *httpDriverGit, checkout string) error {
	// Attribute-selected executable filters and partial clones are not supported.
	// Deny them before status/object inspection; do not reinterpret their output.
	result, err := runner.Run(ctx, checkout, "git", "config", "--get-regexp", `^(filter\.|diff\..*\.(command|textconv)$|extensions\.partialclone$|remote\..*\.(promisor|partialclonefilter)$)`)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err != nil || result.ExitCode != 1 || len(result.Stdout) != 0 || len(result.Stderr) != 0 {
		return errHTTPDriverBinding
	}
	return nil
}

type httpDriverGitOutput struct {
	bytes.Buffer
	remaining int64
	overflow  bool
}

func (w *httpDriverGitOutput) Write(p []byte) (int, error) {
	n := len(p)
	if int64(n) > w.remaining {
		p = p[:w.remaining]
		w.overflow = true
	}
	_, _ = w.Buffer.Write(p)
	w.remaining -= int64(len(p))
	return n, nil
}

func (g *httpDriverGit) command(ctx context.Context, dir string, args ...string) *exec.Cmd {
	prefix := []string{"--no-replace-objects", "-c", "core.hooksPath=" + g.home, "-c", "core.fsmonitor=false", "-c", "core.attributesFile=" + os.DevNull,
		"-c", "gc.auto=0", "-c", "maintenance.auto=false", "-c", "protocol.allow=never", "-c", "fetch.fsckObjects=true", "-c", "transfer.fsckObjects=true",
		"-c", "credential.helper=", "-c", "core.askPass=", "-c", "pack.threads=1", "-c", "index.threads=1", "-c", "pack.windowMemory=32m", "-c", "pack.deltaCacheSize=32m", "-c", "core.bigFileThreshold=16m"}
	cmd := exec.CommandContext(ctx, g.executable, append(prefix, args...)...)
	cmd.Dir = dir
	cmd.WaitDelay = 2 * time.Second
	// No inherited GIT_*, agent token, credential helper, source config, HOME
	// config or alternate object database can participate in quarantine Git.
	for _, key := range []string{"PATH", "SystemRoot", "WINDIR", "TEMP", "TMP", "PATHEXT"} {
		if value, ok := os.LookupEnv(key); ok {
			cmd.Env = append(cmd.Env, key+"="+value)
		}
	}
	cmd.Env = append(cmd.Env, "HOME="+g.home, "USERPROFILE="+g.home, "XDG_CONFIG_HOME="+g.home, "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull,
		"GIT_CONFIG_SYSTEM="+os.DevNull, "GIT_ATTR_NOSYSTEM=1", "GIT_NO_REPLACE_OBJECTS=1", "GIT_NO_LAZY_FETCH=1", "GIT_TERMINAL_PROMPT=0", "GIT_OPTIONAL_LOCKS=0")
	if g.objectDirectory != "" {
		cmd.Env = append(cmd.Env, "GIT_OBJECT_DIRECTORY="+g.objectDirectory)
	}
	return cmd
}

func (g *httpDriverGit) Run(ctx context.Context, dir, executable string, args ...string) (CommandResult, error) {
	if executable != "git" {
		return CommandResult{}, errHTTPDriverBundle
	}
	stdout := &httpDriverGitOutput{remaining: 8 << 20}
	stderr := &httpDriverGitOutput{remaining: httpDispatchSmallBody}
	cmd := g.command(ctx, dir, args...)
	cmd.Stdout, cmd.Stderr = stdout, stderr
	err := cmd.Run()
	result := CommandResult{Stdout: stdout.Bytes(), Stderr: stderr.Bytes()}
	if len(result.Stderr) > 0 {
		result.Stderr = []byte("HTTP dispatch local Git diagnostic omitted")
	}
	if ctx.Err() != nil {
		return result, ctx.Err()
	}
	if stdout.overflow || stderr.overflow {
		return result, errHTTPDriverBundle
	}
	if err == nil {
		return result, nil
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		result.ExitCode = exit.ExitCode()
		return result, nil
	}
	return result, errHTTPDriverBundle
}

func (g *httpDriverGit) checked(ctx context.Context, dir string, args ...string) ([]byte, error) {
	result, err := g.Run(ctx, dir, "git", args...)
	if err != nil {
		return nil, err
	}
	if result.ExitCode != 0 {
		return nil, errHTTPDriverBundle
	}
	return result.Stdout, nil
}

func (g *httpDriverGit) stream(ctx context.Context, dir string, in io.Reader, out io.Writer, args ...string) error {
	cmd := g.command(ctx, dir, args...)
	cmd.Stdin, cmd.Stdout = in, out
	stderr := &httpDriverGitOutput{remaining: httpDispatchSmallBody}
	cmd.Stderr = stderr
	err := cmd.Run()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err != nil || stderr.overflow {
		return errHTTPDriverBundle
	}
	return nil
}

type httpDriverLimitedWriter struct {
	out       io.Writer
	remaining int64
}

func (w *httpDriverLimitedWriter) Write(p []byte) (int, error) {
	if int64(len(p)) > w.remaining {
		return 0, errHTTPDriverBundle
	}
	n, err := w.out.Write(p)
	w.remaining -= int64(n)
	return n, err
}

func newHTTPDriverQuarantine(ctx context.Context, stage, checkout, source string) (*httpDriverGit, string, error) {
	if !httpDriverSHA1.MatchString(source) {
		return nil, "", errHTTPDriverBundle
	}
	executable, err := exec.LookPath("git")
	if err != nil {
		return nil, "", errHTTPDriverBundle
	}
	executable, err = filepath.Abs(executable)
	if err != nil {
		return nil, "", errHTTPDriverBundle
	}
	home := filepath.Join(stage, "empty-home")
	if err := os.Mkdir(home, 0o700); err != nil {
		return nil, "", errHTTPDriverState
	}
	runner := &httpDriverGit{executable: executable, home: home}
	seed, quarantine := filepath.Join(stage, "export.git"), filepath.Join(stage, "quarantine.git")
	if _, err := runner.checked(ctx, stage, "init", "--bare", "--object-format=sha1", "--template=", seed); err != nil {
		return nil, "", err
	}
	if _, err := runner.checked(ctx, stage, "init", "--bare", "--object-format=sha1", "--template=", quarantine); err != nil {
		return nil, "", err
	}
	objects, err := httpDriverSourceObjects(ctx, checkout)
	if err != nil {
		return nil, "", err
	}
	export := *runner
	export.objectDirectory = objects
	if _, err := export.checked(ctx, seed, "update-ref", "refs/heads/source", source); err != nil {
		return nil, "", err
	}
	file, err := os.OpenFile(filepath.Join(stage, "source.bundle"), os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return nil, "", errHTTPDriverState
	}
	defer file.Close()
	if err := export.stream(ctx, seed, nil, &httpDriverLimitedWriter{out: file, remaining: 128 << 20}, "bundle", "create", "--version=2", "-", "refs/heads/source"); err != nil {
		return nil, "", err
	}
	if err := file.Sync(); err != nil {
		return nil, "", errHTTPDriverState
	}
	if _, err := runner.checked(ctx, quarantine, "bundle", "verify", file.Name()); err != nil {
		return nil, "", err
	}
	if _, err := runner.checked(ctx, quarantine, "bundle", "unbundle", file.Name()); err != nil {
		return nil, "", err
	}
	if _, err := runner.checked(ctx, quarantine, "update-ref", "refs/heads/source", source); err != nil {
		return nil, "", err
	}
	if _, err := runner.checked(ctx, quarantine, "fsck", "--full", "--strict", "--no-reflogs", "--no-dangling", source); err != nil {
		return nil, "", err
	}
	return runner, quarantine, nil
}

func httpDriverGitPathFile(name string) (string, error) {
	file, err := os.Open(name)
	if err != nil {
		return "", err
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, 4097))
	if err != nil || len(raw) > 4096 {
		return "", errHTTPDriverBundle
	}
	value := strings.TrimSuffix(strings.TrimSuffix(string(raw), "\n"), "\r")
	if strings.ContainsAny(value, "\r\n\x00") || value == "" {
		return "", errHTTPDriverBundle
	}
	return value, nil
}

func httpDriverSourceObjects(ctx context.Context, checkout string) (string, error) {
	gitDir := filepath.Join(checkout, ".git")
	info, err := os.Lstat(gitDir)
	if err != nil {
		return "", errHTTPDriverBundle
	}
	if info.Mode().IsRegular() {
		value, err := httpDriverGitPathFile(gitDir)
		if err != nil || !strings.HasPrefix(value, "gitdir: ") {
			return "", errHTTPDriverBundle
		}
		gitDir = strings.TrimPrefix(value, "gitdir: ")
		if !filepath.IsAbs(gitDir) {
			gitDir = filepath.Join(checkout, gitDir)
		}
	} else if !info.IsDir() {
		return "", errHTTPDriverBundle
	}
	gitDir, err = httpDriverResolvedPath(gitDir)
	if err != nil {
		return "", errHTTPDriverBundle
	}
	common, err := httpDriverGitPathFile(filepath.Join(gitDir, "commondir"))
	if err == nil {
		if !filepath.IsAbs(common) {
			common = filepath.Join(gitDir, common)
		}
		gitDir, err = httpDriverResolvedPath(common)
		if err != nil {
			return "", errHTTPDriverBundle
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", errHTTPDriverBundle
	}
	objects, err := httpDriverResolvedPath(filepath.Join(gitDir, "objects"))
	if err != nil {
		return "", errHTTPDriverBundle
	}
	entries := 0
	err = filepath.WalkDir(objects, func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return errHTTPDriverBundle
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		entries++
		if entries > 200000 || (entry.Type()&os.ModeSymlink) != 0 || (!entry.IsDir() && !entry.Type().IsRegular()) ||
			entry.Name() == "alternates" || entry.Name() == "http-alternates" || strings.HasSuffix(entry.Name(), ".promisor") {
			return errHTTPDriverBundle
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	return objects, nil
}
