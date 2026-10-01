package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"modernc.org/sqlite"
)

const (
	IndexBackupReasonManual  = "manual"
	IndexBackupReasonMigrate = "migrate"
	IndexBackupReasonRebuild = "rebuild"
	IndexBackupReasonRestore = "restore"

	IndexBackupRetentionManual    = "manual"
	IndexBackupRetentionAutomatic = "automatic"

	indexRecoveryManifestVersion = 1
	indexRecoveryJournalVersion  = 1
	indexRecoverySpaceMargin     = uint64(64 * 1024 * 1024)
)

var (
	availableIndexDiskBytes = indexDiskFreeBytes
	publishStagedIndex      = os.Rename
)

// IndexValidation describes a schema-agnostic integrity check.
type IndexValidation struct {
	Path       string `json:"path"`
	Schema     int    `json:"schema"`
	Bytes      int64  `json:"bytes"`
	PageCount  int64  `json:"page_count"`
	PageSize   int64  `json:"page_size"`
	Integrity  string `json:"integrity"`
	Compatible string `json:"compatibility"`
}

// IndexBackup records a verified, self-contained SQLite snapshot.
type IndexBackup struct {
	Path            string `json:"path"`
	ManifestPath    string `json:"manifest_path"`
	SourcePath      string `json:"source_path,omitempty"`
	SourceSchema    int    `json:"source_schema"`
	SupportedSchema int    `json:"supported_schema"`
	Bytes           int64  `json:"bytes"`
	PageCount       int64  `json:"page_count"`
	PageSize        int64  `json:"page_size"`
	CreatedAt       string `json:"created_at"`
	Reason          string `json:"reason"`
	Retention       string `json:"retention"`
	Integrity       string `json:"integrity"`
}

// IndexReplacement reports publication of a verified staged index.
type IndexReplacement struct {
	ActivePath             string       `json:"active_path"`
	ActiveSchema           int          `json:"active_schema"`
	Compatibility          string       `json:"compatibility"`
	DisplacedIndexBackup   *IndexBackup `json:"displaced_index_backup,omitempty"`
	PreservedIndexPath     string       `json:"preserved_index_path,omitempty"`
	RepositoryFilesWritten bool         `json:"repository_files_written"`
}

// IndexRecoveryJournal preserves the exact files involved in an interrupted swap.
type IndexRecoveryJournal struct {
	Version      int    `json:"version"`
	OperationID  string `json:"operation_id"`
	Operation    string `json:"operation"`
	ActivePath   string `json:"active_path"`
	StagedPath   string `json:"staged_path"`
	RollbackPath string `json:"rollback_path"`
	BackupPath   string `json:"backup_path,omitempty"`
	SourceSchema int    `json:"source_schema"`
	TargetSchema int    `json:"target_schema"`
	Phase        string `json:"phase"`
	StartedAt    string `json:"started_at"`
	UpdatedAt    string `json:"updated_at"`
}

// RecoveryPendingError prevents normal index use while a durable swap journal exists.
type RecoveryPendingError struct {
	JournalPath string
	Journal     IndexRecoveryJournal
}

func (err *RecoveryPendingError) Error() string {
	return fmt.Sprintf("index recovery is incomplete (%s at %s); run 'ds doctor' and resolve it with 'ds index restore <backup-file>'", err.Journal.Phase, err.JournalPath)
}

// ValidateIndex verifies a database without applying DevSpecs migrations.
func ValidateIndex(ctx context.Context, dbPath string) (IndexValidation, error) {
	return validateIndexWithDSN(ctx, dbPath, readOnlyIndexDSN(dbPath))
}

func validateIndexSnapshot(ctx context.Context, dbPath string) (IndexValidation, error) {
	return validateIndexWithDSN(ctx, dbPath, immutableIndexDSN(dbPath))
}

