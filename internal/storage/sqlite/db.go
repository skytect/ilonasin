package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"ilonasin/internal/credentials"
	"ilonasin/internal/home"
	"ilonasin/internal/metadata"
)

type Store struct {
	DB *sql.DB
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
	for _, stmt := range migration001 {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	_, err = tx.ExecContext(ctx, `
		INSERT OR IGNORE INTO migrations(version, name, applied_at)
		VALUES(1, 'initial_schema', ?)
	`, time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) InsertClientToken(ctx context.Context, label, token string) error {
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO client_tokens(label, token_hash, token_prefix, token_last4, created_at)
		VALUES(?, ?, ?, ?, ?)
	`, label, credentials.HashToken(token), credentials.Prefix(token), credentials.Last4(token), time.Now().UTC().Format(time.RFC3339Nano))
	return err
}

func (s *Store) FindClientTokenByHash(ctx context.Context, hash string) (credentials.ClientTokenRecord, error) {
	row := s.DB.QueryRowContext(ctx, `
		SELECT id, label, token_hash, token_prefix, token_last4, disabled_at IS NOT NULL
		FROM client_tokens
		WHERE token_hash = ?
	`, hash)
	var rec credentials.ClientTokenRecord
	if err := row.Scan(&rec.ID, &rec.Label, &rec.TokenHash, &rec.TokenPrefix, &rec.TokenLast4, &rec.Disabled); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return credentials.ClientTokenRecord{}, fmt.Errorf("client token not found")
		}
		return credentials.ClientTokenRecord{}, err
	}
	return rec, nil
}

func (s *Store) RecordRequestMetadata(ctx context.Context, m metadata.Request) error {
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO request_metadata(
			started_at, client_token_id, requested_provider_instance, requested_model,
			resolved_provider_instance, resolved_model, http_status, error_class,
			retry_count, fallback_count, prompt_tokens, completion_tokens,
			total_tokens, reasoning_tokens, total_latency_ms
		) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, m.StartedAt.UTC().Format(time.RFC3339Nano), m.ClientTokenID, m.RequestedProviderInstance,
		m.RequestedModel, m.ResolvedProviderInstance, m.ResolvedModel, m.HTTPStatus,
		m.ErrorClass, m.RetryCount, m.FallbackCount, m.PromptTokens, m.CompletionTokens,
		m.TotalTokens, m.ReasoningTokens, m.TotalLatencyMS)
	return err
}
