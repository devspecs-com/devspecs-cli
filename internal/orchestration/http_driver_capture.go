package orchestration

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strconv"
	"time"
)

type httpDriverCaptureExec struct{ tokenEnv string }

func (r httpDriverCaptureExec) Run(ctx context.Context, dir, executable string, args ...string) (CommandResult, error) {
	command := exec.CommandContext(ctx, executable, args...)
	command.Dir = dir
	command.WaitDelay = 2 * time.Second
	for _, key := range []string{"PATH", "SystemRoot", "WINDIR", "TEMP", "TMP", "PATHEXT", "HOME", "USERPROFILE", "HOMEDRIVE", "HOMEPATH", "APPDATA", "LOCALAPPDATA", "XDG_CONFIG_HOME", "DEVSPECS_HOME"} {
		if key != r.tokenEnv {
			if value, ok := os.LookupEnv(key); ok {
				command.Env = append(command.Env, key+"="+value)
			}
		}
	}
	command.Env = append(command.Env, "DEVSPECS_TELEMETRY=0", "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_SYSTEM="+os.DevNull, "GIT_ATTR_NOSYSTEM=1", "GIT_NO_REPLACE_OBJECTS=1", "GIT_NO_LAZY_FETCH=1", "GIT_TERMINAL_PROMPT=0", "GIT_OPTIONAL_LOCKS=0")
	settings := [][2]string{{"core.hooksPath", os.DevNull}, {"core.fsmonitor", "false"}, {"core.attributesFile", os.DevNull}, {"credential.helper", ""}, {"core.askPass", ""}, {"protocol.allow", "never"}, {"gc.auto", "0"}, {"maintenance.auto", "false"}}
	command.Env = append(command.Env, "GIT_CONFIG_COUNT="+strconv.Itoa(len(settings)))
	for i, setting := range settings {
		command.Env = append(command.Env, "GIT_CONFIG_KEY_"+strconv.Itoa(i)+"="+setting[0], "GIT_CONFIG_VALUE_"+strconv.Itoa(i)+"="+setting[1])
	}
	stdout := &httpDriverGitOutput{remaining: maxDispatchFileBytes}
	stderr := &httpDriverGitOutput{remaining: httpDispatchSmallBody}
	command.Stdout, command.Stderr = stdout, stderr
	err := command.Run()
	if ctx.Err() != nil {
		return CommandResult{}, ctx.Err()
	}
	if stdout.overflow || stderr.overflow {
		return CommandResult{}, errHTTPDriverGoal
	}
	result := CommandResult{Stdout: stdout.Bytes()}
	if err == nil {
		return result, nil
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		result.ExitCode = exit.ExitCode()
		return result, nil
	}
	return CommandResult{}, errHTTPDriverGoal
}
