package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"ilonasin/internal/credentials"
)

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

func (s *Store) UpdateOAuthAccountMetadata(ctx context.Context, credentialID int64, displayLabel, planLabel string, updatedAt time.Time) error {
	if displayLabel == "" && planLabel == "" {
		return nil
	}
	res, err := s.DB.ExecContext(ctx, `
		UPDATE provider_accounts
		SET display_label = CASE WHEN ? != '' THEN ? ELSE display_label END,
			plan_label = CASE WHEN ? != '' THEN ? ELSE plan_label END,
			updated_at = ?
		WHERE credential_id = ?
	`, displayLabel, displayLabel, planLabel, planLabel, updatedAt.UTC().Format(time.RFC3339Nano), credentialID)
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
