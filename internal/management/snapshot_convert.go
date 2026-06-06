package management

import (
	"ilonasin/internal/credentials"
	"ilonasin/internal/metadata"
)

func snapshotProviderInstances(rows []ProviderInstance) []ProviderInstance {
	out := make([]ProviderInstance, 0, len(rows))
	for _, row := range rows {
		row.BaseURL = safeBaseURL(row.BaseURL)
		out = append(out, row)
	}
	return out
}

func upstreamCredentialsFromCredentials(rows []credentials.UpstreamCredentialMetadata) []UpstreamCredential {
	out := make([]UpstreamCredential, 0, len(rows))
	for _, row := range rows {
		out = append(out, upstreamCredentialFromCredentials(row))
	}
	return out
}

func credentialPoolGroupsFromCredentials(rows []credentials.CredentialPoolGroupMetadata) []CredentialPoolGroup {
	out := make([]CredentialPoolGroup, 0, len(rows))
	for _, row := range rows {
		out = append(out, credentialPoolGroupFromCredentials(row))
	}
	return out
}

func oauthCredentialsFromCredentials(rows []credentials.OAuthCredentialMetadata) []OAuthCredential {
	out := make([]OAuthCredential, 0, len(rows))
	for _, row := range rows {
		out = append(out, OAuthCredential{
			ID:                        row.ID,
			ProviderInstanceID:        row.ProviderInstanceID,
			Label:                     row.Label,
			AccountDisplayLabel:       row.AccountDisplayLabel,
			PlanLabel:                 row.PlanLabel,
			Scopes:                    row.Scopes,
			ExpiresAt:                 row.ExpiresAt,
			LastRefreshAt:             row.LastRefreshAt,
			RefreshFailureClass:       row.RefreshFailureClass,
			RefreshFailureDescription: row.RefreshFailureDescription,
			CreatedAt:                 row.CreatedAt,
			DisabledAt:                row.DisabledAt,
			Disabled:                  row.Disabled,
		})
	}
	return out
}

func providerAccountsFromCredentials(rows []credentials.ProviderAccountMetadata) []ProviderAccount {
	out := make([]ProviderAccount, 0, len(rows))
	for _, row := range rows {
		out = append(out, ProviderAccount{
			ProviderInstanceID: row.ProviderInstanceID,
			CredentialID:       row.CredentialID,
			DisplayLabel:       row.DisplayLabel,
			PlanLabel:          row.PlanLabel,
			CreatedAt:          row.CreatedAt,
		})
	}
	return out
}

func modelMetadataFromMetadata(rows []metadata.ModelCacheRow) []ModelMetadata {
	out := make([]ModelMetadata, 0, len(rows))
	for _, row := range rows {
		out = append(out, ModelMetadata{
			ProviderInstanceID: row.ProviderInstanceID,
			ModelID:            row.ModelID,
			DisplayName:        row.DisplayName,
			Capabilities:       row.CapabilityFlags,
			UpdatedAt:          row.UpdatedAt,
		})
	}
	return out
}

