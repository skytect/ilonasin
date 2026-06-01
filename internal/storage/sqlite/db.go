package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"ilonasin/internal/credentials"
	"ilonasin/internal/home"
	"ilonasin/internal/metadata"
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
	createdAt, err := parseSQLiteTime(created)
	if err != nil {
		return credentials.LocalTokenMetadata{}, err
	}
	meta.CreatedAt = createdAt
	if disabled.Valid {
		disabledAt, err := parseSQLiteTime(disabled.String)
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
			provider_instance_id, kind, label, secret_prefix, secret_last4, fallback_group, created_at, updated_at
		) VALUES(?, ?, ?, ?, ?, ?, ?, ?)
	`, meta.ProviderInstanceID, meta.Kind, meta.Label, meta.SecretPrefix, meta.SecretLast4, meta.FallbackGroup,
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
		FallbackGroup:      meta.FallbackGroup,
		CreatedAt:          meta.CreatedAt.UTC(),
	}, nil
}

func (s *Store) ListUpstreamCredentials(ctx context.Context) ([]credentials.UpstreamCredentialMetadata, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, provider_instance_id, kind, label, secret_prefix, secret_last4, fallback_group, created_at, disabled_at
		FROM provider_credentials
		WHERE kind = 'api_key'
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

func (s *Store) UpsertOAuthCredentialForAccountHash(ctx context.Context, meta credentials.NewOAuthCredential, accessToken, refreshToken string) (credentials.OAuthCredentialMetadata, error) {
	for attempt := 0; attempt < 2; attempt++ {
		row, retry, err := s.upsertOAuthCredentialForAccountHash(ctx, meta, accessToken, refreshToken)
		if retry {
			continue
		}
		return row, err
	}
	return credentials.OAuthCredentialMetadata{}, credentials.ErrDuplicateCredential
}

func (s *Store) upsertOAuthCredentialForAccountHash(ctx context.Context, meta credentials.NewOAuthCredential, accessToken, refreshToken string) (credentials.OAuthCredentialMetadata, bool, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return credentials.OAuthCredentialMetadata{}, false, err
	}
	defer tx.Rollback()
	ts := meta.CreatedAt.UTC().Format(time.RFC3339Nano)
	account, err := findProviderAccountForUpdate(ctx, tx, meta.ProviderInstanceID, meta.AccountHash)
	if err != nil {
		return credentials.OAuthCredentialMetadata{}, false, err
	}
	if account.exists {
		row, err := upsertExistingOAuthCredentialTx(ctx, tx, account, meta, accessToken, refreshToken, ts)
		if err != nil {
			return credentials.OAuthCredentialMetadata{}, false, err
		}
		if err := tx.Commit(); err != nil {
			return credentials.OAuthCredentialMetadata{}, false, err
		}
		return row, false, nil
	}
	row, retry, err := insertOAuthCredentialTx(ctx, tx, meta, accessToken, refreshToken, ts)
	if err != nil || retry {
		return credentials.OAuthCredentialMetadata{}, retry, err
	}
	if err := tx.Commit(); err != nil {
		return credentials.OAuthCredentialMetadata{}, false, err
	}
	return row, false, nil
}

func (s *Store) InsertOAuthCredential(ctx context.Context, meta credentials.NewOAuthCredential, accessToken, refreshToken string) (credentials.OAuthCredentialMetadata, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return credentials.OAuthCredentialMetadata{}, err
	}
	defer tx.Rollback()
	ts := meta.CreatedAt.UTC().Format(time.RFC3339Nano)
	row, retry, err := insertOAuthCredentialTx(ctx, tx, meta, accessToken, refreshToken, ts)
	if err != nil {
		return credentials.OAuthCredentialMetadata{}, err
	}
	if retry {
		return credentials.OAuthCredentialMetadata{}, credentials.ErrDuplicateCredential
	}
	if err := tx.Commit(); err != nil {
		return credentials.OAuthCredentialMetadata{}, err
	}
	return row, nil
}

type providerAccountMatch struct {
	exists       bool
	id           int64
	credentialID sql.NullInt64
}

func findProviderAccountForUpdate(ctx context.Context, tx *sql.Tx, providerInstanceID, accountHash string) (providerAccountMatch, error) {
	var account providerAccountMatch
	err := tx.QueryRowContext(ctx, `
		SELECT id, credential_id
		FROM provider_accounts
		WHERE provider_instance_id = ? AND account_hash = ?
	`, providerInstanceID, accountHash).Scan(&account.id, &account.credentialID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return providerAccountMatch{}, nil
		}
		return providerAccountMatch{}, err
	}
	account.exists = true
	return account, nil
}

func insertOAuthCredentialTx(ctx context.Context, tx *sql.Tx, meta credentials.NewOAuthCredential, accessToken, refreshToken, ts string) (credentials.OAuthCredentialMetadata, bool, error) {
	res, err := tx.ExecContext(ctx, `
		INSERT INTO provider_credentials(
			provider_instance_id, kind, label, secret_prefix, secret_last4, fallback_group, created_at, updated_at
		) VALUES(?, 'oauth', ?, '', '', ?, ?, ?)
	`, meta.ProviderInstanceID, meta.Label, credentials.DefaultFallbackGroup, ts, ts)
	if err != nil {
		if isUniqueConstraint(err) {
			return credentials.OAuthCredentialMetadata{}, false, credentials.ErrDuplicateCredential
		}
		return credentials.OAuthCredentialMetadata{}, false, err
	}
	credentialID, err := res.LastInsertId()
	if err != nil {
		return credentials.OAuthCredentialMetadata{}, false, err
	}
	accessID, err := insertCredentialSecret(ctx, tx, credentialID, "oauth_access", accessToken, ts)
	if err != nil {
		return credentials.OAuthCredentialMetadata{}, false, err
	}
	refreshID, err := insertCredentialSecret(ctx, tx, credentialID, "oauth_refresh", refreshToken, ts)
	if err != nil {
		return credentials.OAuthCredentialMetadata{}, false, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO oauth_tokens(
			credential_id, access_token_secret_id, refresh_token_secret_id, expires_at, scopes
		) VALUES(?, ?, ?, ?, ?)
	`, credentialID, accessID, refreshID, nullableTime(meta.ExpiresAt), meta.Scopes); err != nil {
		return credentials.OAuthCredentialMetadata{}, false, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO provider_accounts(
			provider_instance_id, credential_id, account_hash, display_label, plan_label, created_at, updated_at
		) VALUES(?, ?, ?, ?, ?, ?, ?)
	`, meta.ProviderInstanceID, credentialID, meta.AccountHash, meta.AccountDisplayLabel, meta.PlanLabel, ts, ts); err != nil {
		if isUniqueConstraint(err) {
			return credentials.OAuthCredentialMetadata{}, true, nil
		}
		return credentials.OAuthCredentialMetadata{}, false, err
	}
	return credentials.OAuthCredentialMetadata{
		ID:                  credentialID,
		ProviderInstanceID:  meta.ProviderInstanceID,
		Label:               meta.Label,
		AccountDisplayLabel: meta.AccountDisplayLabel,
		PlanLabel:           meta.PlanLabel,
		Scopes:              meta.Scopes,
		ExpiresAt:           cloneTime(meta.ExpiresAt),
		CreatedAt:           meta.CreatedAt.UTC(),
	}, false, nil
}

func upsertExistingOAuthCredentialTx(ctx context.Context, tx *sql.Tx, account providerAccountMatch, meta credentials.NewOAuthCredential, accessToken, refreshToken, ts string) (credentials.OAuthCredentialMetadata, error) {
	credentialID, createdAt, ok, err := existingOAuthCredentialForAccount(ctx, tx, account, meta.ProviderInstanceID)
	if err != nil {
		return credentials.OAuthCredentialMetadata{}, err
	}
	if !ok {
		row, err := insertOAuthCredentialWithoutAccountTx(ctx, tx, meta, accessToken, refreshToken, ts)
		if err != nil {
			return credentials.OAuthCredentialMetadata{}, err
		}
		if err := updateProviderAccountTx(ctx, tx, account.id, row.ID, meta.AccountDisplayLabel, meta.PlanLabel, ts); err != nil {
			return credentials.OAuthCredentialMetadata{}, err
		}
		return row, nil
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE provider_credentials
		SET label = ?, disabled_at = NULL, updated_at = ?
		WHERE id = ? AND provider_instance_id = ? AND kind = 'oauth'
	`, meta.Label, ts, credentialID, meta.ProviderInstanceID); err != nil {
		if isUniqueConstraint(err) {
			return credentials.OAuthCredentialMetadata{}, credentials.ErrDuplicateCredential
		}
		return credentials.OAuthCredentialMetadata{}, err
	}
	accessID, err := upsertCredentialSecret(ctx, tx, credentialID, "oauth_access", accessToken, ts)
	if err != nil {
		return credentials.OAuthCredentialMetadata{}, err
	}
	refreshID, err := upsertCredentialSecret(ctx, tx, credentialID, "oauth_refresh", refreshToken, ts)
	if err != nil {
		return credentials.OAuthCredentialMetadata{}, err
	}
	if err := upsertOAuthTokenRow(ctx, tx, credentialID, accessID, refreshID, meta, ts); err != nil {
		return credentials.OAuthCredentialMetadata{}, err
	}
	if err := updateProviderAccountTx(ctx, tx, account.id, credentialID, meta.AccountDisplayLabel, meta.PlanLabel, ts); err != nil {
		return credentials.OAuthCredentialMetadata{}, err
	}
	return credentials.OAuthCredentialMetadata{
		ID:                  credentialID,
		ProviderInstanceID:  meta.ProviderInstanceID,
		Label:               meta.Label,
		AccountDisplayLabel: meta.AccountDisplayLabel,
		PlanLabel:           meta.PlanLabel,
		Scopes:              meta.Scopes,
		ExpiresAt:           cloneTime(meta.ExpiresAt),
		CreatedAt:           createdAt,
	}, nil
}

