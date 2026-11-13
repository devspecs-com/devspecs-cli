package snapshot

import (
	"bytes"
	"context"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	GitTimeout     = 30 * time.Second
	MaxTreeBytes   = 8 << 20
	MaxTreeEntries = 32768
)

// LocalGit only reads an operator-selected local repository. It never clones,
// fetches, resolves a caller URL, checks out, invokes a hook or reads worktree bytes.
type LocalGit struct {
	repoID         string
	repositoryPath string
	refs           map[string]bool
	binary         string
}

func NewLocalGit(repositoryPath, repoID string, approvedRefs []string) (*LocalGit, error) {
	if !ValidID(repoID) || !filepath.IsAbs(repositoryPath) || strings.HasPrefix(repositoryPath, `\\`) || strings.HasPrefix(repositoryPath, "//") || len(approvedRefs) == 0 || len(approvedRefs) > MaxRoots {
		return nil, ErrProvenance
	}
	binary, err := gitBinary()
	if err != nil {
		return nil, ErrProvenance
	}
	refs := make(map[string]bool)
	for _, ref := range approvedRefs {
		if !ValidRef(ref) || refs[ref] {
			return nil, ErrProvenance
		}
		refs[ref] = true
	}
	return &LocalGit{repoID: repoID, repositoryPath: filepath.Clean(repositoryPath), refs: refs, binary: binary}, nil
}

func gitBinary() (string, error) {
	binary, err := exec.LookPath("git")
	if err != nil {
		return "", ErrProvenance
	}
	binary, err = filepath.Abs(binary)
	if err != nil {
		return "", ErrProvenance
	}
	return binary, nil
}

type cappedOutput struct {
	bytes.Buffer
	limit int
}

func (b *cappedOutput) Write(data []byte) (int, error) {
	if len(data) > b.limit-b.Len() {
		return 0, ErrInvalid
	}
	return b.Buffer.Write(data)
}

func gitEnvironment() []string {
	var env []string
	for _, name := range []string{"SystemRoot", "WINDIR", "PATH", "TMP", "TEMP"} {
		if value, ok := os.LookupEnv(name); ok {
			env = append(env, name+"="+value)
		}
	}
	return append(env,
		"GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_SYSTEM="+os.DevNull, "GIT_CONFIG_GLOBAL="+os.DevNull,
		"GIT_CONFIG_COUNT=0", "GIT_NO_REPLACE_OBJECTS=1", "GIT_NO_LAZY_FETCH=1",
		"GIT_TERMINAL_PROMPT=0", "GIT_OPTIONAL_LOCKS=0", "GIT_GRAFT_FILE="+os.DevNull,
		"GIT_ATTR_NOSYSTEM=1", "LC_ALL=C")
}

func (g *LocalGit) run(ctx context.Context, limit int, args ...string) ([]byte, error) {
	options := []string{"--no-pager", "--no-replace-objects", "--literal-pathspecs", "-C", g.repositoryPath,
		"-c", "protocol.allow=never", "-c", "core.fsmonitor=false", "-c", "core.hooksPath=" + os.DevNull,
		"-c", "core.attributesFile=" + os.DevNull, "-c", "credential.helper=", "-c", "gc.auto=0", "-c", "maintenance.auto=false"}
	cmd := exec.CommandContext(ctx, g.binary, append(options, args...)...)
	cmd.Env = gitEnvironment()
	cmd.WaitDelay = 2 * time.Second
	output := &cappedOutput{limit: limit}
	cmd.Stdout, cmd.Stderr = output, io.Discard
	if cmd.Run() != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, ErrProvenance
	}
	if output.Len() == 0 {
		return []byte{}, nil
	}
	return output.Bytes(), nil
}