func requestSummariesFromMetadata(rows []metadata.RequestSummary) []RequestSummary {
	out := make([]RequestSummary, 0, len(rows))
	for _, row := range rows {
		out = append(out, RequestSummary{
			ID:                             row.ID,
			StartedAt:                      row.StartedAt,
			ProviderInstanceID:             row.ProviderInstanceID,
			ModelID:                        row.ModelID,
			Endpoint:                       row.Endpoint,
			Stream:                         row.Stream,
			ProviderType:                   row.ProviderType,
			MessageCount:                   row.MessageCount,
			ToolCount:                      row.ToolCount,
			ImageCount:                     row.ImageCount,
			RequestedServiceTier:           row.RequestedServiceTier,
			EffectiveServiceTier:           row.EffectiveServiceTier,
			ReasoningEffort:                row.ReasoningEffort,
			ReasoningSummary:               row.ReasoningSummary,
			ReasoningMaxTokens:             row.ReasoningMaxTokens,
			ReasoningEnabled:               row.ReasoningEnabled,
			ReasoningExclude:               row.ReasoningExclude,
			ThinkingType:                   row.ThinkingType,
			MaxOutputTokens:                row.MaxOutputTokens,
			RequestedProviderID:            row.RequestedProviderID,
			RequestedModelID:               row.RequestedModelID,
			ResolvedProviderID:             row.ResolvedProviderID,
			ResolvedModelID:                row.ResolvedModelID,
			CredentialID:                   row.CredentialID,
			CredentialLabel:                row.CredentialLabel,
			HTTPStatus:                     row.HTTPStatus,
			ErrorClass:                     row.ErrorClass,
			RetryCount:                     row.RetryCount,
			AuthRetryCount:                 row.AuthRetryCount,
			AttemptCount:                   row.AttemptCount,
			FallbackCount:                  row.FallbackCount,
			FallbackReason:                 row.FallbackReason,
			PromptTokens:                   row.PromptTokens,
			CompletionTokens:               row.CompletionTokens,
			TotalTokens:                    row.TotalTokens,
			ReasoningTokens:                row.ReasoningTokens,
			CacheHitTokens:                 row.CacheHitTokens,
			CacheMissTokens:                row.CacheMissTokens,
			CacheWriteTokens:               row.CacheWriteTokens,
			ReasoningTokenRate:             row.ReasoningTokenRate,
			CacheHitRate:                   row.CacheHitRate,
			CacheMissRate:                  row.CacheMissRate,
			CacheWriteRate:                 row.CacheWriteRate,
			CostMicrounits:                 row.CostMicrounits,
			TotalLatencyMS:                 row.TotalLatencyMS,
			UpstreamLatencyMS:              row.UpstreamLatencyMS,
			TimeToFirstTokenMS:             row.TimeToFirstTokenMS,
			OutputTokensPerSecond:          row.OutputTokensPerSecond,
			OutputTokensPerSecondTotal:     row.OutputTokensPerSecondTotal,
			OutputTokensPerSecondAfterTTFT: row.OutputTokensPerSecondAfterTTFT,
			StreamCompletionStatus:         row.StreamCompletionStatus,
			StreamChunkCount:               row.StreamChunkCount,
		})
	}
	return out
}

func usageSummariesFromMetadata(rows []metadata.UsageSummary) []UsageSummary {
	out := make([]UsageSummary, 0, len(rows))
	for _, row := range rows {
		out = append(out, UsageSummary{
			ProviderInstanceID: row.ProviderInstanceID,
			RequestCount:       row.RequestCount,
			PromptTokens:       row.PromptTokens,
			CompletionTokens:   row.CompletionTokens,
			TotalTokens:        row.TotalTokens,
			ReasoningTokens:    row.ReasoningTokens,
			CacheHitTokens:     row.CacheHitTokens,
			CacheMissTokens:    row.CacheMissTokens,
			CacheWriteTokens:   row.CacheWriteTokens,
			ReasoningTokenRate: row.ReasoningTokenRate,
			CacheHitRate:       row.CacheHitRate,
			CacheMissRate:      row.CacheMissRate,
			CacheWriteRate:     row.CacheWriteRate,
			CostMicrounits:     row.CostMicrounits,
		})
	}
	return out
}

func localTokenUsageSummariesFromMetadata(rows []metadata.LocalTokenUsageSummary) []LocalTokenUsageSummary {
	out := make([]LocalTokenUsageSummary, 0, len(rows))
	for _, row := range rows {
		out = append(out, LocalTokenUsageSummary{
			LocalTokenID:       row.LocalTokenID,
			RequestCount:       row.RequestCount,
			OKCount:            row.OKCount,
			WarningCount:       row.WarningCount,
			ErrorCount:         row.ErrorCount,
			PromptTokens:       row.PromptTokens,
			CompletionTokens:   row.CompletionTokens,
			TotalTokens:        row.TotalTokens,
			ReasoningTokens:    row.ReasoningTokens,
			CacheHitTokens:     row.CacheHitTokens,
			CacheMissTokens:    row.CacheMissTokens,
			CacheWriteTokens:   row.CacheWriteTokens,
			ReasoningTokenRate: row.ReasoningTokenRate,
			CacheHitRate:       row.CacheHitRate,
			CacheMissRate:      row.CacheMissRate,
			CacheWriteRate:     row.CacheWriteRate,
			AverageLatencyMS:   row.AverageLatencyMS,
			LatestRequestAt:    row.LatestRequestAt,
		})
	}
	return out
}

