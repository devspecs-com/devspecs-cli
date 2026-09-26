// Package hubstore owns the durable local hub authority. It does not use the
// rebuildable source index or infer a scope from remote Git history.
package hubstore

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/devspecs-com/devspecs-cli/internal/config"
	sqlite "modernc.org/sqlite"
)

var (
	ErrNotFound            = errors.New("hub: not found")
	ErrConflict            = errors.New("hub: conflict")
	ErrUnauthorized        = errors.New("hub: unauthorized")
	ErrInvalidInput        = errors.New("hub: invalid input")
	ErrArchived            = errors.New("hub: archived")
	ErrExpired             = errors.New("hub: expired")
	ErrUnsupportedFormat   = errors.New("hub: unsupported or damaged format")
	ErrBindingConflict     = errors.New("hub: repository binding conflict")
	ErrBusyRetryable       = errors.New("hub: busy; retryable")
	ErrIdempotencyConflict = errors.New("hub: idempotency conflict")
	ErrReplayGap           = errors.New("hub: replay gap")
	ErrCursorConflict      = errors.New("hub: cursor conflict")
)

type Options struct {
	// Home defaults to config.HomeDir when empty.
	Home string
	// Now is sampled after writer admission and can be injected in tests.
	Now func() time.Time
}

type DB struct {
	sql         *sql.DB
	path        string
	authorityID string
	now         func() time.Time
	readOnly    bool
}

func (d *DB) Close() error        { return d.sql.Close() }
func (d *DB) Path() string        { return d.path }
func (d *DB) AuthorityID() string { return d.authorityID }

// Open creates a v6 authority or migrates a verified older authority.
func Open(ctx context.Context, opts Options) (*DB, error) { return open(ctx, opts, false) }

// OpenReadOnly never creates a home, database, or repository marker.
func OpenReadOnly(ctx context.Context, opts Options) (*DB, error) { return open(ctx, opts, true) }

func open(ctx context.Context, opts Options, readOnly bool) (_ *DB, err error) {
	home := opts.Home
	if home == "" {
		home, err = config.HomeDir()
		if err != nil {
			return nil, err
		}
	}
	home, err = filepath.Abs(home)
	if err != nil {
		return nil, err
	}
	if runtime.GOOS == "windows" && strings.HasPrefix(home, `\\`) {
		return nil, fmt.Errorf("hub home on network path: %w", ErrInvalidInput)
	}
	path := filepath.Join(home, "hub.sqlite")
	if !readOnly {
		if err = os.MkdirAll(home, 0o700); err != nil {
			return nil, err
		}
	}
	_, statErr := os.Stat(path)
	fresh := errors.Is(statErr, os.ErrNotExist)
	if statErr != nil && !fresh {
		return nil, statErr
	}
	if fresh && readOnly {
		return nil, ErrNotFound
	}
	if fresh {
		f, createErr := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o600)
		if createErr == nil {
			err = f.Close()
			if err != nil {
				return nil, err
			}
		} else if errors.Is(createErr, os.ErrExist) {
			fresh = false
		} else {
			return nil, createErr
		}
	}
	mode := "rw"
	if readOnly {
		mode = "ro"
	}
	// SQLite URI mode=rw prevents accidental recreation after the stat check.
	dsn := sqliteDSN(path, mode)
	raw, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	raw.SetMaxOpenConns(1)
	raw.SetMaxIdleConns(1)
	d := &DB{sql: raw, path: path, now: opts.Now, readOnly: readOnly}
	if d.now == nil {
		d.now = time.Now
	}
	defer func() {
		if err != nil {
			_ = raw.Close()
		}
	}()
	if fresh {
		if err = d.initialize(ctx); err != nil {
			return nil, err
		}
	} else {
		if err = d.probeReady(ctx); err != nil {
			return nil, err
		}
	}
	if !readOnly {
		if err = d.configure(ctx); err != nil {
			return nil, err
		}
		if err = d.migrate(ctx); err != nil {
			return nil, err
		}
		// A migration by another process cannot leave this handle writing a newer format.
		if err = d.probe(ctx); err != nil {
			return nil, err
		}
	} else {
		if err = d.probe(ctx); err != nil {
			return nil, err
		}
		// This is a connection-local safety setting and does not mutate the file.
		if _, err = d.sql.ExecContext(ctx, "PRAGMA foreign_keys=ON"); err != nil {
			return nil, err
		}
		var enabled int
		if err = d.sql.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&enabled); err != nil {
			return nil, err
		}
		if enabled != 1 {
			return nil, ErrUnsupportedFormat
		}
	}
	return d, nil
}

