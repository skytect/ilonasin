package sqlite

import (
	"context"
	"database/sql"
	"sort"
	"time"

	"ilonasin/internal/metadata"
)

const activeQuotaFallbackCooldown = 10 * time.Minute
const activeQuotaSnapshotCandidateLimit = 500

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
		var credentialID int64
		var httpStatus int
		var errorClass string
		var observed string
		var retryAfter, resetAt sql.NullString
		if err := rows.Scan(&credentialID, &observed, &httpStatus, &errorClass, &retryAfter, &resetAt); err != nil {
			return nil, err
		}
		if seen[credentialID] {
			continue
		}
		seen[credentialID] = true
		block, active, err := activeQuotaBlockFromSQLite(credentialID, observed, httpStatus, errorClass, retryAfter, resetAt, now)
		if err != nil {
			return nil, err
		}
		if !active {
			continue
		}
		out = append(out, block)
	}
	return out, rows.Err()
}

func (s *Store) ActiveQuotaBlockSnapshot(ctx context.Context, now time.Time) ([]metadata.ActiveQuotaBlockSummary, error) {
	if now.IsZero() {
		now = time.Now()
	}
	now = now.UTC()
	nowText := now.Format(time.RFC3339Nano)
	rows, err := s.DB.QueryContext(ctx, `
		WITH ranked AS (
			SELECT qe.provider_instance_id, qe.model_id, qe.credential_id,
				COALESCE(pc.label, '') AS credential_label,
				qe.observed_at, qe.http_status, qe.error_class, qe.retry_after, qe.reset_at,
				ROW_NUMBER() OVER (
					PARTITION BY qe.provider_instance_id, qe.model_id, qe.credential_id
					ORDER BY qe.observed_at DESC, qe.id DESC
				) AS row_rank
			FROM quota_events qe
			LEFT JOIN provider_credentials pc ON pc.id = qe.credential_id
			WHERE qe.credential_id IS NOT NULL
		)
		SELECT provider_instance_id, model_id, credential_id, credential_label,
			observed_at, http_status, error_class, retry_after, reset_at
		FROM ranked
		WHERE row_rank = 1
			AND (
				julianday(observed_at) + (? / 86400.0) > julianday(?)
				OR (retry_after IS NOT NULL AND retry_after != '' AND julianday(retry_after) > julianday(?))
				OR (reset_at IS NOT NULL AND reset_at != '' AND julianday(reset_at) > julianday(?))
			)
		ORDER BY observed_at DESC, provider_instance_id ASC, model_id ASC, credential_id ASC
		LIMIT ?
	`, activeQuotaFallbackCooldown.Seconds(), nowText, nowText, nowText, activeQuotaSnapshotCandidateLimit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []metadata.ActiveQuotaBlockSummary
	for rows.Next() {
		var providerInstanceID, modelID, credentialLabel, observed, errorClass string
		var credentialID int64
		var httpStatus int
		var retryAfter, resetAt sql.NullString
		if err := rows.Scan(&providerInstanceID, &modelID, &credentialID, &credentialLabel,
			&observed, &httpStatus, &errorClass, &retryAfter, &resetAt); err != nil {
			return nil, err
		}
		block, active, err := activeQuotaBlockFromSQLite(credentialID, observed, httpStatus, errorClass, retryAfter, resetAt, now)
		if err != nil {
			return nil, err
		}
		if !active {
			continue
		}
		out = append(out, metadata.ActiveQuotaBlockSummary{
			ObservedAt:         block.ObservedAt,
			ProviderInstanceID: providerInstanceID,
			ModelID:            modelID,
			CredentialID:       block.CredentialID,
			CredentialLabel:    credentialLabel,
			HTTPStatus:         block.HTTPStatus,
			ErrorClass:         block.ErrorClass,
			RetryAfter:         block.RetryAfter,
			ResetAt:            block.ResetAt,
			ActiveUntil:        block.ActiveUntil,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].ActiveUntil.Equal(out[j].ActiveUntil) {
			return out[i].ActiveUntil.Before(out[j].ActiveUntil)
		}
		if out[i].ProviderInstanceID != out[j].ProviderInstanceID {
			return out[i].ProviderInstanceID < out[j].ProviderInstanceID
		}
		if out[i].ModelID != out[j].ModelID {
			return out[i].ModelID < out[j].ModelID
		}
		return out[i].CredentialID < out[j].CredentialID
	})
	return out, nil
}

func activeQuotaBlockFromSQLite(credentialID int64, observed string, httpStatus int, errorClass string, retryAfter, resetAt sql.NullString, now time.Time) (metadata.ActiveQuotaBlock, bool, error) {
	observedAt, err := parseSQLiteTime(observed)
	if err != nil {
		return metadata.ActiveQuotaBlock{}, false, err
	}
	block := metadata.ActiveQuotaBlock{
		ObservedAt:   observedAt.UTC(),
		CredentialID: credentialID,
		HTTPStatus:   httpStatus,
		ErrorClass:   errorClass,
	}
	activeUntil := block.ObservedAt.Add(activeQuotaFallbackCooldown)
	if retryAfter.Valid && retryAfter.String != "" {
		parsed, err := parseSQLiteTime(retryAfter.String)
		if err != nil {
			return metadata.ActiveQuotaBlock{}, false, err
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
			return metadata.ActiveQuotaBlock{}, false, err
		}
		parsed = parsed.UTC()
		block.ResetAt = &parsed
		if parsed.After(activeUntil) {
			activeUntil = parsed
		}
	}
	if !activeUntil.After(now.UTC()) {
		return block, false, nil
	}
	block.ActiveUntil = activeUntil
	return block, true, nil
}
