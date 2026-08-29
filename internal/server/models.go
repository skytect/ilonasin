package server

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"sort"

	"ilonasin/internal/credentials"
	"ilonasin/internal/metadata"
	"ilonasin/internal/provider"
)

type modelDiscoveryAttempt struct {
	models []provider.ModelMetadata
	live   bool
}

func (s *Server) handleModels(w http.ResponseWriter, r *http.Request, _ credentials.VerifiedLocalToken) {
	ctx := r.Context()
	cacheByProvider := map[string][]metadata.ModelCacheRow{}
	if s.cache != nil {
		cached, err := s.cache.ListModelCache(ctx)
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
		if ctx.Err() != nil {
			return
		}
		if !instance.ModelDiscovery {
			continue
		}
		credentialsSet, err := s.resolveModelCredentials(ctx, instance)
		if err != nil {
			if errors.Is(err, credentials.ErrNoEligibleCredential) {
				continue
			}
			if errors.Is(err, credentials.ErrOAuthRefreshFailed) {
				attempted++
				fallback, ok := s.modelDiscoveryFallback(instance, cacheByProvider[instance.ID])
				if !ok {
					failedWithoutCache++
				} else {
					all = append(all, fallback...)
				}
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
			fallback, ok := s.modelDiscoveryFallback(instance, cacheByProvider[instance.ID])
			if !ok {
				failedWithoutCache++
				continue
			}
			all = append(all, fallback...)
			continue
		}
		attempt := s.discoverModelsWithCredentials(ctx, instance, discoverer, credentialsSet)
		if ctx.Err() != nil {
			return
		}
		if attempt.live && len(attempt.models) > 0 {
			if s.cache != nil {
				if err := s.cache.ReplaceModelCache(ctx, instance.ID, modelCacheRowsFromProvider(attempt.models)); err != nil {
					writeError(w, http.StatusInternalServerError, "model cache is unavailable", "api_error", "model_cache_unavailable")
					return
				}
			}
			if instance.Type == "codex" {
				s.lastGoodCodexModels.put(instance.ID, attempt.models)
			}
			all = append(all, attempt.models...)
			continue
		}
		fallback, ok := s.modelDiscoveryFallback(instance, cacheByProvider[instance.ID])
		if !ok {
			failedWithoutCache++
			continue
		}
		all = append(all, fallback...)
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

func (s *Server) modelDiscoveryFallback(instance provider.Instance, persisted []metadata.ModelCacheRow) ([]provider.ModelMetadata, bool) {
	if instance.Type == "codex" {
		return s.lastGoodCodexModels.get(instance.ID)
	}
	if len(persisted) == 0 {
		return nil, false
	}
	return providerModelsFromCacheRows(persisted), true
}

func (s *Server) discoverModelsWithCredentials(ctx context.Context, instance provider.Instance, discoverer provider.ModelDiscoverer, credentialsSet []provider.BearerCredential) modelDiscoveryAttempt {
	union := false
	if policy, ok := discoverer.(provider.CredentialCatalogUnionPolicy); ok {
		union = policy.RequiresCredentialCatalogUnion(instance)
	}
	if !union {
		for _, credential := range credentialsSet {
			if models, ok := s.discoverModelsWithCredential(ctx, instance, discoverer, credential); ok {
				return modelDiscoveryAttempt{models: models, live: true}
			}
			if ctx.Err() != nil {
				return modelDiscoveryAttempt{}
			}
		}
		return modelDiscoveryAttempt{}
	}

	byID := make(map[string]provider.ModelMetadata)
	for _, credential := range credentialsSet {
		models, ok := s.discoverModelsWithCredential(ctx, instance, discoverer, credential)
		now := s.now().UTC()
		if !ok {
			s.credentialModelCatalogs.fail(now, instance.ID, credential.ID)
			if ctx.Err() != nil {
				return modelDiscoveryAttempt{}
			}
			continue
		}
		s.credentialModelCatalogs.put(now, instance.ID, credential.ID, providerModelIDs(models))
		for _, model := range models {
			if _, exists := byID[model.ModelID]; !exists {
				byID[model.ModelID] = model
			}
		}
	}
	if len(byID) == 0 {
		return modelDiscoveryAttempt{}
	}
	models := make([]provider.ModelMetadata, 0, len(byID))
	for _, model := range byID {
		models = append(models, model)
	}
	sort.Slice(models, func(i, j int) bool { return models[i].ModelID < models[j].ModelID })
	return modelDiscoveryAttempt{models: models, live: true}
}

func (s *Server) discoverModelsWithCredential(ctx context.Context, instance provider.Instance, discoverer provider.ModelDiscoverer, credential provider.BearerCredential) ([]provider.ModelMetadata, bool) {
	if ctx.Err() != nil {
		return nil, false
	}
	result, err := discoverer.ListModels(ctx, provider.ModelRequest{
		Instance:   instance,
		Credential: credential,
	})
	s.recordHealth(ctx, healthFromModelDiscovery(instance, credential, result, err))
	if ctx.Err() != nil {
		return nil, false
	}
	if err == nil && len(result.Models) > 0 {
		return result.Models, true
	}
	if !s.shouldRefreshOAuthAfterModel401(instance, result) {
		return nil, false
	}
	refreshed, refreshErr := s.refreshOAuthCredentialForRetryIfBearer(ctx, credential)
	if refreshErr != nil || ctx.Err() != nil {
		return nil, false
	}
	result, err = discoverer.ListModels(ctx, provider.ModelRequest{
		Instance:   instance,
		Credential: refreshed,
	})
	s.recordHealth(ctx, healthFromModelDiscovery(instance, refreshed, result, err))
	if ctx.Err() != nil || err != nil || len(result.Models) == 0 {
		return nil, false
	}
	return result.Models, true
}
