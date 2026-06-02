package sqlite

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"ilonasin/internal/metadata"
)

func (s *Store) ReplaceModelCache(ctx context.Context, providerInstanceID string, models []metadata.ModelCacheRow) error {
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
		model = metadata.NormalizeModelCacheRow(model)
		if model.ProviderInstanceID != providerInstanceID {
			return fmt.Errorf("model cache provider mismatch")
		}
		serviceTiers, err := encodeModelCacheServiceTiers(model.ServiceTiers)
		if err != nil {
			return err
		}
		inputModalities, err := encodeModelCacheInputModalities(model.InputModalities)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO model_cache(
				provider_instance_id, model_id, display_name, capability_flags,
				context_length, default_service_tier, service_tiers_json,
				input_modalities_json, updated_at
			) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, model.ProviderInstanceID, model.ModelID, model.DisplayName, model.CapabilityFlags,
			nullableInt(model.ContextLength), model.DefaultServiceTier, serviceTiers,
			inputModalities, model.UpdatedAt.UTC().Format(time.RFC3339Nano)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) ListModelCache(ctx context.Context) ([]metadata.ModelCacheRow, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT provider_instance_id, model_id, display_name, capability_flags,
			COALESCE(context_length, 0), default_service_tier, service_tiers_json,
			input_modalities_json, updated_at
		FROM model_cache
		ORDER BY provider_instance_id ASC, model_id ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []metadata.ModelCacheRow
	for rows.Next() {
		var model metadata.ModelCacheRow
		var updated, serviceTiersJSON, inputModalitiesJSON string
		if err := rows.Scan(&model.ProviderInstanceID, &model.ModelID, &model.DisplayName,
			&model.CapabilityFlags, &model.ContextLength, &model.DefaultServiceTier,
			&serviceTiersJSON, &inputModalitiesJSON, &updated); err != nil {
			return nil, err
		}
		updatedAt, err := parseSQLiteTime(updated)
		if err != nil {
			return nil, err
		}
		model.UpdatedAt = updatedAt
		model.ServiceTiers = decodeModelCacheServiceTiers(serviceTiersJSON)
		model.InputModalities = decodeModelCacheInputModalities(inputModalitiesJSON)
		out = append(out, metadata.NormalizeModelCacheRow(model))
	}
	return out, rows.Err()
}

func encodeModelCacheServiceTiers(values []metadata.ModelServiceTier) (string, error) {
	values = metadata.NormalizeModelServiceTiers(values)
	if len(values) == 0 {
		return "[]", nil
	}
	data, err := json.Marshal(values)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func encodeModelCacheInputModalities(values []string) (string, error) {
	values = metadata.NormalizeModelInputModalities(values)
	if len(values) == 0 {
		return "[]", nil
	}
	data, err := json.Marshal(values)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func decodeModelCacheServiceTiers(value string) []metadata.ModelServiceTier {
	var out []metadata.ModelServiceTier
	if err := json.Unmarshal([]byte(value), &out); err != nil {
		return nil
	}
	return metadata.NormalizeModelServiceTiers(out)
}

func decodeModelCacheInputModalities(value string) []string {
	var out []string
	if err := json.Unmarshal([]byte(value), &out); err != nil {
		return nil
	}
	return metadata.NormalizeModelInputModalities(out)
}