func sqliteDSN(path, mode string) string {
	uriPath := filepath.ToSlash(path)
	if runtime.GOOS == "windows" {
		uriPath = "/" + uriPath
	}
	return (&url.URL{Scheme: "file", Path: uriPath, RawQuery: "mode=" + mode}).String()
}

func (d *DB) configure(ctx context.Context) error {
	for _, pragma := range []string{"PRAGMA busy_timeout=2000", "PRAGMA foreign_keys=ON", "PRAGMA journal_mode=WAL", "PRAGMA synchronous=FULL"} {
		if _, err := d.sql.ExecContext(ctx, pragma); err != nil {
			return fmt.Errorf("configure %s: %w", pragma, err)
		}
	}
	var fk int
	if err := d.sql.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&fk); err != nil {
		return err
	}
	if fk != 1 {
		return fmt.Errorf("foreign keys disabled: %w", ErrUnsupportedFormat)
	}
	return nil
}

func (d *DB) initialize(ctx context.Context) error {
	if err := d.configure(ctx); err != nil {
		return err
	}
	tx, err := d.sql.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, schemaV1); err != nil {
		return err
	}
	id, err := randomID()
	if err != nil {
		return err
	}
	now := d.now().UTC().UnixMilli()
	digest := fmt.Sprintf("%x", sha256.Sum256([]byte(schemaV1)))
	if _, err = tx.ExecContext(ctx, "INSERT INTO hub_meta (singleton, db_id, format_version) VALUES (1, ?, 1)", id); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO schema_migrations VALUES (1, ?, ?)", digest, now); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, fmt.Sprintf("PRAGMA application_id=%d", applicationID)); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "PRAGMA user_version=1"); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	d.authorityID = id
	if err := d.migrate(ctx); err != nil {
		return err
	}
	return d.checkIntegrity(ctx)
}

// A second opener can observe the exclusively created file before its first
// transaction commits. Wait briefly for a complete, verifiable authority.
func (d *DB) probeReady(ctx context.Context) error {
	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	for {
		err := d.probeVersion(ctx, true)
		if !errors.Is(err, errInitializationPending) && !errors.Is(classifySQLite(err), ErrBusyRetryable) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return ErrUnsupportedFormat
		case <-time.After(20 * time.Millisecond):
		}
	}
}

var errInitializationPending = errors.New("hub: initialization pending")

func (d *DB) probe(ctx context.Context) error {
	return d.probeVersion(ctx, false)
}

func (d *DB) probeVersion(ctx context.Context, allowV1 bool) error {
	tx, err := d.sql.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var appID, version int
	if err := tx.QueryRowContext(ctx, "PRAGMA application_id").Scan(&appID); err != nil {
		return err
	}
	if err := tx.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	if appID == 0 && version == 0 {
		return errInitializationPending
	}
	if appID != applicationID || (version != schemaVersion && !(allowV1 && version >= 1 && version < schemaVersion)) {
		return fmt.Errorf("hub application_id=%d version=%d, supported=%d/%d: %w", appID, version, applicationID, schemaVersion, ErrUnsupportedFormat)
	}
	var id, digest string
	var format int
	if err := tx.QueryRowContext(ctx, "SELECT db_id, format_version FROM hub_meta WHERE singleton=1").Scan(&id, &format); err != nil {
		return fmt.Errorf("hub metadata: %w: %w", ErrUnsupportedFormat, err)
	}
	if err := tx.QueryRowContext(ctx, "SELECT digest FROM schema_migrations WHERE version=1").Scan(&digest); err != nil {
		return fmt.Errorf("hub migration: %w: %w", ErrUnsupportedFormat, err)
	}
	if format != version || digest != fmt.Sprintf("%x", sha256.Sum256([]byte(schemaV1))) || id == "" {
		return ErrUnsupportedFormat
	}
	var migrationCount int
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM schema_migrations").Scan(&migrationCount); err != nil {
		return err
	}
	if migrationCount != version {
		return ErrUnsupportedFormat
	}
	if version >= 2 {
		if err := tx.QueryRowContext(ctx, "SELECT digest FROM schema_migrations WHERE version=2").Scan(&digest); err != nil {
			return ErrUnsupportedFormat
		}
		if digest != fmt.Sprintf("%x", sha256.Sum256([]byte(schemaV2))) {
			return ErrUnsupportedFormat
		}
	}
	if version >= 3 {
		if err := tx.QueryRowContext(ctx, "SELECT digest FROM schema_migrations WHERE version=3").Scan(&digest); err != nil {
			return ErrUnsupportedFormat
		}
		if digest != fmt.Sprintf("%x", sha256.Sum256([]byte(schemaV3))) {
			return ErrUnsupportedFormat
		}
	}
	if version >= 4 {
		if err := tx.QueryRowContext(ctx, "SELECT digest FROM schema_migrations WHERE version=4").Scan(&digest); err != nil {
			return ErrUnsupportedFormat
		}
		if digest != fmt.Sprintf("%x", sha256.Sum256([]byte(schemaV4))) {
			return ErrUnsupportedFormat
		}
		var secret []byte
		if err := tx.QueryRowContext(ctx, "SELECT secret FROM pull_token_secret WHERE singleton=1").Scan(&secret); err != nil || len(secret) != 32 {
			return ErrUnsupportedFormat
		}
	}
	if version >= 5 {
		if err := tx.QueryRowContext(ctx, "SELECT digest FROM schema_migrations WHERE version=5").Scan(&digest); err != nil || digest != fmt.Sprintf("%x", sha256.Sum256([]byte(schemaV5))) {
			return ErrUnsupportedFormat
		}
	}
	if version >= 6 {
		if err := tx.QueryRowContext(ctx, "SELECT digest FROM schema_migrations WHERE version=6").Scan(&digest); err != nil || digest != fmt.Sprintf("%x", sha256.Sum256([]byte(schemaV6))) {
			return ErrUnsupportedFormat
		}
	}
	d.authorityID = id
	return tx.Commit()
}

