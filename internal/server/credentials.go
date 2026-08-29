package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"ilonasin/internal/credentials"
	"ilonasin/internal/provider"
)

func (s *Server) resolveModelCredentials(ctx context.Context, instance provider.Instance) ([]provider.BearerCredential, error) {
	if instance.APIKey {
		credentialsSet, err := s.upstreams.ResolveAPIKeys(ctx, instance.ID)
		if err != nil {
			return nil, err
		}
		out := make([]provider.BearerCredential, 0, len(credentialsSet))
		for _, credential := range credentialsSet {
			out = append(out, provider.BearerCredential{
				ID:                 credential.ID,
				ProviderInstanceID: credential.ProviderInstanceID,
				Kind:               provider.CredentialKindAPIKey,
				BearerToken:        credential.APIKey,
			})
		}
		return out, nil
	}
	if instance.OAuth {
		if s.oauth == nil {
			return nil, credentials.ErrNoEligibleCredential
		}
		credentialsSet, err := s.oauth.ResolveOAuthBearers(ctx, instance.ID, s.now().UTC())
		if s.canRefreshCodexOAuth(instance) && errors.Is(err, credentials.ErrNoEligibleCredential) {
			if refreshErr := s.refresh.RefreshOAuthProviderCredential(ctx, instance.ID); refreshErr == nil {
				credentialsSet, err = s.oauth.ResolveOAuthBearers(ctx, instance.ID, s.now().UTC())
				if err != nil {
					return nil, fmt.Errorf("%w: oauth refresh did not yield bearer", credentials.ErrOAuthRefreshFailed)
				}
			} else {
				return nil, fmt.Errorf("%w: oauth refresh unavailable", credentials.ErrOAuthRefreshFailed)
			}
		}
		if err != nil {
			return nil, err
		}
		out := make([]provider.BearerCredential, 0, len(credentialsSet))
		for _, credential := range credentialsSet {
			out = append(out, provider.BearerCredential{
				ID:                      credential.ID,
				ProviderInstanceID:      credential.ProviderInstanceID,
				Kind:                    provider.CredentialKindOAuthAccess,
				BearerToken:             credential.BearerToken,
				ChatGPTAccountID:        credential.ChatGPTAccountID,
				ChatGPTAccountIsFedRAMP: credential.ChatGPTAccountIsFedRAMP,
			})
		}
		return out, nil
	}
	return nil, credentials.ErrNoEligibleCredential
}

func (s *Server) resolveModelCredentialsForModel(ctx context.Context, instance provider.Instance, modelID string) ([]provider.BearerCredential, error) {
	credentialsSet, err := s.resolveModelCredentials(ctx, instance)
	if err != nil {
		return nil, err
	}
	discoverer, scope, discovererAvailable := s.modelAvailabilityScope(instance, modelID)
	if scope != provider.ModelAvailabilityCredentialCatalog {
		return credentialsSet, nil
	}
	if !discovererAvailable {
		return nil, credentials.ErrNoEligibleCredential
	}

	s.refreshMissingCredentialModelCatalogs(ctx, instance, discoverer, credentialsSet)
	eligible := make([]provider.BearerCredential, 0, len(credentialsSet))
	for _, credential := range credentialsSet {
		catalog, known, cached := s.credentialModelCatalogs.lookup(s.now().UTC(), instance.ID, credential.ID)
		if !cached || !known {
			continue
		}
		if _, advertised := catalog[modelID]; advertised {
			eligible = append(eligible, credential)
		}
	}
	if len(eligible) == 0 {
		return nil, credentials.ErrNoEligibleCredential
	}
	return eligible, nil
}

