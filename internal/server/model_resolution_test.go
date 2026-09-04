package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"ilonasin/internal/credentials"
	"ilonasin/internal/metadata"
	"ilonasin/internal/provider"
	"ilonasin/internal/storage/sqlite"
)

type resolutionRegistry []provider.Instance

func (r resolutionRegistry) Get(id string) (provider.Instance, bool) {
	for _, instance := range r {
		if instance.ID == id {
			return instance, true
		}
	}
	return provider.Instance{}, false
}

func (r resolutionRegistry) List() []provider.Instance { return append([]provider.Instance(nil), r...) }

type resolutionModelCache struct {
	mu   sync.Mutex
	rows []metadata.ModelCacheRow
}

func (c *resolutionModelCache) ListModelCache(context.Context) ([]metadata.ModelCacheRow, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]metadata.ModelCacheRow(nil), c.rows...), nil
}

func (c *resolutionModelCache) ReplaceModelCache(_ context.Context, id string, rows []metadata.ModelCacheRow) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]metadata.ModelCacheRow, 0, len(c.rows)+len(rows))
	for _, row := range c.rows {
		if row.ProviderInstanceID != id {
			out = append(out, row)
		}
	}
	c.rows = append(out, rows...)
	return nil
}

func (c *resolutionModelCache) MergeModelCache(_ context.Context, id string, rows []metadata.ModelCacheRow) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	byID := make(map[string]metadata.ModelCacheRow)
	for _, row := range rows {
		byID[row.ModelID] = row
	}
	for i, row := range c.rows {
		if row.ProviderInstanceID == id {
			if updated, ok := byID[row.ModelID]; ok {
				c.rows[i] = updated
				delete(byID, row.ModelID)
			}
		}
	}
	for _, row := range byID {
		c.rows = append(c.rows, row)
	}
	return nil
}

func newResolutionServer(discoverer provider.ModelDiscoverer, cache ModelCache, ids ...int64) (*Server, provider.Instance) {
	instance := provider.Instance{ID: "catalog-provider", Type: "codex", OAuth: true, Chat: true, ModelDiscovery: true}
	resolved := make([]credentials.ResolvedOAuthBearerCredential, 0, len(ids))
	for _, id := range ids {
		resolved = append(resolved, credentials.ResolvedOAuthBearerCredential{ID: id, ProviderInstanceID: instance.ID, BearerToken: "test-bearer"})
	}
	srv := New(resolutionRegistry{instance}, nil, nil, testOAuthResolver{credentials: resolved}, nil,
		provider.StaticModelDiscoverers{"codex": discoverer}, cache, nil)
	return srv, instance
}

func TestBareModelDiscoversWithoutCatalogWarmup(t *testing.T) {
	for _, useCache := range []bool{false, true} {
		name := "without_persistence"
		var cache ModelCache
		if useCache {
			name = "with_persistence"
			cache = &resolutionModelCache{}
		}
		t.Run(name, func(t *testing.T) {
			discoverer := &testCatalogDiscoverer{catalogs: map[int64][]string{101: {"new-model"}}}
			srv, instance := newResolutionServer(discoverer, cache, 101)
			addr, err := srv.resolveModelAddress(context.Background(), "new-model")
			if err != nil || addr.ProviderInstanceID != instance.ID || addr.ProviderModelID != "new-model" {
				t.Fatalf("cold bare model did not resolve: address=%+v err=%v", addr, err)
			}
			if discoverer.callCount(101) != 1 {
				t.Fatalf("expected one discovery, got %d", discoverer.callCount(101))
			}
		})
	}
}

func TestExplicitAndMalformedModelsDoNotDiscover(t *testing.T) {
	discoverer := &testCatalogDiscoverer{}
	srv, _ := newResolutionServer(discoverer, &resolutionModelCache{}, 101)
	for _, model := range []string{"catalog-provider/unknown", "catalog-provider/nested/model", "catalog-provider/", "/unknown", ""} {
		_, _ = srv.resolveModelAddress(context.Background(), model)
	}
	if discoverer.callCount(101) != 0 {
		t.Fatal("explicit or malformed model triggered discovery")
	}
}