func existingOAuthCredentialForAccount(ctx context.Context, tx *sql.Tx, account providerAccountMatch, providerInstanceID string) (int64, time.Time, bool, error) {
	if !account.credentialID.Valid {
		return 0, time.Time{}, false, nil
	}
	var created string
	err := tx.QueryRowContext(ctx, `
		SELECT id, created_at
		FROM provider_credentials
		WHERE id = ? AND provider_instance_id = ? AND kind = 'oauth'
	`, account.credentialID.Int64, providerInstanceID).Scan(&account.credentialID.Int64, &created)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, time.Time{}, false, nil
		}
		return 0, time.Time{}, false, err
	}
	createdAt, err := parseSQLiteTime(created)
	if err != nil {
		return 0, time.Time{}, false, err
	}
	return account.credentialID.Int64, createdAt, true, nil
}

func insertOAuthCredentialWithoutAccountTx(ctx context.Context, tx *sql.Tx, meta credentials.NewOAuthCredential, accessToken, refreshToken, ts string) (credentials.OAuthCredentialMetadata, error) {
	res, err := tx.ExecContext(ctx, `
		INSERT INTO provider_credentials(
			provider_instance_id, kind, label, secret_prefix, secret_last4, fallback_group, created_at, updated_at
		) VALUES(?, 'oauth', ?, '', '', ?, ?, ?)
	`, meta.ProviderInstanceID, meta.Label, credentials.DefaultFallbackGroup, ts, ts)
	if err != nil {
		if isUniqueConstraint(err) {
			return credentials.OAuthCredentialMetadata{}, credentials.ErrDuplicateCredential
		}
		return credentials.OAuthCredentialMetadata{}, err
	}
	credentialID, err := res.LastInsertId()
	if err != nil {
		return credentials.OAuthCredentialMetadata{}, err
	}
	accessID, err := insertCredentialSecret(ctx, tx, credentialID, "oauth_access", accessToken, ts)
	if err != nil {
		return credentials.OAuthCredentialMetadata{}, err
	}
	refreshID, err := insertCredentialSecret(ctx, tx, credentialID, "oauth_refresh", refreshToken, ts)
	if err != nil {
		return credentials.OAuthCredentialMetadata{}, err
	}
	if err := upsertOAuthTokenRow(ctx, tx, credentialID, accessID, refreshID, meta, ts); err != nil {
		return credentials.OAuthCredentialMetadata{}, err
	}
	return credentials.OAuthCredentialMetadata{
		ID:                  credentialID,
		ProviderInstanceID:  meta.ProviderInstanceID,
		Label:               meta.Label,
		AccountDisplayLabel: meta.AccountDisplayLabel,
		PlanLabel:           meta.PlanLabel,
		Scopes:              meta.Scopes,
		ExpiresAt:           cloneTime(meta.ExpiresAt),
		CreatedAt:           meta.CreatedAt.UTC(),
	}, nil
}

