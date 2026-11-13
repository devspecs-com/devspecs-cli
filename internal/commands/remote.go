package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/devspecs-com/devspecs-cli/internal/config"
	"github.com/devspecs-com/devspecs-cli/internal/hosted"
	"github.com/devspecs-com/devspecs-cli/internal/repo"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

type remoteFailure struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e remoteFailure) Error() string        { return e.Message }
func remoteError(code, message string) error { return remoteFailure{code, message} }

type remoteResult struct {
	Schema           string                 `json:"schema"`
	SchemaVersion    int                    `json:"schema_version"`
	Status           string                 `json:"status"`
	Remote           *config.RemoteConfig   `json:"remote,omitempty"`
	Remotes          *[]config.RemoteConfig `json:"remotes,omitempty"`
	Identity         *hosted.Identity       `json:"identity,omitempty"`
	CredentialSource string                 `json:"credential_source,omitempty"`
	URL              string                 `json:"url,omitempty"`
	Error            *remoteFailure         `json:"error,omitempty"`
}

// PrintCommandError preserves legacy errors and emits only sanitized versioned
// JSON for the new remote/auth surface, including Cobra argument/flag errors.
func PrintCommandError(out io.Writer, err error, args []string) {
	isRemote := false
	asJSON := false
	for _, arg := range args {
		if arg == "--" {
			break
		}
		if !strings.HasPrefix(arg, "-") {
			isRemote = arg == "remote" || arg == "login" || arg == "logout"
			break
		}
	}
	for _, arg := range args {
		if arg == "--" {
			break
		}
		if arg == "--json" || strings.HasPrefix(arg, "--json=") {
			asJSON = true
		}
		if arg == "--json=false" {
			asJSON = false
		}
	}
	if !isRemote {
		fmt.Fprintln(out, err)
		return
	}
	failure := remoteFailure{"invalid_arguments", "invalid remote command arguments"}
	var known remoteFailure
	if errors.As(err, &known) {
		failure = known
	}
	if !asJSON {
		fmt.Fprintln(out, failure.Message)
		return
	}
	_ = json.NewEncoder(out).Encode(remoteResult{Schema: "devspecs.remote.result", SchemaVersion: 1, Status: "error", Error: &failure})
}

func outputRemote(cmd *cobra.Command, asJSON bool, result remoteResult) error {
	result.Schema = "devspecs.remote.result"
	result.SchemaVersion = 1
	if asJSON {
		if json.NewEncoder(cmd.OutOrStdout()).Encode(result) != nil {
			return remoteError("output_failed", "unable to write remote result")
		}
		return nil
	}
	if result.URL != "" {
		fmt.Fprintln(cmd.OutOrStdout(), result.URL)
	} else if result.Remotes != nil {
		for _, r := range *result.Remotes {
			fmt.Fprintf(cmd.OutOrStdout(), "%s  %s  tenant=%s repo=%s\n", r.Name, r.URL, r.TenantID, r.RepoID)
		}
		if len(*result.Remotes) == 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "No hosted remotes configured")
		}
	} else {
		fmt.Fprintf(cmd.OutOrStdout(), "%s: %s\n", result.Remote.Name, result.Status)
	}
	return nil
}

type remoteDocument struct {
	node yaml.Node
	cfg  config.RepoConfig
	path string
}

func loadRemoteDocument() (remoteDocument, error) {
	wd, err := os.Getwd()
	if err != nil {
		return remoteDocument{}, remoteError("config_failed", "unable to locate remote configuration")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	repository := repo.DetectContext(ctx, wd)
	if !repository.IsGit {
		return remoteDocument{}, remoteError("repository_required", "hosted remote commands require a Git repository; no configuration was changed")
	}
	if ctx.Err() != nil {
		return remoteDocument{}, remoteError("repository_unavailable", "unable to inspect the Git repository within the remote configuration deadline")
	}
	path := config.RepoConfigPath(repository.RootPath)
	return loadRemoteDocumentFile(path)
}

func loadRemoteDocumentFile(path string) (remoteDocument, error) {
	if err := guardRemoteConfigPath(path); err != nil {
		return remoteDocument{}, err
	}
	doc := remoteDocument{path: path}
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		doc.cfg = *config.DefaultRepoConfig()
		if doc.node.Encode(&doc.cfg) != nil {
			return remoteDocument{}, remoteError("config_failed", "unable to prepare remote configuration")
		}
		// Node.Encode produces the root mapping rather than a document wrapper.
		root := doc.node
		doc.node = yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{&root}}
		return doc, nil
	}
	if err != nil {
		return remoteDocument{}, remoteError("config_failed", "unable to read remote configuration")
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, 1048577))
	if err != nil || len(data) > 1048576 {
		return remoteDocument{}, remoteError("config_failed", "remote configuration exceeds the supported read boundary")
	}
	var extra yaml.Node
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	if decoder.Decode(&doc.node) != nil || decoder.Decode(&extra) != io.EOF || len(doc.node.Content) != 1 || doc.node.Content[0].Kind != yaml.MappingNode || doc.node.Decode(&doc.cfg) != nil || doc.cfg.Version != 1 || config.ValidateRepoConfig(&doc.cfg) != nil {
		return remoteDocument{}, remoteError("config_failed", "invalid or unsupported remote configuration; no data changed")
	}
	return doc, nil
}

