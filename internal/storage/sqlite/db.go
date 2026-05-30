package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"ilonasin/internal/credentials"
	"ilonasin/internal/home"
	"ilonasin/internal/metadata"
	"ilonasin/internal/provider"
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
		for _, stmt := range m.stmts {
			if _, err := tx.ExecContext(ctx, stmt); err != nil {
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

func (s *Store) InsertLocalToken(ctx context.Context, meta credentials.NewLocalTokenMetadata) (credentials.LocalTokenMetadata, error) {
	res, err := s.DB.ExecContext(ctx, `
		INSERT INTO client_tokens(label, token_hash, token_prefix, token_last4, created_at)
		VALUES(?, ?, ?, ?, ?)
	`, meta.Label, meta.TokenHash, meta.TokenPrefix, meta.TokenLast4, meta.CreatedAt.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return credentials.LocalTokenMetadata{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return credentials.LocalTokenMetadata{}, err
	}
	return credentials.LocalTokenMetadata{
		ID:          id,
		Label:       meta.Label,
		TokenPrefix: meta.TokenPrefix,
		TokenLast4:  meta.TokenLast4,
		CreatedAt:   meta.CreatedAt.UTC(),
	}, nil
}

func (s *Store) ListLocalTokens(ctx context.Context) ([]credentials.LocalTokenMetadata, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, label, token_prefix, token_last4, created_at, disabled_at
		FROM client_tokens
		ORDER BY id ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []credentials.LocalTokenMetadata
	for rows.Next() {
		meta, err := scanLocalTokenMetadata(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, meta)
	}
	return out, rows.Err()
}

func (s *Store) DisableLocalToken(ctx context.Context, id int64, disabledAt time.Time) error {
	res, err := s.DB.ExecContext(ctx, `
		UPDATE client_tokens
		SET disabled_at = COALESCE(disabled_at, ?)
		WHERE id = ?
	`, disabledAt.UTC().Format(time.RFC3339Nano), id)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return fmt.Errorf("client token not found")
	}
	return nil
}

func (s *Store) FindLocalTokenByHash(ctx context.Context, hash string) (credentials.LocalTokenAuthRecord, error) {
	row := s.DB.QueryRowContext(ctx, `
		SELECT id, label, token_hash, token_prefix, token_last4, disabled_at IS NOT NULL
		FROM client_tokens
		WHERE token_hash = ?
	`, hash)
	var rec credentials.LocalTokenAuthRecord
	var unusedPrefix, unusedLast4 string
	if err := row.Scan(&rec.ID, &rec.Label, &rec.TokenHash, &unusedPrefix, &unusedLast4, &rec.Disabled); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return credentials.LocalTokenAuthRecord{}, fmt.Errorf("client token not found")
		}
		return credentials.LocalTokenAuthRecord{}, err
	}
	return rec, nil
}

type localTokenScanner interface {
	Scan(dest ...any) error
}

func scanLocalTokenMetadata(row localTokenScanner) (credentials.LocalTokenMetadata, error) {
	var meta credentials.LocalTokenMetadata
	var created string
	var disabled sql.NullString
	if err := row.Scan(&meta.ID, &meta.Label, &meta.TokenPrefix, &meta.TokenLast4, &created, &disabled); err != nil {
		return credentials.LocalTokenMetadata{}, err
	}
	createdAt, err := time.Parse(time.RFC3339Nano, created)
	if err != nil {
		return credentials.LocalTokenMetadata{}, err
	}
	meta.CreatedAt = createdAt
	if disabled.Valid {
		disabledAt, err := time.Parse(time.RFC3339Nano, disabled.String)
		if err != nil {
			return credentials.LocalTokenMetadata{}, err
		}
		meta.DisabledAt = &disabledAt
		meta.Disabled = true
	}
	return meta, nil
}

