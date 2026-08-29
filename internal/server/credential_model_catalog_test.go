package server

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"testing"
	"time"

	"ilonasin/internal/credentials"
	"ilonasin/internal/openai"
	"ilonasin/internal/provider"
	"ilonasin/internal/routing"
)

const testAccountScopedModel = "gpt-daybreak-blue-latest"

type testOAuthResolver struct {
	credentials []credentials.ResolvedOAuthBearerCredential
}

func (r testOAuthResolver) ResolveOAuthBearers(context.Context, string, time.Time) ([]credentials.ResolvedOAuthBearerCredential, error) {
	return append([]credentials.ResolvedOAuthBearerCredential(nil), r.credentials...), nil
}

type testCatalogDiscoverer struct {
	mu       sync.Mutex
	catalogs map[int64][]string
	failures map[int64]bool
	calls    map[int64]int
}

func (d *testCatalogDiscoverer) ModelAvailabilityScope(_ provider.Instance, modelID string) provider.ModelAvailabilityScope {
	if modelID == testAccountScopedModel {
		return provider.ModelAvailabilityCredentialCatalog
	}
	return provider.ModelAvailabilityShared
}

func (d *testCatalogDiscoverer) RequiresCredentialCatalogUnion(provider.Instance) bool { return true }

func (d *testCatalogDiscoverer) ListModels(_ context.Context, req provider.ModelRequest) (provider.ModelResult, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.calls == nil {
		d.calls = make(map[int64]int)
	}
	d.calls[req.Credential.ID]++
	if d.failures[req.Credential.ID] {
		return provider.ModelResult{StatusCode: http.StatusBadGateway, ErrorClass: "upstream_network_error"}, errors.New("catalog unavailable")
	}
	ids := d.catalogs[req.Credential.ID]
	models := make([]provider.ModelMetadata, 0, len(ids))
	for _, id := range ids {
		models = append(models, provider.ModelMetadata{ProviderInstanceID: req.Instance.ID, ModelID: id})
	}
	return provider.ModelResult{StatusCode: http.StatusOK, Models: models}, nil
}

func (d *testCatalogDiscoverer) callCount(id int64) int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.calls[id]
}

func newCatalogRoutingServer(now *time.Time, discoverer *testCatalogDiscoverer, ids ...int64) (*Server, provider.Instance) {
	instance := provider.Instance{ID: "pragnition-codex", Type: "codex", OAuth: true, Chat: true, ModelDiscovery: true}
	resolved := make([]credentials.ResolvedOAuthBearerCredential, 0, len(ids))
	for _, id := range ids {
		resolved = append(resolved, credentials.ResolvedOAuthBearerCredential{ID: id, ProviderInstanceID: instance.ID, BearerToken: "test-bearer"})
	}
	srv := NewWithClock(nil, nil, nil, testOAuthResolver{credentials: resolved}, nil,
		provider.StaticModelDiscoverers{"codex": discoverer}, nil, nil, func() time.Time { return *now })
	return srv, instance
}

func TestAccountScopedModelSelectsOnlyAdvertisingCredentials(t *testing.T) {
	now := time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)
	discoverer := &testCatalogDiscoverer{catalogs: map[int64][]string{
		101: {"gpt-5.6-sol", testAccountScopedModel},
		202: {"gpt-5.6-sol"},
		303: {testAccountScopedModel},
	}}
	srv, instance := newCatalogRoutingServer(&now, discoverer, 101, 202, 303)

	got, err := srv.resolveModelCredentialsForModel(context.Background(), instance, testAccountScopedModel)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != 101 || got[1].ID != 303 {
		t.Fatalf("expected only advertising credentials in resolver order, got IDs %v", credentialIDs(got))
	}
}

func TestAccountScopedModelNeverRetriesOntoIneligibleCredential(t *testing.T) {
	now := time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)
	discoverer := &testCatalogDiscoverer{catalogs: map[int64][]string{
		101: {testAccountScopedModel},
		202: {"gpt-5.6-sol"},
		303: {testAccountScopedModel},
	}}
	srv, instance := newCatalogRoutingServer(&now, discoverer, 101, 202, 303)
	eligible, err := srv.resolveModelCredentialsForModel(context.Background(), instance, testAccountScopedModel)
	if err != nil {
		t.Fatal(err)
	}
	adapter := &testRetryChatAdapter{}
	req, _ := http.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	exec := srv.executeNonStreamingChat(req, nonStreamContext{
		start:       now,
		token:       credentials.VerifiedLocalToken{ID: 7},
		address:     routing.ModelAddress{ProviderInstanceID: instance.ID, ProviderModelID: testAccountScopedModel},
		instance:    instance,
		credentials: eligible,
		adapter:     adapter,
		request:     openai.ChatCompletionRequest{AffinityKey: "stable-session"},
	})
	if exec.final.result.StatusCode != http.StatusOK {
		t.Fatalf("expected eligible failover to succeed, got status %d", exec.final.result.StatusCode)
	}
	if len(adapter.attempts) != 2 || adapter.attempts[0] == 202 || adapter.attempts[1] == 202 {
		t.Fatalf("ineligible credential was attempted or eligible retry was missing: %v", adapter.attempts)
	}
}

func TestAccountScopedModelFailsClosedWithZeroEligibleCredentials(t *testing.T) {
	now := time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)
	discoverer := &testCatalogDiscoverer{catalogs: map[int64][]string{101: {"gpt-5.6-sol"}, 202: {"gpt-5.6-sol"}}}
	srv, instance := newCatalogRoutingServer(&now, discoverer, 101, 202)

	got, err := srv.resolveModelCredentialsForModel(context.Background(), instance, testAccountScopedModel)
	if !errors.Is(err, credentials.ErrNoEligibleCredential) || len(got) != 0 {
		t.Fatalf("expected fail-closed no-eligible result, got credentials=%v err=%v", credentialIDs(got), err)
	}
}

