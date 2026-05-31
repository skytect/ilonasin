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
	"ilonasin/internal/provider"
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

func (s *Store) InsertOAuthCredential(ctx context.Context, meta credentials.NewOAuthCredential, accessToken, refreshToken string) (credentials.OAuthCredentialMetadata, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return credentials.OAuthCredentialMetadata{}, err
	}
	defer tx.Rollback()
	ts := meta.CreatedAt.UTC().Format(time.RFC3339Nano)
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
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO oauth_tokens(
			credential_id, access_token_secret_id, refresh_token_secret_id, expires_at, scopes
		) VALUES(?, ?, ?, ?, ?)
	`, credentialID, accessID, refreshID, nullableTime(meta.ExpiresAt), meta.Scopes); err != nil {
		return credentials.OAuthCredentialMetadata{}, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO provider_accounts(
			provider_instance_id, credential_id, account_hash, display_label, plan_label, created_at, updated_at
		) VALUES(?, ?, ?, ?, ?, ?, ?)
	`, meta.ProviderInstanceID, credentialID, meta.AccountHash, meta.AccountDisplayLabel, meta.PlanLabel, ts, ts); err != nil {
		return credentials.OAuthCredentialMetadata{}, err
	}
	if err := tx.Commit(); err != nil {
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
			COALESCE(ot.refresh_failure_class, ''), pc.created_at, pc.disabled_at
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
			&lastRefresh, &row.RefreshFailureClass, &created, &disabled); err != nil {
			return nil, err
		}
		createdAt, err := time.Parse(time.RFC3339Nano, created)
		if err != nil {
			return nil, err
		}
		row.CreatedAt = createdAt
		if expires.Valid {
			expiresAt, err := time.Parse(time.RFC3339Nano, expires.String)
			if err != nil {
				return nil, err
			}
			row.ExpiresAt = &expiresAt
		}
		if lastRefresh.Valid {
			refreshAt, err := time.Parse(time.RFC3339Nano, lastRefresh.String)
			if err != nil {
				return nil, err
			}
			row.LastRefreshAt = &refreshAt
		}
		if disabled.Valid {
			disabledAt, err := time.Parse(time.RFC3339Nano, disabled.String)
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
		createdAt, err := time.Parse(time.RFC3339Nano, created)
		if err != nil {
			return nil, err
		}
		row.CreatedAt = createdAt
		out = append(out, row)
	}
	return out, rows.Err()
}