func (d *DB) migrate(ctx context.Context) error {
	var currentVersion int
	if err := d.sql.QueryRowContext(ctx, "PRAGMA user_version").Scan(&currentVersion); err != nil {
		return err
	}
	if currentVersion == schemaVersion {
		return nil
	}
	tx, err := d.sql.BeginTx(ctx, nil)
	if err != nil {
		return classifySQLite(err)
	}
	defer tx.Rollback()
	// Serialize concurrent openers before inspecting the version.
	if _, err := tx.ExecContext(ctx, "UPDATE hub_meta SET format_version=format_version WHERE singleton=1"); err != nil {
		return classifySQLite(err)
	}
	var version, format int
	var id, digest string
	if err := tx.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	if version == schemaVersion {
		return nil
	}
	if version < 1 || version >= schemaVersion {
		return ErrUnsupportedFormat
	}
	if err := tx.QueryRowContext(ctx, "SELECT db_id, format_version FROM hub_meta WHERE singleton=1").Scan(&id, &format); err != nil {
		return ErrUnsupportedFormat
	}
	if err := tx.QueryRowContext(ctx, "SELECT digest FROM schema_migrations WHERE version=1").Scan(&digest); err != nil {
		return ErrUnsupportedFormat
	}
	var count int
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM schema_migrations").Scan(&count); err != nil {
		return err
	}
	if id == "" || format != version || count != version || digest != fmt.Sprintf("%x", sha256.Sum256([]byte(schemaV1))) {
		return ErrUnsupportedFormat
	}
	if version == 1 {
		if _, err := tx.ExecContext(ctx, schemaV2); err != nil {
			return err
		}
		digest = fmt.Sprintf("%x", sha256.Sum256([]byte(schemaV2)))
		if _, err := tx.ExecContext(ctx, "INSERT INTO schema_migrations VALUES (2, ?, ?)", digest, d.now().UTC().UnixMilli()); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "PRAGMA user_version=2"); err != nil {
			return err
		}
	} else {
		if err := tx.QueryRowContext(ctx, "SELECT digest FROM schema_migrations WHERE version=2").Scan(&digest); err != nil {
			return ErrUnsupportedFormat
		}
		if digest != fmt.Sprintf("%x", sha256.Sum256([]byte(schemaV2))) {
			return ErrUnsupportedFormat
		}
	}
	if version < 3 {
		if _, err := tx.ExecContext(ctx, schemaV3); err != nil {
			return err
		}
		digest = fmt.Sprintf("%x", sha256.Sum256([]byte(schemaV3)))
		if _, err := tx.ExecContext(ctx, "INSERT INTO schema_migrations VALUES (3, ?, ?)", digest, d.now().UTC().UnixMilli()); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "PRAGMA user_version=3"); err != nil {
			return err
		}
	} else {
		if err := tx.QueryRowContext(ctx, "SELECT digest FROM schema_migrations WHERE version=3").Scan(&digest); err != nil || digest != fmt.Sprintf("%x", sha256.Sum256([]byte(schemaV3))) {
			return ErrUnsupportedFormat
		}
	}
	if version < 4 {
		if _, err := tx.ExecContext(ctx, schemaV4); err != nil {
			return err
		}
		secret := make([]byte, 32)
		if _, err := rand.Read(secret); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO pull_token_secret VALUES (1, ?)", secret); err != nil {
			return err
		}
		digest = fmt.Sprintf("%x", sha256.Sum256([]byte(schemaV4)))
		if _, err := tx.ExecContext(ctx, "INSERT INTO schema_migrations VALUES (4, ?, ?)", digest, d.now().UTC().UnixMilli()); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "PRAGMA user_version=4"); err != nil {
			return err
		}
	} else if err := tx.QueryRowContext(ctx, "SELECT digest FROM schema_migrations WHERE version=4").Scan(&digest); err != nil || digest != fmt.Sprintf("%x", sha256.Sum256([]byte(schemaV4))) {
		return ErrUnsupportedFormat
	}
	if version < 5 {
		if _, err := tx.ExecContext(ctx, schemaV5); err != nil {
			return err
		}
		digest = fmt.Sprintf("%x", sha256.Sum256([]byte(schemaV5)))
		if _, err := tx.ExecContext(ctx, "INSERT INTO schema_migrations VALUES (5, ?, ?)", digest, d.now().UTC().UnixMilli()); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "PRAGMA user_version=5"); err != nil {
			return err
		}
	} else if err := tx.QueryRowContext(ctx, "SELECT digest FROM schema_migrations WHERE version=5").Scan(&digest); err != nil || digest != fmt.Sprintf("%x", sha256.Sum256([]byte(schemaV5))) {
		return ErrUnsupportedFormat
	}
	if _, err := tx.ExecContext(ctx, schemaV6); err != nil {
		return err
	}
	digest = fmt.Sprintf("%x", sha256.Sum256([]byte(schemaV6)))
	if _, err := tx.ExecContext(ctx, "INSERT INTO schema_migrations VALUES (6, ?, ?)", digest, d.now().UTC().UnixMilli()); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "PRAGMA user_version=6"); err != nil {
		return err
	}
	return tx.Commit()
}