func upsertCredentialSecret(ctx context.Context, tx *sql.Tx, credentialID int64, kind, material, ts string) (int64, error) {
	var id int64
	err := tx.QueryRowContext(ctx, `
		SELECT id
		FROM credential_secrets
		WHERE credential_id = ? AND secret_kind = ?
	`, credentialID, kind).Scan(&id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return insertCredentialSecret(ctx, tx, credentialID, kind, material, ts)
		}
		return 0, err
	}
	if err := updateCredentialSecret(ctx, tx, credentialID, id, kind, material, ts); err != nil {
		return 0, err
	}
	return id, nil
}

func upsertOAuthTokenRow(ctx context.Context, tx *sql.Tx, credentialID, accessID, refreshID int64, meta credentials.NewOAuthCredential, ts string) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO oauth_tokens(
			credential_id, access_token_secret_id, refresh_token_secret_id,
			expires_at, scopes, last_refresh_at, refresh_failure_class, refresh_failure_description
		) VALUES(?, ?, ?, ?, ?, NULL, '', '')
		ON CONFLICT(credential_id) DO UPDATE SET
			access_token_secret_id = excluded.access_token_secret_id,
			refresh_token_secret_id = excluded.refresh_token_secret_id,
			expires_at = excluded.expires_at,
			scopes = excluded.scopes,
			last_refresh_at = NULL,
			refresh_failure_class = '',
			refresh_failure_description = ''
	`, credentialID, accessID, refreshID, nullableTime(meta.ExpiresAt), meta.Scopes)
	return err
}

func updateProviderAccountTx(ctx context.Context, tx *sql.Tx, accountID, credentialID int64, displayLabel, planLabel, ts string) error {
	res, err := tx.ExecContext(ctx, `
		UPDATE provider_accounts
		SET credential_id = ?, display_label = ?, plan_label = ?, updated_at = ?
		WHERE id = ?
	`, credentialID, displayLabel, planLabel, ts, accountID)
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

func insertCredentialSecret(ctx context.Context, tx *sql.Tx, credentialID int64, kind, material, ts string) (int64, error) {
	res, err := tx.ExecContext(ctx, `
		INSERT INTO credential_secrets(credential_id, secret_kind, secret_material, created_at, updated_at)
		VALUES(?, ?, ?, ?, ?)
	`, credentialID, kind, material, ts, ts)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) ListOAuthCredentials(ctx context.Context) ([]credentials.OAuthCredentialMetadata, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT pc.id, pc.provider_instance_id, pc.label,
			COALESCE(pa.display_label, ''), COALESCE(pa.plan_label, ''),
			ot.scopes, ot.expires_at, ot.last_refresh_at,
			COALESCE(ot.refresh_failure_class, ''), COALESCE(ot.refresh_failure_description, ''),
			pc.created_at, pc.disabled_at
		FROM provider_credentials pc
		JOIN oauth_tokens ot ON ot.credential_id = pc.id
		LEFT JOIN provider_accounts pa ON pa.credential_id = pc.id
		WHERE pc.kind = 'oauth'
		ORDER BY pc.id ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []credentials.OAuthCredentialMetadata
	for rows.Next() {
		var row credentials.OAuthCredentialMetadata
		var created string
		var expires, lastRefresh, disabled sql.NullString
		if err := rows.Scan(&row.ID, &row.ProviderInstanceID, &row.Label,
			&row.AccountDisplayLabel, &row.PlanLabel, &row.Scopes, &expires,
			&lastRefresh, &row.RefreshFailureClass, &row.RefreshFailureDescription, &created, &disabled); err != nil {
			return nil, err
		}
		createdAt, err := parseSQLiteTime(created)
		if err != nil {
			return nil, err
		}
		row.CreatedAt = createdAt
		if expires.Valid {
			expiresAt, err := parseSQLiteTime(expires.String)
			if err != nil {
				return nil, err
			}
			row.ExpiresAt = &expiresAt
		}
		if lastRefresh.Valid {
			refreshAt, err := parseSQLiteTime(lastRefresh.String)
			if err != nil {
				return nil, err
			}
			row.LastRefreshAt = &refreshAt
		}
		if disabled.Valid {
			disabledAt, err := parseSQLiteTime(disabled.String)
			if err != nil {
				return nil, err
			}
			row.DisabledAt = &disabledAt
			row.Disabled = true
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (s *Store) ListProviderAccounts(ctx context.Context) ([]credentials.ProviderAccountMetadata, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, provider_instance_id, COALESCE(credential_id, 0), display_label, plan_label, created_at
		FROM provider_accounts
		ORDER BY id ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []credentials.ProviderAccountMetadata
	for rows.Next() {
		var row credentials.ProviderAccountMetadata
		var created string
		if err := rows.Scan(&row.ID, &row.ProviderInstanceID, &row.CredentialID,
			&row.DisplayLabel, &row.PlanLabel, &created); err != nil {
			return nil, err
		}
		createdAt, err := parseSQLiteTime(created)
		if err != nil {
			return nil, err
		}
		row.CreatedAt = createdAt
		out = append(out, row)
	}
	return out, rows.Err()
}

func (s *Store) MarkOAuthRefreshFailure(ctx context.Context, credentialID int64, failureClass, failureDescription string, now time.Time) error {
	res, err := s.DB.ExecContext(ctx, `
		UPDATE oauth_tokens
		SET refresh_failure_class = ?, refresh_failure_description = ?, last_refresh_at = ?
		WHERE credential_id = ?
	`, failureClass, failureDescription, now.UTC().Format(time.RFC3339Nano), credentialID)
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

func (s *Store) DisableUpstreamCredential(ctx context.Context, id int64, disabledAt time.Time) error {
	res, err := s.DB.ExecContext(ctx, `
		UPDATE provider_credentials
		SET disabled_at = COALESCE(disabled_at, ?), updated_at = ?
		WHERE id = ? AND kind = 'api_key'
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
		SELECT pc.id, pc.provider_instance_id, pc.label, pc.fallback_group, cs.secret_material
		FROM provider_credentials pc
		JOIN credential_secrets cs ON cs.credential_id = pc.id AND cs.secret_kind = 'api_key'
		WHERE pc.provider_instance_id = ?
			AND pc.kind = 'api_key'
			AND pc.disabled_at IS NULL
		ORDER BY pc.id ASC
		LIMIT 1
	`, providerInstanceID)
	var out credentials.ResolvedAPIKeyCredential
	if err := row.Scan(&out.ID, &out.ProviderInstanceID, &out.Label, &out.FallbackGroup, &out.APIKey); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return credentials.ResolvedAPIKeyCredential{}, credentials.ErrNoEligibleCredential
		}
		return credentials.ResolvedAPIKeyCredential{}, err
	}
	return out, nil
}

func (s *Store) ResolveAPIKeyCredentials(ctx context.Context, providerInstanceID string) ([]credentials.ResolvedAPIKeyCredential, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT pc.id, pc.provider_instance_id, pc.label, pc.fallback_group, cs.secret_material
		FROM provider_credentials pc
		JOIN credential_secrets cs ON cs.credential_id = pc.id AND cs.secret_kind = 'api_key'
		WHERE pc.provider_instance_id = ?
			AND pc.kind = 'api_key'
			AND pc.disabled_at IS NULL
		ORDER BY pc.id ASC
	`, providerInstanceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var all []credentials.ResolvedAPIKeyCredential
	for rows.Next() {
		var out credentials.ResolvedAPIKeyCredential
		if err := rows.Scan(&out.ID, &out.ProviderInstanceID, &out.Label, &out.FallbackGroup, &out.APIKey); err != nil {
			return nil, err
		}
		all = append(all, out)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(all) == 0 {
		return nil, credentials.ErrNoEligibleCredential
	}
	return all, nil
}

