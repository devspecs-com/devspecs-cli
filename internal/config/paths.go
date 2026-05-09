// Package config manages DevSpecs configuration: global home directory,
// repo-local .devspecs/config.yaml, and sensible defaults.
package config

import (
	"os"
	"path/filepath"
)

// HomeDir returns the DevSpecs global directory (~/.devspecs or DEVSPECS_HOME override).
func HomeDir() (string, error) {
	if env := os.Getenv("DEVSPECS_HOME"); env != "" {
		return env, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".devspecs"), nil
}

// DBPath returns the path to the global SQLite database.
func DBPath() (string, error) {
	dir, err := HomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "devspecs.db"), nil
}
