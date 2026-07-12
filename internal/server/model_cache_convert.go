package server

import (
	"ilonasin/internal/metadata"
	"ilonasin/internal/provider"
)

func modelCacheRowsFromProvider(rows []provider.ModelMetadata) []metadata.ModelCacheRow {
	out := make([]metadata.ModelCacheRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, metadata.NormalizeModelCacheRow(metadata.ModelCacheRow{
			ProviderInstanceID:       row.ProviderInstanceID,
			ModelID:                  row.ModelID,
			DisplayName:              row.DisplayName,
			CapabilityFlags:          row.CapabilityFlags,
			ContextLength:            row.ContextLength,
			MaxContextWindow:         row.MaxContextWindow,
			DefaultReasoningLevel:    row.DefaultReasoningLevel,
			SupportedReasoningLevels: modelReasoningLevelsFromProvider(row.SupportedReasoningLevels),
			DefaultServiceTier:       row.DefaultServiceTier,
			ServiceTiers:             modelServiceTiersFromProvider(row.ServiceTiers),
			InputModalities:          row.InputModalities,
			UpdatedAt:                row.UpdatedAt,
		}))
	}
	return out
}

func providerModelsFromCacheRows(rows []metadata.ModelCacheRow) []provider.ModelMetadata {
	out := make([]provider.ModelMetadata, 0, len(rows))
	for _, row := range rows {
		row = metadata.NormalizeModelCacheRow(row)
		out = append(out, provider.ModelMetadata{
			ProviderInstanceID:       row.ProviderInstanceID,
			ModelID:                  row.ModelID,
			DisplayName:              row.DisplayName,
			CapabilityFlags:          row.CapabilityFlags,
			ContextLength:            row.ContextLength,
			MaxContextWindow:         row.MaxContextWindow,
			DefaultReasoningLevel:    row.DefaultReasoningLevel,
			SupportedReasoningLevels: providerReasoningLevelsFromMetadata(row.SupportedReasoningLevels),
			DefaultServiceTier:       row.DefaultServiceTier,
			ServiceTiers:             providerServiceTiersFromMetadata(row.ServiceTiers),
			InputModalities:          row.InputModalities,
			UpdatedAt:                row.UpdatedAt,
		})
	}
	return out
}

func modelReasoningLevelsFromProvider(rows []provider.ModelReasoningLevel) []metadata.ModelReasoningLevel {
	out := make([]metadata.ModelReasoningLevel, 0, len(rows))
	for _, row := range rows {
		out = append(out, metadata.ModelReasoningLevel{
			Effort:      row.Effort,
			Description: row.Description,
		})
	}
	return metadata.NormalizeModelReasoningLevels(out)
}

func providerReasoningLevelsFromMetadata(rows []metadata.ModelReasoningLevel) []provider.ModelReasoningLevel {
	rows = metadata.NormalizeModelReasoningLevels(rows)
	out := make([]provider.ModelReasoningLevel, 0, len(rows))
	for _, row := range rows {
		out = append(out, provider.ModelReasoningLevel{
			Effort:      row.Effort,
			Description: row.Description,
		})
	}
	return out
}

func modelServiceTiersFromProvider(rows []provider.ModelServiceTier) []metadata.ModelServiceTier {
	out := make([]metadata.ModelServiceTier, 0, len(rows))
	for _, row := range rows {
		out = append(out, metadata.ModelServiceTier{
			ID:          row.ID,
			Name:        row.Name,
			Description: row.Description,
		})
	}
	return metadata.NormalizeModelServiceTiers(out)
}

func providerServiceTiersFromMetadata(rows []metadata.ModelServiceTier) []provider.ModelServiceTier {
	rows = metadata.NormalizeModelServiceTiers(rows)
	out := make([]provider.ModelServiceTier, 0, len(rows))
	for _, row := range rows {
		out = append(out, provider.ModelServiceTier{
			ID:          row.ID,
			Name:        row.Name,
			Description: row.Description,
		})
	}
	return out
}