// CheckIntegrity runs a full SQLite integrity and foreign key check. Call it
// before maintenance operations such as backing up an existing authority.
func (d *DB) CheckIntegrity(ctx context.Context) error {
	if err := d.probe(ctx); err != nil {
		return err
	}
	return d.checkIntegrity(ctx)
}

func (d *DB) checkIntegrity(ctx context.Context) error {
	var integrity string
	if err := d.sql.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&integrity); err != nil {
		return err
	}
	if integrity != "ok" {
		return fmt.Errorf("hub integrity: %s: %w", integrity, ErrUnsupportedFormat)
	}
	rows, err := d.sql.QueryContext(ctx, "PRAGMA foreign_key_check")
	if err != nil {
		return err
	}
	defer rows.Close()
	if rows.Next() {
		return fmt.Errorf("hub foreign key violation: %w", ErrUnsupportedFormat)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	return nil
}

func (d *DB) write(ctx context.Context, fn func(*sql.Tx, int64) error) (err error) {
	defer func() { err = classifySQLite(err) }()
	if d.readOnly {
		return ErrUnauthorized
	}
	tx, err := d.sql.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// First write obtains SQLite's single-writer slot before sampling the clock.
	if _, err := tx.ExecContext(ctx, "UPDATE hub_meta SET format_version=format_version WHERE singleton=1"); err != nil {
		return err
	}
	var id string
	var version int
	if err := tx.QueryRowContext(ctx, "SELECT db_id, format_version FROM hub_meta WHERE singleton=1").Scan(&id, &version); err != nil {
		return err
	}
	if id != d.authorityID || version != schemaVersion {
		return ErrUnsupportedFormat
	}
	var userVersion int
	if err := tx.QueryRowContext(ctx, "PRAGMA user_version").Scan(&userVersion); err != nil {
		return err
	}
	if userVersion != schemaVersion {
		return ErrUnsupportedFormat
	}
	if err := fn(tx, d.now().UTC().UnixMilli()); err != nil {
		return err
	}
	return tx.Commit()
}

func classifySQLite(err error) error {
	if err == nil {
		return nil
	}
	var sqliteErr *sqlite.Error
	if errors.As(err, &sqliteErr) && (sqliteErr.Code()&0xff == 5 || sqliteErr.Code()&0xff == 6) {
		return fmt.Errorf("%w: %v", ErrBusyRetryable, err)
	}
	return err
}

func randomID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}
