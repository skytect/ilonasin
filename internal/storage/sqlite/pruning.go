package sqlite

import (
	"context"
	"database/sql"
	"log/slog"
	"time"

	"ilonasin/internal/metadata"
)

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
		startedAt, err := parseSQLiteTime(started)
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
		occurredAt, err := parseSQLiteTime(occurred)
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
		occurredAt, err := parseSQLiteTime(occurred)
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
		observedAt, err := parseSQLiteTime(observed)
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
