package sqlite

import (
	"context"
	"database/sql"
	"time"

	"ilonasin/internal/metadata"
)

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
