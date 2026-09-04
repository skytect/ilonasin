package server

import (
	"context"
	"fmt"
	"strings"

	"ilonasin/internal/metadata"
	"ilonasin/internal/provider"
	"ilonasin/internal/routing"
)

func (s *Server) resolveModelAddress(ctx context.Context, model string) (routing.ModelAddress, error) {
	requested := model
	model, selector, err := routing.SplitAccountSelector(model)
	if err != nil {
		return routing.ModelAddress{}, err
	}
	addr, err := routing.ParseModelAddress(requested)
	if err == nil {
		if selector != "" && s.registry != nil {
			if instance, ok := s.registry.Get(addr.ProviderInstanceID); ok && instance.Type != "codex" {
				return routing.ModelAddress{}, fmt.Errorf("daybreak account selection requires a Codex provider")
			}
		}
		return addr, nil
	}
	if strings.Contains(model, "/") || strings.TrimSpace(model) == "" {
		return routing.ModelAddress{}, err
	}
	if s.registry == nil {
		return routing.ModelAddress{}, fmt.Errorf("no providers are configured")
	}
	rows, cacheErr := s.modelResolutionRows(ctx)
	if cacheErr != nil {
		return routing.ModelAddress{}, cacheErr
	}
	if match, unique := s.uniqueSelectedModelAddress(rows, model, selector); unique {
		return match, nil
	}
	catalog, refreshErr := s.refreshModelCatalog(ctx)
	if refreshErr != nil {
		return routing.ModelAddress{}, fmt.Errorf("cannot resolve bare model: %w; use <provider_instance_id>/<provider_model_id>", refreshErr)
	}
	if match, unique := s.uniqueSelectedModelAddress(catalog.addresses, model, selector); unique {
		return match, nil
	}
	if selector != "" {
		return routing.ModelAddress{}, fmt.Errorf("model and account selector have no unique matching provider")
	}
	return routing.ModelAddress{}, fmt.Errorf("bare model has no unique provider match; use <provider_instance_id>/<provider_model_id>")
}

func (s *Server) uniqueSelectedModelAddress(rows []metadata.ModelCacheRow, model, selector string) (routing.ModelAddress, bool) {
	if selector == "" {
		return s.uniqueModelAddress(rows, model)
	}
	eligibleProviders := map[string]bool{}
	required := provider.ModelCatalogRequirements(model, selector)
	for _, row := range rows {
		if row.ModelID == required[1] {
			if instance, ok := s.registry.Get(row.ProviderInstanceID); ok && instance.Type == "codex" {
				eligibleProviders[instance.ID] = true
			}
		}
	}
	filtered := make([]metadata.ModelCacheRow, 0, len(rows))
	for _, row := range rows {
		if eligibleProviders[row.ProviderInstanceID] {
			filtered = append(filtered, row)
		}
	}
	addr, unique := s.uniqueModelAddress(filtered, model)
	addr.AccountSelector = selector
	return addr, unique
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
