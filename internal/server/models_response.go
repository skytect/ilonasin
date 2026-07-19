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
			ContextLength:            cloneInt64Pointer(row.ContextLength),
			MaxContextWindow:         cloneInt64Pointer(row.MaxContextWindow),
			DefaultReasoningLevel:    cloneStringPointer(row.DefaultReasoningLevel),
			SupportedReasoningLevels: openAIModelReasoningLevelsFromProvider(row.SupportedReasoningLevels),
			DefaultServiceTier:       cloneStringPointer(row.DefaultServiceTier),
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
		Description:                cloneStringPointer(row.Description),
		AdditionalSpeedTiers:       append([]string{}, row.AdditionalSpeedTiers...),
		AvailabilityNUX:            openAICodexAvailabilityNUXFromProvider(row.AvailabilityNUX),
		Upgrade:                    openAICodexUpgradeFromProvider(row.Upgrade),
		BaseInstructions:           row.BaseInstructions,
		ModelMessages:              openAICodexModelMessagesFromProvider(row.ModelMessages),
		IncludeSkillsInstructions:  row.IncludeSkillsInstructions,
		SupportsReasoningSummaries: row.SupportsReasoningSummaries,
		DefaultReasoningSummary:    row.DefaultReasoningSummary,
		SupportVerbosity:           row.SupportVerbosity,
		DefaultVerbosity:           cloneStringPointer(row.DefaultVerbosity),
		ApplyPatchToolType:         cloneStringPointer(row.ApplyPatchToolType),
		WebSearchToolType:          row.WebSearchToolType,
		TruncationPolicy: openai.ModelTruncationPolicy{
			Mode:  row.TruncationPolicy.Mode,
			Limit: row.TruncationPolicy.Limit,
		},
		ExperimentalSupportedTools: append([]string{}, row.ExperimentalSupportedTools...),
		SupportsImageDetailOrig:    row.SupportsImageDetailOrig,
		AutoCompactTokenLimit:      cloneInt64Pointer(row.AutoCompactTokenLimit),
		CompHash:                   cloneStringPointer(row.CompHash),
		EffectiveContextWindowPct:  row.EffectiveContextWindowPct,
		SupportsSearchTool:         row.SupportsSearchTool,
		UseResponsesLite:           row.UseResponsesLite,
		AutoReviewModelOverride:    cloneStringPointer(row.AutoReviewModelOverride),
		ToolMode:                   cloneStringPointer(row.ToolMode),
		MultiAgentVersion:          cloneStringPointer(row.MultiAgentVersion),
	}
}

func openAICodexAvailabilityNUXFromProvider(row *provider.CodexModelAvailabilityNUX) *openai.CodexModelAvailabilityNUX {
	if row == nil {
		return nil
	}
	return &openai.CodexModelAvailabilityNUX{Message: row.Message}
}

func openAICodexUpgradeFromProvider(row *provider.CodexModelUpgrade) *openai.CodexModelUpgrade {
	if row == nil {
		return nil
	}
	return &openai.CodexModelUpgrade{Model: row.Model, MigrationMarkdown: row.MigrationMarkdown}
}

func openAICodexModelMessagesFromProvider(row *provider.CodexModelMessages) *openai.CodexModelMessages {
	if row == nil {
		return nil
	}
	out := &openai.CodexModelMessages{
		InstructionsTemplate: cloneStringPointer(row.InstructionsTemplate),
	}
	if row.InstructionsVariables != nil {
		out.InstructionsVariables = &openai.CodexModelInstructionVariables{
			PersonalityDefault:   cloneStringPointer(row.InstructionsVariables.PersonalityDefault),
			PersonalityFriendly:  cloneStringPointer(row.InstructionsVariables.PersonalityFriendly),
			PersonalityPragmatic: cloneStringPointer(row.InstructionsVariables.PersonalityPragmatic),
		}
	}
	if row.Approvals != nil {
		out.Approvals = &openai.CodexModelApprovalMessages{
			OnRequest:           cloneStringPointer(row.Approvals.OnRequest),
			OnRequestAutoReview: cloneStringPointer(row.Approvals.OnRequestAutoReview),
		}
	}
	return out
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
