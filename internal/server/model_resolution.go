package server

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"ilonasin/internal/routing"
)

func (s *Server) resolveModelAddress(ctx context.Context, model string) (routing.ModelAddress, error) {
	addr, err := routing.ParseModelAddress(model)
	if err == nil {
		return addr, nil
	}
	if strings.Contains(model, "/") || s.cache == nil {
		return routing.ModelAddress{}, err
	}

	rows, cacheErr := s.cache.ListModelCache(ctx)
	if cacheErr != nil {
		return routing.ModelAddress{}, fmt.Errorf("model cache is unavailable")
	}
	matches := map[string]routing.ModelAddress{}
	for _, row := range rows {
		if row.ModelID != model {
			continue
		}
		if _, ok := s.registry.Get(row.ProviderInstanceID); !ok {
			continue
		}
		matches[row.ProviderInstanceID] = routing.ModelAddress{
			ProviderInstanceID: row.ProviderInstanceID,
			ProviderModelID:    row.ModelID,
		}
	}
	if len(matches) == 1 {
		for _, match := range matches {
			return match, nil
		}
	}
	if len(matches) > 1 {
		providers := make([]string, 0, len(matches))
		for providerID := range matches {
			providers = append(providers, providerID)
		}
		sort.Strings(providers)
		return routing.ModelAddress{}, fmt.Errorf("model %q is ambiguous across providers %s; use <provider_instance_id>/<provider_model_id>", model, strings.Join(providers, ", "))
	}
	return routing.ModelAddress{}, err
}