func (s *Server) modelAvailabilityScope(instance provider.Instance, modelID string) (provider.ModelDiscoverer, provider.ModelAvailabilityScope, bool) {
	scope := provider.BuiltInModelAvailabilityScope(instance, modelID)
	if s == nil || s.models == nil {
		return nil, scope, false
	}
	discoverer, ok := s.models.ForProvider(instance.Type)
	if !ok {
		return nil, scope, false
	}
	if policy, ok := discoverer.(provider.ModelAvailabilityPolicy); ok &&
		policy.ModelAvailabilityScope(instance, modelID) == provider.ModelAvailabilityCredentialCatalog {
		scope = provider.ModelAvailabilityCredentialCatalog
	}
	return discoverer, scope, true
}

func (s *Server) refreshMissingCredentialModelCatalogs(ctx context.Context, instance provider.Instance, discoverer provider.ModelDiscoverer, credentialsSet []provider.BearerCredential) {
	now := s.now().UTC()
	missing := make([]provider.BearerCredential, 0, len(credentialsSet))
	for _, credential := range credentialsSet {
		if _, _, cached := s.credentialModelCatalogs.lookup(now, instance.ID, credential.ID); !cached {
			missing = append(missing, credential)
		}
	}
	if len(missing) == 0 {
		return
	}
	type result struct {
		credential provider.BearerCredential
		models     []provider.ModelMetadata
		live       bool
	}
	results := make(chan result, len(missing))
	for _, credential := range missing {
		go func() {
			models, live := s.discoverModelsWithCredential(ctx, instance, discoverer, credential)
			results <- result{credential: credential, models: models, live: live}
		}()
	}
	for range missing {
		result := <-results
		observedAt := s.now().UTC()
		if !result.live {
			s.credentialModelCatalogs.fail(observedAt, instance.ID, result.credential.ID)
			continue
		}
		s.credentialModelCatalogs.put(observedAt, instance.ID, result.credential.ID, providerModelIDs(result.models))
	}
}

func providerModelIDs(models []provider.ModelMetadata) []string {
	ids := make([]string, 0, len(models))
	for _, model := range models {
		ids = append(ids, model.ModelID)
	}
	return ids
}

func authRetryableChatAttempt(result provider.ChatResult) bool {
	return result.StatusCode == http.StatusBadGateway && result.ErrorClass == "upstream_auth_failed"
}

func authRetryableStreamAttempt(summary provider.ChatStreamSummary, sinkStarted bool) bool {
	return !sinkStarted && !summary.Started && summary.PreStreamError && summary.StatusCode == http.StatusBadGateway && summary.ErrorClass == "upstream_auth_failed"
}

func (s *Server) refreshOAuthCredentialForRetryIfBearer(ctx context.Context, credential provider.BearerCredential) (provider.BearerCredential, error) {
	if s.refresh == nil {
		return provider.BearerCredential{}, credentials.ErrNoEligibleCredential
	}
	if err := s.refresh.RefreshOAuthCredentialIfBearer(ctx, credential.ID, credential.BearerToken); err != nil {
		return provider.BearerCredential{}, err
	}
	refreshed, err := s.refresh.ResolveOAuthBearerByID(ctx, credential.ID, s.now().UTC())
	if err != nil {
		return provider.BearerCredential{}, err
	}
	return provider.BearerCredential{
		ID:                      refreshed.ID,
		ProviderInstanceID:      refreshed.ProviderInstanceID,
		Kind:                    provider.CredentialKindOAuthAccess,
		BearerToken:             refreshed.BearerToken,
		ChatGPTAccountID:        refreshed.ChatGPTAccountID,
		ChatGPTAccountIsFedRAMP: refreshed.ChatGPTAccountIsFedRAMP,
	}, nil
}

func providerChatCredential(credential provider.BearerCredential) provider.ChatCredential {
	return provider.ChatCredential{
		ID:                      credential.ID,
		ProviderInstanceID:      credential.ProviderInstanceID,
		Kind:                    credential.Kind,
		BearerToken:             credential.BearerToken,
		ChatGPTAccountID:        credential.ChatGPTAccountID,
		ChatGPTAccountIsFedRAMP: credential.ChatGPTAccountIsFedRAMP,
	}
}
