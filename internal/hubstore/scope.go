package hubstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

const markerFile = ".devspecs-hub-incarnation"

type Scope struct {
	ID        string
	CommonDir string
}

type Actor struct {
	ID         string
	EnrolledAt int64 // UTC milliseconds
}

type repoEvidence struct{ path, key, marker, identity string }

func checkEvidence(ev repoEvidence) error {
	data, err := os.ReadFile(filepath.Join(ev.path, markerFile))
	if err != nil {
		return ErrBindingConflict
	}
	identity, err := platformFileIdentity(ev.path)
	if err != nil {
		return ErrBindingConflict
	}
	if strings.TrimSpace(string(data)) != ev.marker || identity != ev.identity {
		return ErrBindingConflict
	}
	return nil
}

func resolveGit(ctx context.Context, repoPath string, createMarker bool) (repoEvidence, error) {
	if repoPath == "" {
		return repoEvidence{}, ErrInvalidInput
	}
	out, err := exec.CommandContext(ctx, "git", "-C", repoPath, "rev-parse", "--path-format=absolute", "--git-common-dir").Output()
	if err != nil {
		return repoEvidence{}, fmt.Errorf("resolve Git common dir: %w", err)
	}
	common := strings.TrimSpace(string(out))
	if !filepath.IsAbs(common) {
		return repoEvidence{}, fmt.Errorf("nonabsolute Git common dir: %w", ErrBindingConflict)
	}
	common, err = filepath.EvalSymlinks(common)
	if err != nil {
		return repoEvidence{}, err
	}
	common, err = filepath.Abs(common)
	if err != nil {
		return repoEvidence{}, err
	}
	common = filepath.Clean(common)
	key := common
	if runtime.GOOS == "windows" {
		key = strings.ToLower(key)
	}
	identity, err := platformFileIdentity(common)
	if err != nil {
		return repoEvidence{}, err
	}
	ev := repoEvidence{path: common, key: key, identity: identity}
	if createMarker {
		if err := createRepoMarker(common); err != nil {
			return repoEvidence{}, err
		}
	}
	return readRepoMarker(ev)
}

func createRepoMarker(common string) error {
	markerPath := filepath.Join(common, markerFile)
	marker, genErr := randomID()
	if genErr != nil {
		return genErr
	}
	f, createErr := os.OpenFile(markerPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if createErr == nil {
		if _, err := f.WriteString(marker + "\n"); err != nil {
			_ = f.Close()
			return err
		}
		if err := f.Sync(); err != nil {
			_ = f.Close()
			return err
		}
		if err := f.Close(); err != nil {
			return err
		}
	} else if !errors.Is(createErr, os.ErrExist) {
		return createErr
	}
	return nil
}

func readRepoMarker(ev repoEvidence) (repoEvidence, error) {
	markerPath := filepath.Join(ev.path, markerFile)
	data, err := os.ReadFile(markerPath)
	if errors.Is(err, os.ErrNotExist) {
		return ev, ErrNotFound
	}
	if err != nil {
		return repoEvidence{}, err
	}
	marker := strings.TrimSpace(string(data))
	if len(marker) != 32 || !isHex(marker) {
		return repoEvidence{}, ErrBindingConflict
	}
	ev.marker = marker
	return ev, nil
}

func isHex(s string) bool {
	for _, c := range s {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

// LookupRepo checks a binding without creating a marker or enrolling a scope.
func (d *DB) LookupRepo(ctx context.Context, repoPath string) (Scope, error) {
	ev, err := resolveGit(ctx, repoPath, false)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return Scope{}, err
	}
	var scope Scope
	var marker, identity string
	err = d.sql.QueryRowContext(ctx, "SELECT scope_id, marker, file_identity FROM scope_bindings WHERE common_dir=?", ev.key).Scan(&scope.ID, &marker, &identity)
	if errors.Is(err, sql.ErrNoRows) {
		return Scope{}, ErrNotFound
	}
	if err != nil {
		return Scope{}, err
	}
	if ev.marker == "" || marker != ev.marker || identity != ev.identity {
		return Scope{}, ErrBindingConflict
	}
	scope.CommonDir = ev.path
	return scope, nil
}

// EnrollRepo gives each home its own scope for a Git common directory.
func (d *DB) EnrollRepo(ctx context.Context, repoPath string) (Scope, error) {
	if d.readOnly {
		return Scope{}, ErrUnauthorized
	}
	ev, markerErr := resolveGit(ctx, repoPath, false)
	if markerErr != nil && !errors.Is(markerErr, ErrNotFound) {
		return Scope{}, markerErr
	}
	var scope Scope
	err := d.write(ctx, func(tx *sql.Tx, now int64) error {
		var marker, identity string
		err := tx.QueryRowContext(ctx, "SELECT scope_id, marker, file_identity FROM scope_bindings WHERE common_dir=?", ev.key).Scan(&scope.ID, &marker, &identity)
		if err == nil {
			if markerErr != nil || marker != ev.marker || identity != ev.identity || checkEvidence(ev) != nil {
				return ErrBindingConflict
			}
			scope.CommonDir = ev.path
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if errors.Is(markerErr, ErrNotFound) {
			if err := createRepoMarker(ev.path); err != nil {
				return err
			}
			ev, err = readRepoMarker(ev)
			if err != nil {
				return err
			}
		}
		if err := checkEvidence(ev); err != nil {
			return err
		}
		// An unchanged file identity at a different, now missing path signals a move.
		var oldPath string
		err = tx.QueryRowContext(ctx, "SELECT common_dir FROM scope_bindings WHERE marker=? AND file_identity=? LIMIT 1", ev.marker, ev.identity).Scan(&oldPath)
		if err == nil {
			return ErrBindingConflict
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		id, err := randomID()
		if err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, "INSERT INTO repo_scopes (scope_id, kind, enrolled_at) VALUES (?, 'git', ?)", id, now); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, "INSERT INTO scope_bindings VALUES (?, ?, ?, ?)", id, ev.key, ev.marker, ev.identity); err != nil {
			return err
		}
		scope = Scope{ID: id, CommonDir: ev.path}
		return nil
	})
	return scope, err
}

// EnrollActor stores a caller-chosen stable local attribution ID. An empty ID
// allocates an opaque one. Same-account clients may claim any enrolled actor.
func (d *DB) EnrollActor(ctx context.Context, actorID string) (Actor, error) {
	if actorID == "" {
		var err error
		actorID, err = randomID()
		if err != nil {
			return Actor{}, err
		}
	}
	if len(actorID) > 64 {
		return Actor{}, ErrInvalidInput
	}
	for _, c := range actorID {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_' || c == '.') {
			return Actor{}, ErrInvalidInput
		}
	}
	var a Actor
	err := d.write(ctx, func(tx *sql.Tx, now int64) error {
		_, err := tx.ExecContext(ctx, "INSERT INTO actors VALUES (?, ?) ON CONFLICT(actor_id) DO NOTHING", actorID, now)
		if err != nil {
			return err
		}
		return tx.QueryRowContext(ctx, "SELECT actor_id, enrolled_at FROM actors WHERE actor_id=?", actorID).Scan(&a.ID, &a.EnrolledAt)
	})
	return a, err
}
