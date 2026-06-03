package sqlite

import (
	"context"
	"database/sql"
	"time"

	"ilonasin/internal/credentials"
)

func (s *Store) InsertAPIKeyCredential(ctx context.Context, meta credentials.NewUpstreamCredential, apiKey string) (credentials.UpstreamCredentialMetadata, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return credentials.UpstreamCredentialMetadata{}, err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `
		INSERT INTO provider_credentials(
			provider_instance_id, kind, label, secret_prefix, secret_last4, pool_group, created_at, updated_at
		) VALUES(?, ?, ?, ?, ?, ?, ?, ?)
	`, meta.ProviderInstanceID, meta.Kind, meta.Label, meta.SecretPrefix, meta.SecretLast4, meta.PoolGroup,
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
		PoolGroup:          meta.PoolGroup,
		CreatedAt:          meta.CreatedAt.UTC(),
	}, nil
}

func (s *Store) ListUpstreamCredentials(ctx context.Context) ([]credentials.UpstreamCredentialMetadata, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, provider_instance_id, kind, label, secret_prefix, secret_last4, pool_group, created_at, disabled_at
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

func (s *Store) ResolveAPIKeyCredentials(ctx context.Context, providerInstanceID string) ([]credentials.ResolvedAPIKeyCredential, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT pc.id, pc.provider_instance_id, pc.label, pc.pool_group, cs.secret_material
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
		if err := rows.Scan(&out.ID, &out.ProviderInstanceID, &out.Label, &out.PoolGroup, &out.APIKey); err != nil {
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

func scanUpstreamCredentialMetadata(row rowScanner) (credentials.UpstreamCredentialMetadata, error) {
	var meta credentials.UpstreamCredentialMetadata
	var created string
	var disabled sql.NullString
	if err := row.Scan(&meta.ID, &meta.ProviderInstanceID, &meta.Kind, &meta.Label, &meta.SecretPrefix, &meta.SecretLast4, &meta.PoolGroup, &created, &disabled); err != nil {
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