func validateIndexWithDSN(ctx context.Context, dbPath, dsn string) (IndexValidation, error) {
	validation := IndexValidation{Path: dbPath}
	info, err := os.Stat(dbPath)
	if err != nil {
		return validation, fmt.Errorf("stat index: %w", err)
	}
	if !info.Mode().IsRegular() {
		return validation, fmt.Errorf("index path is not a regular file")
	}
	validation.Bytes = info.Size()
	database, err := sql.Open("sqlite", dsn)
	if err != nil {
		return validation, fmt.Errorf("open index for validation: %w", err)
	}
	defer database.Close()
	database.SetMaxOpenConns(1)

	if err := database.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&validation.Integrity); err != nil {
		return validation, fmt.Errorf("check index integrity: %w", err)
	}
	if !strings.EqualFold(strings.TrimSpace(validation.Integrity), "ok") {
		return validation, fmt.Errorf("check index integrity: %s", validation.Integrity)
	}
	var migrationTables int
	if err := database.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'schema_migrations'",
	).Scan(&migrationTables); err != nil {
		return validation, fmt.Errorf("inspect schema table: %w", err)
	}
	if migrationTables != 1 {
		return validation, fmt.Errorf("index is missing schema_migrations")
	}
	if err := database.QueryRowContext(ctx, "SELECT COALESCE(MAX(version), 0) FROM schema_migrations").Scan(&validation.Schema); err != nil {
		return validation, fmt.Errorf("inspect schema version: %w", err)
	}
	if err := database.QueryRowContext(ctx, "PRAGMA page_count").Scan(&validation.PageCount); err != nil {
		return validation, fmt.Errorf("inspect index page count: %w", err)
	}
	if err := database.QueryRowContext(ctx, "PRAGMA page_size").Scan(&validation.PageSize); err != nil {
		return validation, fmt.Errorf("inspect index page size: %w", err)
	}
	if validation.PageCount <= 0 || validation.PageSize <= 0 {
		return validation, fmt.Errorf("index has invalid page geometry")
	}
	validation.Compatible = classifyIndexCompatibility(validation.Schema)
	return validation, nil
}

// CreateIndexBackup creates a verified online snapshot. Callers performing a
// mutation must hold the index writer lease for the source database.
func CreateIndexBackup(ctx context.Context, sourcePath, outputPath, reason, retention string) (IndexBackup, error) {
	source, err := ValidateIndex(ctx, sourcePath)
	if err != nil {
		return IndexBackup{}, fmt.Errorf("validate backup source: %w", err)
	}
	if reason == "" {
		reason = IndexBackupReasonManual
	}
	if retention == "" {
		retention = IndexBackupRetentionManual
	}
	now := time.Now().UTC()
	operationID, err := newIndexRecoveryID()
	if err != nil {
		return IndexBackup{}, err
	}
	if outputPath == "" {
		outputPath = managedIndexBackupPath(sourcePath, source.Schema, reason, now, operationID)
	}
	outputPath, err = filepath.Abs(outputPath)
	if err != nil {
		return IndexBackup{}, fmt.Errorf("resolve backup path: %w", err)
	}
	if sameIndexPath(sourcePath, outputPath) {
		return IndexBackup{}, fmt.Errorf("backup destination must differ from the active index")
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return IndexBackup{}, fmt.Errorf("create backup directory: %w", err)
	}
	if _, err := os.Stat(outputPath); err == nil {
		return IndexBackup{}, fmt.Errorf("backup destination already exists: %s", outputPath)
	} else if !errors.Is(err, os.ErrNotExist) {
		return IndexBackup{}, fmt.Errorf("inspect backup destination: %w", err)
	}
	logicalBytes := uint64(source.PageCount) * uint64(source.PageSize)
	if err := requireIndexDiskSpace(filepath.Dir(outputPath), logicalBytes); err != nil {
		return IndexBackup{}, err
	}
	temporaryPath := outputPath + ".tmp-" + operationID
	defer func() { _ = removeIndexFiles(temporaryPath) }()
	if err := onlineBackupIndex(ctx, sourcePath, temporaryPath); err != nil {
		return IndexBackup{}, fmt.Errorf("create online index backup: %w", err)
	}
	verified, err := validateIndexSnapshot(ctx, temporaryPath)
	if err != nil {
		return IndexBackup{}, fmt.Errorf("validate index backup: %w", err)
	}
	if verified.Schema != source.Schema {
		return IndexBackup{}, fmt.Errorf("validate index backup: schema changed from v%d to v%d", source.Schema, verified.Schema)
	}
	if err := syncIndexFile(temporaryPath); err != nil {
		return IndexBackup{}, err
	}
	if err := os.Rename(temporaryPath, outputPath); err != nil {
		return IndexBackup{}, fmt.Errorf("publish index backup: %w", err)
	}
	record := IndexBackup{
		Path:            outputPath,
		ManifestPath:    outputPath + ".json",
		SourcePath:      sourcePath,
		SourceSchema:    source.Schema,
		SupportedSchema: SchemaVersion,
		Bytes:           verified.Bytes,
		PageCount:       verified.PageCount,
		PageSize:        verified.PageSize,
		CreatedAt:       now.Format(time.RFC3339Nano),
		Reason:          reason,
		Retention:       retention,
		Integrity:       verified.Integrity,
	}
	manifest := struct {
		Version int `json:"version"`
		IndexBackup
	}{Version: indexRecoveryManifestVersion, IndexBackup: record}
	if err := writeIndexJSON(record.ManifestPath, manifest); err != nil {
		_ = os.Remove(outputPath)
		return IndexBackup{}, fmt.Errorf("write backup manifest: %w", err)
	}
	return record, nil
}

