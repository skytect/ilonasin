package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"ilonasin/internal/credentials"
	"ilonasin/internal/provider"
)

func (s *Server) resolveModelCredential(ctx context.Context, instance provider.Instance) (provider.BearerCredential, error) {
	if instance.APIKey && !instance.Placeholder {
		credential, err := s.upstreams.ResolveAPIKey(ctx, instance.ID)
		if err != nil {
			return provider.BearerCredential{}, err
		}
		return provider.BearerCredential{
			ID:                 credential.ID,
			ProviderInstanceID: credential.ProviderInstanceID,
			Kind:               provider.CredentialKindAPIKey,
			BearerToken:        credential.APIKey,
		}, nil
	}
	if instance.OAuth {
		if s.oauth == nil {
			return provider.BearerCredential{}, credentials.ErrNoEligibleCredential
		}
		credential, err := s.oauth.ResolveOAuthBearer(ctx, instance.ID, s.now().UTC())
		if err != nil && errors.Is(err, credentials.ErrNoEligibleCredential) && s.refresh != nil && instance.Type == "codex" {
			if refreshErr := s.refresh.RefreshOAuthProviderCredential(ctx, instance.ID); refreshErr == nil {
				credential, err = s.oauth.ResolveOAuthBearer(ctx, instance.ID, s.now().UTC())
				if err != nil {
					return provider.BearerCredential{}, fmt.Errorf("%w: oauth refresh did not yield bearer", credentials.ErrOAuthRefreshFailed)
				}
			} else {
				return provider.BearerCredential{}, fmt.Errorf("%w: oauth refresh unavailable", credentials.ErrOAuthRefreshFailed)
			}
		}
		if err != nil {
			return provider.BearerCredential{}, err
		}
		return provider.BearerCredential{
			ID:                 credential.ID,
			ProviderInstanceID: credential.ProviderInstanceID,
			Kind:               provider.CredentialKindOAuthAccess,
			BearerToken:        credential.BearerToken,
		}, nil
	}
	return provider.BearerCredential{}, credentials.ErrNoEligibleCredential
}

func (s *Server) shouldRefreshOAuthAfterChat401(instance provider.Instance, result provider.ChatResult) bool {
	return instance.Type == "codex" && instance.OAuth && result.StatusCode == http.StatusUnauthorized && s.refresh != nil
}

func (s *Server) shouldRefreshOAuthAfterStream401(instance provider.Instance, summary provider.ChatStreamSummary) bool {
	return instance.Type == "codex" && instance.OAuth && summary.StatusCode == http.StatusUnauthorized && summary.PreStreamError && !summary.Started && s.refresh != nil
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
		ID:                 refreshed.ID,
		ProviderInstanceID: refreshed.ProviderInstanceID,
		Kind:               provider.CredentialKindOAuthAccess,
		BearerToken:        refreshed.BearerToken,
	}, nil
}

func providerAPIKey(credential credentials.ResolvedAPIKeyCredential) provider.ChatCredential {
	return provider.ChatCredential{
		ID:                 credential.ID,
		ProviderInstanceID: credential.ProviderInstanceID,
		Kind:               provider.CredentialKindAPIKey,
		BearerToken:        credential.APIKey,
	}
}