func latencySummariesFromMetadata(rows []metadata.LatencySummary) []LatencySummary {
	out := make([]LatencySummary, 0, len(rows))
	for _, row := range rows {
		out = append(out, LatencySummary{
			ProviderInstanceID:        row.ProviderInstanceID,
			RequestCount:              row.RequestCount,
			AverageLatencyMS:          row.AverageLatencyMS,
			AverageUpstreamLatencyMS:  row.AverageUpstreamLatencyMS,
			AverageTimeToFirstTokenMS: row.AverageTimeToFirstTokenMS,
			AverageOutputTPS:          row.AverageOutputTPS,
			AverageOutputTPSTotal:     row.AverageOutputTPSTotal,
			AverageOutputTPSAfterTTFT: row.AverageOutputTPSAfterTTFT,
		})
	}
	return out
}

func streamSummariesFromMetadata(rows []metadata.StreamSummary) []StreamSummary {
	out := make([]StreamSummary, 0, len(rows))
	for _, row := range rows {
		out = append(out, StreamSummary{
			CompletionStatus: row.CompletionStatus,
			StreamCount:      row.StreamCount,
			ChunkCount:       row.ChunkCount,
		})
	}
	return out
}

func healthSummariesFromMetadata(rows []metadata.HealthSummary) []HealthSummary {
	out := make([]HealthSummary, 0, len(rows))
	for _, row := range rows {
		out = append(out, HealthSummary{
			ProviderInstanceID: row.ProviderInstanceID,
			ModelID:            row.ModelID,
			CredentialID:       row.CredentialID,
			CredentialLabel:    row.CredentialLabel,
			EventClass:         row.EventClass,
			HTTPStatus:         row.HTTPStatus,
			ErrorClass:         row.ErrorClass,
			OccurredAt:         row.OccurredAt,
			RetryAfter:         row.RetryAfter,
		})
	}
	return out
}

func fallbackSummariesFromMetadata(rows []metadata.FallbackSummary) []FallbackSummary {
	out := make([]FallbackSummary, 0, len(rows))
	for _, row := range rows {
		out = append(out, FallbackSummary{
			ID:                  row.ID,
			RequestMetadataID:   row.RequestMetadataID,
			OccurredAt:          row.OccurredAt,
			ProviderInstanceID:  row.ProviderInstanceID,
			ModelID:             row.ModelID,
			FromCredentialID:    row.FromCredentialID,
			FromCredentialLabel: row.FromCredentialLabel,
			ToCredentialID:      row.ToCredentialID,
			ToCredentialLabel:   row.ToCredentialLabel,
			Reason:              row.Reason,
		})
	}
	return out
}

func quotaSummariesFromMetadata(rows []metadata.QuotaSummary) []QuotaSummary {
	out := make([]QuotaSummary, 0, len(rows))
	for _, row := range rows {
		out = append(out, QuotaSummary{
			ObservedAt:         row.ObservedAt,
			ProviderInstanceID: row.ProviderInstanceID,
			ModelID:            row.ModelID,
			CredentialID:       row.CredentialID,
			CredentialLabel:    row.CredentialLabel,
			Source:             row.Source,
			HTTPStatus:         row.HTTPStatus,
			ErrorClass:         row.ErrorClass,
			RetryAfter:         row.RetryAfter,
			ResetAt:            row.ResetAt,
			Count:              row.Count,
		})
	}
	return out
}
