package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"time"

	"ilonasin/internal/metadata"
)

func (s *Store) UpsertSubscriptionUsageSnapshot(ctx context.Context, m metadata.SubscriptionUsageSnapshot) error {
	if m.ObservedAt.IsZero() {
		m.ObservedAt = time.Now()
	}
	detailsJSON, err := encodeBankedResetDetails(m.BankedResetInventory.Details)
	if err != nil {
		return err
	}
	_, err = s.DB.ExecContext(ctx, `
		INSERT INTO subscription_usage_snapshots(
			observed_at, provider_instance_id, credential_id, account_display_label,
			plan_label, limit_id, limit_name, plan_type, reached_type,
			primary_used_percent, primary_window_minutes, primary_reset_at,
			secondary_used_percent, secondary_window_minutes, secondary_reset_at,
			source, error_class, stale, banked_reset_available_count,
			banked_reset_details_available, banked_reset_detail_error_class,
			banked_reset_details_json
		) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(provider_instance_id, credential_id, limit_id) DO UPDATE SET
			observed_at = CASE WHEN excluded.stale = 1 THEN observed_at ELSE excluded.observed_at END,
			account_display_label = excluded.account_display_label,
			plan_label = excluded.plan_label,
			limit_name = excluded.limit_name,
			plan_type = excluded.plan_type,
			reached_type = excluded.reached_type,
			primary_used_percent = CASE WHEN excluded.stale = 1 THEN primary_used_percent ELSE excluded.primary_used_percent END,
			primary_window_minutes = CASE WHEN excluded.stale = 1 THEN primary_window_minutes ELSE excluded.primary_window_minutes END,
			primary_reset_at = CASE WHEN excluded.stale = 1 THEN primary_reset_at ELSE excluded.primary_reset_at END,
			secondary_used_percent = CASE WHEN excluded.stale = 1 THEN secondary_used_percent ELSE excluded.secondary_used_percent END,
			secondary_window_minutes = CASE WHEN excluded.stale = 1 THEN secondary_window_minutes ELSE excluded.secondary_window_minutes END,
			secondary_reset_at = CASE WHEN excluded.stale = 1 THEN secondary_reset_at ELSE excluded.secondary_reset_at END,
			source = CASE WHEN excluded.stale = 1 THEN source ELSE excluded.source END,
			error_class = excluded.error_class,
			stale = excluded.stale,
			banked_reset_available_count = CASE WHEN excluded.stale = 1 THEN banked_reset_available_count ELSE excluded.banked_reset_available_count END,
			banked_reset_details_available = CASE WHEN excluded.stale = 1 THEN banked_reset_details_available ELSE excluded.banked_reset_details_available END,
			banked_reset_detail_error_class = CASE WHEN excluded.stale = 1 THEN banked_reset_detail_error_class ELSE excluded.banked_reset_detail_error_class END,
			banked_reset_details_json = CASE WHEN excluded.stale = 1 THEN banked_reset_details_json ELSE excluded.banked_reset_details_json END
	`, m.ObservedAt.UTC().Format(time.RFC3339Nano), m.ProviderInstanceID, nullableInt64(m.CredentialID),
		m.AccountDisplayLabel, m.PlanLabel, m.LimitID, m.LimitName, m.PlanType, m.ReachedType,
		m.PrimaryUsedPercent, m.PrimaryWindowMinutes, nullableTime(m.PrimaryResetAt),
		m.SecondaryUsedPercent, m.SecondaryWindowMinutes, nullableTime(m.SecondaryResetAt),
		m.Source, m.ErrorClass, boolToInt(m.Stale), nullableIntPtr(m.BankedResetInventory.AvailableCount),
		boolToInt(m.BankedResetInventory.DetailsAvailable), m.BankedResetInventory.DetailErrorClass,
		string(detailsJSON))
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
			secondary_reset_at, source, error_class, stale,
			banked_reset_available_count, banked_reset_details_available,
			banked_reset_detail_error_class, banked_reset_details_json
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
		var bankedAvailable sql.NullInt64
		var bankedDetailsJSON string
		var stale int
		var bankedDetailsAvailable int
		if err := rows.Scan(&observed, &row.ProviderInstanceID, &row.CredentialID,
			&row.AccountDisplayLabel, &row.PlanLabel, &row.LimitID, &row.LimitName,
			&row.PlanType, &row.ReachedType, &row.PrimaryUsedPercent,
			&row.PrimaryWindowMinutes, &primaryReset, &row.SecondaryUsedPercent,
			&row.SecondaryWindowMinutes, &secondaryReset, &row.Source,
			&row.ErrorClass, &stale, &bankedAvailable, &bankedDetailsAvailable,
			&row.BankedResetInventory.DetailErrorClass, &bankedDetailsJSON); err != nil {
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
		if bankedAvailable.Valid {
			available := int(bankedAvailable.Int64)
			row.BankedResetInventory.AvailableCount = &available
		}
		row.BankedResetInventory.DetailsAvailable = bankedDetailsAvailable != 0
		if bankedDetailsJSON != "" {
			details, err := decodeBankedResetDetails([]byte(bankedDetailsJSON))
			if err != nil {
				return nil, err
			}
			row.BankedResetInventory.Details = details
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func nullableIntPtr(v *int) any {
	if v == nil {
		return nil
	}
	return *v
}

func encodeBankedResetDetails(details []metadata.BankedResetDetail) ([]byte, error) {
	if details == nil {
		details = []metadata.BankedResetDetail{}
	}
	return json.Marshal(details)
}

func decodeBankedResetDetails(body []byte) ([]metadata.BankedResetDetail, error) {
	var details []metadata.BankedResetDetail
	if err := json.Unmarshal(body, &details); err != nil {
		return nil, err
	}
	if details == nil {
		details = []metadata.BankedResetDetail{}
	}
	return details, nil
}
