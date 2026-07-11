package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sync"
	"time"
)

const (
	// Mirrors OpenAI Codex rust-v0.135.0 at the time this compatibility layer
	// was introduced. It remains the safe fallback when live version discovery is
	// unavailable.
	CodexClientVersion = "0.135.0"

	DefaultCodexVersionRegistryURL     = "https://registry.npmjs.org/@openai/codex/latest"
	DefaultCodexVersionCacheTTL        = time.Hour
	DefaultCodexVersionFailureCacheTTL = 10 * time.Minute
	DefaultCodexVersionFetchTimeout    = 5 * time.Second
	MaxCodexVersionBodyBytes           = 1 << 20
)

var stableSemverRE = regexp.MustCompile(`^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$`)

type CodexClientVersionResolver interface {
	Version(ctx context.Context) string
}

type CachedCodexClientVersionResolver struct {
	Client          *http.Client
	RegistryURL     string
	FallbackVersion string
	TTL             time.Duration
	FailureTTL      time.Duration
	FetchTimeout    time.Duration
	MaxBodyBytes    int64
	Now             func() time.Time

	mu        sync.Mutex
	cached    string
	expires   time.Time
	fetching  bool
	fetchDone chan struct{}
}

func NewCachedCodexClientVersionResolver(client *http.Client) *CachedCodexClientVersionResolver {
	if client == nil {
		client = &http.Client{Timeout: DefaultCodexVersionFetchTimeout}
	} else if client.Timeout == 0 {
		clone := *client
		clone.Timeout = DefaultCodexVersionFetchTimeout
		client = &clone
	}
	return &CachedCodexClientVersionResolver{
		Client:          client,
		RegistryURL:     DefaultCodexVersionRegistryURL,
		FallbackVersion: CodexClientVersion,
		TTL:             DefaultCodexVersionCacheTTL,
		FailureTTL:      DefaultCodexVersionFailureCacheTTL,
		FetchTimeout:    DefaultCodexVersionFetchTimeout,
		MaxBodyBytes:    MaxCodexVersionBodyBytes,
		Now:             time.Now,
	}
}

func (r *CachedCodexClientVersionResolver) Version(ctx context.Context) string {
	if r == nil {
		return CodexClientVersion
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if ctx.Err() != nil {
		return r.fallback()
	}
	for {
		now := r.now()
		r.mu.Lock()
		if r.cached != "" && now.Before(r.expires) {
			version := r.cached
			r.mu.Unlock()
			return version
		}
		if r.fetching {
			done := r.fetchDone
			r.mu.Unlock()
			select {
			case <-done:
				continue
			case <-ctx.Done():
				return r.fallback()
			}
		}
		done := make(chan struct{})
		r.fetching = true
		r.fetchDone = done
		r.mu.Unlock()
		return r.fetchAndCache(done)
	}
}

func (r *CachedCodexClientVersionResolver) fetchAndCache(done chan struct{}) string {
	fetchCtx, cancel := context.WithTimeout(context.Background(), r.fetchTimeout())
	defer cancel()

	version, err := r.fetch(fetchCtx)
	now := r.now()
	cacheVersion := version
	expires := now.Add(r.ttl())
	if err != nil {
		cacheVersion = r.fallback()
		expires = now.Add(r.failureTTL())
	}

	r.mu.Lock()
	r.cached = cacheVersion
	r.expires = expires
	r.fetching = false
	if r.fetchDone == done {
		r.fetchDone = nil
	}
	close(done)
	r.mu.Unlock()
	return cacheVersion
}

func (r *CachedCodexClientVersionResolver) fetch(ctx context.Context) (string, error) {
	url := r.RegistryURL
	if url == "" {
		url = DefaultCodexVersionRegistryURL
	}
	client := r.Client
	if client == nil {
		client = http.DefaultClient
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("codex version registry status %d", resp.StatusCode)
	}
	limit := r.MaxBodyBytes
	if limit <= 0 {
		limit = MaxCodexVersionBodyBytes
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return "", err
	}
	if int64(len(body)) > limit {
		return "", errors.New("codex version registry body exceeded limit")
	}
	var payload struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", err
	}
	if !validStableCodexClientVersion(payload.Version) {
		return "", fmt.Errorf("invalid codex version %q", payload.Version)
	}
	return payload.Version, nil
}

func (r *CachedCodexClientVersionResolver) fallback() string {
	if r.FallbackVersion != "" && validStableCodexClientVersion(r.FallbackVersion) {
		return r.FallbackVersion
	}
	return CodexClientVersion
}

func (r *CachedCodexClientVersionResolver) ttl() time.Duration {
	if r.TTL > 0 {
		return r.TTL
	}
	return DefaultCodexVersionCacheTTL
}

func (r *CachedCodexClientVersionResolver) failureTTL() time.Duration {
	if r.FailureTTL > 0 {
		return r.FailureTTL
	}
	return DefaultCodexVersionFailureCacheTTL
}

func (r *CachedCodexClientVersionResolver) fetchTimeout() time.Duration {
	if r.FetchTimeout > 0 {
		return r.FetchTimeout
	}
	return DefaultCodexVersionFetchTimeout
}

func (r *CachedCodexClientVersionResolver) now() time.Time {
	if r.Now != nil {
		return r.Now()
	}
	return time.Now()
}

func validStableCodexClientVersion(version string) bool {
	return stableSemverRE.MatchString(version)
}
