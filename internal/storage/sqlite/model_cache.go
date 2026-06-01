package sqlite

import (
	"context"
	"fmt"
	"time"

	"ilonasin/internal/provider"
)

func (s *Store) ReplaceModelCache(ctx context.Context, providerInstanceID string, models []provider.ModelMetadata) error {
	if len(models) == 0 {
		return fmt.Errorf("model cache replacement requires at least one model")
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM model_cache WHERE provider_instance_id = ?`, providerInstanceID); err != nil {
		return err
	}
	for _, model := range models {
		if model.ProviderInstanceID != providerInstanceID {
			return fmt.Errorf("model cache provider mismatch")
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO model_cache(
				provider_instance_id, model_id, display_name, capability_flags,
				context_length, updated_at
			) VALUES(?, ?, ?, ?, ?, ?)
		`, model.ProviderInstanceID, model.ModelID, model.DisplayName, model.CapabilityFlags,
			nullableInt(model.ContextLength), model.UpdatedAt.UTC().Format(time.RFC3339Nano)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) ListModelCache(ctx context.Context) ([]provider.ModelMetadata, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT provider_instance_id, model_id, display_name, capability_flags,
			COALESCE(context_length, 0), updated_at
		FROM model_cache
		ORDER BY provider_instance_id ASC, model_id ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []provider.ModelMetadata
	for rows.Next() {
		var model provider.ModelMetadata
		var updated string
		if err := rows.Scan(&model.ProviderInstanceID, &model.ModelID, &model.DisplayName,
			&model.CapabilityFlags, &model.ContextLength, &updated); err != nil {
			return nil, err
		}
		updatedAt, err := parseSQLiteTime(updated)
		if err != nil {
			return nil, err
		}
		model.UpdatedAt = updatedAt
		out = append(out, model)
	}
	return out, rows.Err()
}
