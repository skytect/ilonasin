package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"time"

	"ilonasin/internal/credentials"
	"ilonasin/internal/metadata"
	"ilonasin/internal/openai"
	"ilonasin/internal/provider"
	"ilonasin/internal/routing"
)

const maxRequestBodyBytes = 1 << 20

type Server struct {
	registry  ProviderRegistry
	auth      credentials.LocalTokenVerifier
	upstreams credentials.UpstreamCredentialResolver
	oauth     credentials.OAuthBearerResolver
	refresh   credentials.OAuthProviderRefreshController
	adapters  provider.ChatAdapters
	models    provider.ModelDiscoverers
	cache     ModelCache
	meta      MetadataRecorder
	now       func() time.Time
}

type MetadataRecorder interface {
	RecordRequestMetadata(context.Context, metadata.Request) (int64, error)
	RecordStreamMetrics(context.Context, metadata.Stream) error
	RecordHealthEvent(context.Context, metadata.HealthEvent) error
	RecordFallbackEvent(context.Context, metadata.FallbackEvent) error
}

type ProviderRegistry interface {
	Get(id string) (provider.Instance, bool)
	List() []provider.Instance
}

type ModelCache interface {
	ReplaceModelCache(context.Context, string, []provider.ModelMetadata) error
	ListModelCache(context.Context) ([]provider.ModelMetadata, error)
}

func New(registry ProviderRegistry, auth credentials.LocalTokenVerifier, upstreams credentials.UpstreamCredentialResolver, oauth credentials.OAuthBearerResolver, adapters provider.ChatAdapters, models provider.ModelDiscoverers, cache ModelCache, meta MetadataRecorder) *Server {
	return NewWithClock(registry, auth, upstreams, oauth, adapters, models, cache, meta, time.Now)
}

func NewWithClock(registry ProviderRegistry, auth credentials.LocalTokenVerifier, upstreams credentials.UpstreamCredentialResolver, oauth credentials.OAuthBearerResolver, adapters provider.ChatAdapters, models provider.ModelDiscoverers, cache ModelCache, meta MetadataRecorder, now func() time.Time) *Server {
	if now == nil {
		now = time.Now
	}
	refresh, _ := oauth.(credentials.OAuthProviderRefreshController)
	return &Server{registry: registry, auth: auth, upstreams: upstreams, oauth: oauth, refresh: refresh, adapters: adapters, models: models, cache: cache, meta: meta, now: now}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/models", s.withAuth(s.handleModels))
	mux.HandleFunc("POST /v1/chat/completions", s.withAuth(s.handleChatCompletions))
	return mux
}

func (s *Server) withAuth(next func(http.ResponseWriter, *http.Request, credentials.VerifiedLocalToken)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rec, err := s.auth.VerifyBearer(r.Context(), r.Header.Get("Authorization"))
		if err != nil {
			writeError(w, http.StatusUnauthorized, "missing or invalid bearer token", "authentication_error", "unauthorized")
			return
		}
		next(w, r, rec)
	}
}

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

func (s *Server) resolveModelCredential(ctx context.Context, instance provider.Instance) (provider.BearerCredential, error) {
	if instance.APIKey && !instance.Placeholder {
		credential, err := s.upstreams.ResolveAPIKey(ctx, instance.ID)
		if err != nil {
			return provider.BearerCredential{}, err
		}
		return provider.BearerCredential{
			ID:                 credential.ID,
			ProviderInstanceID: credential.ProviderInstanceID,
			Kind:               provider.CredentialKindAPIKey,
			BearerToken:        credential.APIKey,
		}, nil
	}
	if instance.OAuth {
		if s.oauth == nil {
			return provider.BearerCredential{}, credentials.ErrNoEligibleCredential
		}
		credential, err := s.oauth.ResolveOAuthBearer(ctx, instance.ID, s.now().UTC())
		if err != nil && errors.Is(err, credentials.ErrNoEligibleCredential) && s.refresh != nil && instance.Type == "codex" {
			if refreshErr := s.refresh.RefreshOAuthProviderCredential(ctx, instance.ID); refreshErr == nil {
				credential, err = s.oauth.ResolveOAuthBearer(ctx, instance.ID, s.now().UTC())
				if err != nil {
					return provider.BearerCredential{}, fmt.Errorf("%w: oauth refresh did not yield bearer", credentials.ErrOAuthRefreshFailed)
				}
			} else {
				return provider.BearerCredential{}, fmt.Errorf("%w: oauth refresh unavailable", credentials.ErrOAuthRefreshFailed)
			}
		}
		if err != nil {
			return provider.BearerCredential{}, err
		}
		return provider.BearerCredential{
			ID:                 credential.ID,
			ProviderInstanceID: credential.ProviderInstanceID,
			Kind:               provider.CredentialKindOAuthAccess,
			BearerToken:        credential.BearerToken,
		}, nil
	}
	return provider.BearerCredential{}, credentials.ErrNoEligibleCredential
}

func (s *Server) shouldRefreshOAuthAfterModel401(instance provider.Instance, result provider.ModelResult) bool {
	return instance.Type == "codex" && instance.OAuth && result.StatusCode == http.StatusUnauthorized && s.refresh != nil
}

func (s *Server) isOAuthAuthFailure(instance provider.Instance, result provider.ModelResult) bool {
	return instance.Type == "codex" && instance.OAuth && result.StatusCode == http.StatusUnauthorized
}

