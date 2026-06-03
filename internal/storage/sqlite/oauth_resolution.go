package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"ilonasin/internal/credentials"
)

func (s *Store) ResolveOAuthBearerCredentials(ctx context.Context, providerInstanceID string, now time.Time) ([]credentials.ResolvedOAuthBearerCredential, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT pc.id, pc.provider_instance_id, pc.fallback_group, ot.access_token_secret_id, ot.expires_at, COALESCE(ot.refresh_failure_class, '')
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
		if err := rows.Scan(&row.credential.ID, &row.credential.ProviderInstanceID, &row.fallback, &row.accessSecret, &row.expires, &row.refreshFailure); err != nil {
			return nil, err
		}
		row.credential.PoolGroup = row.fallback
		candidates = append(candidates, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(candidates) == 0 {
		return nil, credentials.ErrNoEligibleCredential
	}
	out := make([]credentials.ResolvedOAuthBearerCredential, 0, len(candidates))
	for _, row := range candidates {
		credential, ok, err := s.materializeOAuthBearer(ctx, row, now, true)
		if err != nil {
			return nil, err
		}
		if ok {
			out = append(out, credential)
		}
	}
	if len(out) == 0 {
		return nil, credentials.ErrNoEligibleCredential
	}
	return out, nil
}

type oauthBearerRow struct {
	credential     credentials.ResolvedOAuthBearerCredential
	fallback       string
	accessSecret   sql.NullInt64
	expires        sql.NullString
	refreshFailure sql.NullString
}

func (s *Store) materializeOAuthBearer(ctx context.Context, row oauthBearerRow, now time.Time, skipIneligible bool) (credentials.ResolvedOAuthBearerCredential, bool, error) {
	credential := row.credential
	if !row.accessSecret.Valid {
		if skipIneligible {
			return credentials.ResolvedOAuthBearerCredential{}, false, nil
		}
		return credentials.ResolvedOAuthBearerCredential{}, false, nil
	}
	if terminalOAuthRefreshFailure(row.refreshFailure.String) {
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

func terminalOAuthRefreshFailure(failureClass string) bool {
	switch failureClass {
	case "refresh_token_expired", "refresh_token_invalidated", "refresh_token_reused", "refresh_invalid_grant", "refresh_access_denied":
		return true
	default:
		return false
	}
}

func (s *Store) ResolveOAuthBearerCredentialByID(ctx context.Context, credentialID int64, now time.Time) (credentials.ResolvedOAuthBearerCredential, error) {
	var out credentials.ResolvedOAuthBearerCredential
	var accessSecretID int64
	var expires sql.NullString
	var refreshFailure sql.NullString
	err := s.DB.QueryRowContext(ctx, `
		SELECT pc.id, pc.provider_instance_id, pc.fallback_group, ot.access_token_secret_id, ot.expires_at, COALESCE(ot.refresh_failure_class, '')
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
	`, credentialID).Scan(&out.ID, &out.ProviderInstanceID, &out.PoolGroup, &accessSecretID, &expires, &refreshFailure)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return credentials.ResolvedOAuthBearerCredential{}, credentials.ErrNoEligibleCredential
		}
		return credentials.ResolvedOAuthBearerCredential{}, err
	}
	if terminalOAuthRefreshFailure(refreshFailure.String) {
		return credentials.ResolvedOAuthBearerCredential{}, credentials.ErrNoEligibleCredential
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
		SELECT pc.id, pc.provider_instance_id, pa.account_hash, ot.access_token_secret_id, ot.refresh_token_secret_id
		FROM provider_credentials pc
		JOIN oauth_tokens ot ON ot.credential_id = pc.id
		JOIN provider_accounts pa ON pa.credential_id = pc.id
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
			AND COALESCE(ot.refresh_failure_class, '') NOT IN (
				'refresh_token_expired', 'refresh_token_invalidated', 'refresh_token_reused',
				'refresh_invalid_grant', 'refresh_access_denied'
			)
	`, credentialID).Scan(&out.ID, &out.ProviderInstanceID, &out.AccountHash, &out.AccessTokenSecretID, &out.RefreshTokenSecretID)
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
		SELECT pc.id, pc.provider_instance_id, pa.account_hash, ot.access_token_secret_id, ot.refresh_token_secret_id
		FROM provider_credentials pc
		JOIN oauth_tokens ot ON ot.credential_id = pc.id
		JOIN provider_accounts pa ON pa.credential_id = pc.id
		WHERE pc.provider_instance_id = ?
			AND pc.kind = 'oauth'
			AND pc.disabled_at IS NULL
			AND COALESCE(ot.refresh_failure_class, '') NOT IN (
				'refresh_token_expired', 'refresh_token_invalidated', 'refresh_token_reused',
				'refresh_invalid_grant', 'refresh_access_denied'
			)
		ORDER BY pc.id ASC
		LIMIT 1
	`, providerInstanceID).Scan(&out.ID, &out.ProviderInstanceID, &out.AccountHash, &accessSecretID, &refreshSecretID)
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
