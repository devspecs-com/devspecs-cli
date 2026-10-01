package commands

import (
	"os"
	"runtime"
	"strings"
)

const evalTelemetryMode = "0"

type evalEnvironmentValue struct {
	value   string
	present bool
}

func setEvalHome(home string) (func(), error) {
	previousHome, hadHome := os.LookupEnv("DEVSPECS_HOME")
	previousTelemetry, hadTelemetry := os.LookupEnv("DEVSPECS_TELEMETRY")
	if err := os.Setenv("DEVSPECS_HOME", home); err != nil {
		return nil, err
	}
	if err := os.Setenv("DEVSPECS_TELEMETRY", evalTelemetryMode); err != nil {
		restoreEnvironmentValue("DEVSPECS_HOME", evalEnvironmentValue{value: previousHome, present: hadHome})
		return nil, err
	}
	return func() {
		restoreEnvironmentValue("DEVSPECS_TELEMETRY", evalEnvironmentValue{value: previousTelemetry, present: hadTelemetry})
		restoreEnvironmentValue("DEVSPECS_HOME", evalEnvironmentValue{value: previousHome, present: hadHome})
	}, nil
}

func evalChildEnvironment(home string) []string {
	return environmentWithOverrides(os.Environ(), map[string]string{
		"DEVSPECS_HOME":      home,
		"DEVSPECS_TELEMETRY": evalTelemetryMode,
	})
}

func environmentWithOverrides(base []string, overrides map[string]string) []string {
	out := make([]string, 0, len(base)+len(overrides))
	for _, item := range base {
		key, _, ok := strings.Cut(item, "=")
		if ok && environmentOverridesKey(overrides, key) {
			continue
		}
		out = append(out, item)
	}
	for _, key := range []string{"DEVSPECS_HOME", "DEVSPECS_TELEMETRY"} {
		if value, ok := overrides[key]; ok {
			out = append(out, key+"="+value)
		}
	}
	return out
}

func environmentOverridesKey(overrides map[string]string, candidate string) bool {
	for key := range overrides {
		if runtime.GOOS == "windows" && strings.EqualFold(candidate, key) {
			return true
		}
		if candidate == key {
			return true
		}
	}
	return false
}

func restoreEnvironmentValue(key string, previous evalEnvironmentValue) {
	if previous.present {
		_ = os.Setenv(key, previous.value)
		return
	}
	_ = os.Unsetenv(key)
}