func (s *Server) shouldRefreshOAuthAfterChat401(instance provider.Instance, result provider.ChatResult) bool {
	return instance.Type == "codex" && instance.OAuth && result.StatusCode == http.StatusUnauthorized && s.refresh != nil
}

func (s *Server) shouldRefreshOAuthAfterStream401(instance provider.Instance, summary provider.ChatStreamSummary) bool {
	return instance.Type == "codex" && instance.OAuth && summary.StatusCode == http.StatusUnauthorized && summary.PreStreamError && !summary.Started && s.refresh != nil
}

func (s *Server) refreshOAuthCredentialForRetryIfBearer(ctx context.Context, credential provider.BearerCredential) (provider.BearerCredential, error) {
	if s.refresh == nil {
		return provider.BearerCredential{}, credentials.ErrNoEligibleCredential
	}
	if err := s.refresh.RefreshOAuthCredentialIfBearer(ctx, credential.ID, credential.BearerToken); err != nil {
		return provider.BearerCredential{}, err
	}
	refreshed, err := s.refresh.ResolveOAuthBearerByID(ctx, credential.ID, s.now().UTC())
	if err != nil {
		return provider.BearerCredential{}, err
	}
	return provider.BearerCredential{
		ID:                 refreshed.ID,
		ProviderInstanceID: refreshed.ProviderInstanceID,
		Kind:               provider.CredentialKindOAuthAccess,
		BearerToken:        refreshed.BearerToken,
	}, nil
}

func (s *Server) handleChatCompletions(w http.ResponseWriter, r *http.Request, token credentials.VerifiedLocalToken) {
	start := time.Now()
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
	req, err := openai.DecodeChatCompletion(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error(), "invalid_request_error", "invalid_json")
		return
	}
	if err := req.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error(), "invalid_request_error", "unsupported_request")
		return
	}
	addr, err := routing.ParseModelAddress(req.Model)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error(), "invalid_request_error", "invalid_model")
		return
	}
	instance, ok := s.registry.Get(addr.ProviderInstanceID)
	if !ok {
		writeError(w, http.StatusNotFound, "provider instance is not configured", "invalid_request_error", "provider_not_configured")
		return
	}
	if !instance.Chat || (!instance.APIKey && !instance.OAuth) || (instance.Placeholder && instance.Type != "codex") {
		_ = s.record(r.Context(), metadata.Request{
			StartedAt:                 start,
			ClientTokenID:             token.ID,
			RequestedProviderInstance: addr.ProviderInstanceID,
			RequestedModel:            addr.ProviderModelID,
			ResolvedProviderInstance:  addr.ProviderInstanceID,
			ResolvedModel:             addr.ProviderModelID,
			HTTPStatus:                http.StatusNotImplemented,
			ErrorClass:                "provider_unimplemented",
			TotalLatencyMS:            time.Since(start).Milliseconds(),
		})
		writeError(w, http.StatusNotImplemented, "provider credential type is not implemented in this slice", "invalid_request_error", "provider_unimplemented")
		return
	}
	if s.adapters == nil {
		_ = s.record(r.Context(), metadata.Request{
			StartedAt:                 start,
			ClientTokenID:             token.ID,
			RequestedProviderInstance: addr.ProviderInstanceID,
			RequestedModel:            addr.ProviderModelID,
			ResolvedProviderInstance:  addr.ProviderInstanceID,
			ResolvedModel:             addr.ProviderModelID,
			HTTPStatus:                http.StatusNotImplemented,
			ErrorClass:                "provider_unimplemented",
			TotalLatencyMS:            time.Since(start).Milliseconds(),
		})
		writeError(w, http.StatusNotImplemented, "provider adapter is not implemented", "invalid_request_error", "provider_unimplemented")
		return
	}
	adapter, ok := s.adapters.ForProvider(instance.Type)
	if !ok {
		_ = s.record(r.Context(), metadata.Request{
			StartedAt:                 start,
			ClientTokenID:             token.ID,
			RequestedProviderInstance: addr.ProviderInstanceID,
			RequestedModel:            addr.ProviderModelID,
			ResolvedProviderInstance:  addr.ProviderInstanceID,
			ResolvedModel:             addr.ProviderModelID,
			HTTPStatus:                http.StatusNotImplemented,
			ErrorClass:                "provider_unimplemented",
			TotalLatencyMS:            time.Since(start).Milliseconds(),
		})
		writeError(w, http.StatusNotImplemented, "provider adapter is not implemented", "invalid_request_error", "provider_unimplemented")
		return
	}
	if err := adapter.ValidateChatRequest(instance, req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error(), "invalid_request_error", "unsupported_request")
		return
	}
	if instance.Type == "codex" {
		credential, err := s.resolveModelCredential(r.Context(), instance)
		if err != nil {
			_ = s.record(r.Context(), metadata.Request{
				StartedAt:                 start,
				ClientTokenID:             token.ID,
				RequestedProviderInstance: addr.ProviderInstanceID,
				RequestedModel:            addr.ProviderModelID,
				ResolvedProviderInstance:  addr.ProviderInstanceID,
				ResolvedModel:             addr.ProviderModelID,
				HTTPStatus:                http.StatusUnauthorized,
				ErrorClass:                "credential_unavailable",
				TotalLatencyMS:            time.Since(start).Milliseconds(),
			})
			writeError(w, http.StatusUnauthorized, "no eligible upstream credential is available", "invalid_request_error", "credential_unavailable")
			return
		}
		if req.Stream {
			s.handleSingleCredentialStreamingChat(w, r, singleStreamContext{
				start:      start,
				token:      token,
				address:    addr,
				instance:   instance,
				credential: credential,
				adapter:    adapter,
				request:    req,
			})
			return
		}
		s.handleSingleCredentialChat(w, r, singleChatContext{
			start:      start,
			token:      token,
			address:    addr,
			instance:   instance,
			credential: credential,
			adapter:    adapter,
			request:    req,
		})
		return
	}
	credentialsSet, err := s.upstreams.ResolveAPIKeys(r.Context(), addr.ProviderInstanceID)
	if err != nil {
		_ = s.record(r.Context(), metadata.Request{
			StartedAt:                 start,
			ClientTokenID:             token.ID,
			RequestedProviderInstance: addr.ProviderInstanceID,
			RequestedModel:            addr.ProviderModelID,
			ResolvedProviderInstance:  addr.ProviderInstanceID,
			ResolvedModel:             addr.ProviderModelID,
			HTTPStatus:                http.StatusUnauthorized,
			ErrorClass:                "credential_unavailable",
			TotalLatencyMS:            time.Since(start).Milliseconds(),
		})
		writeError(w, http.StatusUnauthorized, "no eligible upstream credential is available", "invalid_request_error", "credential_unavailable")
		return
	}
	if req.Stream {
		s.handleStreamingChat(w, r, streamContext{
			start:       start,
			token:       token,
			address:     addr,
			instance:    instance,
			credentials: credentialsSet,
			adapter:     adapter,
			request:     req,
		})
		return
	}
	s.handleNonStreamingChat(w, r, nonStreamContext{
		start:       start,
		token:       token,
		address:     addr,
		instance:    instance,
		credentials: credentialsSet,
		adapter:     adapter,
		request:     req,
	})
}