func (s *Store) MarkOAuthRefreshFailure(ctx context.Context, credentialID int64, failureClass string, now time.Time) error {
	res, err := s.DB.ExecContext(ctx, `
		UPDATE oauth_tokens
		SET refresh_failure_class = ?, last_refresh_at = ?
		WHERE credential_id = ?
	`, failureClass, now.UTC().Format(time.RFC3339Nano), credentialID)
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
	group := all[0].FallbackGroup
	enabled, err := s.fallbackGroupEnabled(ctx, providerInstanceID, credentials.CredentialKindAPIKey, group)
	if err != nil {
		return nil, err
	}
	if !enabled {
		return all[:1], nil
	}
	out := all[:0]
	for _, cred := range all {
		if cred.FallbackGroup == group {
			out = append(out, cred)
		}
	}
	if len(out) == 0 {
		return nil, credentials.ErrNoEligibleCredential
	}
	return out, nil
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
		expiresAt, err := time.Parse(time.RFC3339Nano, expires.String)
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
	group := primary.FallbackGroup
	enabled, err := s.fallbackGroupEnabled(ctx, providerInstanceID, credentials.CredentialKindOAuth, group)
	if err != nil {
		return nil, err
	}
	if !enabled {
		return []credentials.ResolvedOAuthBearerCredential{primary}, nil
	}
	out := []credentials.ResolvedOAuthBearerCredential{primary}
	for _, row := range candidates[1:] {
		if row.fallback != group {
			continue
		}
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
		expiresAt, err := time.Parse(time.RFC3339Nano, row.expires.String)
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
		expiresAt, err := time.Parse(time.RFC3339Nano, expires.String)
		if err != nil {
			return credentials.ResolvedOAuthBearerCredential{}, err
		}
		expiresAt = expiresAt.UTC()
		if !expiresAt.After(now.UTC()) {
			return credentials.ResolvedOAuthBearerCredential{}, credentials.ErrNoEligibleCredential
		}
		out.ExpiresAt = &expiresAt
	} else if expires.Valid {
		expiresAt, err := time.Parse(time.RFC3339Nano, expires.String)
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
		SET expires_at = ?, last_refresh_at = ?, refresh_failure_class = ''
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
			retry_count, fallback_count, fallback_reason, prompt_tokens, completion_tokens,
			total_tokens, reasoning_tokens, cache_hit_tokens, cache_write_tokens, cost_microunits,
			total_latency_ms, time_to_first_token_ms,
			output_tokens_per_second
		) VALUES(?, NULLIF(?, 0), NULLIF(?, 0), ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, m.StartedAt.UTC().Format(time.RFC3339Nano), m.ClientTokenID, m.CredentialID, m.RequestedProviderInstance,
		m.RequestedModel, m.ResolvedProviderInstance, m.ResolvedModel, m.HTTPStatus,
		m.ErrorClass, m.RetryCount, m.FallbackCount, m.FallbackReason, m.PromptTokens, m.CompletionTokens,
		m.TotalTokens, m.ReasoningTokens, m.CacheHitTokens, m.CacheWriteTokens, m.CostMicrounits, m.TotalLatencyMS, m.TimeToFirstTokenMS,
		m.OutputTokensPerSecond)
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	if s.Logger != nil {
		s.Logger.LogAttrs(ctx, slog.LevelInfo, "metadata recorded",
			slog.String("event", "metadata_recorded"),
			slog.Int64("metadata_id", id),
			slog.String("provider_instance", m.ResolvedProviderInstance),
			slog.Int("status", m.HTTPStatus),
			slog.String("error_class", m.ErrorClass),
		)
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
	if err == nil && s.Logger != nil {
		s.Logger.LogAttrs(ctx, slog.LevelInfo, "stream metadata recorded",
			slog.String("event", "stream_recorded"),
			slog.Int64("metadata_id", m.RequestMetadataID),
			slog.String("stream_status", m.CompletionStatus),
			slog.Int("chunk_count", m.ChunkCount),
		)
	}
	return err
}

func (s *Store) RecordHealthEvent(ctx context.Context, m metadata.HealthEvent) error {
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO health_events(
			occurred_at, provider_instance_id, credential_id, model_id,
			event_class, http_status, normalized_error_class, retry_after
		) VALUES(?, ?, ?, ?, ?, ?, ?, ?)
	`, m.OccurredAt.UTC().Format(time.RFC3339Nano), m.ProviderInstanceID,
		nullableInt64(m.CredentialID), m.ModelID, m.EventClass, nullableInt(m.HTTPStatus), m.ErrorClass,
		nullableTime(m.RetryAfter))
	if err == nil && s.Logger != nil {
		s.Logger.LogAttrs(ctx, slog.LevelInfo, "health event recorded",
			slog.String("event", "health_recorded"),
			slog.String("provider_instance", m.ProviderInstanceID),
			slog.Int64("credential_id", m.CredentialID),
			slog.String("health_event", m.EventClass),
			slog.Int("status", m.HTTPStatus),
			slog.String("error_class", m.ErrorClass),
		)
	}
	return err
}

func (s *Store) RecordFallbackEvent(ctx context.Context, m metadata.FallbackEvent) error {
	allowed := 0
	if m.AllowedByPolicy {
		allowed = 1
	}
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO fallback_events(
			request_metadata_id, occurred_at, provider_instance_id, model_id,
			from_credential_id, to_credential_id, reason, allowed_by_policy
		) VALUES(?, ?, ?, ?, ?, ?, ?, ?)
	`, m.RequestMetadataID, m.OccurredAt.UTC().Format(time.RFC3339Nano),
		m.ProviderInstanceID, m.ModelID, nullableInt64(m.FromCredentialID),
		nullableInt64(m.ToCredentialID), m.Reason, allowed)
	if err == nil && s.Logger != nil {
		s.Logger.LogAttrs(ctx, slog.LevelInfo, "fallback event recorded",
			slog.String("event", "fallback_recorded"),
			slog.String("provider_instance", m.ProviderInstanceID),
			slog.Int64("metadata_id", m.RequestMetadataID),
			slog.Int64("from_credential_id", m.FromCredentialID),
			slog.Int64("to_credential_id", m.ToCredentialID),
			slog.Bool("allowed", m.AllowedByPolicy),
		)
	}
	return err
}

