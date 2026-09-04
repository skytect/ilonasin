package app

import (
	"context"
	"errors"
	"strings"

	"ilonasin/internal/metadata"
	"ilonasin/internal/openai"
	"ilonasin/internal/provider"
)

type keepaliveProviderRegistry interface {
	List() []keepaliveProvider
}

type keepaliveProvider struct {
	ID             string
	Type           string
	BaseURL        string
	AuthIssuer     string
	AuthStyle      string
	APIKey         bool
	OAuth          bool
	OAuthRefresh   bool
	Chat           bool
	ModelDiscovery bool
}

func supportsCodexOAuthKeepalive(instance keepaliveProvider) bool {
	return metadata.SupportsCodexOAuth(instance.Type, instance.OAuth)
}

type keepaliveCredential struct {
	ID                      int64
	ProviderInstanceID      string
	BearerToken             string
	ChatGPTAccountID        string
	ChatGPTAccountIsFedRAMP bool
}

type keepaliveChatRequest struct {
	Provider      keepaliveProvider
	UpstreamModel string
	Request       openai.ChatCompletionRequest
	Credential    keepaliveCredential
}

type keepaliveChatResult struct {
	StatusCode int
	ErrorClass string
	Usage      openai.Usage
}

type keepaliveChatClient interface {
	CompleteKeepaliveChat(ctx context.Context, req keepaliveChatRequest) (keepaliveChatResult, error)
}

type keepaliveUsageClient interface {
	RefreshKeepaliveUsage(ctx context.Context, provider keepaliveProvider, credential keepaliveCredential) error
}

type keepaliveProviderRegistryAdapter struct {
	registry provider.Registry
}

func (a keepaliveProviderRegistryAdapter) List() []keepaliveProvider {
	rows := a.registry.List()
	out := make([]keepaliveProvider, 0, len(rows))
	for _, row := range rows {
		out = append(out, keepaliveProviderFromProvider(row))
	}
	return out
}

type keepaliveChatAdapter struct {
	adapter provider.ChatAdapter
	models  provider.ModelDiscoverer
}

func (a keepaliveChatAdapter) CompleteKeepaliveChat(ctx context.Context, req keepaliveChatRequest) (keepaliveChatResult, error) {
	catalog, err := a.models.ListModels(ctx, provider.ModelRequest{
		Instance:   providerInstanceFromKeepalive(req.Provider),
		Credential: providerBearerCredentialFromKeepalive(req.Credential),
	})
	if err != nil {
		return keepaliveChatResult{StatusCode: catalog.StatusCode, ErrorClass: firstNonEmpty(catalog.ErrorClass, "model_discovery_failed")}, err
	}
	model, err := selectKeepaliveModel(catalog.Models, req.UpstreamModel)
	if err != nil {
		return keepaliveChatResult{ErrorClass: "model_unavailable"}, err
	}
	req.Request.Model = model
	result, err := a.adapter.CompleteChat(ctx, provider.ChatRequest{
		Instance:      providerInstanceFromKeepalive(req.Provider),
		UpstreamModel: model,
		Request:       req.Request,
		Credential:    providerChatCredentialFromKeepalive(req.Credential),
	})
	return keepaliveChatResult{
		StatusCode: result.StatusCode,
		ErrorClass: result.ErrorClass,
		Usage:      result.Usage,
	}, err
}

func selectKeepaliveModel(models []provider.ModelMetadata, configured string) (string, error) {
	configured = strings.TrimSpace(configured)
	var selected *provider.ModelMetadata
	for i := range models {
		model := &models[i]
		if _, selector := provider.AccountSelectorFromModelID(model.ModelID); selector {
			continue
		}
		if configured != "" {
			if model.ModelID == configured {
				return model.ModelID, nil
			}
			continue
		}
		if model.ModelID == "" || model.Codex == nil || model.Codex.Visibility != "list" {
			continue
		}
		// Follow the provider's model-picker priority, with a stable tie break.
		if selected == nil || model.Codex.Priority < selected.Codex.Priority ||
			(model.Codex.Priority == selected.Codex.Priority && model.ModelID < selected.ModelID) {
			selected = model
		}
	}
	if selected != nil {
		return selected.ModelID, nil
	}
	return "", errors.New("no advertised keepalive model available")
}

type keepaliveUsageAdapter struct {
	client provider.CodexSubscriptionUsageClient
}

func (a keepaliveUsageAdapter) RefreshKeepaliveUsage(ctx context.Context, row keepaliveProvider, credential keepaliveCredential) error {
	if a.client == nil {
		return nil
	}
	_, _ = a.client.FetchCodexSubscriptionUsage(ctx, provider.CodexSubscriptionUsageRequest{
		Instance:   providerInstanceFromKeepalive(row),
		Credential: providerBearerCredentialFromKeepalive(credential),
	})
	return nil
}

func keepaliveProviderFromProvider(row provider.Instance) keepaliveProvider {
	return keepaliveProvider{
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

func providerInstanceFromKeepalive(row keepaliveProvider) provider.Instance {
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

func providerBearerCredentialFromKeepalive(row keepaliveCredential) provider.BearerCredential {
	return provider.BearerCredential{
		ID:                      row.ID,
		ProviderInstanceID:      row.ProviderInstanceID,
		Kind:                    provider.CredentialKindOAuthAccess,
		BearerToken:             row.BearerToken,
		ChatGPTAccountID:        row.ChatGPTAccountID,
		ChatGPTAccountIsFedRAMP: row.ChatGPTAccountIsFedRAMP,
	}
}

func providerChatCredentialFromKeepalive(row keepaliveCredential) provider.ChatCredential {
	return provider.ChatCredential{
		ID:                      row.ID,
		ProviderInstanceID:      row.ProviderInstanceID,
		Kind:                    provider.CredentialKindOAuthAccess,
		BearerToken:             row.BearerToken,
		ChatGPTAccountID:        row.ChatGPTAccountID,
		ChatGPTAccountIsFedRAMP: row.ChatGPTAccountIsFedRAMP,
	}
}

func keepaliveProviderRegistryFromProvider(registry provider.Registry) keepaliveProviderRegistryAdapter {
	return keepaliveProviderRegistryAdapter{registry: registry}
}

func keepaliveChatClientFromProvider(adapter provider.ChatAdapter, models provider.ModelDiscoverer) keepaliveChatClient {
	if adapter == nil || models == nil {
		return nil
	}
	return keepaliveChatAdapter{adapter: adapter, models: models}
}

func keepaliveUsageClientFromProvider(client provider.CodexSubscriptionUsageClient) keepaliveUsageClient {
	if client == nil {
		return nil
	}
	return keepaliveUsageAdapter{client: client}
}
