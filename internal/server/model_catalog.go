package server

import (
	"context"
	"errors"
	"sync"
	"time"

	"ilonasin/internal/credentials"
	"ilonasin/internal/metadata"
	"ilonasin/internal/provider"
)

const (
	modelCatalogRefreshTimeout    = 90 * time.Second
	modelCatalogRefreshCooldown   = time.Second
	modelCatalogCredentialTimeout = 30 * time.Second
)

var (
	errModelCacheUnavailable   = errors.New("model cache is unavailable")
	errModelCredentialResolver = errors.New("upstream credential resolver failed")
	errModelDiscoveryFailed    = errors.New("model discovery failed")
)

// Catalog addresses contain only safe persisted metadata. They are routing
// hints, never proof that a credential is entitled to use a model.
type modelCatalog struct {
	models    []provider.ModelMetadata
	addresses []metadata.ModelCacheRow
}

type modelCatalogFlight struct {
	done     chan struct{}
	catalog  modelCatalog
	err      error
	finished time.Time
}

type modelCatalogRefresh struct {
	mu     sync.Mutex
	flight *modelCatalogFlight
	last   *modelCatalogFlight
}

// Both complete and partial observations persist safe routing hints. Prompt
// metadata and credential entitlement remain outside this address cache.
func (s *Server) modelResolutionRows(ctx context.Context) ([]metadata.ModelCacheRow, error) {
	var rows []metadata.ModelCacheRow
	if s.cache != nil {
		var err error
		rows, err = s.cache.ListModelCache(ctx)
		if err != nil {
			return nil, errModelCacheUnavailable
		}
	}
	return rows, nil
}

// refreshModelCatalog coalesces listings and bare-name misses into one bounded
// discovery pass. Callers can cancel their wait without cancelling other callers.
// A single short cooldown also bounds repeated misses without retaining user keys.
func (s *Server) refreshModelCatalog(ctx context.Context) (modelCatalog, error) {
	if err := ctx.Err(); err != nil {
		return modelCatalog{}, err
	}
	state := &s.catalogRefresh
	state.mu.Lock()
	flight := state.flight
	if flight == nil && state.last != nil && time.Since(state.last.finished) < modelCatalogRefreshCooldown {
		flight = state.last
	}
	if flight == nil {
		flight = &modelCatalogFlight{done: make(chan struct{})}
		state.flight = flight
		go func() {
			refreshCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), modelCatalogRefreshTimeout)
			defer cancel()
			flight.catalog, flight.err = s.discoverModelCatalog(refreshCtx)
			state.mu.Lock()
			flight.finished = time.Now()
			state.last = flight
			state.flight = nil
			close(flight.done)
			state.mu.Unlock()
			// Listing metadata can include provider instructions. Do not retain
			// an additional unbounded-lifetime copy beyond the cooldown.
			time.AfterFunc(modelCatalogRefreshCooldown, func() {
				state.mu.Lock()
				defer state.mu.Unlock()
				if state.last == flight {
					state.last = nil
				}
			})
		}()
	}
	state.mu.Unlock()
	select {
	case <-ctx.Done():
		return modelCatalog{}, ctx.Err()
	case <-flight.done:
		return flight.catalog, flight.err
	}
}

func (s *Server) discoverModelCatalog(ctx context.Context) (modelCatalog, error) {
	cacheByProvider := map[string][]metadata.ModelCacheRow{}
	{
		cached, err := s.modelResolutionRows(ctx)
		if err != nil {
			return modelCatalog{}, errModelCacheUnavailable
		}
		for _, row := range cached {
			cacheByProvider[row.ProviderInstanceID] = append(cacheByProvider[row.ProviderInstanceID], row)
		}
	}
	var catalog modelCatalog
	attempted, failedWithoutCache := 0, 0
	instances := s.registry.List()
	type providerResult struct {
		attempt modelDiscoveryAttempt
		err     error
	}
	results := make([]chan providerResult, len(instances))
	workers := make(chan struct{}, 4)
	for i, instance := range instances {
		results[i] = make(chan providerResult, 1)
		go func(out chan<- providerResult, instance provider.Instance) {
			if !instance.ModelDiscovery {
				out <- providerResult{}
				return
			}
			select {
			case workers <- struct{}{}:
				defer func() { <-workers }()
			case <-ctx.Done():
				out <- providerResult{err: ctx.Err()}
				return
			}
			credentialsSet, err := s.resolveModelCredentials(ctx, instance)
			var attempt modelDiscoveryAttempt
			if err == nil && s.models != nil {
				if discoverer, ok := s.models.ForProvider(instance.Type); ok {
					attempt = s.discoverModelsWithCredentials(ctx, instance, discoverer, credentialsSet)
				}
			}
			out <- providerResult{attempt: attempt, err: err}
		}(results[i], instance)
	}
	for i, instance := range instances {
		var result providerResult
		select {
		case result = <-results[i]:
		case <-ctx.Done():
			return modelCatalog{}, ctx.Err()
		}
		if err := ctx.Err(); err != nil {
			return modelCatalog{}, err
		}
		persisted := cacheByProvider[instance.ID]
		if !instance.ModelDiscovery {
			catalog.addresses = append(catalog.addresses, persisted...)
			continue
		}
		attempt, err := result.attempt, result.err
		if err != nil && !errors.Is(err, credentials.ErrNoEligibleCredential) && !errors.Is(err, credentials.ErrOAuthRefreshFailed) {
			return modelCatalog{}, errModelCredentialResolver
		}
		if err := ctx.Err(); err != nil {
			return modelCatalog{}, err
		}
		if attempt.complete {
			rows := modelCacheRowsFromProvider(attempt.models)
			if s.cache != nil {
				if err := s.cache.ReplaceModelCache(ctx, instance.ID, rows); err != nil {
					return modelCatalog{}, errModelCacheUnavailable
				}
			}
			persisted = rows
			if instance.Type == "codex" {
				s.lastGoodCodexModels.put(instance.ID, attempt.models)
			}
		}
		catalog.addresses = append(catalog.addresses, persisted...)
		if attempt.live {
			catalog.models = append(catalog.models, attempt.models...)
			if !attempt.complete {
				rows := modelCacheRowsFromProvider(attempt.models)
				if s.cache != nil {
					if err := s.cache.MergeModelCache(ctx, instance.ID, rows); err != nil {
						return modelCatalog{}, errModelCacheUnavailable
					}
				}
				catalog.addresses = append(catalog.addresses, rows...)
			}
			attempted++
			continue
		}
		if errors.Is(err, credentials.ErrNoEligibleCredential) {
			continue
		}
		attempted++
		if fallback, ok := s.modelDiscoveryFallback(instance, persisted); ok {
			catalog.models = append(catalog.models, fallback...)
		} else {
			failedWithoutCache++
		}
	}
	if len(catalog.models) == 0 && attempted > 0 && failedWithoutCache == attempted {
		return catalog, errModelDiscoveryFailed
	}
	return catalog, nil
}