func (s *Store) RecordQuotaObservation(ctx context.Context, m metadata.QuotaObservation) error {
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO quota_events(
			request_metadata_id, observed_at, provider_instance_id, credential_id,
			model_id, source, http_status, error_class, retry_after, reset_at
		) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, nullableInt64(m.RequestMetadataID), m.ObservedAt.UTC().Format(time.RFC3339Nano),
		m.ProviderInstanceID, nullableInt64(m.CredentialID), m.ModelID, m.Source,
		nullableInt(m.HTTPStatus), m.ErrorClass, nullableTime(m.RetryAfter), nullableTime(m.ResetAt))
	if err == nil && s.Logger != nil {
		s.Logger.LogAttrs(ctx, slog.LevelInfo, "quota observation recorded",
			slog.String("event", "quota_recorded"),
			slog.String("provider_instance", m.ProviderInstanceID),
			slog.Int64("credential_id", m.CredentialID),
			slog.Int64("metadata_id", m.RequestMetadataID),
			slog.String("quota_source", m.Source),
			slog.Int("status", m.HTTPStatus),
			slog.String("error_class", m.ErrorClass),
		)
	}
	return err
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
		startedAt, err := time.Parse(time.RFC3339Nano, started)
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
		occurredAt, err := time.Parse(time.RFC3339Nano, occurred)
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
		occurredAt, err := time.Parse(time.RFC3339Nano, occurred)
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
		observedAt, err := time.Parse(time.RFC3339Nano, observed)
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

