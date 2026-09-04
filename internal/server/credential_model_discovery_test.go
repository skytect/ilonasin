package server

import (
	"context"
	"errors"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"ilonasin/internal/credentials"
	"ilonasin/internal/metadata"
	"ilonasin/internal/openai"
	"ilonasin/internal/provider"
	"ilonasin/internal/routing"
	"ilonasin/internal/storage/sqlite"
)

func TestPartialLiveAmbiguityOverridesUniquePersistedAlias(t *testing.T) {
	discoverer := &testCatalogDiscoverer{catalogs: map[int64][]string{101: {"same-model"}}, failures: map[int64]bool{202: true}}
	cache, err := sqlite.Open(context.Background(), filepath.Join(t.TempDir(), "catalog.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer cache.Close()
	for _, row := range []metadata.ModelCacheRow{{ProviderInstanceID: "catalog-provider", ModelID: "same-model"}, {ProviderInstanceID: "second-provider", ModelID: "previous-model"}} {
		if err := cache.ReplaceModelCache(context.Background(), row.ProviderInstanceID, []metadata.ModelCacheRow{row}); err != nil {
			t.Fatal(err)
		}
	}
	srv, first := newResolutionServer(discoverer, cache, 101, 202)
	second := first
	second.ID = "second-provider"
	srv.registry = resolutionRegistry{first, second}
	if _, err := srv.refreshModelCatalog(context.Background()); err != nil {
		t.Fatal(err)
	}
	// A newly constructed server has no in-memory catalog observations.
	srv, _ = newResolutionServer(discoverer, cache, 101, 202)
	srv.registry = resolutionRegistry{first, second}
	if addr, err := srv.resolveModelAddress(context.Background(), "same-model"); err == nil {
		t.Fatalf("known live ambiguity bypassed through persisted fast path: %+v", addr)
	}
}

func TestCredentialDiscoveryCancellationDoesNotBlockFreshProof(t *testing.T) {
	discoverer := &blockingResolutionDiscoverer{
		testCatalogDiscoverer: &testCatalogDiscoverer{catalogs: map[int64][]string{101: {"model"}}},
		started:               make(chan struct{}), release: make(chan struct{}),
	}
	srv, instance := newResolutionServer(discoverer, &resolutionModelCache{}, 101)
	slow := provider.BearerCredential{ID: 101, BearerToken: "test-bearer"}
	fast := provider.BearerCredential{ID: 202, BearerToken: "fast-bearer"}
	srv.credentialModelCatalogs.put(srv.now(), modelCatalogKey(instance.ID, fast), []string{"model"})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		srv.refreshMissingCredentialModelCatalogs(ctx, instance, discoverer, []provider.BearerCredential{slow})
		close(done)
	}()
	select {
	case <-discoverer.started:
	case <-time.After(time.Second):
		t.Fatal("slow observation did not start")
	}
	released := false
	defer func() {
		if !released {
			close(discoverer.release)
		}
	}()
	fastDone := make(chan bool, 1)
	go func() {
		_, eligible := srv.credentialModelEligibleForAttempt(context.Background(), instance, "model", fast)
		fastDone <- eligible
	}()
	select {
	case eligible := <-fastDone:
		if !eligible {
			t.Fatal("fresh proof was rejected")
		}
	case <-time.After(time.Second):
		t.Fatal("fresh proof blocked on unrelated network discovery")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("canceled discovery waiter remained blocked")
	}
	// A listing joins the still-running fetch after its original caller leaves.
	listingDone := make(chan credentialModelObservation, 1)
	go func() {
		listingDone <- srv.observeCredentialModels(context.Background(), instance, discoverer, slow, false)
	}()
	close(discoverer.release)
	released = true
	select {
	case observation := <-listingDone:
		if !observation.live {
			t.Fatal("shared fetch was canceled with its first caller")
		}
	case <-time.After(time.Second):
		t.Fatal("shared fetch did not complete")
	}
	if discoverer.callCount(101) != 1 {
		t.Fatalf("shared discovery fetched %d times", discoverer.callCount(101))
	}
}

type catalogRefreshController struct {
	credentials.OAuthProviderRefreshController
	credential credentials.ResolvedOAuthBearerCredential
}

func (c catalogRefreshController) RefreshOAuthCredentialIfBearer(context.Context, int64, string) error {
	return nil
}
func (c catalogRefreshController) ResolveOAuthBearerByID(context.Context, int64, time.Time) (credentials.ResolvedOAuthBearerCredential, error) {
	return c.credential, nil
}

type bearerCatalogDiscoverer struct{ testCatalogDiscoverer }

func (d *bearerCatalogDiscoverer) ListModels(ctx context.Context, req provider.ModelRequest) (provider.ModelResult, error) {
	if req.Credential.BearerToken == "old-bearer" {
		return provider.ModelResult{StatusCode: http.StatusUnauthorized}, errors.New("expired bearer")
	}
	return d.testCatalogDiscoverer.ListModels(ctx, req)
}

func TestCatalogRefreshReturnsObservedBearerAndProofGeneration(t *testing.T) {
	discoverer := &bearerCatalogDiscoverer{testCatalogDiscoverer{catalogs: map[int64][]string{101: {"model"}}}}
	srv, instance := newResolutionServer(discoverer, &resolutionModelCache{}, 101)
	srv.oauth = testOAuthResolver{credentials: []credentials.ResolvedOAuthBearerCredential{{ID: 101, ProviderInstanceID: instance.ID, BearerToken: "old-bearer"}}}
	srv.refresh = catalogRefreshController{credential: credentials.ResolvedOAuthBearerCredential{ID: 101, ProviderInstanceID: instance.ID, BearerToken: "new-bearer"}}
	eligible, err := srv.resolveModelCredentialsForModel(context.Background(), instance, "model")
	if err != nil || len(eligible) != 1 || eligible[0].BearerToken != "new-bearer" {
		t.Fatalf("refreshed observation did not propagate actual bearer: count=%d err=%v", len(eligible), err)
	}
	old := provider.BearerCredential{ID: 101, BearerToken: "old-bearer"}
	if _, known, _ := srv.credentialModelCatalogs.lookup(srv.now(), modelCatalogKey(instance.ID, old)); known {
		t.Fatal("new bearer catalog was attributed to old generation")
	}
	if _, known, cached := srv.credentialModelCatalogs.lookup(srv.now(), modelCatalogKey(instance.ID, eligible[0])); !known || !cached {
		t.Fatal("observed bearer has no catalog proof")
	}
}

type modelHealthRecorder struct {
	MetadataRecorder
	contextErr error
	count      int
}

func (r *modelHealthRecorder) RecordHealthEvent(ctx context.Context, _ metadata.HealthEvent) error {
	r.contextErr = ctx.Err()
	r.count++
	return r.contextErr
}

func TestCatalogTimeoutHealthUsesLiveRecordingContext(t *testing.T) {
	discoverer := &slowResolutionDiscoverer{testCatalogDiscoverer: &testCatalogDiscoverer{}, slowID: 101}
	srv, instance := newResolutionServer(discoverer, &resolutionModelCache{}, 101)
	recorder := &modelHealthRecorder{}
	srv.meta = recorder
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	srv.fetchCredentialModels(ctx, instance, discoverer, provider.BearerCredential{ID: 101})
	if recorder.count != 1 || recorder.contextErr != nil {
		t.Fatalf("timeout health was canceled: count=%d err=%v", recorder.count, recorder.contextErr)
	}
}

func TestSelectedModelRequiresModelAndSelectorOnSameCredential(t *testing.T) {
	discoverer := &testCatalogDiscoverer{catalogs: map[int64][]string{
		101: {"gpt-5.6-sol", "gpt-daybreak-blue-latest"},
		202: {"gpt-5.6-sol"},
		303: {"gpt-daybreak-blue-latest"},
	}}
	srv, instance := newResolutionServer(discoverer, &resolutionModelCache{}, 101, 202, 303)
	eligible, err := srv.resolveModelCredentialsForModel(context.Background(), instance, "gpt-5.6-sol", "daybreak-blue")
	if err != nil || len(eligible) != 1 || eligible[0].ID != 101 {
		t.Fatalf("selector and model proof were combined across credentials: IDs=%v err=%v", credentialIDs(eligible), err)
	}
}

func TestSelectedModelRetriesStayInsideDiscoveredCohort(t *testing.T) {
	discoverer := &testCatalogDiscoverer{catalogs: map[int64][]string{
		101: {"gpt-5.6-sol", "gpt-daybreak-blue-latest"},
		202: {"gpt-5.6-sol", "gpt-daybreak-red-latest"},
		303: {"gpt-5.6-sol", "gpt-daybreak-blue-latest"},
	}}
	srv, instance := newResolutionServer(discoverer, &resolutionModelCache{}, 101, 202, 303)
	red, err := srv.resolveModelCredentialsForModel(context.Background(), instance, "gpt-5.6-sol", "daybreak-red")
	if err != nil || len(red) != 1 || red[0].ID != 202 {
		t.Fatalf("red selector ignored live advertisement: IDs=%v err=%v", credentialIDs(red), err)
	}
	if unknown, err := srv.resolveModelCredentialsForModel(context.Background(), instance, "gpt-5.6-sol", "daybreak-unadvertised"); !errors.Is(err, credentials.ErrNoEligibleCredential) || len(unknown) != 0 {
		t.Fatalf("unknown cohort unexpectedly eligible: IDs=%v err=%v", credentialIDs(unknown), err)
	}
	blue, err := srv.resolveModelCredentialsForModel(context.Background(), instance, "gpt-5.6-sol", "daybreak-blue")
	if err != nil {
		t.Fatal(err)
	}
	adapter := &testRetryChatAdapter{}
	req, _ := http.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	exec := srv.executeNonStreamingChat(req, nonStreamContext{
		start: time.Now(), token: credentials.VerifiedLocalToken{ID: 7},
		address:  routing.ModelAddress{ProviderInstanceID: instance.ID, ProviderModelID: "gpt-5.6-sol", AccountSelector: "daybreak-blue"},
		instance: instance, credentials: blue, adapter: adapter,
		request: openai.ChatCompletionRequest{AffinityKey: "cohort-test"},
	})
	if exec.final.result.StatusCode != http.StatusOK || len(adapter.attempts) != 2 || adapter.attempts[0] == 202 || adapter.attempts[1] == 202 {
		t.Fatalf("selected retry escaped cohort or failed: attempts=%v status=%d", adapter.attempts, exec.final.result.StatusCode)
	}
}