func (d *remoteDocument) write() error {
	if config.ValidateRemotes(d.cfg.Remotes) != nil {
		return remoteError("config_failed", "invalid remote metadata; no data changed")
	}
	root := d.node.Content[0]
	var remotes yaml.Node
	if remotes.Encode(d.cfg.Remotes) != nil {
		return remoteError("config_failed", "unable to encode remote configuration")
	}
	found := false
	for i := 0; i < len(root.Content); i += 2 {
		if root.Content[i].Value == "remotes" {
			root.Content[i+1] = &remotes
			found = true
			break
		}
	}
	if !found {
		root.Content = append(root.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "remotes"}, &remotes)
	}
	data, err := yaml.Marshal(&d.node)
	if err != nil || len(data) > 1048576 {
		return remoteError("config_failed", "unable to encode bounded remote configuration")
	}
	if err := guardRemoteConfigPath(d.path); err != nil {
		return err
	}
	if writeAtomicFile(d.path, data, 0o644) != nil {
		return remoteError("config_failed", "unable to atomically write remote configuration")
	}
	return nil
}

// Reject static path redirection without changing the shared atomic writer.
// Concurrent same-user replacement between checks and I/O is not prevented.
func guardRemoteConfigPath(path string) error {
	denied := remoteError("config_boundary_invalid", "remote configuration requires a non-redirected .devspecs directory and regular config.yaml file inside the Git root")
	parent, err := os.Lstat(filepath.Dir(path))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil || !parent.IsDir() || parent.Mode()&os.ModeSymlink != 0 || remoteConfigReparsePoint(parent) {
		return denied
	}
	leaf, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil || !leaf.Mode().IsRegular() || leaf.Mode()&os.ModeSymlink != 0 || remoteConfigReparsePoint(leaf) {
		return denied
	}
	return nil
}

func configuredRemote(name string) (config.RemoteConfig, error) {
	doc, err := loadRemoteDocument()
	if err != nil {
		return config.RemoteConfig{}, err
	}
	for _, r := range doc.cfg.Remotes {
		if r.Name == name {
			return r, nil
		}
	}
	return config.RemoteConfig{}, remoteError("remote_missing", "named hosted remote is not configured")
}

func NewRemoteCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "remote", Short: "Manage opt-in hosted remotes and explicitly publish committed snapshots"}
	var addJSON, listJSON, removeJSON bool
	var tenant, repo string
	add := &cobra.Command{Use: "add <name> <url>", Short: "Add explicit HTTPS remote metadata without contacting it", Args: cobra.ExactArgs(2), RunE: func(cmd *cobra.Command, args []string) error {
		r := config.RemoteConfig{Version: 1, Name: args[0], URL: args[1], TenantID: tenant, RepoID: repo}
		if config.ValidateRemotes([]config.RemoteConfig{r}) != nil {
			return remoteError("invalid_remote", "remote requires a name, canonical HTTPS base URL and explicit tenant/repo UUIDs")
		}
		doc, err := loadRemoteDocument()
		if err != nil {
			return err
		}
		for _, existing := range doc.cfg.Remotes {
			if existing.Name == r.Name {
				return remoteError("remote_exists", "remote already exists; remove metadata explicitly before changing it")
			}
		}
		doc.cfg.Remotes = append(doc.cfg.Remotes, r)
		if err := doc.write(); err != nil {
			return err
		}
		return outputRemote(cmd, addJSON, remoteResult{Status: "added", Remote: &r})
	}}
	add.Flags().StringVar(&tenant, "tenant-id", "", "Approved hosted tenant UUID")
	add.Flags().StringVar(&repo, "repo-id", "", "Approved hosted repository UUID")
	add.Flags().BoolVar(&addJSON, "json", false, "Output versioned JSON")
	list := &cobra.Command{Use: "list", Short: "List local remote metadata without reading credentials", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		doc, err := loadRemoteDocument()
		if err != nil {
			return err
		}
		remotes := append([]config.RemoteConfig{}, doc.cfg.Remotes...)
		return outputRemote(cmd, listJSON, remoteResult{Status: "listed", Remotes: &remotes})
	}}
	list.Flags().BoolVar(&listJSON, "json", false, "Output versioned JSON")
	remove := &cobra.Command{Use: "remove <name>", Short: "Remove metadata only; logout first to revoke a saved session", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		doc, err := loadRemoteDocument()
		if err != nil {
			return err
		}
		for i, r := range doc.cfg.Remotes {
			if r.Name == args[0] {
				doc.cfg.Remotes = append(doc.cfg.Remotes[:i], doc.cfg.Remotes[i+1:]...)
				if err := doc.write(); err != nil {
					return err
				}
				return outputRemote(cmd, removeJSON, remoteResult{Status: "removed", Remote: &r})
			}
		}
		return remoteError("remote_missing", "named hosted remote is not configured")
	}}
	remove.Flags().BoolVar(&removeJSON, "json", false, "Output versioned JSON")
	cmd.AddCommand(add, list, remove, newRemotePushCmd(nativeRemotePushDeps))
	return cmd
}
