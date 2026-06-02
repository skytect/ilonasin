package server

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"ilonasin/internal/credentials"
	"ilonasin/internal/metadata"
	"ilonasin/internal/provider"
)

type modelDiscoveryAttempt struct {
	models []provider.ModelMetadata
	live   bool
}

func (s *Server) handleModels(w http.ResponseWriter, r *http.Request, _ credentials.VerifiedLocalToken) {
	cacheByProvider := map[string][]metadata.ModelCacheRow{}
	if s.cache != nil {
		cached, err := s.cache.ListModelCache(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "model cache is unavailable", "api_error", "model_cache_unavailable")
			return
		}
		for _, row := range cached {
			cacheByProvider[row.ProviderInstanceID] = append(cacheByProvider[row.ProviderInstanceID], row)
		}
	}
	var all []provider.ModelMetadata
	attempted := 0
	failedWithoutCache := 0
	for _, instance := range s.registry.List() {
		if !instance.ModelDiscovery {
			continue
		}
		credentialsSet, err := s.resolveModelCredentials(r.Context(), instance)
		if err != nil {
			if errors.Is(err, credentials.ErrNoEligibleCredential) {
				continue
			}
			if errors.Is(err, credentials.ErrOAuthRefreshFailed) {
				attempted++
				failedWithoutCache++
				continue
			}
			writeError(w, http.StatusInternalServerError, "upstream credential resolver failed", "api_error", "credential_resolver_failed")
			return
		}
		attempted++
		var discoverer provider.ModelDiscoverer
		ok := false
		if s.models != nil {
			discoverer, ok = s.models.ForProvider(instance.Type)
		}
		if !ok {
			cached := cacheByProvider[instance.ID]
			if len(cached) == 0 {
				failedWithoutCache++
				continue
			}
			all = append(all, providerModelsFromCacheRows(cached)...)
			continue
		}
		attempt := s.discoverModelsWithCredentials(r.Context(), instance, discoverer, credentialsSet)
		if attempt.live && len(attempt.models) > 0 {
			if s.cache != nil {
				if err := s.cache.ReplaceModelCache(r.Context(), instance.ID, modelCacheRowsFromProvider(attempt.models)); err != nil {
					writeError(w, http.StatusInternalServerError, "model cache is unavailable", "api_error", "model_cache_unavailable")
					return
				}
			}
			all = append(all, attempt.models...)
			continue
		}
		cached := cacheByProvider[instance.ID]
		if len(cached) == 0 {
			failedWithoutCache++
			continue
		}
		all = append(all, providerModelsFromCacheRows(cached)...)
	}
	if len(all) == 0 && attempted > 0 && failedWithoutCache == attempted {
		s.logHTTP(r, http.StatusBadGateway, "models_route", "model_discovery_failed")
		writeError(w, http.StatusBadGateway, "model discovery failed", "api_error", "model_discovery_failed")
		return
	}
	resp := modelsResponseFromMetadata(all)
	if s.logger != nil {
		s.logAttrs(r, levelForStatus(http.StatusOK, ""), "models route complete",
			slog.String("event", "models_route"),
			slog.Int("status", http.StatusOK),
			slog.Int("model_count", len(resp.Data)),
		)
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) discoverModelsWithCredentials(ctx context.Context, instance provider.Instance, discoverer provider.ModelDiscoverer, credentialsSet []provider.BearerCredential) modelDiscoveryAttempt {
	for _, credential := range credentialsSet {
		result, err := discoverer.ListModels(ctx, provider.ModelRequest{
			Instance:   instance,
			Credential: credential,
		})
		s.recordHealth(ctx, healthFromModelDiscovery(instance, credential, result, err))
		if err == nil && len(result.Models) > 0 {
			return modelDiscoveryAttempt{models: result.Models, live: true}
		}
		if !s.shouldRefreshOAuthAfterModel401(instance, result) {
			continue
		}
		refreshed, refreshErr := s.refreshOAuthCredentialForRetryIfBearer(ctx, credential)
		if refreshErr != nil {
			continue
		}
		result, err = discoverer.ListModels(ctx, provider.ModelRequest{
			Instance:   instance,
			Credential: refreshed,
		})
		s.recordHealth(ctx, healthFromModelDiscovery(instance, refreshed, result, err))
		if err == nil && len(result.Models) > 0 {
			return modelDiscoveryAttempt{models: result.Models, live: true}
		}
	}
	return modelDiscoveryAttempt{}
}

func (s *Server) shouldRefreshOAuthAfterModel401(instance provider.Instance, result provider.ModelResult) bool {
	return instance.Type == "codex" && instance.OAuth && result.StatusCode == http.StatusUnauthorized && s.refresh != nil
}