func TestAccountScopedModelFailsClosedWithoutDiscoveryCapability(t *testing.T) {
	now := time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)
	instance := provider.Instance{ID: "pragnition-codex", Type: "codex", OAuth: true}
	oauth := testOAuthResolver{credentials: []credentials.ResolvedOAuthBearerCredential{{ID: 101, ProviderInstanceID: instance.ID, BearerToken: "test-bearer"}}}
	srv := NewWithClock(nil, nil, nil, oauth, nil, nil, nil, nil, func() time.Time { return now })

	got, err := srv.resolveModelCredentialsForModel(context.Background(), instance, testAccountScopedModel)
	if !errors.Is(err, credentials.ErrNoEligibleCredential) || len(got) != 0 {
		t.Fatalf("expected missing discovery capability to fail closed, got credentials=%v err=%v", credentialIDs(got), err)
	}
}

func TestAccountScopedModelStaleFailedDiscoveryFailsClosed(t *testing.T) {
	now := time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)
	discoverer := &testCatalogDiscoverer{catalogs: map[int64][]string{101: {testAccountScopedModel}}, failures: map[int64]bool{}}
	srv, instance := newCatalogRoutingServer(&now, discoverer, 101)
	if _, err := srv.resolveModelCredentialsForModel(context.Background(), instance, testAccountScopedModel); err != nil {
		t.Fatalf("initial live discovery failed: %v", err)
	}

	now = now.Add(credentialModelCatalogTTL + time.Nanosecond)
	discoverer.mu.Lock()
	discoverer.failures[101] = true
	discoverer.mu.Unlock()
	got, err := srv.resolveModelCredentialsForModel(context.Background(), instance, testAccountScopedModel)
	if !errors.Is(err, credentials.ErrNoEligibleCredential) || len(got) != 0 {
		t.Fatalf("expected stale failed discovery to fail closed, got credentials=%v err=%v", credentialIDs(got), err)
	}
}

func TestSharedModelRoutingDoesNotRequireCatalogDiscovery(t *testing.T) {
	now := time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)
	discoverer := &testCatalogDiscoverer{catalogs: map[int64][]string{101: {"gpt-5.6-sol"}, 202: {"gpt-5.6-sol"}}}
	srv, instance := newCatalogRoutingServer(&now, discoverer, 101, 202)

	got, err := srv.resolveModelCredentialsForModel(context.Background(), instance, "gpt-5.6-sol")
	if err != nil || len(got) != 2 {
		t.Fatalf("expected unchanged shared-model credential pool, got IDs=%v err=%v", credentialIDs(got), err)
	}
	if discoverer.callCount(101) != 0 || discoverer.callCount(202) != 0 {
		t.Fatalf("shared model unexpectedly triggered catalog discovery")
	}
}

func TestCredentialCatalogUnionExposesAccountScopedModel(t *testing.T) {
	now := time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)
	discoverer := &testCatalogDiscoverer{catalogs: map[int64][]string{
		101: {"gpt-5.6-sol"},
		202: {"gpt-5.6-sol", testAccountScopedModel},
	}}
	srv, instance := newCatalogRoutingServer(&now, discoverer, 101, 202)
	credentialsSet, err := srv.resolveModelCredentials(context.Background(), instance)
	if err != nil {
		t.Fatal(err)
	}
	attempt := srv.discoverModelsWithCredentials(context.Background(), instance, discoverer, credentialsSet)
	if !attempt.live || !containsProviderModel(attempt.models, testAccountScopedModel) {
		t.Fatalf("expected unioned discovery to expose %q, got live=%v models=%v", testAccountScopedModel, attempt.live, providerModelIDs(attempt.models))
	}
	if discoverer.callCount(101) != 1 || discoverer.callCount(202) != 1 {
		t.Fatalf("expected every credential catalog to be queried once")
	}
}

func credentialIDs(values []provider.BearerCredential) []int64 {
	ids := make([]int64, 0, len(values))
	for _, value := range values {
		ids = append(ids, value.ID)
	}
	return ids
}

func containsProviderModel(models []provider.ModelMetadata, modelID string) bool {
	for _, model := range models {
		if model.ModelID == modelID {
			return true
		}
	}
	return false
}

type testRetryChatAdapter struct {
	attempts []int64
}

func (a *testRetryChatAdapter) ValidateChatRequest(provider.Instance, openai.ChatCompletionRequest) error {
	return nil
}

func (a *testRetryChatAdapter) CompleteChat(_ context.Context, req provider.ChatRequest) (provider.ChatResult, error) {
	a.attempts = append(a.attempts, req.Credential.ID)
	if len(a.attempts) == 1 {
		return provider.ChatResult{StatusCode: http.StatusBadGateway, UpstreamStatusCode: http.StatusServiceUnavailable, ErrorClass: "upstream_http_error"}, errors.New("retryable availability failure")
	}
	return provider.ChatResult{StatusCode: http.StatusOK, Body: []byte(`{"ok":true}`)}, nil
}

func (a *testRetryChatAdapter) StreamChat(context.Context, provider.ChatRequest, provider.ChatStreamSink) (provider.ChatStreamSummary, error) {
	return provider.ChatStreamSummary{}, errors.New("not used")
}
