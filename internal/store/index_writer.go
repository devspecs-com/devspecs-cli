package store

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/gofrs/flock"
)

const indexWriterRetryDelay = 250 * time.Millisecond

// IndexWriterLease is a crash-safe, cross-process admission token for a full
// index mutation. The lock file may remain on disk, but the operating-system
// lock is released when the process or descriptor exits.
type IndexWriterLease struct {
	lock *flock.Flock
}

// AcquireIndexWriter waits for exclusive index-writer admission. onWait runs
// once when another process already owns the writer slot.
func (db *DB) AcquireIndexWriter(ctx context.Context, onWait func()) (*IndexWriterLease, error) {
	if db == nil || db.path == "" {
		return nil, fmt.Errorf("acquire index writer: database path is unavailable")
	}
	return AcquireIndexWriter(ctx, db.path, onWait)
}

// AcquireIndexWriter waits for writer admission before a database is opened.
// This is required by rebuild, which replaces the database file itself.
func AcquireIndexWriter(ctx context.Context, dbPath string, onWait func()) (*IndexWriterLease, error) {
	if dbPath == "" {
		return nil, fmt.Errorf("acquire index writer: database path is unavailable")
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("acquire index writer: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, fmt.Errorf("acquire index writer: create database directory: %w", err)
	}
	fileLock := flock.New(dbPath + ".writer.lock")
	locked, err := fileLock.TryLock()
	if err != nil {
		_ = fileLock.Close()
		return nil, fmt.Errorf("acquire index writer: %w", err)
	}
	if !locked {
		if onWait != nil {
			onWait()
		}
		locked, err = fileLock.TryLockContext(ctx, indexWriterRetryDelay)
	}
	if err != nil {
		_ = fileLock.Close()
		return nil, fmt.Errorf("wait for index writer: %w", err)
	}
	if !locked {
		_ = fileLock.Close()
		return nil, fmt.Errorf("wait for index writer: %w", ctx.Err())
	}
	return &IndexWriterLease{lock: fileLock}, nil
}

// Release relinquishes writer admission.
func (lease *IndexWriterLease) Release() error {
	if lease == nil || lease.lock == nil {
		return nil
	}
	unlockErr := lease.lock.Unlock()
	closeErr := lease.lock.Close()
	lease.lock = nil
	return errors.Join(unlockErr, closeErr)
}