func (s *Store) ResolveOAuthBearerCredential(ctx context.Context, providerInstanceID string, now time.Time) (credentials.ResolvedOAuthBearerCredential, error) {
	var out credentials.ResolvedOAuthBearerCredential
	var accessSecretID sql.NullInt64
	var expires sql.NullString
	err := s.DB.QueryRowContext(ctx, `
		SELECT pc.id, pc.provider_instance_id, pc.fallback_group, ot.access_token_secret_id, ot.expires_at
		FROM provider_credentials pc
		LEFT JOIN oauth_tokens ot ON ot.credential_id = pc.id
		WHERE pc.provider_instance_id = ?
			AND pc.kind = 'oauth'
			AND pc.disabled_at IS NULL
		ORDER BY pc.id ASC
		LIMIT 1
	`, providerInstanceID).Scan(&out.ID, &out.ProviderInstanceID, &out.FallbackGroup, &accessSecretID, &expires)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return credentials.ResolvedOAuthBearerCredential{}, credentials.ErrNoEligibleCredential
		}
		return credentials.ResolvedOAuthBearerCredential{}, err
	}
	if !accessSecretID.Valid {
		return credentials.ResolvedOAuthBearerCredential{}, credentials.ErrNoEligibleCredential
	}
	if expires.Valid {
		expiresAt, err := parseSQLiteTime(expires.String)
		if err != nil {
			return credentials.ResolvedOAuthBearerCredential{}, err
		}
		expiresAt = expiresAt.UTC()
		if !expiresAt.After(now.UTC()) {
			return credentials.ResolvedOAuthBearerCredential{}, credentials.ErrNoEligibleCredential
		}
		out.ExpiresAt = &expiresAt
	}
	if err := s.DB.QueryRowContext(ctx, `
		SELECT secret_material
		FROM credential_secrets
		WHERE id = ?
			AND credential_id = ?
			AND secret_kind = 'oauth_access'
	`, accessSecretID.Int64, out.ID).Scan(&out.BearerToken); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return credentials.ResolvedOAuthBearerCredential{}, credentials.ErrNoEligibleCredential
		}
		return credentials.ResolvedOAuthBearerCredential{}, err
	}
	routing := credentials.ParseChatGPTRoutingClaims(out.BearerToken)
	out.ChatGPTAccountID = routing.AccountID
	out.ChatGPTAccountIsFedRAMP = routing.FedRAMP
	return out, nil
}