type nonStreamContext struct {
	start       time.Time
	token       credentials.VerifiedLocalToken
	address     routing.ModelAddress
	instance    provider.Instance
	credentials []credentials.ResolvedAPIKeyCredential
	adapter     provider.ChatAdapter
	request     openai.ChatCompletionRequest
}

type chatAttempt struct {
	credential credentials.ResolvedAPIKeyCredential
	result     provider.ChatResult
	err        error
}

type singleChatContext struct {
	start      time.Time
	token      credentials.VerifiedLocalToken
	address    routing.ModelAddress
	instance   provider.Instance
	credential provider.BearerCredential
	adapter    provider.ChatAdapter
	request    openai.ChatCompletionRequest
}

func (s *Server) handleSingleCredentialChat(w http.ResponseWriter, r *http.Request, sc singleChatContext) {
	result, err := sc.adapter.CompleteChat(r.Context(), provider.ChatRequest{
		Instance:      sc.instance,
		UpstreamModel: sc.address.ProviderModelID,
		Request:       sc.request,
		Credential: provider.ChatCredential{
			ID:                 sc.credential.ID,
			ProviderInstanceID: sc.credential.ProviderInstanceID,
			Kind:               sc.credential.Kind,
			BearerToken:        sc.credential.BearerToken,
		},
	})
	retryCount := 0
	if s.shouldRefreshOAuthAfterChat401(sc.instance, result) {
		refreshed, refreshErr := s.refreshOAuthCredentialForRetryIfBearer(r.Context(), sc.credential)
		if refreshErr != nil {
			result = provider.ChatResult{StatusCode: http.StatusBadGateway, ContentType: "application/json", ErrorClass: "upstream_auth_failed", Latency: time.Since(sc.start)}
			err = refreshErr
		} else {
			sc.credential = refreshed
			retryCount = 1
			result, err = sc.adapter.CompleteChat(r.Context(), provider.ChatRequest{
				Instance:      sc.instance,
				UpstreamModel: sc.address.ProviderModelID,
				Request:       sc.request,
				Credential: provider.ChatCredential{
					ID:                 sc.credential.ID,
					ProviderInstanceID: sc.credential.ProviderInstanceID,
					Kind:               sc.credential.Kind,
					BearerToken:        sc.credential.BearerToken,
				},
			})
			if s.shouldRefreshOAuthAfterChat401(sc.instance, result) {
				result.StatusCode = http.StatusBadGateway
				result.ErrorClass = "upstream_auth_failed"
			}
		}
	}
	if shouldRecordChatHealth(result) {
		s.recordHealth(r.Context(), healthFromSingleChatAttempt(sc.address, singleChatAttempt{credential: sc.credential, result: result, err: err}))
	}
	status := normalizedChatStatus(result)
	errorClass := normalizedChatErrorClass(result, status)
	if err != nil && errorClass == "" {
		errorClass = "upstream_unavailable"
	}
	recordCtx := r.Context()
	if errorClass == "client_disconnected" {
		var cancel context.CancelFunc
		recordCtx, cancel = context.WithTimeout(context.WithoutCancel(r.Context()), 5*time.Second)
		defer cancel()
	}
	_, _ = s.recordWithID(recordCtx, metadata.Request{
		StartedAt:                 sc.start,
		ClientTokenID:             sc.token.ID,
		CredentialID:              sc.credential.ID,
		RequestedProviderInstance: sc.address.ProviderInstanceID,
		RequestedModel:            sc.address.ProviderModelID,
		ResolvedProviderInstance:  sc.address.ProviderInstanceID,
		ResolvedModel:             resolvedChatModel(sc.address.ProviderModelID, result.ResolvedModel),
		HTTPStatus:                status,
		ErrorClass:                errorClass,
		RetryCount:                retryCount,
		PromptTokens:              result.Usage.PromptTokens,
		CompletionTokens:          result.Usage.CompletionTokens,
		TotalTokens:               result.Usage.TotalTokens,
		ReasoningTokens:           result.Usage.ReasoningTokens,
		CacheHitTokens:            result.Usage.CachedTokens,
		CacheWriteTokens:          result.Usage.CacheWriteTokens,
		CostMicrounits:            result.Usage.CostMicrounits,
		TotalLatencyMS:            time.Since(sc.start).Milliseconds(),
	})
	if errorClass == "client_disconnected" {
		return
	}
	if status < 200 || status >= 300 {
		writeError(w, status, "upstream request failed", "api_error", errorClass)
		return
	}
	if err != nil {
		writeError(w, http.StatusBadGateway, "upstream request failed", "api_error", errorClass)
		return
	}
	writeRaw(w, status, result.ContentType, result.Body)
}

