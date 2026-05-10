package commands

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/devspecs-com/devspecs-cli/internal/config"
	"github.com/devspecs-com/devspecs-cli/internal/store"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// NewConfigCmd creates the ds config command group.
func NewConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage DevSpecs configuration",
	}

	cmd.AddCommand(newConfigShowCmd())
	cmd.AddCommand(newConfigPathsCmd())
	cmd.AddCommand(newConfigAddSourceCmd())
	cmd.AddCommand(newConfigSetCmd())
	return cmd
}

func newConfigShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "Print the effective configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConfigShow(cmd)
		},
	}
}

func newConfigPathsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "paths",
		Short: "List all scan paths with existence status",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConfigPaths(cmd)
		},
	}
}

func newConfigAddSourceCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "add-source <type> <path>",
		Short: "Add a source entry to config.yaml",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConfigAddSource(cmd, args[0], args[1])
		},
	}
}

func newConfigSetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "set <key> <value>",
		Short: "Set a top-level config value",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConfigSet(cmd, args[0], args[1])
		},
	}
}

func runConfigShow(cmd *cobra.Command) error {
	wd, err := os.Getwd()
	if err != nil {
		return err
	}
	repoRoot := resolveRepoRootFromWd(wd)

	cfg, err := config.LoadRepoConfig(repoRoot)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	out := cmd.OutOrStdout()
	if cfg == nil {
		cfg = config.DefaultRepoConfig()
		fmt.Fprintln(out, "(defaults — no .devspecs/config.yaml found)")
		fmt.Fprintln(out)
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	fmt.Fprint(out, string(data))
	return nil
}

func runConfigPaths(cmd *cobra.Command) error {
	wd, err := os.Getwd()
	if err != nil {
		return err
	}
	repoRoot := resolveRepoRootFromWd(wd)

	cfg, err := config.LoadRepoConfig(repoRoot)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	if cfg == nil {
		cfg = config.DefaultRepoConfig()
	}

	out := cmd.OutOrStdout()
	for _, src := range cfg.Sources {
		paths := src.Paths
		if src.Path != "" && len(paths) == 0 {
			paths = []string{src.Path}
		}
		for _, p := range paths {
			absPath := filepath.Join(repoRoot, p)
			status := "[ok]"
			if _, err := os.Stat(absPath); os.IsNotExist(err) {
				status = "[missing]"
			}
			fmt.Fprintf(out, "%s  %s  (%s)\n", status, absPath, src.Type)
		}
	}
	return nil
}

func runConfigAddSource(cmd *cobra.Command, sourceType, sourcePath string) error {
	wd, err := os.Getwd()
	if err != nil {
		return err
	}
	repoRoot := resolveRepoRootFromWd(wd)

	cfg, err := config.LoadRepoConfig(repoRoot)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	if cfg == nil {
		cfg = config.DefaultRepoConfig()
	}

	for _, src := range cfg.Sources {
		if src.Type == sourceType {
			if src.Path == sourcePath {
				fmt.Fprintf(cmd.OutOrStdout(), "Source %s:%s already exists\n", sourceType, sourcePath)
				return nil
			}
			for _, p := range src.Paths {
				if p == sourcePath {
					fmt.Fprintf(cmd.OutOrStdout(), "Source %s:%s already exists\n", sourceType, sourcePath)
					return nil
				}
			}
		}
	}

	found := false
	for i, src := range cfg.Sources {
		if src.Type == sourceType {
			cfg.Sources[i].Paths = append(cfg.Sources[i].Paths, sourcePath)
			found = true
			break
		}
	}
	if !found {
		cfg.Sources = append(cfg.Sources, config.SourceConfig{Type: sourceType, Path: sourcePath})
	}

	if err := config.WriteRepoConfig(repoRoot, cfg); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Added source %s:%s\n", sourceType, sourcePath)
	return nil
}

func runConfigSet(cmd *cobra.Command, key, value string) error {
	wd, err := os.Getwd()
	if err != nil {
		return err
	}
	repoRoot := resolveRepoRootFromWd(wd)

	cfg, err := config.LoadRepoConfig(repoRoot)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	if cfg == nil {
		cfg = config.DefaultRepoConfig()
	}

	switch key {
	case "version":
		v := 1
		fmt.Sscanf(value, "%d", &v)
		cfg.Version = v
	default:
		return fmt.Errorf("unknown config key: %s", key)
	}

	if err := config.WriteRepoConfig(repoRoot, cfg); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Set %s = %s\n", key, value)
	return nil
}

// displayID returns the short_id if available, otherwise the full ID.
func displayID(art *store.ArtifactRow) string {
	if art.ShortID != "" {
		return art.ShortID
	}
	return art.ID
}