func (s *Store) ResolveOAuthBearerCredentials(ctx context.Context, providerInstanceID string, now time.Time) ([]credentials.ResolvedOAuthBearerCredential, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT pc.id, pc.provider_instance_id, pc.fallback_group, ot.access_token_secret_id, ot.expires_at
		FROM provider_credentials pc
		LEFT JOIN oauth_tokens ot ON ot.credential_id = pc.id
		WHERE pc.provider_instance_id = ?
			AND pc.kind = 'oauth'
			AND pc.disabled_at IS NULL
		ORDER BY pc.id ASC
	`, providerInstanceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var candidates []oauthBearerRow
	for rows.Next() {
		var row oauthBearerRow
		if err := rows.Scan(&row.credential.ID, &row.credential.ProviderInstanceID, &row.fallback, &row.accessSecret, &row.expires); err != nil {
			return nil, err
		}
		row.credential.FallbackGroup = row.fallback
		candidates = append(candidates, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(candidates) == 0 {
		return nil, credentials.ErrNoEligibleCredential
	}
	primary, ok, err := s.materializeOAuthBearer(ctx, candidates[0], now, false)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, credentials.ErrNoEligibleCredential
	}
	out := []credentials.ResolvedOAuthBearerCredential{primary}
	for _, row := range candidates[1:] {
		credential, ok, err := s.materializeOAuthBearer(ctx, row, now, true)
		if err != nil {
			return nil, err
		}
		if ok {
			out = append(out, credential)
		}
	}
	return out, nil
}

type oauthBearerRow struct {
	credential   credentials.ResolvedOAuthBearerCredential
	fallback     string
	accessSecret sql.NullInt64
	expires      sql.NullString
}

func (s *Store) materializeOAuthBearer(ctx context.Context, row oauthBearerRow, now time.Time, skipIneligible bool) (credentials.ResolvedOAuthBearerCredential, bool, error) {
	credential := row.credential
	if !row.accessSecret.Valid {
		if skipIneligible {
			return credentials.ResolvedOAuthBearerCredential{}, false, nil
		}
		return credentials.ResolvedOAuthBearerCredential{}, false, nil
	}
	if row.expires.Valid {
		expiresAt, err := parseSQLiteTime(row.expires.String)
		if err != nil {
			return credentials.ResolvedOAuthBearerCredential{}, false, err
		}
		expiresAt = expiresAt.UTC()
		if !expiresAt.After(now.UTC()) {
			if skipIneligible {
				return credentials.ResolvedOAuthBearerCredential{}, false, nil
			}
			return credentials.ResolvedOAuthBearerCredential{}, false, nil
		}
		credential.ExpiresAt = &expiresAt
	}
	if err := s.DB.QueryRowContext(ctx, `
		SELECT secret_material
		FROM credential_secrets
		WHERE id = ?
			AND credential_id = ?
			AND secret_kind = 'oauth_access'
	`, row.accessSecret.Int64, credential.ID).Scan(&credential.BearerToken); err != nil {
		if errors.Is(err, sql.ErrNoRows) && skipIneligible {
			return credentials.ResolvedOAuthBearerCredential{}, false, nil
		}
		if errors.Is(err, sql.ErrNoRows) {
			return credentials.ResolvedOAuthBearerCredential{}, false, nil
		}
		return credentials.ResolvedOAuthBearerCredential{}, false, err
	}
	routing := credentials.ParseChatGPTRoutingClaims(credential.BearerToken)
	credential.ChatGPTAccountID = routing.AccountID
	credential.ChatGPTAccountIsFedRAMP = routing.FedRAMP
	return credential, true, nil
}

func (s *Store) ResolveOAuthBearerCredentialByID(ctx context.Context, credentialID int64, now time.Time) (credentials.ResolvedOAuthBearerCredential, error) {
	var out credentials.ResolvedOAuthBearerCredential
	var accessSecretID int64
	var expires sql.NullString
	err := s.DB.QueryRowContext(ctx, `
		SELECT pc.id, pc.provider_instance_id, pc.fallback_group, ot.access_token_secret_id, ot.expires_at
		FROM provider_credentials pc
		JOIN oauth_tokens ot ON ot.credential_id = pc.id
		JOIN credential_secrets access_secret
			ON access_secret.id = ot.access_token_secret_id
			AND access_secret.credential_id = pc.id
			AND access_secret.secret_kind = 'oauth_access'
		WHERE pc.id = ?
			AND pc.kind = 'oauth'
			AND pc.disabled_at IS NULL
			AND ot.access_token_secret_id IS NOT NULL
	`, credentialID).Scan(&out.ID, &out.ProviderInstanceID, &out.FallbackGroup, &accessSecretID, &expires)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return credentials.ResolvedOAuthBearerCredential{}, credentials.ErrNoEligibleCredential
		}
		return credentials.ResolvedOAuthBearerCredential{}, err
	}
	if expires.Valid && !now.IsZero() {
		expiresAt, err := parseSQLiteTime(expires.String)
		if err != nil {
			return credentials.ResolvedOAuthBearerCredential{}, err
		}
		expiresAt = expiresAt.UTC()
		if !expiresAt.After(now.UTC()) {
			return credentials.ResolvedOAuthBearerCredential{}, credentials.ErrNoEligibleCredential
		}
		out.ExpiresAt = &expiresAt
	} else if expires.Valid {
		expiresAt, err := parseSQLiteTime(expires.String)
		if err != nil {
			return credentials.ResolvedOAuthBearerCredential{}, err
		}
		out.ExpiresAt = &expiresAt
	}
	if err := s.DB.QueryRowContext(ctx, `
		SELECT secret_material
		FROM credential_secrets
		WHERE id = ?
			AND credential_id = ?
			AND secret_kind = 'oauth_access'
	`, accessSecretID, out.ID).Scan(&out.BearerToken); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return credentials.ResolvedOAuthBearerCredential{}, credentials.ErrNoEligibleCredential
		}
		return credentials.ResolvedOAuthBearerCredential{}, err
	}
	routing := credentials.ParseChatGPTRoutingClaims(out.BearerToken)
	out.ChatGPTAccountID = routing.AccountID
	out.ChatGPTAccountIsFedRAMP = routing.FedRAMP
	return out, nil
}

func (s *Store) ResolveOAuthRefreshCredential(ctx context.Context, credentialID int64) (credentials.ResolvedOAuthRefreshCredential, error) {
	var out credentials.ResolvedOAuthRefreshCredential
	err := s.DB.QueryRowContext(ctx, `
		SELECT pc.id, pc.provider_instance_id, ot.access_token_secret_id, ot.refresh_token_secret_id
		FROM provider_credentials pc
		JOIN oauth_tokens ot ON ot.credential_id = pc.id
		JOIN credential_secrets access_secret
			ON access_secret.id = ot.access_token_secret_id
			AND access_secret.credential_id = pc.id
			AND access_secret.secret_kind = 'oauth_access'
		JOIN credential_secrets refresh_secret
			ON refresh_secret.id = ot.refresh_token_secret_id
			AND refresh_secret.credential_id = pc.id
			AND refresh_secret.secret_kind = 'oauth_refresh'
		WHERE pc.id = ?
			AND pc.kind = 'oauth'
			AND pc.disabled_at IS NULL
			AND ot.refresh_token_secret_id IS NOT NULL
			AND ot.access_token_secret_id IS NOT NULL
	`, credentialID).Scan(&out.ID, &out.ProviderInstanceID, &out.AccessTokenSecretID, &out.RefreshTokenSecretID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return credentials.ResolvedOAuthRefreshCredential{}, credentials.ErrNoEligibleCredential
		}
		return credentials.ResolvedOAuthRefreshCredential{}, err
	}
	return out, nil
}

func (s *Store) ResolveOAuthRefreshCredentialForProvider(ctx context.Context, providerInstanceID string) (credentials.ResolvedOAuthRefreshCredential, error) {
	var out credentials.ResolvedOAuthRefreshCredential
	var accessSecretID sql.NullInt64
	var refreshSecretID sql.NullInt64
	err := s.DB.QueryRowContext(ctx, `
		SELECT pc.id, pc.provider_instance_id, ot.access_token_secret_id, ot.refresh_token_secret_id
		FROM provider_credentials pc
		LEFT JOIN oauth_tokens ot ON ot.credential_id = pc.id
		WHERE pc.provider_instance_id = ?
			AND pc.kind = 'oauth'
			AND pc.disabled_at IS NULL
		ORDER BY pc.id ASC
		LIMIT 1
	`, providerInstanceID).Scan(&out.ID, &out.ProviderInstanceID, &accessSecretID, &refreshSecretID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return credentials.ResolvedOAuthRefreshCredential{}, credentials.ErrNoEligibleCredential
		}
		return credentials.ResolvedOAuthRefreshCredential{}, err
	}
	if !accessSecretID.Valid || !refreshSecretID.Valid {
		return credentials.ResolvedOAuthRefreshCredential{}, credentials.ErrNoEligibleCredential
	}
	if err := s.DB.QueryRowContext(ctx, `
		SELECT id
		FROM credential_secrets
		WHERE id = ?
			AND credential_id = ?
			AND secret_kind = 'oauth_access'
	`, accessSecretID.Int64, out.ID).Scan(&out.AccessTokenSecretID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return credentials.ResolvedOAuthRefreshCredential{}, credentials.ErrNoEligibleCredential
		}
		return credentials.ResolvedOAuthRefreshCredential{}, err
	}
	if err := s.DB.QueryRowContext(ctx, `
		SELECT id
		FROM credential_secrets
		WHERE id = ?
			AND credential_id = ?
			AND secret_kind = 'oauth_refresh'
	`, refreshSecretID.Int64, out.ID).Scan(&out.RefreshTokenSecretID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return credentials.ResolvedOAuthRefreshCredential{}, credentials.ErrNoEligibleCredential
		}
		return credentials.ResolvedOAuthRefreshCredential{}, err
	}
	if out.AccessTokenSecretID == 0 || out.RefreshTokenSecretID == 0 {
		return credentials.ResolvedOAuthRefreshCredential{}, credentials.ErrNoEligibleCredential
	}
	return out, nil
}

func (s *Store) ResolveOAuthRefreshToken(ctx context.Context, credentialID, refreshSecretID int64) (string, error) {
	var refreshToken string
	err := s.DB.QueryRowContext(ctx, `
		SELECT secret_material
		FROM credential_secrets
		WHERE id = ?
			AND credential_id = ?
			AND secret_kind = 'oauth_refresh'
	`, refreshSecretID, credentialID).Scan(&refreshToken)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", credentials.ErrNoEligibleCredential
		}
		return "", err
	}
	return refreshToken, nil
}

func (s *Store) UpdateOAuthTokens(ctx context.Context, credentialID int64, update credentials.OAuthTokenUpdate) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var accessSecretID, refreshSecretID int64
	err = tx.QueryRowContext(ctx, `
		SELECT ot.access_token_secret_id, ot.refresh_token_secret_id
		FROM provider_credentials pc
		JOIN oauth_tokens ot ON ot.credential_id = pc.id
		WHERE pc.id = ?
			AND pc.kind = 'oauth'
			AND pc.disabled_at IS NULL
			AND ot.access_token_secret_id IS NOT NULL
			AND ot.refresh_token_secret_id IS NOT NULL
	`, credentialID).Scan(&accessSecretID, &refreshSecretID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return credentials.ErrNoEligibleCredential
		}
		return err
	}
	ts := update.RefreshedAt.UTC().Format(time.RFC3339Nano)
	if err := updateCredentialSecret(ctx, tx, credentialID, accessSecretID, "oauth_access", update.AccessToken, ts); err != nil {
		return err
	}
	if update.RefreshToken != "" {
		if err := updateCredentialSecret(ctx, tx, credentialID, refreshSecretID, "oauth_refresh", update.RefreshToken, ts); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE oauth_tokens
		SET expires_at = ?, last_refresh_at = ?, refresh_failure_class = '', refresh_failure_description = ''
		WHERE credential_id = ?
	`, nullableTime(update.ExpiresAt), ts, credentialID); err != nil {
		return err
	}
	return tx.Commit()
}