func (s *Store) InsertAPIKeyCredential(ctx context.Context, meta credentials.NewUpstreamCredential, apiKey string) (credentials.UpstreamCredentialMetadata, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return credentials.UpstreamCredentialMetadata{}, err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `
		INSERT INTO provider_credentials(
			provider_instance_id, kind, label, secret_prefix, secret_last4, created_at, updated_at
		) VALUES(?, ?, ?, ?, ?, ?, ?)
	`, meta.ProviderInstanceID, meta.Kind, meta.Label, meta.SecretPrefix, meta.SecretLast4,
		meta.CreatedAt.UTC().Format(time.RFC3339Nano), meta.CreatedAt.UTC().Format(time.RFC3339Nano))
	if err != nil {
		if isUniqueConstraint(err) {
			return credentials.UpstreamCredentialMetadata{}, credentials.ErrDuplicateCredential
		}
		return credentials.UpstreamCredentialMetadata{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return credentials.UpstreamCredentialMetadata{}, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO credential_secrets(credential_id, secret_kind, secret_material, created_at, updated_at)
		VALUES(?, 'api_key', ?, ?, ?)
	`, id, apiKey, meta.CreatedAt.UTC().Format(time.RFC3339Nano), meta.CreatedAt.UTC().Format(time.RFC3339Nano)); err != nil {
		return credentials.UpstreamCredentialMetadata{}, err
	}
	if err := tx.Commit(); err != nil {
		return credentials.UpstreamCredentialMetadata{}, err
	}
	return credentials.UpstreamCredentialMetadata{
		ID:                 id,
		ProviderInstanceID: meta.ProviderInstanceID,
		Kind:               meta.Kind,
		Label:              meta.Label,
		SecretPrefix:       meta.SecretPrefix,
		SecretLast4:        meta.SecretLast4,
		CreatedAt:          meta.CreatedAt.UTC(),
	}, nil
}

func (s *Store) ListUpstreamCredentials(ctx context.Context) ([]credentials.UpstreamCredentialMetadata, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, provider_instance_id, kind, label, secret_prefix, secret_last4, created_at, disabled_at
		FROM provider_credentials
		ORDER BY id ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []credentials.UpstreamCredentialMetadata
	for rows.Next() {
		meta, err := scanUpstreamCredentialMetadata(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, meta)
	}
	return out, rows.Err()
}

func (s *Store) DisableUpstreamCredential(ctx context.Context, id int64, disabledAt time.Time) error {
	res, err := s.DB.ExecContext(ctx, `
		UPDATE provider_credentials
		SET disabled_at = COALESCE(disabled_at, ?), updated_at = ?
		WHERE id = ?
	`, disabledAt.UTC().Format(time.RFC3339Nano), disabledAt.UTC().Format(time.RFC3339Nano), id)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return credentials.ErrCredentialNotFound
	}
	return nil
}

func (s *Store) ResolveAPIKeyCredential(ctx context.Context, providerInstanceID string) (credentials.ResolvedAPIKeyCredential, error) {
	row := s.DB.QueryRowContext(ctx, `
		SELECT pc.id, pc.provider_instance_id, pc.label, cs.secret_material
		FROM provider_credentials pc
		JOIN credential_secrets cs ON cs.credential_id = pc.id AND cs.secret_kind = 'api_key'
		WHERE pc.provider_instance_id = ?
			AND pc.kind = 'api_key'
			AND pc.disabled_at IS NULL
		ORDER BY pc.id ASC
		LIMIT 1
	`, providerInstanceID)
	var out credentials.ResolvedAPIKeyCredential
	if err := row.Scan(&out.ID, &out.ProviderInstanceID, &out.Label, &out.APIKey); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return credentials.ResolvedAPIKeyCredential{}, credentials.ErrNoEligibleCredential
		}
		return credentials.ResolvedAPIKeyCredential{}, err
	}
	return out, nil
}

func scanUpstreamCredentialMetadata(row localTokenScanner) (credentials.UpstreamCredentialMetadata, error) {
	var meta credentials.UpstreamCredentialMetadata
	var created string
	var disabled sql.NullString
	if err := row.Scan(&meta.ID, &meta.ProviderInstanceID, &meta.Kind, &meta.Label, &meta.SecretPrefix, &meta.SecretLast4, &created, &disabled); err != nil {
		return credentials.UpstreamCredentialMetadata{}, err
	}
	createdAt, err := time.Parse(time.RFC3339Nano, created)
	if err != nil {
		return credentials.UpstreamCredentialMetadata{}, err
	}
	meta.CreatedAt = createdAt
	if disabled.Valid {
		disabledAt, err := time.Parse(time.RFC3339Nano, disabled.String)
		if err != nil {
			return credentials.UpstreamCredentialMetadata{}, err
		}
		meta.DisabledAt = &disabledAt
		meta.Disabled = true
	}
	return meta, nil
}

func isUniqueConstraint(err error) bool {
	return strings.Contains(err.Error(), "UNIQUE constraint failed")
}

func (s *Store) RecordRequestMetadata(ctx context.Context, m metadata.Request) (int64, error) {
	res, err := s.DB.ExecContext(ctx, `
		INSERT INTO request_metadata(
			started_at, client_token_id, credential_id, requested_provider_instance, requested_model,
			resolved_provider_instance, resolved_model, http_status, error_class,
			retry_count, fallback_count, prompt_tokens, completion_tokens,
			total_tokens, reasoning_tokens, total_latency_ms, time_to_first_token_ms,
			output_tokens_per_second
		) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, m.StartedAt.UTC().Format(time.RFC3339Nano), m.ClientTokenID, nullableInt64(m.CredentialID), m.RequestedProviderInstance,
		m.RequestedModel, m.ResolvedProviderInstance, m.ResolvedModel, m.HTTPStatus,
		m.ErrorClass, m.RetryCount, m.FallbackCount, m.PromptTokens, m.CompletionTokens,
		m.TotalTokens, m.ReasoningTokens, m.TotalLatencyMS, m.TimeToFirstTokenMS,
		m.OutputTokensPerSecond)
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	return id, nil
}