func (s *Server) handleNonStreamingChat(w http.ResponseWriter, r *http.Request, nc nonStreamContext) {
	var final chatAttempt
	var fallbackEvents []metadata.FallbackEvent
	for i, credential := range nc.credentials {
		result, err := nc.adapter.CompleteChat(r.Context(), provider.ChatRequest{
			Instance:      nc.instance,
			UpstreamModel: nc.address.ProviderModelID,
			Request:       nc.request,
			Credential:    providerAPIKey(credential),
		})
		final = chatAttempt{credential: credential, result: result, err: err}
		if shouldRecordChatHealth(result) {
			s.recordHealth(r.Context(), healthFromChatAttempt(nc.address, final))
		}
		if !retryableChatAttempt(result, err) || i == len(nc.credentials)-1 {
			break
		}
		next := nc.credentials[i+1]
		fallbackEvents = append(fallbackEvents, metadata.FallbackEvent{
			OccurredAt:         time.Now(),
			ProviderInstanceID: nc.address.ProviderInstanceID,
			ModelID:            nc.address.ProviderModelID,
			FromCredentialID:   credential.ID,
			ToCredentialID:     next.ID,
			Reason:             "availability_retry",
			AllowedByPolicy:    true,
		})
	}
	status := localChatStatus(final.result, final.err)
	errorClass := localChatErrorClass(final.result, final.err, status)
	requestID, _ := s.recordWithID(r.Context(), metadata.Request{
		StartedAt:                 nc.start,
		ClientTokenID:             nc.token.ID,
		CredentialID:              final.credential.ID,
		RequestedProviderInstance: nc.address.ProviderInstanceID,
		RequestedModel:            nc.address.ProviderModelID,
		ResolvedProviderInstance:  nc.address.ProviderInstanceID,
		ResolvedModel:             resolvedChatModel(nc.address.ProviderModelID, final.result.ResolvedModel),
		HTTPStatus:                status,
		ErrorClass:                errorClass,
		RetryCount:                len(fallbackEvents),
		FallbackCount:             len(fallbackEvents),
		FallbackReason:            fallbackReason(fallbackEvents),
		PromptTokens:              final.result.Usage.PromptTokens,
		CompletionTokens:          final.result.Usage.CompletionTokens,
		TotalTokens:               final.result.Usage.TotalTokens,
		ReasoningTokens:           final.result.Usage.ReasoningTokens,
		CacheHitTokens:            final.result.Usage.CachedTokens,
		CacheWriteTokens:          final.result.Usage.CacheWriteTokens,
		CostMicrounits:            final.result.Usage.CostMicrounits,
		TotalLatencyMS:            time.Since(nc.start).Milliseconds(),
	})
	s.recordFallbacks(r.Context(), requestID, fallbackEvents)
	if final.err != nil && final.result.InvalidBody {
		writeError(w, http.StatusBadGateway, "upstream returned an invalid chat completion response", "api_error", "upstream_invalid_response")
		return
	}
	if final.err != nil && final.result.BodyTruncated {
		writeError(w, http.StatusBadGateway, "upstream response body exceeded the configured limit", "api_error", "upstream_body_too_large")
		return
	}
	if retryableChatAttempt(final.result, final.err) {
		writeError(w, http.StatusBadGateway, "upstream request failed", "api_error", "upstream_unavailable")
		return
	}
	if final.err != nil && final.result.Body == nil {
		writeError(w, http.StatusBadGateway, "upstream request failed", "api_error", errorClass)
		return
	}
	if status < 200 || status >= 300 {
		writeError(w, status, "upstream request failed", "api_error", errorClass)
		return
	}
	writeRaw(w, status, final.result.ContentType, final.result.Body)
}

type streamContext struct {
	start       time.Time
	token       credentials.VerifiedLocalToken
	address     routing.ModelAddress
	instance    provider.Instance
	credentials []credentials.ResolvedAPIKeyCredential
	adapter     provider.ChatAdapter
	request     openai.ChatCompletionRequest
}

