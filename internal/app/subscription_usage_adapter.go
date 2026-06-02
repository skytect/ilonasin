package app

import (
	"context"
	"errors"

	"ilonasin/internal/management"
	"ilonasin/internal/provider"
)

var errNilSubscriptionUsageClient = errors.New("subscription usage client is unavailable")

type subscriptionUsageProviderAdapter struct {
	client provider.CodexSubscriptionUsageClient
}

func (a subscriptionUsageProviderAdapter) FetchSubscriptionUsage(ctx context.Context, req management.SubscriptionUsageFetchRequest) (management.SubscriptionUsageFetchResult, error) {
	if a.client == nil {
		return management.SubscriptionUsageFetchResult{}, errNilSubscriptionUsageClient
	}
	result, err := a.client.FetchCodexSubscriptionUsage(ctx, provider.CodexSubscriptionUsageRequest{
		Instance:   providerInstanceFromSubscriptionUsage(req.Provider),
		Credential: providerBearerCredentialFromSubscriptionUsage(req.Credential),
	})
	return subscriptionUsageResultFromProvider(result), err
}

func providerInstanceFromSubscriptionUsage(row management.SubscriptionUsageProvider) provider.Instance {
	return provider.Instance{
		ID:             row.ID,
		Type:           row.Type,
		BaseURL:        row.BaseURL,
		AuthIssuer:     row.AuthIssuer,
		AuthStyle:      row.AuthStyle,
		APIKey:         row.APIKey,
		OAuth:          row.OAuth,
		OAuthRefresh:   row.OAuthRefresh,
		Chat:           row.Chat,
		ModelDiscovery: row.ModelDiscovery,
	}
}

func providerBearerCredentialFromSubscriptionUsage(row management.SubscriptionUsageBearerCredential) provider.BearerCredential {
	return provider.BearerCredential{
		ID:                      row.ID,
		ProviderInstanceID:      row.ProviderInstanceID,
		Kind:                    provider.CredentialKindOAuthAccess,
		BearerToken:             row.BearerToken,
		ChatGPTAccountID:        row.ChatGPTAccountID,
		ChatGPTAccountIsFedRAMP: row.ChatGPTAccountIsFedRAMP,
	}
}

func subscriptionUsageResultFromProvider(result provider.CodexSubscriptionUsageResult) management.SubscriptionUsageFetchResult {
	out := management.SubscriptionUsageFetchResult{
		ErrorClass: result.ErrorClass,
		StatusCode: result.StatusCode,
	}
	out.Snapshots = make([]management.SubscriptionUsageFetchSnapshot, 0, len(result.Snapshots))
	for _, snapshot := range result.Snapshots {
		out.Snapshots = append(out.Snapshots, management.SubscriptionUsageFetchSnapshot{
			LimitID:     snapshot.LimitID,
			LimitName:   snapshot.LimitName,
			PlanType:    snapshot.PlanType,
			ReachedType: snapshot.ReachedType,
			Primary:     subscriptionUsageWindowFromProvider(snapshot.Primary),
			Secondary:   subscriptionUsageWindowFromProvider(snapshot.Secondary),
		})
	}
	return out
}

func subscriptionUsageWindowFromProvider(window *provider.CodexRateLimitWindow) *management.SubscriptionUsageFetchWindow {
	if window == nil {
		return nil
	}
	return &management.SubscriptionUsageFetchWindow{
		UsedPercent:   window.UsedPercent,
		WindowMinutes: window.WindowMinutes,
		ResetsAt:      window.ResetsAt,
	}
}
