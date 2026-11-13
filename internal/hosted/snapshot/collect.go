package snapshot

import (
	"context"
	"os"
	"path/filepath"
	"strings"
)

// Evidence describes local committed facts, not server-approved source identity.
// The service independently verifies its own source/ref enrollment before publish.
type Evidence struct {
	Snapshot Snapshot
	RefTip   string
	Parents  []Snapshot
}

func Collect(ctx context.Context, repositoryPath, repoID string, policy Policy, commit, ref string) (Snapshot, error) {
	if !ValidCommit(commit) || !ValidRef(ref) || policy.Validate() != nil {
		return Snapshot{}, ErrInvalid
	}
	root, err := RepositoryRoot(ctx, repositoryPath)
	if err != nil {
		return Snapshot{}, err
	}
	git, err := NewLocalGit(root, repoID, []string{ref})
	if err != nil {
		return Snapshot{}, err
	}
	evidence, err := git.ReadCommitted(ctx, policy, commit, ref)
	if err != nil {
		return Snapshot{}, err
	}
	immutable := make(map[Key]string)
	check := func(s Snapshot) error {
		for _, record := range s.Records {
			if record.Kind != "checkpoint" {
				continue
			}
			digest, err := CheckpointDigest(record)
			if err != nil {
				return err
			}
			if prior, found := immutable[record.Key()]; found && prior != digest {
				return ErrConflict
			}
			immutable[record.Key()] = digest
		}
		return nil
	}
	if err := check(evidence.Snapshot); err != nil {
		return Snapshot{}, err
	}
	for _, parent := range evidence.Parents {
		if err := check(parent); err != nil {
			return Snapshot{}, err
		}
	}
	return evidence.Snapshot, nil
}

func RepositoryRoot(ctx context.Context, start string) (string, error) {
	abs, err := filepath.Abs(start)
	if err != nil {
		return "", ErrProvenance
	}
	physical, err := resolvedPath(abs)
	if err != nil {
		return "", ErrProvenance
	}
	for {
		info, err := os.Lstat(filepath.Join(physical, ".git"))
		if err == nil {
			if !info.IsDir() && !info.Mode().IsRegular() {
				return "", ErrProvenance
			}
			break
		}
		if !os.IsNotExist(err) || filepath.Dir(physical) == physical {
			return "", ErrProvenance
		}
		physical = filepath.Dir(physical)
	}
	binary, err := gitBinary()
	if err != nil {
		return "", err
	}
	g := &LocalGit{repositoryPath: abs, binary: binary}
	ctx, cancel := context.WithTimeout(ctx, GitTimeout)
	defer cancel()
	reported, err := g.run(ctx, 4096, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", err
	}
	root, err := resolvedPath(strings.TrimSpace(string(reported)))
	if err != nil {
		return "", ErrProvenance
	}
	actual, err := os.Stat(root)
	if err != nil {
		return "", ErrProvenance
	}
	expected, err := os.Stat(physical)
	if err != nil || !os.SameFile(actual, expected) {
		return "", ErrProvenance
	}
	return root, nil
}
