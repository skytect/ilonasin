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
	models   []provider.ModelMetadata
	live     bool
	complete bool
}

func (s *Server) handleModels(w http.ResponseWriter, r *http.Request, _ credentials.VerifiedLocalToken) {
	catalog, err := s.refreshModelCatalog(r.Context())
	if err != nil {
		if r.Context().Err() != nil {
			return
		}
		status, code := http.StatusBadGateway, "model_discovery_failed"
		if errors.Is(err, errModelCacheUnavailable) {
			status, code = http.StatusInternalServerError, "model_cache_unavailable"
		} else if errors.Is(err, errModelCredentialResolver) {
			status, code = http.StatusInternalServerError, "credential_resolver_failed"
		}
		s.logHTTP(r, status, "models_route", code)
		writeError(w, status, err.Error(), "api_error", code)
		return
	}
	resp := modelsResponseFromMetadata(catalog.models)
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
				return modelDiscoveryAttempt{models: models, live: true, complete: true}
			}
			if ctx.Err() != nil {
				return modelDiscoveryAttempt{}
			}
		}
		return modelDiscoveryAttempt{}
	}

	complete := true
	byID := make(map[string]provider.ModelMetadata)
	// Discover account-scoped catalogs concurrently with bounded worker count.
	// One slow credential must not consume the entire shared refresh budget
	// before the other credentials have even been queried.
	type credentialResult struct {
		models []provider.ModelMetadata
		live   bool
	}
	results := make([]chan credentialResult, len(credentialsSet))
	workers := make(chan struct{}, 4)
	for i, credential := range credentialsSet {
		results[i] = make(chan credentialResult, 1)
		go func(out chan<- credentialResult, credential provider.BearerCredential) {
			select {
			case workers <- struct{}{}:
				defer func() { <-workers }()
			case <-ctx.Done():
				out <- credentialResult{}
				return
			}
			models, live := s.discoverModelsWithCredential(ctx, instance, discoverer, credential)
			out <- credentialResult{models: models, live: live}
		}(results[i], credential)
	}
	for i, credential := range credentialsSet {
		result := <-results[i]
		models, ok := result.models, result.live
		now := s.now().UTC()
		key := modelCatalogKey(instance.ID, credential)
		if !ok {
			complete = false
			s.credentialModelCatalogs.fail(now, key)
			if ctx.Err() != nil {
				return modelDiscoveryAttempt{}
			}
			continue
		}
		s.credentialModelCatalogs.put(now, key, providerModelIDs(models))
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
	return modelDiscoveryAttempt{models: models, live: true, complete: complete}
}

func (s *Server) discoverModelsWithCredential(ctx context.Context, instance provider.Instance, discoverer provider.ModelDiscoverer, credential provider.BearerCredential) ([]provider.ModelMetadata, bool) {
	ctx, cancel := context.WithTimeout(ctx, modelCatalogCredentialTimeout)
	defer cancel()
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