func (g *LocalGit) checkObjectStore(ctx context.Context) error {
	configuration, err := g.run(ctx, 8192, "config", "--local", "--list")
	if err != nil {
		return err
	}
	for _, line := range strings.Split(strings.ToLower(string(configuration)), "\n") {
		key, _, _ := strings.Cut(line, "=")
		if key == "extensions.partialclone" || (strings.HasPrefix(key, "remote.") && strings.HasSuffix(key, ".promisor")) {
			return ErrProvenance
		}
	}
	common, err := g.run(ctx, 4096, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return err
	}
	dir := strings.TrimSpace(string(common))
	if !filepath.IsAbs(dir) {
		return ErrProvenance
	}
	for _, leaf := range []string{"objects/info/alternates", "info/grafts", "shallow"} {
		_, err := os.Lstat(filepath.Join(dir, filepath.FromSlash(leaf)))
		if !os.IsNotExist(err) {
			return ErrProvenance
		}
	}
	packs, err := os.ReadDir(filepath.Join(dir, "objects", "pack"))
	if err != nil && !os.IsNotExist(err) {
		return ErrProvenance
	}
	for _, pack := range packs {
		if strings.HasSuffix(pack.Name(), ".promisor") {
			return ErrProvenance
		}
	}
	return nil
}

func (g *LocalGit) ReadCommitted(ctx context.Context, policy Policy, commit, ref string) (Evidence, error) {
	if policy.Validate() != nil || !ValidCommit(commit) || !g.refs[ref] {
		return Evidence{}, ErrProvenance
	}
	ctx, cancel := context.WithTimeout(ctx, GitTimeout)
	defer cancel()
	if err := g.checkObjectStore(ctx); err != nil {
		return Evidence{}, err
	}
	tipBytes, err := g.run(ctx, 128, "show-ref", "--verify", "--hash", "--", ref)
	if err != nil {
		return Evidence{}, err
	}
	tip := strings.TrimSpace(string(tipBytes))
	if !ValidCommit(tip) {
		return Evidence{}, ErrProvenance
	}
	if _, err = g.run(ctx, 1, "merge-base", "--is-ancestor", commit, tip); err != nil {
		return Evidence{}, ErrProvenance
	}
	commitBytes, err := g.run(ctx, MaxFileBytes, "cat-file", "commit", commit)
	if err != nil || gitObjectID("commit", commitBytes, len(commit)) != commit {
		return Evidence{}, ErrProvenance
	}
	parentsBytes, err := g.run(ctx, 1024, "rev-list", "--parents", "--max-count=1", commit, "--")
	if err != nil {
		return Evidence{}, err
	}
	parents := strings.Fields(string(parentsBytes))
	if len(parents) == 0 || parents[0] != commit || len(parents)-1 > MaxParents {
		return Evidence{}, ErrProvenance
	}
	snapshot, err := g.collect(ctx, policy, commit, ref)
	if err != nil {
		return Evidence{}, err
	}
	evidence := Evidence{Snapshot: snapshot, RefTip: tip}
	for _, parent := range parents[1:] {
		if !ValidCommit(parent) {
			return Evidence{}, ErrProvenance
		}
		parentSnapshot, err := g.collect(ctx, policy, parent, ref)
		if err != nil {
			return Evidence{}, err
		}
		evidence.Parents = append(evidence.Parents, parentSnapshot)
	}
	return evidence, nil
}

func gitObjectID(kind string, data []byte, width int) string {
	var h hash.Hash = sha1.New()
	if width == 64 {
		h = sha256.New()
	}
	_, _ = fmt.Fprintf(h, "%s %d\x00", kind, len(data))
	_, _ = h.Write(data)
	return hex.EncodeToString(h.Sum(nil))
}

type gitEntry struct {
	mode, kind, oid string
	size            int
}

type gitCorpus struct {
	source  *LocalGit
	ctx     context.Context
	entries map[string]gitEntry
	files   map[string]File
	total   int
}

