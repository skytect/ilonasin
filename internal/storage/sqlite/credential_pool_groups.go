package sqlite

import (
	"context"

	"ilonasin/internal/credentials"
)

func (s *Store) ListCredentialPoolGroups(ctx context.Context) ([]credentials.CredentialPoolGroupMetadata, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT pc.provider_instance_id, pc.kind AS credential_kind, pc.pool_group AS group_label, COUNT(*) AS credential_count
		FROM provider_credentials pc
		LEFT JOIN oauth_tokens ot ON ot.credential_id = pc.id
		WHERE pc.kind IN ('api_key', 'oauth')
			AND pc.disabled_at IS NULL
			AND (
				pc.kind != 'oauth'
				OR COALESCE(ot.refresh_failure_class, '') NOT IN (
					'refresh_token_expired', 'refresh_token_invalidated', 'refresh_token_reused',
					'refresh_invalid_grant', 'refresh_access_denied'
				)
			)
		GROUP BY pc.provider_instance_id, pc.kind, pc.pool_group
		ORDER BY provider_instance_id ASC, credential_kind ASC, group_label ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []credentials.CredentialPoolGroupMetadata
	for rows.Next() {
		var row credentials.CredentialPoolGroupMetadata
		if err := rows.Scan(&row.ProviderInstanceID, &row.CredentialKind, &row.GroupLabel, &row.CredentialCount); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}
