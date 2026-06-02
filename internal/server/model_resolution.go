package server

import (
	"context"
	"strings"

	"ilonasin/internal/routing"
)

func (s *Server) resolveModelAddress(ctx context.Context, model string) (routing.ModelAddress, error) {
	addr, err := routing.ParseModelAddress(model)
	if err == nil || strings.Contains(model, "/") || s.cache == nil {
		return addr, err
	}
	rows, cacheErr := s.cache.ListModelCache(ctx)
	if cacheErr != nil {
		return routing.ModelAddress{}, err
	}
	var match routing.ModelAddress
	for _, row := range rows {
		if row.ModelID != model {
			continue
		}
		if match.ProviderInstanceID != "" {
			return routing.ModelAddress{}, err
		}
		match = routing.ModelAddress{ProviderInstanceID: row.ProviderInstanceID, ProviderModelID: row.ModelID}
	}
	if match.ProviderInstanceID == "" {
		return routing.ModelAddress{}, err
	}
	return match, nil
}
