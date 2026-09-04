package server

import (
	"context"
	"net/http"
	"sync"
	"time"

	"ilonasin/internal/provider"
)

type credentialModelObservation struct {
	credential provider.BearerCredential
	models     []provider.ModelMetadata
	live       bool
	transient  bool
}

type credentialModelFlight struct {
	done        chan struct{}
	observation credentialModelObservation
}

// Only active requests retain full model metadata. Completed observations keep
// the existing bounded model-ID proof cache, never a second prompt cache.
type credentialModelDiscovery struct {
	mu      sync.Mutex
	flights map[credentialModelCatalogKey]*credentialModelFlight
}

func (s *Server) observeCredentialModels(ctx context.Context, instance provider.Instance, discoverer provider.ModelDiscoverer, credential provider.BearerCredential, fresh bool) credentialModelObservation {
	if ctx.Err() != nil {
		return credentialModelObservation{credential: credential}
	}
	key := modelCatalogKey(instance.ID, credential)
	state := &s.credentialDiscovery
	state.mu.Lock()
	if state.flights == nil {
		state.flights = make(map[credentialModelCatalogKey]*credentialModelFlight)
	}
	flight := state.flights[key]
	if flight == nil {
		if !fresh {
			if _, known, cached := s.credentialModelCatalogs.lookup(s.now().UTC(), key); cached {
				state.mu.Unlock()
				return credentialModelObservation{credential: credential, live: known}
			}
		}
		if len(state.flights) >= maxCredentialModelCatalogs {
			state.mu.Unlock()
			return credentialModelObservation{credential: credential}
		}
		flight = &credentialModelFlight{done: make(chan struct{})}
		state.flights[key] = flight
		go func() {
			fetchCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), modelCatalogCredentialTimeout)
			defer cancel()
			observation := s.fetchCredentialModels(fetchCtx, instance, discoverer, credential)
			observedKey := modelCatalogKey(instance.ID, observation.credential)
			if observedKey != key {
				s.credentialModelCatalogs.fail(s.now().UTC(), key)
			}
			if observation.live {
				s.credentialModelCatalogs.put(s.now().UTC(), observedKey, providerModelIDs(observation.models))
			} else {
				s.credentialModelCatalogs.recordFailure(s.now().UTC(), observedKey, observation.transient)
			}
			state.mu.Lock()
			flight.observation = observation
			delete(state.flights, key)
			close(flight.done)
			state.mu.Unlock()
		}()
	}
	state.mu.Unlock()
	select {
	case <-ctx.Done():
		return credentialModelObservation{credential: credential}
	case <-flight.done:
		return flight.observation
	}
}

func (s *Server) fetchCredentialModels(ctx context.Context, instance provider.Instance, discoverer provider.ModelDiscoverer, credential provider.BearerCredential) credentialModelObservation {
	observation := credentialModelObservation{credential: credential}
	result, err := discoverer.ListModels(ctx, provider.ModelRequest{Instance: instance, Credential: credential})
	observation.live = ctx.Err() == nil && err == nil && len(result.Models) > 0
	observation.transient = transientModelDiscoveryFailure(ctx, result)
	s.recordModelDiscoveryHealth(ctx, instance, credential, result, err)
	if observation.live {
		observation.models = result.Models
		return observation
	}
	if ctx.Err() != nil || !s.shouldRefreshOAuthAfterModel401(instance, result) {
		return observation
	}
	refreshed, refreshErr := s.refreshOAuthCredentialForRetryIfBearer(ctx, credential)
	if refreshErr != nil || ctx.Err() != nil {
		return observation
	}
	observation.credential = refreshed
	result, err = discoverer.ListModels(ctx, provider.ModelRequest{Instance: instance, Credential: refreshed})
	observation.live = ctx.Err() == nil && err == nil && len(result.Models) > 0
	observation.transient = transientModelDiscoveryFailure(ctx, result)
	s.recordModelDiscoveryHealth(ctx, instance, refreshed, result, err)
	if observation.live {
		observation.models = result.Models
	}
	return observation
}

func transientModelDiscoveryFailure(ctx context.Context, result provider.ModelResult) bool {
	// Authentication and other definitive client rejections override a deadline
	// that happened to expire while their response was being processed.
	if result.StatusCode >= 400 && result.StatusCode < 500 {
		return result.StatusCode == http.StatusRequestTimeout || result.StatusCode == http.StatusTooManyRequests
	}
	switch result.ErrorClass {
	case "upstream_invalid_response", "upstream_body_too_large", "provider_config_error", "upstream_request_error":
		return false
	}
	if ctx.Err() != nil || result.StatusCode >= 500 {
		return true
	}
	switch result.ErrorClass {
	case "upstream_timeout", "upstream_network_error", "client_disconnected":
		return true
	}
	return false
}

func (s *Server) recordModelDiscoveryHealth(ctx context.Context, instance provider.Instance, credential provider.BearerCredential, result provider.ModelResult, err error) {
	// A network deadline must not cancel the metadata describing that deadline.
	recordCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
	defer cancel()
	s.recordHealth(recordCtx, healthFromModelDiscovery(instance, credential, result, err))
}
