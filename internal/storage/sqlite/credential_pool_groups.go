package sqlite

import (
	"context"

	"ilonasin/internal/credentials"
)

func (s *Store) ListCredentialPoolGroups(ctx context.Context) ([]credentials.CredentialPoolGroupMetadata, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT provider_instance_id, kind AS credential_kind, pool_group AS group_label, COUNT(*) AS credential_count
		FROM provider_credentials
		WHERE kind IN ('api_key', 'oauth')
			AND disabled_at IS NULL
		GROUP BY provider_instance_id, kind, pool_group
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