// NewIndexStagePath returns a collision-resistant sibling path for a staged index.
func NewIndexStagePath(activePath string) (string, error) {
	id, err := newIndexRecoveryID()
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(activePath), "."+filepath.Base(activePath)+".stage-"+id), nil
}

// RequireIndexRebuildSpace checks peak local space before creating a staged
// rebuild. This implementation needs room for both the stage and a verified
// rollback backup in addition to the existing active index.
func RequireIndexRebuildSpace(ctx context.Context, activePath string) error {
	estimate := uint64(0)
	if info, err := os.Stat(activePath); err == nil {
		if !info.Mode().IsRegular() {
			return fmt.Errorf("active index is not a regular file")
		}
		estimate = uint64(info.Size())
		if walInfo, walErr := os.Stat(activePath + "-wal"); walErr == nil && walInfo.Mode().IsRegular() {
			estimate += uint64(walInfo.Size())
		}
		if validation, validationErr := ValidateIndex(ctx, activePath); validationErr == nil {
			logical := uint64(validation.PageCount) * uint64(validation.PageSize)
			if logical > estimate {
				estimate = logical
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect active index for rebuild: %w", err)
	}
	return requireIndexDiskSpace(filepath.Dir(activePath), estimate*2)
}

// ReplaceIndexWithStage verifies and atomically publishes a staged database.
// The active database is backed up before the first rename.
func ReplaceIndexWithStage(ctx context.Context, activePath, stagedPath, reason string, expectedSchema int) (IndexReplacement, error) {
	stage, err := checkpointAndValidateIndex(ctx, stagedPath)
	if err != nil {
		return IndexReplacement{}, fmt.Errorf("validate staged index: %w", err)
	}
	if stage.Schema != expectedSchema {
		return IndexReplacement{}, fmt.Errorf("validate staged index: schema v%d, expected v%d", stage.Schema, expectedSchema)
	}

	var sourceSchema int
	var backup *IndexBackup
	preserveUnreadable := false
	activeExists, err := regularIndexExists(activePath)
	if err != nil {
		return IndexReplacement{}, err
	}
	if activeExists {
		active, validationErr := ValidateIndex(ctx, activePath)
		if validationErr != nil {
			if reason != IndexBackupReasonRebuild {
				return IndexReplacement{}, fmt.Errorf("preserve active index before %s: %w", reason, validationErr)
			}
			preserveUnreadable = true
		} else {
			sourceSchema = active.Schema
			record, backupErr := CreateIndexBackup(ctx, activePath, "", reason, IndexBackupRetentionAutomatic)
			if backupErr != nil {
				return IndexReplacement{}, fmt.Errorf("preserve active index before %s: %w", reason, backupErr)
			}
			backup = &record
		}
	}

	operationID, err := newIndexRecoveryID()
	if err != nil {
		return IndexReplacement{}, err
	}
	rollbackPath := activePath + ".rollback-" + operationID
	now := time.Now().UTC().Format(time.RFC3339Nano)
	journal := IndexRecoveryJournal{
		Version:      indexRecoveryJournalVersion,
		OperationID:  operationID,
		Operation:    reason,
		ActivePath:   activePath,
		StagedPath:   stagedPath,
		RollbackPath: rollbackPath,
		SourceSchema: sourceSchema,
		TargetSchema: expectedSchema,
		Phase:        "prepared",
		StartedAt:    now,
		UpdatedAt:    now,
	}
	if backup != nil {
		journal.BackupPath = backup.Path
	}
	journalPath := IndexRecoveryJournalPath(activePath)
	if err := writeIndexJSON(journalPath, journal); err != nil {
		return IndexReplacement{}, fmt.Errorf("write index recovery journal: %w", err)
	}

	moved, err := moveActiveIndexAside(activePath, rollbackPath)
	if err != nil {
		_ = os.Remove(journalPath)
		return IndexReplacement{}, err
	}
	journal.Phase = "active_preserved"
	journal.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if err := writeIndexJSON(journalPath, journal); err != nil {
		restoreErr := restoreMovedIndex(moved)
		if restoreErr == nil {
			_ = os.Remove(journalPath)
		}
		return IndexReplacement{}, errors.Join(fmt.Errorf("update index recovery journal: %w", err), restoreErr)
	}
	if err := publishStagedIndex(stagedPath, activePath); err != nil {
		restoreErr := restoreMovedIndex(moved)
		if restoreErr == nil {
			_ = os.Remove(journalPath)
		}
		return IndexReplacement{}, errors.Join(fmt.Errorf("publish staged index: %w", err), restoreErr)
	}
	journal.Phase = "stage_published"
	journal.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if err := writeIndexJSON(journalPath, journal); err != nil {
		return IndexReplacement{}, fmt.Errorf("record staged index publication: %w", err)
	}
	published, err := ValidateIndex(ctx, activePath)
	if err != nil || published.Schema != expectedSchema {
		failedPath := stagedPath + ".failed"
		moveFailedErr := os.Rename(activePath, failedPath)
		restoreErr := restoreMovedIndex(moved)
		if moveFailedErr == nil && restoreErr == nil {
			_ = os.Remove(journalPath)
		}
		if err == nil {
			err = fmt.Errorf("published schema v%d, expected v%d", published.Schema, expectedSchema)
		}
		return IndexReplacement{}, errors.Join(fmt.Errorf("validate published index: %w", err), moveFailedErr, restoreErr)
	}
	preservedIndexPath := ""
	if preserveUnreadable {
		preservedIndexPath, err = preserveMovedIndexEvidence(activePath, moved, reason, operationID)
		if err != nil {
			return IndexReplacement{}, fmt.Errorf("preserve unreadable replaced index: %w", err)
		}
	} else if err := removeMovedIndex(moved); err != nil {
		return IndexReplacement{}, fmt.Errorf("remove replaced index files: %w", err)
	}
	journal.Phase = "complete"
	journal.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if err := writeIndexJSON(journalPath, journal); err != nil {
		return IndexReplacement{}, fmt.Errorf("complete index recovery journal: %w", err)
	}
	if err := os.Remove(journalPath); err != nil {
		return IndexReplacement{}, fmt.Errorf("remove completed index recovery journal: %w", err)
	}
	if backup != nil {
		_ = pruneAutomaticIndexBackups(filepath.Dir(activePath), backup.Path)
	}
	return IndexReplacement{
		ActivePath:             activePath,
		ActiveSchema:           published.Schema,
		Compatibility:          published.Compatible,
		DisplacedIndexBackup:   backup,
		PreservedIndexPath:     preservedIndexPath,
		RepositoryFilesWritten: false,
	}, nil
}

// RestoreIndex publishes an exact staged copy of backupPath. The selected
// backup remains untouched and the displaced active index receives a backup.
func RestoreIndex(ctx context.Context, activePath, backupPath string) (IndexReplacement, error) {
	selected, err := validateIndexSnapshot(ctx, backupPath)
	if err != nil {
		return IndexReplacement{}, fmt.Errorf("validate selected backup: %w", err)
	}
	stagedPath, err := NewIndexStagePath(activePath)
	if err != nil {
		return IndexReplacement{}, err
	}
	defer func() { _ = removeIndexFiles(stagedPath) }()
	if err := os.MkdirAll(filepath.Dir(activePath), 0o755); err != nil {
		return IndexReplacement{}, fmt.Errorf("create index directory: %w", err)
	}
	logicalBytes := uint64(selected.PageCount) * uint64(selected.PageSize)
	if err := requireIndexDiskSpace(filepath.Dir(activePath), logicalBytes); err != nil {
		return IndexReplacement{}, err
	}
	if err := onlineBackupIndexSnapshot(ctx, backupPath, stagedPath); err != nil {
		return IndexReplacement{}, fmt.Errorf("stage selected backup: %w", err)
	}
	return ReplaceIndexWithStage(ctx, activePath, stagedPath, IndexBackupReasonRestore, selected.Schema)
}

func migrateIndexCopyOnWrite(ctx context.Context, activePath string) error {
	source, err := ValidateIndex(ctx, activePath)
	if err != nil {
		return fmt.Errorf("validate index before migration: %w", err)
	}
	if source.Schema == SchemaVersion {
		return nil
	}
	if source.Schema > SchemaVersion {
		return &NewerSchemaError{DatabaseVersion: source.Schema, SupportedVersion: SchemaVersion}
	}
	stagedPath, err := NewIndexStagePath(activePath)
	if err != nil {
		return err
	}
	defer func() { _ = removeIndexFiles(stagedPath) }()
	logicalBytes := uint64(source.PageCount) * uint64(source.PageSize)
	if err := requireIndexDiskSpace(filepath.Dir(activePath), logicalBytes*2); err != nil {
		return err
	}
	if err := onlineBackupIndex(ctx, activePath, stagedPath); err != nil {
		return fmt.Errorf("stage index migration: %w", err)
	}
	staged, err := openSQLiteIndex(stagedPath)
	if err != nil {
		return err
	}
	migrationErr := staged.migrate()
	closeErr := staged.Close()
	if err := errors.Join(migrationErr, closeErr); err != nil {
		return fmt.Errorf("migrate staged index: %w", err)
	}
	if _, err := ReplaceIndexWithStage(ctx, activePath, stagedPath, IndexBackupReasonMigrate, SchemaVersion); err != nil {
		return fmt.Errorf("publish migrated index: %w", err)
	}
	return nil
}

// IndexRecoveryJournalPath returns the fixed journal path for an active index.
func IndexRecoveryJournalPath(activePath string) string {
	return filepath.Join(filepath.Dir(activePath), "index-recovery.json")
}

// InspectIndexRecovery reads an interrupted operation without changing it.
func InspectIndexRecovery(activePath string) (*IndexRecoveryJournal, error) {
	journalPath := IndexRecoveryJournalPath(activePath)
	body, err := os.ReadFile(journalPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read index recovery journal: %w", err)
	}
	var journal IndexRecoveryJournal
	if err := json.Unmarshal(body, &journal); err != nil {
		return nil, fmt.Errorf("parse index recovery journal: %w", err)
	}
	return &journal, nil
}

func blockPendingIndexRecovery(activePath string) error {
	journal, err := InspectIndexRecovery(activePath)
	if err != nil {
		return err
	}
	if journal == nil || !sameIndexPath(journal.ActivePath, activePath) {
		return nil
	}
	return &RecoveryPendingError{JournalPath: IndexRecoveryJournalPath(activePath), Journal: *journal}
}

func checkpointAndValidateIndex(ctx context.Context, dbPath string) (IndexValidation, error) {
	dsn := fmt.Sprintf("%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(%d)", dbPath, SQLiteBusyTimeoutMS)
	database, err := sql.Open("sqlite", dsn)
	if err != nil {
		return IndexValidation{}, fmt.Errorf("open staged index: %w", err)
	}
	database.SetMaxOpenConns(1)
	if _, err := database.ExecContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		_ = database.Close()
		return IndexValidation{}, fmt.Errorf("checkpoint staged index: %w", err)
	}
	if err := database.Close(); err != nil {
		return IndexValidation{}, fmt.Errorf("close staged index: %w", err)
	}
	_ = os.Remove(dbPath + "-wal")
	_ = os.Remove(dbPath + "-shm")
	return validateIndexSnapshot(ctx, dbPath)
}

func onlineBackupIndex(ctx context.Context, sourcePath, destinationPath string) error {
	return onlineBackupIndexWithDSN(ctx, readOnlyIndexDSN(sourcePath), destinationPath)
}

func onlineBackupIndexSnapshot(ctx context.Context, sourcePath, destinationPath string) error {
	return onlineBackupIndexWithDSN(ctx, immutableIndexDSN(sourcePath), destinationPath)
}

func onlineBackupIndexWithDSN(ctx context.Context, sourceDSN, destinationPath string) error {
	database, err := sql.Open("sqlite", sourceDSN)
	if err != nil {
		return err
	}
	defer database.Close()
	database.SetMaxOpenConns(1)
	connection, err := database.Conn(ctx)
	if err != nil {
		return err
	}
	defer connection.Close()
	type backuper interface {
		NewBackup(string) (*sqlite.Backup, error)
	}
	return connection.Raw(func(driverConnection any) error {
		provider, ok := driverConnection.(backuper)
		if !ok {
			return fmt.Errorf("sqlite driver does not support online backup")
		}
		backup, err := provider.NewBackup(destinationPath)
		if err != nil {
			return err
		}
		finished := false
		defer func() {
			if !finished {
				_ = backup.Finish()
			}
		}()
		for {
			if err := ctx.Err(); err != nil {
				return err
			}
			more, err := backup.Step(256)
			if err != nil {
				return err
			}
			if !more {
				break
			}
		}
		if err := backup.Finish(); err != nil {
			return err
		}
		finished = true
		return nil
	})
}

func requireIndexDiskSpace(destinationDir string, estimate uint64) error {
	available, err := availableIndexDiskBytes(destinationDir)
	if err != nil {
		return err
	}
	margin := estimate / 10
	if margin < indexRecoverySpaceMargin {
		margin = indexRecoverySpaceMargin
	}
	required := estimate + margin
	if available < required {
		return fmt.Errorf("insufficient free space for index recovery: need at least %d bytes, have %d bytes", required, available)
	}
	return nil
}

type movedIndexFile struct {
	from string
	to   string
}

func moveActiveIndexAside(activePath, rollbackPath string) ([]movedIndexFile, error) {
	var moved []movedIndexFile
	for _, suffix := range []string{"", "-wal", "-shm"} {
		from := activePath + suffix
		if _, err := os.Stat(from); errors.Is(err, os.ErrNotExist) {
			continue
		} else if err != nil {
			restoreErr := restoreMovedIndex(moved)
			return nil, errors.Join(fmt.Errorf("inspect active index file: %w", err), restoreErr)
		}
		to := rollbackPath + suffix
		if err := os.Rename(from, to); err != nil {
			restoreErr := restoreMovedIndex(moved)
			return nil, errors.Join(fmt.Errorf("preserve active index file: %w", err), restoreErr)
		}
		moved = append(moved, movedIndexFile{from: from, to: to})
	}
	return moved, nil
}

func restoreMovedIndex(moved []movedIndexFile) error {
	var restoreErr error
	for index := len(moved) - 1; index >= 0; index-- {
		entry := moved[index]
		if err := os.Rename(entry.to, entry.from); err != nil {
			restoreErr = errors.Join(restoreErr, fmt.Errorf("restore %s: %w", entry.from, err))
		}
	}
	return restoreErr
}

func removeMovedIndex(moved []movedIndexFile) error {
	var removeErr error
	for _, entry := range moved {
		if err := os.Remove(entry.to); err != nil && !errors.Is(err, os.ErrNotExist) {
			removeErr = errors.Join(removeErr, err)
		}
	}
	return removeErr
}

func preserveMovedIndexEvidence(activePath string, moved []movedIndexFile, reason, operationID string) (string, error) {
	now := time.Now().UTC()
	basePath := managedIndexBackupPath(activePath, 0, reason+"-unreadable", now, operationID)
	if err := os.MkdirAll(filepath.Dir(basePath), 0o755); err != nil {
		return "", err
	}
	for _, entry := range moved {
		suffix := strings.TrimPrefix(entry.from, activePath)
		if err := os.Rename(entry.to, basePath+suffix); err != nil {
			return "", err
		}
	}
	return basePath, nil
}

func removeIndexFiles(path string) error {
	var removeErr error
	for _, suffix := range []string{"", "-wal", "-shm"} {
		if err := os.Remove(path + suffix); err != nil && !errors.Is(err, os.ErrNotExist) {
			removeErr = errors.Join(removeErr, err)
		}
	}
	return removeErr
}

func regularIndexExists(path string) (bool, error) {
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("inspect active index: %w", err)
	}
	if !info.Mode().IsRegular() {
		return false, fmt.Errorf("active index is not a regular file")
	}
	return true, nil
}

