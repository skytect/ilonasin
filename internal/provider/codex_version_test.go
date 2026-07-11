package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestCachedCodexVersionResolverFetchesAndCachesLatestNPMVersion(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		if r.URL.Path != "/@openai/codex/latest" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"version":"0.144.1"}`))
	}))
	defer server.Close()

	resolver := NewCachedCodexClientVersionResolver(server.Client())
	resolver.RegistryURL = server.URL + "/@openai/codex/latest"
	resolver.TTL = time.Hour

	first := resolver.Version(context.Background())
	second := resolver.Version(context.Background())

	if first != "0.144.1" || second != "0.144.1" {
		t.Fatalf("expected cached npm version, got first=%q second=%q", first, second)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("expected one registry call, got %d", got)
	}
}

func TestCachedCodexVersionResolverCoalescesConcurrentMisses(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		time.Sleep(50 * time.Millisecond)
		_, _ = w.Write([]byte(`{"version":"0.144.1"}`))
	}))
	defer server.Close()

	resolver := NewCachedCodexClientVersionResolver(server.Client())
	resolver.RegistryURL = server.URL

	const workers = 32
	var wg sync.WaitGroup
	versions := make(chan string, workers)
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			versions <- resolver.Version(context.Background())
		}()
	}
	wg.Wait()
	close(versions)

	for version := range versions {
		if version != "0.144.1" {
			t.Fatalf("expected concurrent caller to receive fetched version, got %q", version)
		}
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("expected one registry call for concurrent miss, got %d", got)
	}
}

func TestCachedCodexVersionResolverFallsBackOnFetchOrInvalidVersion(t *testing.T) {
	for name, body := range map[string]string{
		"http_error":      ``,
		"invalid_json":    `{`,
		"missing_version": `{"name":"@openai/codex"}`,
		"invalid_version": `{"version":"latest"}`,
		"prerelease":      `{"version":"0.145.0-alpha.4"}`,
		"leading_zero":    `{"version":"01.2.3"}`,
	} {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if name == "http_error" {
					http.Error(w, "boom", http.StatusBadGateway)
					return
				}
				_, _ = w.Write([]byte(body))
			}))
			defer server.Close()

			resolver := NewCachedCodexClientVersionResolver(server.Client())
			resolver.RegistryURL = server.URL
			resolver.FallbackVersion = "0.135.0"

			if got := resolver.Version(context.Background()); got != "0.135.0" {
				t.Fatalf("expected fallback version, got %q", got)
			}
		})
	}
}

func TestCachedCodexVersionResolverFailureCacheExpires(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch atomic.AddInt32(&calls, 1) {
		case 1:
			http.Error(w, "boom", http.StatusBadGateway)
		default:
			_, _ = w.Write([]byte(`{"version":"0.144.1"}`))
		}
	}))
	defer server.Close()

	now := time.Date(2026, 7, 11, 0, 0, 0, 0, time.UTC)
	resolver := NewCachedCodexClientVersionResolver(server.Client())
	resolver.RegistryURL = server.URL
	resolver.FailureTTL = time.Minute
	resolver.Now = func() time.Time { return now }

	if got := resolver.Version(context.Background()); got != CodexClientVersion {
		t.Fatalf("expected fallback on first failure, got %q", got)
	}
	if got := resolver.Version(context.Background()); got != CodexClientVersion {
		t.Fatalf("expected cached fallback before failure TTL expiry, got %q", got)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("expected failed fetch cached before expiry, got %d calls", got)
	}

	now = now.Add(time.Minute + time.Nanosecond)
	if got := resolver.Version(context.Background()); got != "0.144.1" {
		t.Fatalf("expected retry after failure TTL expiry, got %q", got)
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Fatalf("expected retry after failure TTL expiry, got %d calls", got)
	}
}

func TestCachedCodexVersionResolverSuccessCacheExpires(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call := atomic.AddInt32(&calls, 1)
		_, _ = w.Write([]byte(fmt.Sprintf(`{"version":"0.144.%d"}`, call)))
	}))
	defer server.Close()

	now := time.Date(2026, 7, 11, 0, 0, 0, 0, time.UTC)
	resolver := NewCachedCodexClientVersionResolver(server.Client())
	resolver.RegistryURL = server.URL
	resolver.TTL = time.Minute
	resolver.Now = func() time.Time { return now }

	if got := resolver.Version(context.Background()); got != "0.144.1" {
		t.Fatalf("expected first fetched version, got %q", got)
	}
	if got := resolver.Version(context.Background()); got != "0.144.1" {
		t.Fatalf("expected cached success before TTL expiry, got %q", got)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("expected success cached before expiry, got %d calls", got)
	}

	now = now.Add(time.Minute + time.Nanosecond)
	if got := resolver.Version(context.Background()); got != "0.144.2" {
		t.Fatalf("expected refresh after success TTL expiry, got %q", got)
	}
}

func TestCachedCodexVersionResolverCancelledContextDoesNotPoisonCache(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		_, _ = w.Write([]byte(`{"version":"0.144.1"}`))
	}))
	defer server.Close()

	resolver := NewCachedCodexClientVersionResolver(server.Client())
	resolver.RegistryURL = server.URL

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if got := resolver.Version(ctx); got != CodexClientVersion {
		t.Fatalf("expected fallback for canceled caller, got %q", got)
	}
	if got := atomic.LoadInt32(&calls); got != 0 {
		t.Fatalf("expected canceled caller not to fetch, got %d calls", got)
	}
	if got := resolver.Version(context.Background()); got != "0.144.1" {
		t.Fatalf("expected later healthy caller to fetch latest version, got %q", got)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("expected exactly one healthy fetch, got %d calls", got)
	}
}

func TestCachedCodexVersionResolverBodyLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"version":"0.144.1"}`))
	}))
	defer server.Close()

	resolver := NewCachedCodexClientVersionResolver(server.Client())
	resolver.RegistryURL = server.URL
	resolver.MaxBodyBytes = 4

	if got := resolver.Version(context.Background()); got != CodexClientVersion {
		t.Fatalf("expected fallback when registry body exceeds limit, got %q", got)
	}
}

func TestCodexModelsURLUsesResolvedClientVersion(t *testing.T) {
	adapter := NewHTTPChatAdapter(nil)
	adapter.CodexVersionResolver = staticCodexClientVersionResolver("0.144.1")

	endpoint, err := adapter.modelsURL(context.Background(), Instance{Type: "codex", BaseURL: "https://chatgpt.com/backend-api/codex"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(endpoint, "client_version=0.144.1") {
		t.Fatalf("expected resolved client_version in endpoint, got %s", endpoint)
	}
}

type staticCodexClientVersionResolver string

func (s staticCodexClientVersionResolver) Version(context.Context) string { return string(s) }
