package sqlite

import (
	"context"
	"database/sql"
	"log/slog"
	"time"

	"ilonasin/internal/metadata"
)

func (s *Store) UpsertSubscriptionUsageSnapshot(ctx context.Context, m metadata.SubscriptionUsageSnapshot) error {
	if m.ObservedAt.IsZero() {
		m.ObservedAt = time.Now()
	}
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO subscription_usage_snapshots(
			observed_at, provider_instance_id, credential_id, account_display_label,
			plan_label, limit_id, limit_name, plan_type, reached_type,
			primary_used_percent, primary_window_minutes, primary_reset_at,
			secondary_used_percent, secondary_window_minutes, secondary_reset_at,
			source, error_class, stale
		) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(provider_instance_id, credential_id, limit_id) DO UPDATE SET
			observed_at = excluded.observed_at,
			account_display_label = excluded.account_display_label,
			plan_label = excluded.plan_label,
			limit_name = excluded.limit_name,
			plan_type = excluded.plan_type,
			reached_type = excluded.reached_type,
			primary_used_percent = excluded.primary_used_percent,
			primary_window_minutes = excluded.primary_window_minutes,
			primary_reset_at = excluded.primary_reset_at,
			secondary_used_percent = excluded.secondary_used_percent,
			secondary_window_minutes = excluded.secondary_window_minutes,
			secondary_reset_at = excluded.secondary_reset_at,
			source = excluded.source,
			error_class = excluded.error_class,
			stale = excluded.stale
	`, m.ObservedAt.UTC().Format(time.RFC3339Nano), m.ProviderInstanceID, nullableInt64(m.CredentialID),
		m.AccountDisplayLabel, m.PlanLabel, m.LimitID, m.LimitName, m.PlanType, m.ReachedType,
		m.PrimaryUsedPercent, m.PrimaryWindowMinutes, nullableTime(m.PrimaryResetAt),
		m.SecondaryUsedPercent, m.SecondaryWindowMinutes, nullableTime(m.SecondaryResetAt),
		m.Source, m.ErrorClass, boolToInt(m.Stale))
	if err == nil && s.Logger != nil {
		s.Logger.LogAttrs(ctx, slog.LevelInfo, "subscription usage snapshot recorded",
			slog.String("event", "subscription_usage_recorded"),
			slog.String("provider_instance", m.ProviderInstanceID),
			slog.Int64("credential_id", m.CredentialID),
			slog.String("limit_id", m.LimitID),
			slog.String("error_class", m.ErrorClass),
		)
	}
	return err
}

func (s *Store) LatestSubscriptionUsageSnapshots(ctx context.Context) ([]metadata.SubscriptionUsageSnapshot, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT observed_at, provider_instance_id, COALESCE(credential_id, 0),
			account_display_label, plan_label, limit_id, limit_name, plan_type,
			reached_type, primary_used_percent, primary_window_minutes,
			primary_reset_at, secondary_used_percent, secondary_window_minutes,
			secondary_reset_at, source, error_class, stale
		FROM subscription_usage_snapshots
		ORDER BY provider_instance_id ASC, COALESCE(credential_id, 0) ASC, limit_id ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []metadata.SubscriptionUsageSnapshot
	for rows.Next() {
		var row metadata.SubscriptionUsageSnapshot
		var observed string
		var primaryReset, secondaryReset sql.NullString
		var stale int
		if err := rows.Scan(&observed, &row.ProviderInstanceID, &row.CredentialID,
			&row.AccountDisplayLabel, &row.PlanLabel, &row.LimitID, &row.LimitName,
			&row.PlanType, &row.ReachedType, &row.PrimaryUsedPercent,
			&row.PrimaryWindowMinutes, &primaryReset, &row.SecondaryUsedPercent,
			&row.SecondaryWindowMinutes, &secondaryReset, &row.Source,
			&row.ErrorClass, &stale); err != nil {
			return nil, err
		}
		observedAt, err := parseSQLiteTime(observed)
		if err != nil {
			return nil, err
		}
		row.ObservedAt = observedAt
		if primaryReset.Valid && primaryReset.String != "" {
			parsed, err := parseSQLiteTime(primaryReset.String)
			if err != nil {
				return nil, err
			}
			row.PrimaryResetAt = &parsed
		}
		if secondaryReset.Valid && secondaryReset.String != "" {
			parsed, err := parseSQLiteTime(secondaryReset.String)
			if err != nil {
				return nil, err
			}
			row.SecondaryResetAt = &parsed
		}
		row.Stale = stale != 0
		out = append(out, row)
	}
	return out, rows.Err()
}