func TestBareModelIgnoresRemovedProviderAndRejectsAmbiguity(t *testing.T) {
	discoverer := &testCatalogDiscoverer{catalogs: map[int64][]string{101: {"shared-model"}}}
	cache := &resolutionModelCache{rows: []metadata.ModelCacheRow{
		{ProviderInstanceID: "removed-provider", ModelID: "shared-model"},
		{ProviderInstanceID: "catalog-provider", ModelID: "shared-model"},
	}}
	srv, instance := newResolutionServer(discoverer, cache, 101)
	addr, err := srv.resolveModelAddress(context.Background(), "shared-model")
	if err != nil || addr.ProviderInstanceID != instance.ID {
		t.Fatalf("removed provider interfered with resolution: address=%+v err=%v", addr, err)
	}
	if discoverer.callCount(101) != 0 {
		t.Fatal("unique cached match triggered discovery")
	}
	other := instance
	other.ID = "second-provider"
	srv.registry = resolutionRegistry{instance, other}
	cache.mu.Lock()
	cache.rows = append(cache.rows, metadata.ModelCacheRow{ProviderInstanceID: other.ID, ModelID: "shared-model"})
	cache.mu.Unlock()
	if _, err := srv.resolveModelAddress(context.Background(), "shared-model"); err == nil {
		t.Fatal("ambiguous bare model resolved to an arbitrary provider")
	}
}

func TestBareUnknownModelsShareRefreshCooldown(t *testing.T) {
	discoverer := &testCatalogDiscoverer{catalogs: map[int64][]string{101: {"known-model"}}}
	srv, _ := newResolutionServer(discoverer, &resolutionModelCache{}, 101)
	for _, model := range []string{"unknown-one", "unknown-two", "unknown-one"} {
		if _, err := srv.resolveModelAddress(context.Background(), model); err == nil {
			t.Fatalf("unknown model %q unexpectedly resolved", model)
		}
	}
	if discoverer.callCount(101) != 1 {
		t.Fatalf("misses did not share catalog cooldown: calls=%d", discoverer.callCount(101))
	}
}