func updateCredentialSecret(ctx context.Context, tx *sql.Tx, credentialID, secretID int64, kind, material, ts string) error {
	res, err := tx.ExecContext(ctx, `
		UPDATE credential_secrets
		SET secret_material = ?, updated_at = ?
		WHERE id = ?
			AND credential_id = ?
			AND secret_kind = ?
	`, material, ts, secretID, credentialID, kind)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return credentials.ErrNoEligibleCredential
	}
	return nil
}

func (s *Store) ListFallbackPolicies(ctx context.Context) ([]credentials.FallbackPolicyMetadata, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT groups.provider_instance_id, groups.credential_kind, groups.group_label,
			COALESCE(p.enabled, 0), groups.credential_count, p.id IS NOT NULL
		FROM (
			SELECT provider_instance_id, kind AS credential_kind, fallback_group AS group_label, COUNT(*) AS credential_count
			FROM provider_credentials
			WHERE kind IN ('api_key', 'oauth')
				AND disabled_at IS NULL
			GROUP BY provider_instance_id, kind, fallback_group
		) groups
		LEFT JOIN credential_fallback_policies p
			ON p.provider_instance_id = groups.provider_instance_id
			AND p.credential_kind = groups.credential_kind
			AND p.group_label = groups.group_label
		ORDER BY groups.provider_instance_id ASC, groups.credential_kind ASC, groups.group_label ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []credentials.FallbackPolicyMetadata
	for rows.Next() {
		var row credentials.FallbackPolicyMetadata
		var enabled, explicit int
		if err := rows.Scan(&row.ProviderInstanceID, &row.CredentialKind, &row.GroupLabel, &enabled, &row.CredentialCount, &explicit); err != nil {
			return nil, err
		}
		row.Enabled = enabled == 1
		row.Explicit = explicit == 1
		out = append(out, row)
	}
	return out, rows.Err()
}

