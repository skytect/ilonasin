package app

import (
	"context"
	"encoding/json"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"ilonasin/internal/credentials"
	"ilonasin/internal/openai"
)

const keepalivePrompt = "Reply exactly: ok"

type keepaliveRunner struct {
	settings  subscriptionKeepaliveSettings
	registry  keepaliveProviderRegistry
	resolver  credentials.OAuthBearerResolver
	usage     keepaliveUsageClient
	adapter   keepaliveChatClient
	logger    *slog.Logger
	now       func() time.Time
	completed map[string]bool
	mu        sync.Mutex
}

type subscriptionKeepaliveSettings struct {
	Enabled           bool
	ScheduleTimes     []string
	Model             string
	MaxOutputTokens   int
	OutputCapVerified bool
}

func startSubscriptionKeepalive(ctx context.Context, settings subscriptionKeepaliveSettings, registry keepaliveProviderRegistry, resolver credentials.OAuthBearerResolver, usage keepaliveUsageClient, adapter keepaliveChatClient, logger *slog.Logger) func() {
	if !settings.Enabled {
		return func() {}
	}
	if !settings.OutputCapVerified {
		logKeepaliveUnavailable(ctx, logger)
		return func() {}
	}
	if resolver == nil || adapter == nil {
		return func() {}
	}
	ctx, cancel := context.WithCancel(ctx)
	runner := &keepaliveRunner{
		settings:  settings,
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
	if !r.settings.OutputCapVerified {
		return
	}
	now := r.now()
	slot := keepaliveSlot(now, r.settings.ScheduleTimes)
	if slot == "" {
		return
	}
	for _, instance := range r.registry.List() {
		if !supportsCodexOAuthKeepalive(instance) {
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

func (r *keepaliveRunner) runCredential(ctx context.Context, now time.Time, slot string, instance keepaliveProvider, bearer credentials.ResolvedOAuthBearerCredential) {
	key := now.Format("2006-01-02") + "\x00" + slot + "\x00" + instance.ID + "\x00" + int64Key(bearer.ID)
	r.mu.Lock()
	if r.completed[key] {
		r.mu.Unlock()
		return
	}
	r.mu.Unlock()

	req := keepaliveRequest(r.settings.Model, r.settings.MaxOutputTokens)
	result, err := r.adapter.CompleteKeepaliveChat(ctx, keepaliveChatRequest{
		Provider:      instance,
		UpstreamModel: req.Model,
		Request:       req,
		Credential: keepaliveCredential{
			ID:                      bearer.ID,
			ProviderInstanceID:      bearer.ProviderInstanceID,
			BearerToken:             bearer.BearerToken,
			ChatGPTAccountID:        bearer.ChatGPTAccountID,
			ChatGPTAccountIsFedRAMP: bearer.ChatGPTAccountIsFedRAMP,
		},
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

func keepaliveRequest(model string, maxOutputTokens int) openai.ChatCompletionRequest {
	model = strings.TrimSpace(model)
	if model == "" {
		model = "gpt-5.5"
	}
	if maxOutputTokens <= 0 {
		maxOutputTokens = 1
	}
	content, _ := json.Marshal(keepalivePrompt)
	return openai.ChatCompletionRequest{
		Model:     model,
		MaxTokens: &maxOutputTokens,
		PresentFields: map[string]bool{
			"max_tokens":       true,
			"provider_options": true,
		},
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
	}
}

func (r *keepaliveRunner) refreshUsage(ctx context.Context, instance keepaliveProvider, bearer credentials.ResolvedOAuthBearerCredential) {
	if r.usage == nil {
		return
	}
	_ = r.usage.RefreshKeepaliveUsage(ctx, instance, keepaliveCredential{
		ID:                      bearer.ID,
		ProviderInstanceID:      bearer.ProviderInstanceID,
		BearerToken:             bearer.BearerToken,
		ChatGPTAccountID:        bearer.ChatGPTAccountID,
		ChatGPTAccountIsFedRAMP: bearer.ChatGPTAccountIsFedRAMP,
	})
}

func keepaliveSlot(now time.Time, schedule []string) string {
	current := now.Format("15:04")
	for _, value := range schedule {
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

func logKeepaliveUnavailable(ctx context.Context, logger *slog.Logger) {
	if logger == nil {
		return
	}
	logger.LogAttrs(ctx, slog.LevelWarn, "subscription_keepalive_unavailable",
		slog.String("event", "subscription_keepalive_unavailable"),
		slog.String("status", "unavailable_output_cap_unverified"),
	)
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
