package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"ilonasin/internal/home"
)

type Store struct {
	DB     *sql.DB
	Logger *slog.Logger
}

func Open(ctx context.Context, path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	u := url.URL{
		Scheme:   "file",
		Path:     path,
		RawQuery: "_journal_mode=WAL&_foreign_keys=on&_busy_timeout=5000&_loc=UTC",
	}
	dsn := u.String()
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, err
	}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := home.SecureFile(path); err != nil {
		_ = db.Close()
		return nil, err
	}
	store := &Store{DB: db}
	if err := store.Migrate(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	for _, sidecar := range []string{path, path + "-wal", path + "-shm"} {
		if err := home.SecureFile(sidecar); err != nil && !os.IsNotExist(err) {
			_ = db.Close()
			return nil, err
		}
	}
	return store, nil
}

func (s *Store) Close() error {
	if s == nil || s.DB == nil {
		return nil
	}
	return s.DB.Close()
}

func (s *Store) Migrate(ctx context.Context) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, stmt := range migration001[:1] {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	if err := rejectNewerMigrationVersion(ctx, tx); err != nil {
		return err
	}
	for _, m := range migrations {
		applied, err := migrationApplied(ctx, tx, m.version)
		if err != nil {
			return err
		}
		if applied {
			continue
		}
		for _, step := range m.steps {
			if err := step(ctx, tx); err != nil {
				return err
			}
		}
		if _, err = tx.ExecContext(ctx, `
			INSERT INTO migrations(version, name, applied_at)
			VALUES(?, ?, ?)
		`, m.version, m.name, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func migrationApplied(ctx context.Context, tx *sql.Tx, version int) (bool, error) {
	var exists int
	err := tx.QueryRowContext(ctx, `SELECT 1 FROM migrations WHERE version = ?`, version).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}
