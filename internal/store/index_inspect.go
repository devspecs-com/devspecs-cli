package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/gofrs/flock"
)

const (
	IndexCompatibilityAbsent  = "absent"
	IndexCompatibilityCurrent = "current"
	IndexCompatibilityOlder   = "older"
	IndexCompatibilityNewer   = "newer"

	IndexWriterStateNotPresent = "not_present"
	IndexWriterStateAvailable  = "available"
	IndexWriterStateHeld       = "held"
	IndexWriterStateUnknown    = "unknown"
)

// IndexInspection describes the on-disk index without creating or migrating it.
type IndexInspection struct {
	Path            string `json:"path"`
	Exists          bool   `json:"exists"`
	DatabaseBytes   int64  `json:"database_bytes"`
	WALBytes        int64  `json:"wal_bytes"`
	DatabaseSchema  int    `json:"database_schema"`
	SupportedSchema int    `json:"supported_schema"`
	Compatibility   string `json:"compatibility"`
}

// InspectIndex opens an existing SQLite index in query-only mode. It does not
// create parent directories, create a database, or run DevSpecs migrations.
func InspectIndex(ctx context.Context, dbPath string) (IndexInspection, error) {
	inspection := IndexInspection{
		Path:            dbPath,
		SupportedSchema: SchemaVersion,
		Compatibility:   IndexCompatibilityAbsent,
	}
	info, err := os.Stat(dbPath)
	if errors.Is(err, os.ErrNotExist) {
		return inspection, nil
	}
	if err != nil {
		return inspection, fmt.Errorf("stat index: %w", err)
	}
	if !info.Mode().IsRegular() {
		return inspection, fmt.Errorf("index path is not a regular file")
	}
	inspection.Exists = true
	inspection.DatabaseBytes = info.Size()
	if walInfo, walErr := os.Stat(dbPath + "-wal"); walErr == nil && walInfo.Mode().IsRegular() {
		inspection.WALBytes = walInfo.Size()
	}

	database, err := sql.Open("sqlite", readOnlyIndexDSN(dbPath))
	if err != nil {
		return inspection, fmt.Errorf("open index read-only: %w", err)
	}
	defer database.Close()
	database.SetMaxOpenConns(1)

	var migrationTables int
	if err := database.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'schema_migrations'",
	).Scan(&migrationTables); err != nil {
		return inspection, fmt.Errorf("inspect schema table: %w", err)
	}
	if migrationTables == 0 {
		return inspection, fmt.Errorf("index is missing schema_migrations")
	}
	if err := database.QueryRowContext(ctx,
		"SELECT COALESCE(MAX(version), 0) FROM schema_migrations",
	).Scan(&inspection.DatabaseSchema); err != nil {
		return inspection, fmt.Errorf("inspect schema version: %w", err)
	}
	inspection.Compatibility = classifyIndexCompatibility(inspection.DatabaseSchema)
	return inspection, nil
}

// InspectIndexWriter observes an existing writer lock without waiting for it.
// The result is an instantaneous diagnostic and does not identify the owner.
func InspectIndexWriter(dbPath string) (string, error) {
	lockPath := dbPath + ".writer.lock"
	info, err := os.Stat(lockPath)
	if errors.Is(err, os.ErrNotExist) {
		return IndexWriterStateNotPresent, nil
	}
	if err != nil {
		return IndexWriterStateUnknown, fmt.Errorf("stat index writer lock: %w", err)
	}
	if !info.Mode().IsRegular() {
		return IndexWriterStateUnknown, fmt.Errorf("index writer lock is not a regular file")
	}

	fileLock := flock.New(lockPath)
	available, err := fileLock.TryLock()
	if err != nil {
		_ = fileLock.Close()
		return IndexWriterStateUnknown, fmt.Errorf("inspect index writer lock: %w", err)
	}
	if !available {
		_ = fileLock.Close()
		return IndexWriterStateHeld, nil
	}
	unlockErr := fileLock.Unlock()
	closeErr := fileLock.Close()
	if err := errors.Join(unlockErr, closeErr); err != nil {
		return IndexWriterStateUnknown, fmt.Errorf("release index writer probe: %w", err)
	}
	return IndexWriterStateAvailable, nil
}

func readOnlyIndexDSN(dbPath string) string {
	absolute, err := filepath.Abs(dbPath)
	if err != nil {
		absolute = dbPath
	}
	uriPath := filepath.ToSlash(absolute)
	if filepath.VolumeName(absolute) != "" && !strings.HasPrefix(uriPath, "/") {
		uriPath = "/" + uriPath
	}
	uri := &url.URL{
		Scheme:   "file",
		Path:     uriPath,
		RawQuery: "mode=ro&_pragma=query_only(ON)&_pragma=busy_timeout(0)",
	}
	return uri.String()
}

func classifyIndexCompatibility(databaseSchema int) string {
	switch {
	case databaseSchema < SchemaVersion:
		return IndexCompatibilityOlder
	case databaseSchema > SchemaVersion:
		return IndexCompatibilityNewer
	default:
		return IndexCompatibilityCurrent
	}
}
