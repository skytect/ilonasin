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
		if err != nil && errors.Is(err, credentials.ErrNoEligibleCredential) && s.refresh != nil && instance.Type == "codex" {
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

func (s *Server) shouldRefreshOAuthAfterChat401(instance provider.Instance, result provider.ChatResult) bool {
	return instance.Type == "codex" && instance.OAuth && result.StatusCode == http.StatusUnauthorized && result.ErrorClass == "upstream_auth_failed" && s.refresh != nil
}

func (s *Server) shouldRefreshOAuthAfterStream401(instance provider.Instance, summary provider.ChatStreamSummary) bool {
	return instance.Type == "codex" && instance.OAuth && summary.StatusCode == http.StatusUnauthorized && summary.ErrorClass == "upstream_auth_failed" && summary.PreStreamError && !summary.Started && s.refresh != nil
}

func (s *Server) shouldRefreshModelCredentialAfterChat401(instance provider.Instance, result provider.ChatResult, credential provider.BearerCredential) bool {
	return instance.Type == "codex" && instance.OAuth && credential.ID != 0 && result.StatusCode == http.StatusUnauthorized && result.ErrorClass == "model_discovery_auth_failed" && s.refresh != nil
}

func (s *Server) shouldRefreshModelCredentialAfterStream401(instance provider.Instance, summary provider.ChatStreamSummary, credential provider.BearerCredential) bool {
	return instance.Type == "codex" && instance.OAuth && credential.ID != 0 && summary.StatusCode == http.StatusUnauthorized && summary.ErrorClass == "model_discovery_auth_failed" && summary.PreStreamError && !summary.Started && s.refresh != nil
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
