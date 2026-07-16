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
			ProviderInstanceID:       row.ProviderInstanceID,
			ModelID:                  row.ModelID,
			DisplayName:              row.DisplayName,
			CapabilityFlags:          row.CapabilityFlags,
			ContextLength:            row.ContextLength,
			MaxContextWindow:         row.MaxContextWindow,
			DefaultReasoningLevel:    row.DefaultReasoningLevel,
			SupportedReasoningLevels: openAIModelReasoningLevelsFromProvider(row.SupportedReasoningLevels),
			DefaultServiceTier:       row.DefaultServiceTier,
			ServiceTiers:             openAIModelServiceTiersFromProvider(row.ServiceTiers),
			InputModalities:          row.InputModalities,
			Codex:                    openAICodexModelMetadataFromProvider(row.Codex),
		})
	}
	return out
}

func openAICodexModelMetadataFromProvider(row *provider.CodexModelMetadata) *openai.CodexModelMetadata {
	if row == nil {
		return nil
	}
	return &openai.CodexModelMetadata{
		ShellType:                  row.ShellType,
		Visibility:                 row.Visibility,
		SupportedInAPI:             row.SupportedInAPI,
		Priority:                   row.Priority,
		BaseInstructions:           row.BaseInstructions,
		SupportsReasoningSummaries: row.SupportsReasoningSummaries,
		SupportVerbosity:           row.SupportVerbosity,
		DefaultVerbosity:           row.DefaultVerbosity,
		ApplyPatchToolType:         row.ApplyPatchToolType,
		WebSearchToolType:          row.WebSearchToolType,
		TruncationPolicy: openai.ModelTruncationPolicy{
			Mode:  row.TruncationPolicy.Mode,
			Limit: row.TruncationPolicy.Limit,
		},
		ExperimentalSupportedTools: append([]string{}, row.ExperimentalSupportedTools...),
		SupportsSearchTool:         row.SupportsSearchTool,
		UseResponsesLite:           row.UseResponsesLite,
		ToolMode:                   row.ToolMode,
		MultiAgentVersion:          row.MultiAgentVersion,
	}
}

func openAIModelReasoningLevelsFromProvider(rows []provider.ModelReasoningLevel) []openai.CodexReasoning {
	out := make([]openai.CodexReasoning, 0, len(rows))
	for _, row := range rows {
		out = append(out, openai.CodexReasoning{
			Effort:      row.Effort,
			Description: row.Description,
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