func (s *Store) RecordStreamMetrics(ctx context.Context, m metadata.Stream) error {
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO stream_metrics(
			request_metadata_id, time_to_first_token_ms, output_tokens_per_second,
			completion_status, chunk_count
		) VALUES(?, ?, ?, ?, ?)
	`, m.RequestMetadataID, m.TimeToFirstTokenMS, m.OutputTokensPerSecond, m.CompletionStatus, m.ChunkCount)
	return err
}

func (s *Store) ReplaceModelCache(ctx context.Context, providerInstanceID string, models []provider.ModelMetadata) error {
	if len(models) == 0 {
		return fmt.Errorf("model cache replacement requires at least one model")
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM model_cache WHERE provider_instance_id = ?`, providerInstanceID); err != nil {
		return err
	}
	for _, model := range models {
		if model.ProviderInstanceID != providerInstanceID {
			return fmt.Errorf("model cache provider mismatch")
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO model_cache(
				provider_instance_id, model_id, display_name, capability_flags,
				context_length, updated_at
			) VALUES(?, ?, ?, ?, ?, ?)
		`, model.ProviderInstanceID, model.ModelID, model.DisplayName, model.CapabilityFlags,
			nullableInt(model.ContextLength), model.UpdatedAt.UTC().Format(time.RFC3339Nano)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) ListModelCache(ctx context.Context) ([]provider.ModelMetadata, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT provider_instance_id, model_id, display_name, capability_flags,
			COALESCE(context_length, 0), updated_at
		FROM model_cache
		ORDER BY provider_instance_id ASC, model_id ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []provider.ModelMetadata
	for rows.Next() {
		var model provider.ModelMetadata
		var updated string
		if err := rows.Scan(&model.ProviderInstanceID, &model.ModelID, &model.DisplayName,
			&model.CapabilityFlags, &model.ContextLength, &updated); err != nil {
			return nil, err
		}
		updatedAt, err := time.Parse(time.RFC3339Nano, updated)
		if err != nil {
			return nil, err
		}
		model.UpdatedAt = updatedAt
		out = append(out, model)
	}
	return out, rows.Err()
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
