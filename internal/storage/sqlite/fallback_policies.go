package sqlite

import (
	"context"

	"ilonasin/internal/credentials"
)

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
