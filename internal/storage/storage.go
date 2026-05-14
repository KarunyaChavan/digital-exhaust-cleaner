// Package storage persists scan sessions and file metadata in SQLite.
package storage

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"digital-exhaust-cleaner/internal/metadata"

	_ "modernc.org/sqlite"
)

//go:embed migrations/schema.sql
var initialSchema string

// Store wraps the SQLite database used by the local analysis engine.
type Store struct {
	db *sql.DB
}

// ScanSession records a single filesystem scan.
type ScanSession struct {
	ID          int64
	RootPath    string
	StartedAt   time.Time
	CompletedAt *time.Time
}

// Open initializes the database file and schema.
func Open(ctx context.Context, dbPath string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, fmt.Errorf("create db directory: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	store := &Store{db: db}
	if err := store.migrate(ctx); err != nil {
		db.Close()
		return nil, err
	}

	return store, nil
}

// Close releases database resources.
func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) migrate(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, initialSchema); err != nil {
		return fmt.Errorf("apply schema: %w", err)
	}
	return nil
}

// StartScan creates a new scan session row.
func (s *Store) StartScan(ctx context.Context, rootPath string) (ScanSession, error) {
	started := time.Now().UTC()
	result, err := s.db.ExecContext(ctx, "INSERT INTO scan_sessions(root_path, started_at) VALUES(?, ?)", rootPath, started.Format(time.RFC3339Nano))
	if err != nil {
		return ScanSession{}, fmt.Errorf("insert scan session: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return ScanSession{}, fmt.Errorf("read scan session id: %w", err)
	}

	return ScanSession{ID: id, RootPath: rootPath, StartedAt: started}, nil
}

// CompleteScan marks a scan session complete.
func (s *Store) CompleteScan(ctx context.Context, sessionID int64) error {
	completed := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.db.ExecContext(ctx, "UPDATE scan_sessions SET completed_at = ? WHERE id = ?", completed, sessionID)
	if err != nil {
		return fmt.Errorf("complete scan session: %w", err)
	}
	return nil
}

// SaveFile stores one metadata record.
func (s *Store) SaveFile(ctx context.Context, sessionID int64, file metadata.File) error {
	return s.insertFile(ctx, s.db, sessionID, file)
}

// SaveFiles stores metadata records in one transaction for better large-scan throughput.
func (s *Store) SaveFiles(ctx context.Context, sessionID int64, files []metadata.File) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin file batch: %w", err)
	}
	defer tx.Rollback()

	directories := make(map[string]struct{}, len(files))
	for _, file := range files {
		directories[file.DirectoryPath] = struct{}{}
	}
	for directory := range directories {
		if err := s.insertDirectory(ctx, tx, sessionID, directory); err != nil {
			return err
		}
	}
	for _, file := range files {
		if err := s.insertFile(ctx, tx, sessionID, file); err != nil {
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit file batch: %w", err)
	}
	return nil
}

type sqlExecutor interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func (s *Store) insertFile(ctx context.Context, exec sqlExecutor, sessionID int64, file metadata.File) error {
	_, err := exec.ExecContext(ctx, `
INSERT INTO files(
    scan_session_id, path, directory_path, name, extension, mime_type, size_bytes,
    created_at, modified_at, accessed_at, path_entropy, sha256, partial_sha256,
    is_hidden, is_symlink
) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		sessionID,
		file.Path,
		file.DirectoryPath,
		file.Name,
		file.Extension,
		file.MIMEType,
		file.SizeBytes,
		formatOptionalTime(file.CreatedAt),
		file.ModifiedAt.Format(time.RFC3339Nano),
		file.AccessedAt.Format(time.RFC3339Nano),
		file.PathEntropy,
		file.SHA256,
		file.PartialSHA256,
		boolInt(file.IsHidden),
		boolInt(file.IsSymlink),
	)
	if err != nil {
		return fmt.Errorf("insert file metadata: %w", err)
	}
	return nil
}

// SaveDirectory records a directory observed during a scan.
func (s *Store) SaveDirectory(ctx context.Context, sessionID int64, path string) error {
	return s.insertDirectory(ctx, s.db, sessionID, path)
}

func (s *Store) insertDirectory(ctx context.Context, exec sqlExecutor, sessionID int64, path string) error {
	_, err := exec.ExecContext(ctx, `
INSERT OR IGNORE INTO directories(scan_session_id, path) VALUES(?, ?)`, sessionID, path)
	if err != nil {
		return fmt.Errorf("insert directory: %w", err)
	}
	return nil
}

// ListFiles returns all files recorded for a scan session.
func (s *Store) ListFiles(ctx context.Context, sessionID int64) ([]metadata.File, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT path, directory_path, name, extension, mime_type, size_bytes, created_at,
       modified_at, accessed_at, path_entropy, sha256, partial_sha256, is_hidden, is_symlink
FROM files WHERE scan_session_id = ?`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("query files: %w", err)
	}
	defer rows.Close()

	var files []metadata.File
	for rows.Next() {
		file, err := scanFile(rows)
		if err != nil {
			return nil, err
		}
		files = append(files, file)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate files: %w", err)
	}
	return files, nil
}

func scanFile(rows *sql.Rows) (metadata.File, error) {
	var file metadata.File
	var created sql.NullString
	var modified string
	var accessed string
	var hidden int
	var symlink int

	err := rows.Scan(
		&file.Path,
		&file.DirectoryPath,
		&file.Name,
		&file.Extension,
		&file.MIMEType,
		&file.SizeBytes,
		&created,
		&modified,
		&accessed,
		&file.PathEntropy,
		&file.SHA256,
		&file.PartialSHA256,
		&hidden,
		&symlink,
	)
	if err != nil {
		return metadata.File{}, fmt.Errorf("scan file row: %w", err)
	}

	if created.Valid {
		parsed, err := time.Parse(time.RFC3339Nano, created.String)
		if err != nil {
			return metadata.File{}, fmt.Errorf("parse created_at: %w", err)
		}
		file.CreatedAt = &parsed
	}

	parsedModified, err := time.Parse(time.RFC3339Nano, modified)
	if err != nil {
		return metadata.File{}, fmt.Errorf("parse modified_at: %w", err)
	}
	parsedAccessed, err := time.Parse(time.RFC3339Nano, accessed)
	if err != nil {
		return metadata.File{}, fmt.Errorf("parse accessed_at: %w", err)
	}

	file.ModifiedAt = parsedModified
	file.AccessedAt = parsedAccessed
	file.IsHidden = hidden == 1
	file.IsSymlink = symlink == 1
	return file, nil
}

func formatOptionalTime(value *time.Time) any {
	if value == nil {
		return nil
	}
	return value.Format(time.RFC3339Nano)
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