type streamAttempt struct {
	credential credentials.ResolvedAPIKeyCredential
	summary    provider.ChatStreamSummary
	err        error
}

type singleStreamContext struct {
	start      time.Time
	token      credentials.VerifiedLocalToken
	address    routing.ModelAddress
	instance   provider.Instance
	credential provider.BearerCredential
	adapter    provider.ChatAdapter
	request    openai.ChatCompletionRequest
}

type singleStreamAttempt struct {
	credential provider.BearerCredential
	summary    provider.ChatStreamSummary
	err        error
}

type singleChatAttempt struct {
	credential provider.BearerCredential
	result     provider.ChatResult
	err        error
}

func providerAPIKey(credential credentials.ResolvedAPIKeyCredential) provider.ChatCredential {
	return provider.ChatCredential{
		ID:                 credential.ID,
		ProviderInstanceID: credential.ProviderInstanceID,
		Kind:               provider.CredentialKindAPIKey,
		BearerToken:        credential.APIKey,
	}
}

func normalizedChatStatus(result provider.ChatResult) int {
	if result.StatusCode != 0 {
		return result.StatusCode
	}
	return http.StatusBadGateway
}

func localChatStatus(result provider.ChatResult, err error) int {
	if retryableChatAttempt(result, err) {
		return http.StatusBadGateway
	}
	return normalizedChatStatus(result)
}

func normalizedChatErrorClass(result provider.ChatResult, status int) string {
	if result.ErrorClass != "" {
		return result.ErrorClass
	}
	if status >= 400 {
		return "upstream_http_error"
	}
	return ""
}

func localChatErrorClass(result provider.ChatResult, err error, status int) string {
	if retryableChatAttempt(result, err) {
		return "upstream_unavailable"
	}
	return normalizedChatErrorClass(result, status)
}

func retryableChatAttempt(result provider.ChatResult, err error) bool {
	if result.InvalidBody || result.BodyTruncated {
		return false
	}
	errorClass := normalizedChatErrorClass(result, normalizedChatStatus(result))
	if errorClass == "upstream_network_error" || errorClass == "upstream_timeout" {
		return true
	}
	return retryableHTTPStatus(result.StatusCode)
}

func shouldRecordChatHealth(result provider.ChatResult) bool {
	return result.ErrorClass != "client_disconnected"
}

func resolvedChatModel(requestedModel, resultModel string) string {
	if resultModel != "" {
		return resultModel
	}
	return requestedModel
}

func retryableStreamAttempt(summary provider.ChatStreamSummary, err error, sinkStarted bool) bool {
	if err == nil || sinkStarted || summary.Started {
		return false
	}
	switch summary.ErrorClass {
	case "upstream_network_error", "upstream_timeout":
		return true
	case "upstream_http_error":
		return retryableHTTPStatus(summary.StatusCode)
	default:
		return false
	}
}

