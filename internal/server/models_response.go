package server

import (
	"ilonasin/internal/openai"
	"ilonasin/internal/provider"
)

func modelsResponseFromMetadata(rows []provider.ModelMetadata) openai.ModelsResponse {
	return openai.ModelsResponseFromMetadata(openAIModelMetadataFromProvider(rows))
}

func openAIModelMetadataFromProvider(rows []provider.ModelMetadata) []openai.ModelMetadata {
	out := make([]openai.ModelMetadata, 0, len(rows))
	for _, row := range rows {
		out = append(out, openai.ModelMetadata{
			ProviderInstanceID: row.ProviderInstanceID,
			ModelID:            row.ModelID,
			DisplayName:        row.DisplayName,
			CapabilityFlags:    row.CapabilityFlags,
			ContextLength:      row.ContextLength,
			DefaultServiceTier: row.DefaultServiceTier,
			ServiceTiers:       openAIModelServiceTiersFromProvider(row.ServiceTiers),
			InputModalities:    row.InputModalities,
		})
	}
	return out
}

func openAIModelServiceTiersFromProvider(rows []provider.ModelServiceTier) []openai.ModelServiceTier {
	out := make([]openai.ModelServiceTier, 0, len(rows))
	for _, row := range rows {
		out = append(out, openai.ModelServiceTier{
			ID:          row.ID,
			Name:        row.Name,
			Description: row.Description,
		})
	}
	return out
}