func (g *LocalGit) collect(ctx context.Context, policy Policy, commit, ref string) (Snapshot, error) {
	commitBytes, err := g.run(ctx, MaxFileBytes, "cat-file", "commit", commit)
	if err != nil || gitObjectID("commit", commitBytes, len(commit)) != commit {
		return Snapshot{}, ErrProvenance
	}
	data, err := g.run(ctx, MaxTreeBytes, "ls-tree", "-r", "-t", "-z", "--full-tree", "--long", commit)
	if err != nil {
		return Snapshot{}, err
	}
	corpus := &gitCorpus{source: g, ctx: ctx, entries: make(map[string]gitEntry), files: make(map[string]File)}
	for _, raw := range bytes.Split(data, []byte{0}) {
		if len(raw) == 0 {
			continue
		}
		header, name, ok := bytes.Cut(raw, []byte{'\t'})
		fields := strings.Fields(string(header))
		if !ok || len(fields) != 4 || !ValidCommit(fields[2]) || len(corpus.entries) >= MaxTreeEntries {
			return Snapshot{}, ErrProvenance
		}
		size := 0
		if fields[1] == "blob" {
			size, err = strconv.Atoi(fields[3])
			if err != nil || size < 0 {
				return Snapshot{}, ErrProvenance
			}
		}
		corpus.entries[string(name)] = gitEntry{fields[0], fields[1], fields[2], size}
	}
	snapshot := Snapshot{Schema: Schema, SchemaVersion: 1, PolicyID: policy.PolicyID, Commit: commit, Ref: ref, Records: []Record{}}
	for _, root := range policy.Roots {
		if err := corpus.checkRoot(root); err != nil {
			return Snapshot{}, err
		}
		var records []Record
		switch root.Kind {
		case "task_parent":
			records, err = corpus.tasks(root, g.repoID)
		case "adr", "rfc", "prd":
			records, err = corpus.documents(root, g.repoID)
		case "workspace_manifest":
			records, err = corpus.workspace(root, g.repoID)
		default:
			return Snapshot{}, ErrUnsupported
		}
		if err != nil {
			return Snapshot{}, err
		}
		snapshot.Records = append(snapshot.Records, records...)
	}
	if err := snapshot.Validate(); err != nil {
		return Snapshot{}, err
	}
	return snapshot, nil
}

func (c *gitCorpus) checkRoot(root Root) error {
	parts := strings.Split(root.Path, "/")
	for i := range parts {
		name := strings.Join(parts[:i+1], "/")
		entry, found := c.entries[name]
		if !found {
			continue
		}
		if entry.kind != "tree" && (i != len(parts)-1 || root.Kind != "workspace_manifest") {
			return ErrProvenance
		}
		if entry.mode == "120000" || entry.mode == "160000" {
			return ErrProvenance
		}
	}
	return nil
}

func (c *gitCorpus) read(name string) (File, error) {
	if !ValidPath(name) {
		return File{}, ErrInvalid
	}
	if file, found := c.files[name]; found {
		return file, nil
	}
	entry, found := c.entries[name]
	if !found || entry.kind != "blob" || (entry.mode != "100644" && entry.mode != "100755") || entry.size > MaxFileBytes || len(c.files) >= MaxFiles || c.total+entry.size > MaxDecodedBytes {
		return File{}, ErrProvenance
	}
	for dir := path.Dir(name); dir != "."; dir = path.Dir(dir) {
		if parent, ok := c.entries[dir]; !ok || parent.kind != "tree" || parent.mode != "040000" {
			return File{}, ErrProvenance
		}
	}
	data, err := c.source.run(c.ctx, MaxFileBytes, "cat-file", "blob", entry.oid)
	if err != nil || len(data) != entry.size || gitObjectID("blob", data, len(entry.oid)) != entry.oid {
		return File{}, ErrProvenance
	}
	file := File{Path: name, SHA256: Hash(data), Content: data}
	c.files[name] = file
	c.total += len(data)
	return file, nil
}