func retryableHTTPStatus(status int) bool {
	switch status {
	case http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}

func healthFromChatAttempt(addr routing.ModelAddress, attempt chatAttempt) metadata.HealthEvent {
	status := normalizedChatStatus(attempt.result)
	errorClass := normalizedChatErrorClass(attempt.result, status)
	eventClass := "upstream_failure"
	if attempt.err == nil && status >= 200 && status < 300 {
		eventClass = "upstream_success"
		errorClass = ""
	}
	retryAfter := attempt.result.RetryAfter
	if eventClass == "upstream_success" {
		retryAfter = nil
	}
	return metadata.HealthEvent{
		OccurredAt:         time.Now(),
		ProviderInstanceID: addr.ProviderInstanceID,
		CredentialID:       attempt.credential.ID,
		ModelID:            addr.ProviderModelID,
		EventClass:         eventClass,
		HTTPStatus:         status,
		ErrorClass:         errorClass,
		RetryAfter:         retryAfter,
	}
}

func healthFromStreamAttempt(addr routing.ModelAddress, attempt streamAttempt) metadata.HealthEvent {
	status := attempt.summary.StatusCode
	if status == 0 {
		status = http.StatusBadGateway
	}
	errorClass := attempt.summary.ErrorClass
	if errorClass == "" && status >= 400 {
		errorClass = "upstream_http_error"
	}
	eventClass := "upstream_failure"
	if attempt.err == nil && status >= 200 && status < 300 {
		eventClass = "upstream_success"
		errorClass = ""
	}
	retryAfter := attempt.summary.RetryAfter
	if eventClass == "upstream_success" {
		retryAfter = nil
	}
	return metadata.HealthEvent{
		OccurredAt:         time.Now(),
		ProviderInstanceID: addr.ProviderInstanceID,
		CredentialID:       attempt.credential.ID,
		ModelID:            addr.ProviderModelID,
		EventClass:         eventClass,
		HTTPStatus:         status,
		ErrorClass:         errorClass,
		RetryAfter:         retryAfter,
	}
}

func healthFromSingleChatAttempt(addr routing.ModelAddress, attempt singleChatAttempt) metadata.HealthEvent {
	status := normalizedChatStatus(attempt.result)
	errorClass := normalizedChatErrorClass(attempt.result, status)
	eventClass := "upstream_failure"
	if attempt.err == nil && status >= 200 && status < 300 {
		eventClass = "upstream_success"
		errorClass = ""
	}
	retryAfter := attempt.result.RetryAfter
	if eventClass == "upstream_success" {
		retryAfter = nil
	}
	return metadata.HealthEvent{
		OccurredAt:         time.Now(),
		ProviderInstanceID: addr.ProviderInstanceID,
		CredentialID:       attempt.credential.ID,
		ModelID:            addr.ProviderModelID,
		EventClass:         eventClass,
		HTTPStatus:         status,
		ErrorClass:         errorClass,
		RetryAfter:         retryAfter,
	}
}

func healthFromModelDiscovery(instance provider.Instance, credential provider.BearerCredential, result provider.ModelResult, err error) metadata.HealthEvent {
	status := result.StatusCode
	if status == 0 {
		status = http.StatusBadGateway
	}
	errorClass := result.ErrorClass
	if errorClass == "" && status >= 400 {
		errorClass = "upstream_http_error"
	}
	eventClass := "upstream_failure"
	if err == nil && len(result.Models) > 0 && status >= 200 && status < 300 {
		eventClass = "upstream_success"
		errorClass = ""
	}
	retryAfter := result.RetryAfter
	if eventClass == "upstream_success" {
		retryAfter = nil
	}
	return metadata.HealthEvent{
		OccurredAt:         time.Now(),
		ProviderInstanceID: instance.ID,
		CredentialID:       credential.ID,
		ModelID:            "",
		EventClass:         eventClass,
		HTTPStatus:         status,
		ErrorClass:         errorClass,
		RetryAfter:         retryAfter,
	}
}

func healthFromSingleStreamAttempt(addr routing.ModelAddress, attempt singleStreamAttempt) metadata.HealthEvent {
	status := attempt.summary.StatusCode
	if status == 0 {
		status = http.StatusBadGateway
	}
	errorClass := attempt.summary.ErrorClass
	if errorClass == "" && status >= 400 {
		errorClass = "upstream_http_error"
	}
	eventClass := "upstream_failure"
	if attempt.err == nil && status >= 200 && status < 300 {
		eventClass = "upstream_success"
		errorClass = ""
	}
	retryAfter := attempt.summary.RetryAfter
	if eventClass == "upstream_success" {
		retryAfter = nil
	}
	return metadata.HealthEvent{
		OccurredAt:         time.Now(),
		ProviderInstanceID: addr.ProviderInstanceID,
		CredentialID:       attempt.credential.ID,
		ModelID:            addr.ProviderModelID,
		EventClass:         eventClass,
		HTTPStatus:         status,
		ErrorClass:         errorClass,
		RetryAfter:         retryAfter,
	}
}

func shouldRecordStreamHealth(summary provider.ChatStreamSummary) bool {
	return summary.ErrorClass != "client_disconnected" && summary.CompletionStatus != "client_disconnected"
}

func fallbackReason(events []metadata.FallbackEvent) string {
	if len(events) == 0 {
		return ""
	}
	return events[0].Reason
}

func (s *Server) handleSingleCredentialStreamingChat(w http.ResponseWriter, r *http.Request, sc singleStreamContext) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		_ = s.record(r.Context(), metadata.Request{
			StartedAt:                 sc.start,
			ClientTokenID:             sc.token.ID,
			RequestedProviderInstance: sc.address.ProviderInstanceID,
			RequestedModel:            sc.address.ProviderModelID,
			ResolvedProviderInstance:  sc.address.ProviderInstanceID,
			ResolvedModel:             sc.address.ProviderModelID,
			HTTPStatus:                http.StatusInternalServerError,
			ErrorClass:                "client_stream_unavailable",
			TotalLatencyMS:            time.Since(sc.start).Milliseconds(),
		})
		writeError(w, http.StatusInternalServerError, "streaming is not available for this response writer", "api_error", "client_stream_unavailable")
		return
	}
	sink := &streamSink{w: w, flusher: flusher}
	summary, err := sc.adapter.StreamChat(r.Context(), provider.ChatRequest{
		Instance:      sc.instance,
		UpstreamModel: sc.address.ProviderModelID,
		Request:       sc.request,
		Credential: provider.ChatCredential{
			ID:                 sc.credential.ID,
			ProviderInstanceID: sc.credential.ProviderInstanceID,
			Kind:               sc.credential.Kind,
			BearerToken:        sc.credential.BearerToken,
		},
	}, sink)
	retryCount := 0
	if s.shouldRefreshOAuthAfterStream401(sc.instance, summary) {
		refreshed, refreshErr := s.refreshOAuthCredentialForRetryIfBearer(r.Context(), sc.credential)
		if refreshErr != nil {
			summary = provider.ChatStreamSummary{StatusCode: http.StatusBadGateway, ErrorClass: "upstream_auth_failed", CompletionStatus: "upstream_error", PreStreamError: true}
			err = refreshErr
		} else {
			sc.credential = refreshed
			retryCount = 1
			summary, err = sc.adapter.StreamChat(r.Context(), provider.ChatRequest{
				Instance:      sc.instance,
				UpstreamModel: sc.address.ProviderModelID,
				Request:       sc.request,
				Credential: provider.ChatCredential{
					ID:                 sc.credential.ID,
					ProviderInstanceID: sc.credential.ProviderInstanceID,
					Kind:               sc.credential.Kind,
					BearerToken:        sc.credential.BearerToken,
				},
			}, sink)
			if s.shouldRefreshOAuthAfterStream401(sc.instance, summary) {
				summary.StatusCode = http.StatusBadGateway
				summary.ErrorClass = "upstream_auth_failed"
			}
		}
	}
	final := singleStreamAttempt{credential: sc.credential, summary: summary, err: err}
	if shouldRecordStreamHealth(summary) {
		s.recordHealth(r.Context(), healthFromSingleStreamAttempt(sc.address, final))
	}
	if final.err != nil && !sink.started {
		localStatus := summary.StatusCode
		if localStatus < 400 || localStatus >= 500 {
			localStatus = http.StatusBadGateway
		}
		summary.StatusCode = localStatus
		errorCode := summary.ErrorClass
		if errorCode == "" {
			errorCode = "upstream_stream_error"
		}
		writeError(w, localStatus, "upstream stream failed", "api_error", errorCode)
	}
	recordCtx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 5*time.Second)
	defer cancel()
	status := summary.StatusCode
	if status == 0 {
		if sink.started {
			status = http.StatusOK
		} else {
			status = http.StatusBadGateway
		}
	}
	errorClass := summary.ErrorClass
	if errorClass == "" && status >= 400 {
		errorClass = "upstream_http_error"
	}
	requestID, _ := s.recordWithID(recordCtx, metadata.Request{
		StartedAt:                 sc.start,
		ClientTokenID:             sc.token.ID,
		CredentialID:              sc.credential.ID,
		RequestedProviderInstance: sc.address.ProviderInstanceID,
		RequestedModel:            sc.address.ProviderModelID,
		ResolvedProviderInstance:  sc.address.ProviderInstanceID,
		ResolvedModel:             resolvedChatModel(sc.address.ProviderModelID, summary.ResolvedModel),
		HTTPStatus:                status,
		ErrorClass:                errorClass,
		RetryCount:                retryCount,
		PromptTokens:              summary.Usage.PromptTokens,
		CompletionTokens:          summary.Usage.CompletionTokens,
		TotalTokens:               summary.Usage.TotalTokens,
		ReasoningTokens:           summary.Usage.ReasoningTokens,
		CacheHitTokens:            summary.Usage.CachedTokens,
		CacheWriteTokens:          summary.Usage.CacheWriteTokens,
		CostMicrounits:            summary.Usage.CostMicrounits,
		TotalLatencyMS:            time.Since(sc.start).Milliseconds(),
		TimeToFirstTokenMS:        summary.TimeToFirstTokenMS,
		OutputTokensPerSecond:     summary.OutputTokensPerSecond,
	})
	completionStatus := summary.CompletionStatus
	if completionStatus == "" {
		completionStatus = "upstream_invalid"
	}
	_ = s.recordStream(recordCtx, metadata.Stream{
		RequestMetadataID:     requestID,
		TimeToFirstTokenMS:    summary.TimeToFirstTokenMS,
		OutputTokensPerSecond: summary.OutputTokensPerSecond,
		CompletionStatus:      completionStatus,
		ChunkCount:            summary.ChunkCount,
	})
}