type blockingResolutionDiscoverer struct {
	*testCatalogDiscoverer
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (d *blockingResolutionDiscoverer) ListModels(ctx context.Context, req provider.ModelRequest) (provider.ModelResult, error) {
	d.once.Do(func() { close(d.started) })
	select {
	case <-ctx.Done():
		return provider.ModelResult{}, ctx.Err()
	case <-d.release:
		return d.testCatalogDiscoverer.ListModels(ctx, req)
	}
}

func TestCatalogRefreshCoalescesAndCallerCancellationIsIndependent(t *testing.T) {
	discoverer := &blockingResolutionDiscoverer{
		testCatalogDiscoverer: &testCatalogDiscoverer{catalogs: map[int64][]string{101: {"new-model"}}},
		started:               make(chan struct{}), release: make(chan struct{}),
	}
	srv, _ := newResolutionServer(discoverer, &resolutionModelCache{}, 101)
	leaderCtx, cancelLeader := context.WithCancel(context.Background())
	defer cancelLeader()
	released := false
	defer func() {
		if !released {
			close(discoverer.release)
		}
	}()
	leaderResult := make(chan error, 1)
	go func() {
		_, err := srv.refreshModelCatalog(leaderCtx)
		leaderResult <- err
	}()
	select {
	case <-discoverer.started:
	case <-time.After(3 * time.Second):
		t.Fatal("refresh did not start")
	}
	const waiters = 12
	results := make(chan error, waiters)
	for range waiters {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			_, err := srv.resolveModelAddress(ctx, "new-model")
			results <- err
		}()
	}
	listing := make(chan int, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		w := httptest.NewRecorder()
		srv.handleModels(w, httptest.NewRequest(http.MethodGet, "/v1/models", nil).WithContext(ctx), credentials.VerifiedLocalToken{})
		listing <- w.Code
	}()
	cancelLeader()
	select {
	case err := <-leaderResult:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled caller did not return context cancellation: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("canceled caller remained blocked on discovery")
	}
	close(discoverer.release)
	released = true
	for range waiters {
		select {
		case err := <-results:
			if err != nil {
				t.Fatalf("independent waiter failed: %v", err)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("independent waiter did not finish")
		}
	}
	select {
	case status := <-listing:
		if status != http.StatusOK {
			t.Fatalf("concurrent model listing failed: status=%d", status)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("concurrent model listing did not finish")
	}
	if discoverer.callCount(101) != 1 {
		t.Fatalf("concurrent callers did not share one refresh: calls=%d", discoverer.callCount(101))
	}
}

func TestPartialCatalogPreservesAliasesWithoutGrantingEntitlement(t *testing.T) {
	discoverer := &testCatalogDiscoverer{catalogs: map[int64][]string{
		101: {"old-shared"}, 202: {testAccountScopedModel},
	}, failures: map[int64]bool{}}
	cache := &resolutionModelCache{}
	srv, instance := newResolutionServer(discoverer, cache, 101, 202)
	if _, err := srv.discoverModelCatalog(context.Background()); err != nil {
		t.Fatal(err)
	}
	discoverer.mu.Lock()
	discoverer.catalogs[101] = []string{"new-shared"}
	discoverer.failures[202] = true
	discoverer.mu.Unlock()
	partial, err := srv.discoverModelCatalog(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !containsProviderModel(partial.models, "new-shared") {
		t.Fatal("partial live catalog lost newly observed model")
	}
	rows, _ := cache.ListModelCache(context.Background())
	if !resolutionHasRow(rows, instance.ID, "old-shared") || !resolutionHasRow(rows, instance.ID, testAccountScopedModel) || !resolutionHasRow(rows, instance.ID, "new-shared") {
		t.Fatalf("partial discovery replaced complete persisted catalog: %+v", rows)
	}
	previous, ok := srv.lastGoodCodexModels.get(instance.ID)
	if !ok || !containsProviderModel(previous, testAccountScopedModel) || !containsProviderModel(previous, "old-shared") {
		t.Fatal("partial discovery replaced complete ephemeral fallback")
	}
	for _, model := range []string{"old-shared", "new-shared", testAccountScopedModel} {
		if _, err := srv.resolveModelAddress(context.Background(), model); err != nil {
			t.Fatalf("retained or live alias %q failed: %v", model, err)
		}
	}
	eligible, err := srv.resolveModelCredentialsForModel(context.Background(), instance, testAccountScopedModel)
	if !errors.Is(err, credentials.ErrNoEligibleCredential) || len(eligible) != 0 {
		t.Fatalf("retained alias granted failed credential entitlement: IDs=%v err=%v", credentialIDs(eligible), err)
	}
	discoverer.mu.Lock()
	discoverer.failures[202] = false
	discoverer.catalogs[202] = []string{"another-shared"}
	discoverer.mu.Unlock()
	full, err := srv.discoverModelCatalog(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	rows, _ = cache.ListModelCache(context.Background())
	if resolutionHasRow(rows, instance.ID, "old-shared") || resolutionHasRow(rows, instance.ID, testAccountScopedModel) || !resolutionHasRow(rows, instance.ID, "new-shared") {
		t.Fatalf("complete refresh did not replace retired aliases: %+v", rows)
	}
	if resolutionHasRow(full.addresses, instance.ID, "old-shared") || resolutionHasRow(full.addresses, instance.ID, testAccountScopedModel) {
		t.Fatal("complete refresh returned retired aliases despite replacing persistence")
	}
}

func resolutionHasRow(rows []metadata.ModelCacheRow, instanceID, modelID string) bool {
	for _, row := range rows {
		if row.ProviderInstanceID == instanceID && row.ModelID == modelID {
			return true
		}
	}
	return false
}

type slowResolutionDiscoverer struct {
	*testCatalogDiscoverer
	slowID int64
}

func (d *slowResolutionDiscoverer) ListModels(ctx context.Context, req provider.ModelRequest) (provider.ModelResult, error) {
	if req.Credential.ID == d.slowID {
		<-ctx.Done()
		return provider.ModelResult{ErrorClass: "upstream_timeout"}, ctx.Err()
	}
	return d.testCatalogDiscoverer.ListModels(ctx, req)
}

func TestSlowCredentialPreservesSnapshotAndDiscoversNewAlias(t *testing.T) {
	discoverer := &testCatalogDiscoverer{catalogs: map[int64][]string{101: {"old-model"}, 202: {"shared-model"}}}
	cache := &resolutionModelCache{}
	srv, instance := newResolutionServer(discoverer, cache, 101, 202)
	if _, err := srv.discoverModelCatalog(context.Background()); err != nil {
		t.Fatal(err)
	}
	discoverer.mu.Lock()
	discoverer.catalogs[202] = []string{"new-model"}
	discoverer.mu.Unlock()
	srv.models = provider.StaticModelDiscoverers{"codex": &slowResolutionDiscoverer{testCatalogDiscoverer: discoverer, slowID: 101}}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	addr, err := srv.resolveModelAddress(ctx, "new-model")
	if err != nil || addr.ProviderInstanceID != instance.ID || addr.ProviderModelID != "new-model" {
		t.Fatalf("slow credential blocked healthy credential alias: address=%+v err=%v", addr, err)
	}
	rows, err := cache.ListModelCache(context.Background())
	if err != nil || !resolutionHasRow(rows, instance.ID, "old-model") || !resolutionHasRow(rows, instance.ID, "new-model") {
		t.Fatalf("timed-out catalog replaced complete persisted snapshot: rows=%+v err=%v", rows, err)
	}
	previous, ok := srv.lastGoodCodexModels.get(instance.ID)
	if !ok || !containsProviderModel(previous, "old-model") {
		t.Fatal("timed-out catalog replaced complete ephemeral snapshot")
	}
}

type resolutionHTTPAdapter struct {
	testRetryChatAdapter
	requests []provider.ChatRequest
}

func (a *resolutionHTTPAdapter) CompleteChat(_ context.Context, req provider.ChatRequest) (provider.ChatResult, error) {
	a.requests = append(a.requests, req)
	return provider.ChatResult{StatusCode: http.StatusOK, ContentType: "application/json", Body: []byte(`{"id":"test","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`)}, nil
}

func TestBareHTTPModelPersistsAcrossServerRecreation(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "ilonasin.sqlite")
	store, err := sqlite.Open(context.Background(), databasePath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	tokens := credentials.Service{Repo: store}
	token, err := tokens.Create(context.Background(), "test-client")
	if err != nil {
		t.Fatal(err)
	}
	discoverer := &testCatalogDiscoverer{catalogs: map[int64][]string{101: {"cold-http-model"}}}
	for pass := range 2 {
		if pass == 1 {
			if err := store.Close(); err != nil {
				t.Fatal(err)
			}
			store, err = sqlite.Open(context.Background(), databasePath)
			if err != nil {
				t.Fatal(err)
			}
			tokens.Repo = store
			discoverer.mu.Lock()
			discoverer.failures = map[int64]bool{101: true}
			discoverer.mu.Unlock()
		}
		srv, instance := newResolutionServer(discoverer, store, 101)
		if pass == 1 {
			addr, err := srv.resolveModelAddress(context.Background(), "cold-http-model")
			if err != nil || addr.ProviderInstanceID != instance.ID || discoverer.callCount(101) != 1 {
				t.Fatalf("server recreation did not use persisted alias: address=%+v err=%v calls=%d", addr, err, discoverer.callCount(101))
			}
		}
		srv.auth = tokens
		adapter := &resolutionHTTPAdapter{}
		srv.adapters = provider.StaticChatAdapters{"codex": adapter}
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"cold-http-model","messages":[{"role":"user","content":"test"}]}`))
		req.Header.Set("Authorization", "Bearer "+token.Token)
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)
		if pass == 1 {
			if w.Code != http.StatusUnauthorized || !strings.Contains(w.Body.String(), "credential_unavailable") || len(adapter.requests) != 0 {
				t.Fatalf("persisted alias bypassed fresh Codex entitlement: status=%d body=%s requests=%d", w.Code, w.Body.String(), len(adapter.requests))
			}
			continue
		}
		if w.Code != http.StatusOK {
			t.Fatalf("bare HTTP request pass %d failed: status=%d body=%s", pass, w.Code, w.Body.String())
		}
		if len(adapter.requests) != 1 || adapter.requests[0].Instance.ID != instance.ID || adapter.requests[0].UpstreamModel != "cold-http-model" {
			t.Fatalf("bare HTTP request dispatched incorrectly: %+v", adapter.requests)
		}
	}
	if discoverer.callCount(101) != 2 {
		t.Fatalf("expected cold discovery and one fresh entitlement check after restart: calls=%d", discoverer.callCount(101))
	}
}

func TestAstraOnlyUsesAdvertisingCredentials(t *testing.T) {
	now := time.Now().UTC()
	discoverer := &testCatalogDiscoverer{catalogs: map[int64][]string{
		101: {"gpt-6-astra"}, 202: {"gpt-5.6-sol"},
	}}
	srv, instance := newCatalogRoutingServer(&now, discoverer, 101, 202)
	eligible, err := srv.resolveModelCredentialsForModel(context.Background(), instance, "gpt-6-astra")
	if err != nil || len(eligible) != 1 || eligible[0].ID != 101 {
		t.Fatalf("Astra was not restricted to its advertising credential: IDs=%v err=%v", credentialIDs(eligible), err)
	}
}
