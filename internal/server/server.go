package server

import (
	"log/slog"
	"time"

	"ilonasin/internal/credentials"
	"ilonasin/internal/provider"
)

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
	quota     QuotaReader
	logger    *slog.Logger
	now       func() time.Time
}

func New(registry ProviderRegistry, auth credentials.LocalTokenVerifier, upstreams credentials.UpstreamCredentialResolver, oauth credentials.OAuthBearerResolver, adapters provider.ChatAdapters, models provider.ModelDiscoverers, cache ModelCache, meta MetadataRecorder) *Server {
	return NewWithClock(registry, auth, upstreams, oauth, adapters, models, cache, meta, time.Now)
}

func NewWithClock(registry ProviderRegistry, auth credentials.LocalTokenVerifier, upstreams credentials.UpstreamCredentialResolver, oauth credentials.OAuthBearerResolver, adapters provider.ChatAdapters, models provider.ModelDiscoverers, cache ModelCache, meta MetadataRecorder, now func() time.Time) *Server {
	if now == nil {
		now = time.Now
	}
	refresh, _ := oauth.(credentials.OAuthProviderRefreshController)
	quota, _ := meta.(QuotaReader)
	return &Server{registry: registry, auth: auth, upstreams: upstreams, oauth: oauth, refresh: refresh, adapters: adapters, models: models, cache: cache, meta: meta, quota: quota, now: now}
}

func (s *Server) WithLogger(logger *slog.Logger) *Server {
	s.logger = logger
	return s
}