func (s *Server) handleStreamingChat(w http.ResponseWriter, r *http.Request, sc streamContext) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		_ = s.record(r.Context(), metadata.Request{
			StartedAt:                 sc.start,
			ClientTokenID:             sc.token.ID,
			RequestedProviderInstance: sc.address.ProviderInstanceID,
			RequestedModel:            sc.address.ProviderModelID,
			ResolvedProviderInstance:  sc.address.ProviderInstanceID,
			ResolvedModel:             sc.address.ProviderModelID,
			HTTPStatus:                http.StatusInternalServerError,
			ErrorClass:                "client_stream_unavailable",
			TotalLatencyMS:            time.Since(sc.start).Milliseconds(),
		})
		writeError(w, http.StatusInternalServerError, "streaming is not available for this response writer", "api_error", "client_stream_unavailable")
		return
	}
	sink := &streamSink{w: w, flusher: flusher}
	var final streamAttempt
	var fallbackEvents []metadata.FallbackEvent
	for i, credential := range sc.credentials {
		summary, err := sc.adapter.StreamChat(r.Context(), provider.ChatRequest{
			Instance:      sc.instance,
			UpstreamModel: sc.address.ProviderModelID,
			Request:       sc.request,
			Credential:    providerAPIKey(credential),
		}, sink)
		final = streamAttempt{credential: credential, summary: summary, err: err}
		if shouldRecordStreamHealth(summary) {
			s.recordHealth(r.Context(), healthFromStreamAttempt(sc.address, final))
		}
		if !retryableStreamAttempt(summary, err, sink.started) || i == len(sc.credentials)-1 {
			break
		}
		next := sc.credentials[i+1]
		fallbackEvents = append(fallbackEvents, metadata.FallbackEvent{
			OccurredAt:         time.Now(),
			ProviderInstanceID: sc.address.ProviderInstanceID,
			ModelID:            sc.address.ProviderModelID,
			FromCredentialID:   credential.ID,
			ToCredentialID:     next.ID,
			Reason:             "availability_retry",
			AllowedByPolicy:    true,
		})
	}
	summary := final.summary
	if final.err != nil && !sink.started {
		localStatus := summary.StatusCode
		if localStatus < 400 || localStatus >= 500 {
			localStatus = http.StatusBadGateway
		}
		summary.StatusCode = localStatus
		writeError(w, localStatus, "upstream stream failed", "api_error", "upstream_stream_error")
	}
	recordCtx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 5*time.Second)
	defer cancel()
	status := summary.StatusCode
	if status == 0 {
		if sink.started {
			status = http.StatusOK
		} else {
			status = http.StatusBadGateway
		}
	}
	errorClass := summary.ErrorClass
	if errorClass == "" && status >= 400 {
		errorClass = "upstream_http_error"
	}
	requestID, _ := s.recordWithID(recordCtx, metadata.Request{
		StartedAt:                 sc.start,
		ClientTokenID:             sc.token.ID,
		CredentialID:              final.credential.ID,
		RequestedProviderInstance: sc.address.ProviderInstanceID,
		RequestedModel:            sc.address.ProviderModelID,
		ResolvedProviderInstance:  sc.address.ProviderInstanceID,
		ResolvedModel:             resolvedChatModel(sc.address.ProviderModelID, summary.ResolvedModel),
		HTTPStatus:                status,
		ErrorClass:                errorClass,
		RetryCount:                len(fallbackEvents),
		FallbackCount:             len(fallbackEvents),
		FallbackReason:            fallbackReason(fallbackEvents),
		PromptTokens:              summary.Usage.PromptTokens,
		CompletionTokens:          summary.Usage.CompletionTokens,
		TotalTokens:               summary.Usage.TotalTokens,
		ReasoningTokens:           summary.Usage.ReasoningTokens,
		CacheHitTokens:            summary.Usage.CachedTokens,
		CacheWriteTokens:          summary.Usage.CacheWriteTokens,
		CostMicrounits:            summary.Usage.CostMicrounits,
		TotalLatencyMS:            time.Since(sc.start).Milliseconds(),
		TimeToFirstTokenMS:        summary.TimeToFirstTokenMS,
		OutputTokensPerSecond:     summary.OutputTokensPerSecond,
	})
	completionStatus := summary.CompletionStatus
	if completionStatus == "" {
		completionStatus = "upstream_invalid"
	}
	_ = s.recordStream(recordCtx, metadata.Stream{
		RequestMetadataID:     requestID,
		TimeToFirstTokenMS:    summary.TimeToFirstTokenMS,
		OutputTokensPerSecond: summary.OutputTokensPerSecond,
		CompletionStatus:      completionStatus,
		ChunkCount:            summary.ChunkCount,
	})
	s.recordFallbacks(recordCtx, requestID, fallbackEvents)
}

