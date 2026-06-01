package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"strings"
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
	home.SecureFile(path)
	store := &Store{DB: db}
	if err := store.Migrate(ctx); err != nil {
		_ = db.Close()
		return nil, err
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

type rowScanner interface {
	Scan(dest ...any) error
}

func isUniqueConstraint(err error) bool {
	return strings.Contains(err.Error(), "UNIQUE constraint failed")
}

func nullableInt(v int) any {
	if v == 0 {
		return nil
	}
	return v
}

func nullableInt64(v int64) any {
	if v == 0 {
		return nil
	}
	return v
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func tokenRate(part, total int) float64 {
	if part <= 0 || total <= 0 {
		return 0
	}
	return float64(part) / float64(total)
}

func cacheMissTokens(promptTokens, cacheHitTokens int) int {
	miss := promptTokens - cacheHitTokens
	if miss < 0 {
		return 0
	}
	return miss
}

func nullableTime(t *time.Time) any {
	if t == nil || t.IsZero() {
		return nil
	}
	return t.UTC().Format(time.RFC3339Nano)
}

func parseSQLiteTime(value string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err == nil {
		return parsed, nil
	}
	if fallback, fallbackErr := time.ParseInLocation("2006-01-02 15:04:05", value, time.UTC); fallbackErr == nil {
		return fallback, nil
	}
	return time.Time{}, err
}

func cloneTime(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	cloned := t.UTC()
	return &cloned
}
