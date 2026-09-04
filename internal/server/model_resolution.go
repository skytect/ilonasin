package server

import (
	"context"
	"fmt"
	"strings"

	"ilonasin/internal/metadata"
	"ilonasin/internal/routing"
)

func (s *Server) resolveModelAddress(ctx context.Context, model string) (routing.ModelAddress, error) {
	addr, err := routing.ParseModelAddress(model)
	if err == nil || strings.Contains(model, "/") || strings.TrimSpace(model) == "" {
		return addr, err
	}
	if s.cache != nil {
		rows, cacheErr := s.cache.ListModelCache(ctx)
		if cacheErr != nil {
			return routing.ModelAddress{}, errModelCacheUnavailable
		}
		if match, unique := s.uniqueModelAddress(rows, model); unique {
			return match, nil
		}
	}
	catalog, refreshErr := s.refreshModelCatalog(ctx)
	if refreshErr != nil {
		return routing.ModelAddress{}, fmt.Errorf("cannot resolve bare model: %w; use <provider_instance_id>/<provider_model_id>", refreshErr)
	}
	if match, unique := s.uniqueModelAddress(catalog.addresses, model); unique {
		return match, nil
	}
	return routing.ModelAddress{}, fmt.Errorf("bare model has no unique provider match; use <provider_instance_id>/<provider_model_id>")
}

func (s *Server) uniqueModelAddress(rows []metadata.ModelCacheRow, model string) (routing.ModelAddress, bool) {
	var match routing.ModelAddress
	for _, row := range rows {
		if row.ModelID != model {
			continue
		}
		if _, configured := s.registry.Get(row.ProviderInstanceID); !configured {
			continue
		}
		if match.ProviderInstanceID != "" && match.ProviderInstanceID != row.ProviderInstanceID {
			return routing.ModelAddress{}, false
		}
		match = routing.ModelAddress{ProviderInstanceID: row.ProviderInstanceID, ProviderModelID: row.ModelID}
	}
	return match, match.ProviderInstanceID != ""
}
