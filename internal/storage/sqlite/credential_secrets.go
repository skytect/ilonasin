package sqlite

import "context"

func (s *Store) ListCredentialSecretMaterial(ctx context.Context) ([]string, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT secret_material
		FROM credential_secrets
		ORDER BY id ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var secret string
		if err := rows.Scan(&secret); err != nil {
			return nil, err
		}
		out = append(out, secret)
	}
	return out, rows.Err()
}