func (s *Store) SetFallbackGroupEnabled(ctx context.Context, providerInstanceID, credentialKind, groupLabel string, enabled bool, now time.Time) error {
	enabledInt := 0
	if enabled {
		enabledInt = 1
	}
	ts := now.UTC().Format(time.RFC3339Nano)
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO credential_fallback_policies(provider_instance_id, credential_kind, group_label, enabled, created_at, updated_at)
		VALUES(?, ?, ?, ?, ?, ?)
		ON CONFLICT(provider_instance_id, credential_kind, group_label)
		DO UPDATE SET enabled = excluded.enabled, updated_at = excluded.updated_at
	`, providerInstanceID, credentialKind, groupLabel, enabledInt, ts, ts)
	return err
}

func (s *Store) fallbackGroupEnabled(ctx context.Context, providerInstanceID, credentialKind, groupLabel string) (bool, error) {
	var enabled int
	err := s.DB.QueryRowContext(ctx, `
		SELECT enabled
		FROM credential_fallback_policies
		WHERE provider_instance_id = ? AND credential_kind = ? AND group_label = ?
	`, providerInstanceID, credentialKind, groupLabel).Scan(&enabled)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return enabled == 1, nil
}

func scanUpstreamCredentialMetadata(row localTokenScanner) (credentials.UpstreamCredentialMetadata, error) {
	var meta credentials.UpstreamCredentialMetadata
	var created string
	var disabled sql.NullString
	if err := row.Scan(&meta.ID, &meta.ProviderInstanceID, &meta.Kind, &meta.Label, &meta.SecretPrefix, &meta.SecretLast4, &meta.FallbackGroup, &created, &disabled); err != nil {
		return credentials.UpstreamCredentialMetadata{}, err
	}
	createdAt, err := parseSQLiteTime(created)
	if err != nil {
		return credentials.UpstreamCredentialMetadata{}, err
	}
	meta.CreatedAt = createdAt
	if disabled.Valid {
		disabledAt, err := parseSQLiteTime(disabled.String)
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

func (s *Store) PruneTelemetryBefore(ctx context.Context, cutoff time.Time) (metadata.PruneResult, error) {
	cutoff = cutoff.UTC()
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return metadata.PruneResult{}, err
	}
	defer tx.Rollback()
	result := metadata.PruneResult{Cutoff: cutoff}

	if err := resetPruneTable(ctx, tx, "ilonasin_prune_request_ids"); err != nil {
		return metadata.PruneResult{}, err
	}
	if err := resetPruneTable(ctx, tx, "ilonasin_prune_fallback_ids"); err != nil {
		return metadata.PruneResult{}, err
	}
	if err := resetPruneTable(ctx, tx, "ilonasin_prune_health_ids"); err != nil {
		return metadata.PruneResult{}, err
	}
	if err := resetPruneTable(ctx, tx, "ilonasin_prune_quota_ids"); err != nil {
		return metadata.PruneResult{}, err
	}

	requestIDs := map[int64]struct{}{}
	requestRows, err := tx.QueryContext(ctx, `SELECT id, started_at FROM request_metadata`)
	if err != nil {
		return metadata.PruneResult{}, err
	}
	requestInsert, err := tx.PrepareContext(ctx, `INSERT INTO ilonasin_prune_request_ids(id) VALUES(?)`)
	if err != nil {
		requestRows.Close()
		return metadata.PruneResult{}, err
	}
	for requestRows.Next() {
		var id int64
		var started string
		if err := requestRows.Scan(&id, &started); err != nil {
			requestInsert.Close()
			requestRows.Close()
			return metadata.PruneResult{}, err
		}
		startedAt, err := parseSQLiteTime(started)
		if err != nil {
			requestInsert.Close()
			requestRows.Close()
			return metadata.PruneResult{}, err
		}
		if startedAt.UTC().Before(cutoff) {
			if _, err := requestInsert.ExecContext(ctx, id); err != nil {
				requestInsert.Close()
				requestRows.Close()
				return metadata.PruneResult{}, err
			}
			requestIDs[id] = struct{}{}
			result.Requests++
		}
	}
	if err := requestRows.Err(); err != nil {
		requestInsert.Close()
		requestRows.Close()
		return metadata.PruneResult{}, err
	}
	requestInsert.Close()
	requestRows.Close()

	if err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM stream_metrics
		WHERE request_metadata_id IN (SELECT id FROM ilonasin_prune_request_ids)
	`).Scan(&result.Streams); err != nil {
		return metadata.PruneResult{}, err
	}

	fallbackRows, err := tx.QueryContext(ctx, `SELECT id, occurred_at, COALESCE(request_metadata_id, 0) FROM fallback_events`)
	if err != nil {
		return metadata.PruneResult{}, err
	}
	fallbackInsert, err := tx.PrepareContext(ctx, `INSERT INTO ilonasin_prune_fallback_ids(id) VALUES(?)`)
	if err != nil {
		fallbackRows.Close()
		return metadata.PruneResult{}, err
	}
	for fallbackRows.Next() {
		var id, requestID int64
		var occurred string
		if err := fallbackRows.Scan(&id, &occurred, &requestID); err != nil {
			fallbackInsert.Close()
			fallbackRows.Close()
			return metadata.PruneResult{}, err
		}
		occurredAt, err := parseSQLiteTime(occurred)
		if err != nil {
			fallbackInsert.Close()
			fallbackRows.Close()
			return metadata.PruneResult{}, err
		}
		_, attachedToPrunedRequest := requestIDs[requestID]
		if occurredAt.UTC().Before(cutoff) || attachedToPrunedRequest {
			if _, err := fallbackInsert.ExecContext(ctx, id); err != nil {
				fallbackInsert.Close()
				fallbackRows.Close()
				return metadata.PruneResult{}, err
			}
			result.Fallbacks++
		}
	}
	if err := fallbackRows.Err(); err != nil {
		fallbackInsert.Close()
		fallbackRows.Close()
		return metadata.PruneResult{}, err
	}
	fallbackInsert.Close()
	fallbackRows.Close()

	healthRows, err := tx.QueryContext(ctx, `SELECT id, occurred_at FROM health_events`)
	if err != nil {
		return metadata.PruneResult{}, err
	}
	healthInsert, err := tx.PrepareContext(ctx, `INSERT INTO ilonasin_prune_health_ids(id) VALUES(?)`)
	if err != nil {
		healthRows.Close()
		return metadata.PruneResult{}, err
	}
	for healthRows.Next() {
		var id int64
		var occurred string
		if err := healthRows.Scan(&id, &occurred); err != nil {
			healthInsert.Close()
			healthRows.Close()
			return metadata.PruneResult{}, err
		}
		occurredAt, err := parseSQLiteTime(occurred)
		if err != nil {
			healthInsert.Close()
			healthRows.Close()
			return metadata.PruneResult{}, err
		}
		if occurredAt.UTC().Before(cutoff) {
			if _, err := healthInsert.ExecContext(ctx, id); err != nil {
				healthInsert.Close()
				healthRows.Close()
				return metadata.PruneResult{}, err
			}
			result.Health++
		}
	}
	if err := healthRows.Err(); err != nil {
		healthInsert.Close()
		healthRows.Close()
		return metadata.PruneResult{}, err
	}
	healthInsert.Close()
	healthRows.Close()

	quotaRows, err := tx.QueryContext(ctx, `SELECT id, observed_at, COALESCE(request_metadata_id, 0) FROM quota_events`)
	if err != nil {
		return metadata.PruneResult{}, err
	}
	quotaInsert, err := tx.PrepareContext(ctx, `INSERT INTO ilonasin_prune_quota_ids(id) VALUES(?)`)
	if err != nil {
		quotaRows.Close()
		return metadata.PruneResult{}, err
	}
	for quotaRows.Next() {
		var id, requestID int64
		var observed string
		if err := quotaRows.Scan(&id, &observed, &requestID); err != nil {
			quotaInsert.Close()
			quotaRows.Close()
			return metadata.PruneResult{}, err
		}
		observedAt, err := parseSQLiteTime(observed)
		if err != nil {
			quotaInsert.Close()
			quotaRows.Close()
			return metadata.PruneResult{}, err
		}
		_, attachedToPrunedRequest := requestIDs[requestID]
		if observedAt.UTC().Before(cutoff) || attachedToPrunedRequest {
			if _, err := quotaInsert.ExecContext(ctx, id); err != nil {
				quotaInsert.Close()
				quotaRows.Close()
				return metadata.PruneResult{}, err
			}
			result.Quotas++
		}
	}
	if err := quotaRows.Err(); err != nil {
		quotaInsert.Close()
		quotaRows.Close()
		return metadata.PruneResult{}, err
	}
	quotaInsert.Close()
	quotaRows.Close()

	if _, err := tx.ExecContext(ctx, `DELETE FROM quota_events WHERE id IN (SELECT id FROM ilonasin_prune_quota_ids)`); err != nil {
		return metadata.PruneResult{}, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM fallback_events WHERE id IN (SELECT id FROM ilonasin_prune_fallback_ids)`); err != nil {
		return metadata.PruneResult{}, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM health_events WHERE id IN (SELECT id FROM ilonasin_prune_health_ids)`); err != nil {
		return metadata.PruneResult{}, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM request_metadata WHERE id IN (SELECT id FROM ilonasin_prune_request_ids)`); err != nil {
		return metadata.PruneResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return metadata.PruneResult{}, err
	}
	if s.Logger != nil {
		s.Logger.LogAttrs(ctx, slog.LevelInfo, "telemetry pruned",
			slog.String("event", "telemetry_pruned"),
			slog.Int("requests", result.Requests),
			slog.Int("streams", result.Streams),
			slog.Int("fallbacks", result.Fallbacks),
			slog.Int("health", result.Health),
			slog.Int("quotas", result.Quotas),
		)
	}
	return result, nil
}

func resetPruneTable(ctx context.Context, tx *sql.Tx, table string) error {
	if _, err := tx.ExecContext(ctx, `CREATE TEMP TABLE IF NOT EXISTS `+table+` (id INTEGER PRIMARY KEY)`); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `DELETE FROM `+table)
	return err
}

const activeQuotaFallbackCooldown = 10 * time.Minute

func (s *Store) ActiveQuotaBlocks(ctx context.Context, providerInstanceID, modelID string, now time.Time) ([]metadata.ActiveQuotaBlock, error) {
	if now.IsZero() {
		now = time.Now()
	}
	now = now.UTC()
	rows, err := s.DB.QueryContext(ctx, `
		SELECT qe.credential_id, qe.observed_at, qe.http_status, qe.error_class, qe.retry_after, qe.reset_at
		FROM quota_events qe
		JOIN (
			SELECT credential_id, MAX(observed_at) AS observed_at
			FROM quota_events
			WHERE provider_instance_id = ?
				AND model_id = ?
				AND credential_id IS NOT NULL
			GROUP BY credential_id
		) latest ON latest.credential_id = qe.credential_id AND latest.observed_at = qe.observed_at
		WHERE qe.provider_instance_id = ?
			AND qe.model_id = ?
			AND qe.credential_id IS NOT NULL
		ORDER BY qe.credential_id ASC, qe.id DESC
	`, providerInstanceID, modelID, providerInstanceID, modelID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	seen := map[int64]bool{}
	var out []metadata.ActiveQuotaBlock
	for rows.Next() {
		var block metadata.ActiveQuotaBlock
		var observed string
		var retryAfter, resetAt sql.NullString
		if err := rows.Scan(&block.CredentialID, &observed, &block.HTTPStatus, &block.ErrorClass, &retryAfter, &resetAt); err != nil {
			return nil, err
		}
		if seen[block.CredentialID] {
			continue
		}
		seen[block.CredentialID] = true
		observedAt, err := parseSQLiteTime(observed)
		if err != nil {
			return nil, err
		}
		block.ObservedAt = observedAt.UTC()
		activeUntil := block.ObservedAt.Add(activeQuotaFallbackCooldown)
		if retryAfter.Valid && retryAfter.String != "" {
			parsed, err := parseSQLiteTime(retryAfter.String)
			if err != nil {
				return nil, err
			}
			parsed = parsed.UTC()
			block.RetryAfter = &parsed
			if parsed.After(activeUntil) {
				activeUntil = parsed
			}
		}
		if resetAt.Valid && resetAt.String != "" {
			parsed, err := parseSQLiteTime(resetAt.String)
			if err != nil {
				return nil, err
			}
			parsed = parsed.UTC()
			block.ResetAt = &parsed
			if parsed.After(activeUntil) {
				activeUntil = parsed
			}
		}
		if !activeUntil.After(now) {
			continue
		}
		block.ActiveUntil = activeUntil
		out = append(out, block)
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