type streamSink struct {
	w       http.ResponseWriter
	flusher http.Flusher
	started bool
}

func (s *streamSink) WriteEvent(_ context.Context, event provider.ChatStreamEvent) error {
	s.start()
	if _, err := s.w.Write([]byte("data: ")); err != nil {
		return err
	}
	if _, err := s.w.Write(event.Data); err != nil {
		return err
	}
	if _, err := s.w.Write([]byte("\n\n")); err != nil {
		return err
	}
	s.flusher.Flush()
	return nil
}

func (s *streamSink) WriteDone(_ context.Context) error {
	s.start()
	if _, err := s.w.Write([]byte("data: [DONE]\n\n")); err != nil {
		return err
	}
	s.flusher.Flush()
	return nil
}

func (s *streamSink) start() {
	if s.started {
		return
	}
	header := s.w.Header()
	header.Set("Content-Type", "text/event-stream")
	header.Set("Cache-Control", "no-cache")
	header.Set("Connection", "keep-alive")
	s.w.WriteHeader(http.StatusOK)
	s.started = true
}

func (s *Server) record(ctx context.Context, m metadata.Request) error {
	if s.meta == nil {
		return nil
	}
	_, err := s.meta.RecordRequestMetadata(ctx, m)
	return err
}

func (s *Server) recordWithID(ctx context.Context, m metadata.Request) (int64, error) {
	if s.meta == nil {
		return 0, nil
	}
	return s.meta.RecordRequestMetadata(ctx, m)
}

func (s *Server) recordStream(ctx context.Context, m metadata.Stream) error {
	if s.meta == nil || m.RequestMetadataID == 0 {
		return nil
	}
	return s.meta.RecordStreamMetrics(ctx, m)
}

func (s *Server) recordHealth(ctx context.Context, m metadata.HealthEvent) error {
	if s.meta == nil {
		return nil
	}
	return s.meta.RecordHealthEvent(ctx, m)
}

func (s *Server) recordFallbacks(ctx context.Context, requestID int64, events []metadata.FallbackEvent) {
	if s.meta == nil || requestID == 0 {
		return
	}
	for _, event := range events {
		event.RequestMetadataID = requestID
		_ = s.meta.RecordFallbackEvent(ctx, event)
	}
}

func writeRaw(w http.ResponseWriter, status int, contentType string, body []byte) {
	if contentType == "" {
		contentType = "application/json"
	}
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

func writeError(w http.ResponseWriter, status int, message, typ, code string) {
	writeJSON(w, status, openai.Error(message, typ, code))
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil && !errors.Is(err, http.ErrHandlerTimeout) {
		return
	}
}
