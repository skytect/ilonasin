package server

import (
	"errors"
	"net/http"
	"sort"

	"ilonasin/internal/credentials"
	"ilonasin/internal/provider"
)

func (s *Server) handleModels(w http.ResponseWriter, r *http.Request, _ credentials.VerifiedLocalToken) {
	type model struct {
		ID      string `json:"id"`
		Object  string `json:"object"`
		OwnedBy string `json:"owned_by"`
	}
	cacheByProvider := map[string][]provider.ModelMetadata{}
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
		credential, err := s.resolveModelCredential(r.Context(), instance)
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
			all = append(all, cached...)
			continue
		}
		result, err := discoverer.ListModels(r.Context(), provider.ModelRequest{
			Instance:   instance,
			Credential: credential,
		})
		s.recordHealth(r.Context(), healthFromModelDiscovery(instance, credential, result, err))
		if s.shouldRefreshOAuthAfterModel401(instance, result) {
			if refreshed, refreshErr := s.refreshOAuthCredentialForRetryIfBearer(r.Context(), credential); refreshErr == nil {
				credential = refreshed
				result, err = discoverer.ListModels(r.Context(), provider.ModelRequest{
					Instance:   instance,
					Credential: credential,
				})
				s.recordHealth(r.Context(), healthFromModelDiscovery(instance, credential, result, err))
			} else {
				failedWithoutCache++
				continue
			}
		}
		if err == nil && len(result.Models) > 0 {
			if s.cache != nil {
				if err := s.cache.ReplaceModelCache(r.Context(), instance.ID, result.Models); err != nil {
					writeError(w, http.StatusInternalServerError, "model cache is unavailable", "api_error", "model_cache_unavailable")
					return
				}
			}
			all = append(all, result.Models...)
			continue
		}
		if s.isOAuthAuthFailure(instance, result) {
			failedWithoutCache++
			continue
		}
		cached := cacheByProvider[instance.ID]
		if len(cached) == 0 {
			failedWithoutCache++
			continue
		}
		all = append(all, cached...)
	}
	if len(all) == 0 && attempted > 0 && failedWithoutCache == attempted {
		writeError(w, http.StatusBadGateway, "model discovery failed", "api_error", "model_discovery_failed")
		return
	}
	sort.SliceStable(all, func(i, j int) bool {
		if all[i].ProviderInstanceID != all[j].ProviderInstanceID {
			return all[i].ProviderInstanceID < all[j].ProviderInstanceID
		}
		return all[i].ModelID < all[j].ModelID
	})
	data := make([]model, 0, len(all))
	for _, row := range all {
		data = append(data, model{
			ID:      row.ProviderInstanceID + "/" + row.ModelID,
			Object:  "model",
			OwnedBy: row.ProviderInstanceID,
		})
	}
	resp := struct {
		Object string  `json:"object"`
		Data   []model `json:"data"`
	}{Object: "list", Data: data}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) shouldRefreshOAuthAfterModel401(instance provider.Instance, result provider.ModelResult) bool {
	return instance.Type == "codex" && instance.OAuth && result.StatusCode == http.StatusUnauthorized && s.refresh != nil
}

func (s *Server) isOAuthAuthFailure(instance provider.Instance, result provider.ModelResult) bool {
	return instance.Type == "codex" && instance.OAuth && result.StatusCode == http.StatusUnauthorized
}