func managedIndexBackupPath(activePath string, schema int, reason string, now time.Time, operationID string) string {
	name := fmt.Sprintf("devspecs-v%d-%s-%s-%s.db", schema, now.Format("20060102T150405Z"), reason, operationID)
	return filepath.Join(filepath.Dir(activePath), "backups", "index", name)
}

func newIndexRecoveryID() (string, error) {
	buffer := make([]byte, 6)
	if _, err := rand.Read(buffer); err != nil {
		return "", fmt.Errorf("generate recovery operation id: %w", err)
	}
	return hex.EncodeToString(buffer), nil
}

func syncIndexFile(path string) error {
	file, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("open index for sync: %w", err)
	}
	syncErr := file.Sync()
	closeErr := file.Close()
	if err := errors.Join(syncErr, closeErr); err != nil {
		return fmt.Errorf("sync index: %w", err)
	}
	return nil
}

func writeIndexJSON(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	temporaryPath := path + ".tmp"
	file, err := os.OpenFile(temporaryPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	_, writeErr := file.Write(body)
	syncErr := file.Sync()
	closeErr := file.Close()
	if err := errors.Join(writeErr, syncErr, closeErr); err != nil {
		_ = os.Remove(temporaryPath)
		return err
	}
	if err := replaceIndexFile(temporaryPath, path); err != nil {
		_ = os.Remove(temporaryPath)
		return err
	}
	return nil
}

func sameIndexPath(left, right string) bool {
	leftPath, leftErr := filepath.Abs(left)
	rightPath, rightErr := filepath.Abs(right)
	if leftErr != nil || rightErr != nil {
		return left == right
	}
	return strings.EqualFold(filepath.Clean(leftPath), filepath.Clean(rightPath))
}

func pruneAutomaticIndexBackups(homeDir, newestPath string) error {
	manifestPaths, err := filepath.Glob(filepath.Join(homeDir, "backups", "index", "*.db.json"))
	if err != nil {
		return err
	}
	type candidate struct {
		backup IndexBackup
		path   string
	}
	var automatic []candidate
	for _, manifestPath := range manifestPaths {
		body, err := os.ReadFile(manifestPath)
		if err != nil {
			continue
		}
		var manifest struct {
			Version int `json:"version"`
			IndexBackup
		}
		if err := json.Unmarshal(body, &manifest); err != nil || manifest.Retention != IndexBackupRetentionAutomatic {
			continue
		}
		automatic = append(automatic, candidate{backup: manifest.IndexBackup, path: manifestPath})
	}
	sort.Slice(automatic, func(i, j int) bool { return automatic[i].backup.CreatedAt > automatic[j].backup.CreatedAt })
	keptSchemas := map[int]bool{}
	for _, item := range automatic {
		keep := sameIndexPath(item.backup.Path, newestPath)
		if !keptSchemas[item.backup.SourceSchema] && len(keptSchemas) < 3 {
			keep = true
			keptSchemas[item.backup.SourceSchema] = true
		}
		if keep {
			continue
		}
		_ = os.Remove(item.backup.Path)
		_ = os.Remove(item.path)
	}
	return nil
}