func (s *Store) RecentRequests(ctx context.Context, limit int) ([]metadata.RequestSummary, error) {
	if limit <= 0 || limit > 50 {
		limit = 5
	}
	rows, err := s.DB.QueryContext(ctx, `
		SELECT rm.id, rm.started_at, rm.resolved_provider_instance, rm.resolved_model,
			rm.requested_provider_instance, rm.requested_model, rm.resolved_provider_instance, rm.resolved_model,
			COALESCE(rm.credential_id, 0), COALESCE(pc.label, ''),
			rm.http_status, rm.error_class, rm.retry_count, rm.fallback_count,
			rm.fallback_reason, rm.prompt_tokens, rm.completion_tokens, rm.total_tokens, rm.reasoning_tokens,
			rm.cache_hit_tokens, rm.cache_write_tokens, rm.cost_microunits,
			rm.total_latency_ms, rm.time_to_first_token_ms, rm.output_tokens_per_second,
			COALESCE(sm.completion_status, ''), COALESCE(sm.chunk_count, 0)
		FROM request_metadata rm
		LEFT JOIN provider_credentials pc ON pc.id = rm.credential_id
		LEFT JOIN (
			SELECT sm1.request_metadata_id, sm1.completion_status, sm1.chunk_count
			FROM stream_metrics sm1
			INNER JOIN (
				SELECT request_metadata_id, MAX(id) AS id
				FROM stream_metrics
				GROUP BY request_metadata_id
			) latest ON latest.id = sm1.id
		) sm ON sm.request_metadata_id = rm.id
		ORDER BY rm.id DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []metadata.RequestSummary
	for rows.Next() {
		var row metadata.RequestSummary
		var started string
		if err := rows.Scan(&row.ID, &started, &row.ProviderInstanceID, &row.ModelID,
			&row.RequestedProviderID, &row.RequestedModelID, &row.ResolvedProviderID, &row.ResolvedModelID,
			&row.CredentialID, &row.CredentialLabel, &row.HTTPStatus, &row.ErrorClass,
			&row.RetryCount, &row.FallbackCount, &row.FallbackReason, &row.PromptTokens, &row.CompletionTokens,
			&row.TotalTokens, &row.ReasoningTokens, &row.CacheHitTokens, &row.CacheWriteTokens, &row.CostMicrounits, &row.TotalLatencyMS,
			&row.TimeToFirstTokenMS, &row.OutputTokensPerSecond,
			&row.StreamCompletionStatus, &row.StreamChunkCount); err != nil {
			return nil, err
		}
		startedAt, err := time.Parse(time.RFC3339Nano, started)
		if err != nil {
			return nil, err
		}
		row.StartedAt = startedAt
		out = append(out, row)
	}
	return out, rows.Err()
}

func (s *Store) UsageByProvider(ctx context.Context) ([]metadata.UsageSummary, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT requested_provider_instance, COUNT(*), COALESCE(SUM(prompt_tokens), 0),
			COALESCE(SUM(completion_tokens), 0), COALESCE(SUM(total_tokens), 0),
			COALESCE(SUM(reasoning_tokens), 0), COALESCE(SUM(cache_hit_tokens), 0),
			COALESCE(SUM(cache_write_tokens), 0), COALESCE(SUM(cost_microunits), 0)
		FROM request_metadata
		GROUP BY requested_provider_instance
		ORDER BY requested_provider_instance ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []metadata.UsageSummary
	for rows.Next() {
		var row metadata.UsageSummary
		if err := rows.Scan(&row.ProviderInstanceID, &row.RequestCount, &row.PromptTokens,
			&row.CompletionTokens, &row.TotalTokens, &row.ReasoningTokens, &row.CacheHitTokens,
			&row.CacheWriteTokens, &row.CostMicrounits); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (s *Store) LatencyByProvider(ctx context.Context) ([]metadata.LatencySummary, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT requested_provider_instance, COUNT(*),
			COALESCE(AVG(total_latency_ms), 0),
			COALESCE(AVG(NULLIF(time_to_first_token_ms, 0)), 0),
			COALESCE(AVG(NULLIF(output_tokens_per_second, 0)), 0)
		FROM request_metadata
		GROUP BY requested_provider_instance
		ORDER BY requested_provider_instance ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []metadata.LatencySummary
	for rows.Next() {
		var row metadata.LatencySummary
		var latency, ttft, tps float64
		if err := rows.Scan(&row.ProviderInstanceID, &row.RequestCount, &latency, &ttft, &tps); err != nil {
			return nil, err
		}
		row.AverageLatencyMS = int64(latency + 0.5)
		row.AverageTimeToFirstTokenMS = int64(ttft + 0.5)
		row.AverageOutputTPS = tps
		out = append(out, row)
	}
	return out, rows.Err()
}

func (s *Store) StreamSummary(ctx context.Context) ([]metadata.StreamSummary, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT completion_status, COUNT(*), COALESCE(SUM(chunk_count), 0)
		FROM stream_metrics
		GROUP BY completion_status
		ORDER BY completion_status ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []metadata.StreamSummary
	for rows.Next() {
		var row metadata.StreamSummary
		if err := rows.Scan(&row.CompletionStatus, &row.StreamCount, &row.ChunkCount); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (s *Store) LatestHealth(ctx context.Context) ([]metadata.HealthSummary, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT he.provider_instance_id, he.model_id, COALESCE(he.credential_id, 0),
			COALESCE(pc.label, ''), he.event_class, COALESCE(he.http_status, 0),
			he.normalized_error_class, he.occurred_at, he.retry_after
		FROM health_events he
		LEFT JOIN provider_credentials pc ON pc.id = he.credential_id
		WHERE he.id IN (
			SELECT MAX(id)
			FROM health_events
			GROUP BY provider_instance_id, COALESCE(credential_id, 0), model_id
		)
		ORDER BY he.provider_instance_id ASC, COALESCE(he.credential_id, 0) ASC, he.model_id ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []metadata.HealthSummary
	for rows.Next() {
		var row metadata.HealthSummary
		var occurred string
		var retryAfter sql.NullString
		if err := rows.Scan(&row.ProviderInstanceID, &row.ModelID, &row.CredentialID,
			&row.CredentialLabel, &row.EventClass, &row.HTTPStatus, &row.ErrorClass,
			&occurred, &retryAfter); err != nil {
			return nil, err
		}
		occurredAt, err := time.Parse(time.RFC3339Nano, occurred)
		if err != nil {
			return nil, err
		}
		row.OccurredAt = occurredAt
		if retryAfter.Valid && retryAfter.String != "" {
			parsed, err := time.Parse(time.RFC3339Nano, retryAfter.String)
			if err != nil {
				return nil, err
			}
			row.RetryAfter = &parsed
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (s *Store) RecentFallbacks(ctx context.Context, limit int) ([]metadata.FallbackSummary, error) {
	if limit <= 0 || limit > 50 {
		limit = 5
	}
	rows, err := s.DB.QueryContext(ctx, `
		SELECT fe.id, COALESCE(fe.request_metadata_id, 0), fe.occurred_at,
			fe.provider_instance_id, fe.model_id,
			COALESCE(fe.from_credential_id, 0), COALESCE(from_pc.label, ''),
			COALESCE(fe.to_credential_id, 0), COALESCE(to_pc.label, ''),
			fe.reason
		FROM fallback_events fe
		LEFT JOIN provider_credentials from_pc ON from_pc.id = fe.from_credential_id
		LEFT JOIN provider_credentials to_pc ON to_pc.id = fe.to_credential_id
		ORDER BY fe.id DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []metadata.FallbackSummary
	for rows.Next() {
		var row metadata.FallbackSummary
		var occurred string
		if err := rows.Scan(&row.ID, &row.RequestMetadataID, &occurred,
			&row.ProviderInstanceID, &row.ModelID, &row.FromCredentialID,
			&row.FromCredentialLabel, &row.ToCredentialID, &row.ToCredentialLabel,
			&row.Reason); err != nil {
			return nil, err
		}
		occurredAt, err := time.Parse(time.RFC3339Nano, occurred)
		if err != nil {
			return nil, err
		}
		row.OccurredAt = occurredAt
		out = append(out, row)
	}
	return out, rows.Err()
}

func (s *Store) QuotaByProvider(ctx context.Context) ([]metadata.QuotaSummary, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT qe.provider_instance_id, qe.model_id, COALESCE(qe.credential_id, 0),
			COALESCE(pc.label, ''), qe.source, qe.http_status, qe.error_class,
			MAX(qe.observed_at), qe.retry_after, qe.reset_at, COUNT(*)
		FROM quota_events qe
		LEFT JOIN provider_credentials pc ON pc.id = qe.credential_id
		GROUP BY qe.provider_instance_id, qe.model_id, COALESCE(qe.credential_id, 0),
			qe.source, qe.http_status, qe.error_class, qe.retry_after, qe.reset_at
		ORDER BY MAX(qe.observed_at) DESC, qe.provider_instance_id ASC, qe.model_id ASC
		LIMIT 20
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []metadata.QuotaSummary
	for rows.Next() {
		var row metadata.QuotaSummary
		var observed string
		var retryAfter, resetAt sql.NullString
		if err := rows.Scan(&row.ProviderInstanceID, &row.ModelID, &row.CredentialID,
			&row.CredentialLabel, &row.Source, &row.HTTPStatus, &row.ErrorClass,
			&observed, &retryAfter, &resetAt, &row.Count); err != nil {
			return nil, err
		}
		observedAt, err := time.Parse(time.RFC3339Nano, observed)
		if err != nil {
			return nil, err
		}
		row.ObservedAt = observedAt
		if retryAfter.Valid && retryAfter.String != "" {
			parsed, err := time.Parse(time.RFC3339Nano, retryAfter.String)
			if err != nil {
				return nil, err
			}
			row.RetryAfter = &parsed
		}
		if resetAt.Valid && resetAt.String != "" {
			parsed, err := time.Parse(time.RFC3339Nano, resetAt.String)
			if err != nil {
				return nil, err
			}
			row.ResetAt = &parsed
		}
		out = append(out, row)
	}
	return out, rows.Err()
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

func nullableTime(t *time.Time) any {
	if t == nil || t.IsZero() {
		return nil
	}
	return t.UTC().Format(time.RFC3339Nano)
}

func cloneTime(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	cloned := t.UTC()
	return &cloned
}
