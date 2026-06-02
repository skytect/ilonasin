package app

import (
	"context"
	"encoding/json"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"ilonasin/internal/config"
	"ilonasin/internal/credentials"
	"ilonasin/internal/openai"
	"ilonasin/internal/provider"
)

const keepalivePrompt = "Reply exactly: ok"

type keepaliveRunner struct {
	cfg       config.SubscriptionKeepaliveConfig
	registry  provider.Registry
	resolver  credentials.OAuthBearerResolver
	usage     provider.CodexSubscriptionUsageClient
	adapter   provider.ChatAdapter
	logger    *slog.Logger
	now       func() time.Time
	completed map[string]bool
	mu        sync.Mutex
}

func startSubscriptionKeepalive(ctx context.Context, cfg config.SubscriptionKeepaliveConfig, registry provider.Registry, resolver credentials.OAuthBearerResolver, usage provider.CodexSubscriptionUsageClient, adapter provider.ChatAdapter, logger *slog.Logger) func() {
	if !cfg.Enabled || resolver == nil || adapter == nil {
		return func() {}
	}
	ctx, cancel := context.WithCancel(ctx)
	runner := &keepaliveRunner{
		cfg:       cfg,
		registry:  registry,
		resolver:  resolver,
		usage:     usage,
		adapter:   adapter,
		logger:    logger,
		now:       time.Now,
		completed: map[string]bool{},
	}
	go runner.loop(ctx)
	return cancel
}

func (r *keepaliveRunner) loop(ctx context.Context) {
	r.runDue(ctx)
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.runDue(ctx)
		}
	}
}

func (r *keepaliveRunner) runDue(ctx context.Context) {
	now := r.now()
	slot := keepaliveSlot(now, r.cfg.ScheduleTimes)
	if slot == "" {
		return
	}
	for _, instance := range r.registry.List() {
		if instance.Type != "codex" || !instance.OAuth {
			continue
		}
		bearers, err := r.resolver.ResolveOAuthBearers(ctx, instance.ID, now.UTC())
		if err != nil {
			r.log(ctx, slog.LevelWarn, "subscription_keepalive_resolve_failed",
				slog.String("provider_instance", instance.ID),
				slog.String("error_class", "credential_unavailable"),
			)
			continue
		}
		for _, bearer := range bearers {
			r.runCredential(ctx, now, slot, instance, bearer)
		}
	}
}

func (r *keepaliveRunner) runCredential(ctx context.Context, now time.Time, slot string, instance provider.Instance, bearer credentials.ResolvedOAuthBearerCredential) {
	key := now.Format("2006-01-02") + "\x00" + slot + "\x00" + instance.ID + "\x00" + int64Key(bearer.ID)
	r.mu.Lock()
	if r.completed[key] {
		r.mu.Unlock()
		return
	}
	r.mu.Unlock()

	req := keepaliveRequest(r.cfg.Model)
	credential := provider.ChatCredential{
		ID:                      bearer.ID,
		ProviderInstanceID:      bearer.ProviderInstanceID,
		Kind:                    provider.CredentialKindOAuthAccess,
		BearerToken:             bearer.BearerToken,
		ChatGPTAccountID:        bearer.ChatGPTAccountID,
		ChatGPTAccountIsFedRAMP: bearer.ChatGPTAccountIsFedRAMP,
	}
	result, err := r.adapter.CompleteChat(ctx, provider.ChatRequest{
		Instance:      instance,
		UpstreamModel: req.Model,
		Request:       req,
		Credential:    credential,
	})
	if err != nil {
		r.log(ctx, slog.LevelWarn, "subscription_keepalive_failed",
			slog.String("provider_instance", instance.ID),
			slog.Int64("credential_id", bearer.ID),
			slog.Int("status", result.StatusCode),
			slog.String("error_class", firstNonEmpty(result.ErrorClass, "upstream_request_failed")),
		)
		return
	}
	r.mu.Lock()
	r.completed[key] = true
	r.mu.Unlock()
	r.log(ctx, slog.LevelInfo, "subscription_keepalive_completed",
		slog.String("provider_instance", instance.ID),
		slog.Int64("credential_id", bearer.ID),
		slog.Int("status", result.StatusCode),
		slog.Int("output_tokens", result.Usage.CompletionTokens),
	)
	r.refreshUsage(ctx, instance, bearer)
}

func keepaliveRequest(model string) openai.ChatCompletionRequest {
	model = strings.TrimSpace(model)
	if model == "" {
		model = "gpt-5.5"
	}
	content, _ := json.Marshal(keepalivePrompt)
	return openai.ChatCompletionRequest{
		Model: model,
		Messages: []openai.Message{{
			Role:    "user",
			Content: content,
		}},
		ReasoningOptions: map[string]any{
			"codex": map[string]any{
				"reasoning": map[string]any{"effort": "minimal"},
				"verbosity": "low",
			},
		},
		PresentFields: map[string]bool{
			"provider_options": true,
		},
	}
}

func (r *keepaliveRunner) refreshUsage(ctx context.Context, instance provider.Instance, bearer credentials.ResolvedOAuthBearerCredential) {
	if r.usage == nil {
		return
	}
	_, _ = r.usage.FetchCodexSubscriptionUsage(ctx, provider.CodexSubscriptionUsageRequest{
		Instance: instance,
		Credential: provider.BearerCredential{
			ID:                      bearer.ID,
			ProviderInstanceID:      bearer.ProviderInstanceID,
			Kind:                    provider.CredentialKindOAuthAccess,
			BearerToken:             bearer.BearerToken,
			ChatGPTAccountID:        bearer.ChatGPTAccountID,
			ChatGPTAccountIsFedRAMP: bearer.ChatGPTAccountIsFedRAMP,
		},
	})
}

func keepaliveSlot(now time.Time, schedule []string) string {
	current := now.Format("15:04")
	for _, value := range config.NormalizeSubscriptionKeepaliveTimes(schedule) {
		if value == current {
			return current
		}
	}
	return ""
}

func int64Key(value int64) string {
	return strconv.FormatInt(value, 10)
}

func (r *keepaliveRunner) log(ctx context.Context, level slog.Level, event string, attrs ...slog.Attr) {
	if r.logger == nil {
		return
	}
	attrs = append([]slog.Attr{slog.String("event", event)}, attrs...)
	r.logger.LogAttrs(ctx, level, event, attrs...)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}
