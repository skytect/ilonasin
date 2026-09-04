package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ilonasin/internal/credentials"
	"ilonasin/internal/metadata"
	"ilonasin/internal/provider"
)

type selectorRouteAuth struct{}

func (selectorRouteAuth) VerifyBearer(context.Context, string) (credentials.VerifiedLocalToken, error) {
	return credentials.VerifiedLocalToken{ID: 7}, nil
}

type selectorRouteRecorder struct{ requests []metadata.Request }

func (r *selectorRouteRecorder) RecordRequestMetadata(_ context.Context, req metadata.Request) (int64, error) {
	r.requests = append(r.requests, req)
	return int64(len(r.requests)), nil
}
func (*selectorRouteRecorder) RecordStreamMetrics(context.Context, metadata.Stream) error { return nil }
func (*selectorRouteRecorder) RecordHealthEvent(context.Context, metadata.HealthEvent) error {
	return nil
}
func (*selectorRouteRecorder) RecordFallbackEvent(context.Context, metadata.FallbackEvent) error {
	return nil
}
func (*selectorRouteRecorder) RecordQuotaObservation(context.Context, metadata.QuotaObservation) error {
	return nil
}

func TestDaybreakRoutesPreserveBaseModelAndSelectorMetadata(t *testing.T) {
	for _, base := range []string{"gpt-5.6-sol", "gpt-6-astra"} {
		for _, endpoint := range []string{"/v1/chat/completions", "/v1/messages"} {
			t.Run(base+endpoint, func(t *testing.T) {
				discoverer := &testCatalogDiscoverer{catalogs: map[int64][]string{
					101: {base, "gpt-daybreak-blue-latest"}, 202: {base, "gpt-daybreak-red-latest"},
				}}
				srv, _ := newResolutionServer(discoverer, &resolutionModelCache{}, 101, 202)
				srv.auth = selectorRouteAuth{}
				adapter := &resolutionHTTPAdapter{}
				srv.adapters = provider.StaticChatAdapters{"codex": adapter}
				recorder := &selectorRouteRecorder{}
				srv.meta = recorder
				requested := base + "/daybreak-blue"
				body := `{"model":"` + requested + `","max_tokens":32,"messages":[{"role":"user","content":"hello"}]}`
				w := httptest.NewRecorder()
				srv.Handler().ServeHTTP(w, httptest.NewRequest(http.MethodPost, endpoint, strings.NewReader(body)))
				if w.Code != http.StatusOK {
					t.Fatalf("route failed: status=%d body=%s", w.Code, w.Body.String())
				}
				if len(adapter.requests) != 1 || adapter.requests[0].UpstreamModel != base || adapter.requests[0].Credential.ID != 101 {
					t.Fatalf("selector changed upstream model or selected wrong cohort: %+v", adapter.requests)
				}
				if len(recorder.requests) != 1 || recorder.requests[0].RequestedModel != requested || recorder.requests[0].ResolvedModel != base {
					t.Fatalf("selector diagnostics or resolved model incorrect: %+v", recorder.requests)
				}
			})
		}
	}
}

func TestModelsResponseExposesKnownContextLength(t *testing.T) {
	length := int64(200000)
	response := modelsResponseFromMetadata([]provider.ModelMetadata{
		{ProviderInstanceID: "codex", ModelID: "known", ContextLength: &length},
		{ProviderInstanceID: "codex", ModelID: "unknown"},
	})
	encoded, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	var body struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(encoded, &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Data) != 2 || body.Data[0]["context_length"] != float64(length) {
		t.Fatalf("known context length missing: %s", encoded)
	}
	if _, exists := body.Data[1]["context_length"]; exists {
		t.Fatalf("unknown context length was fabricated: %s", encoded)
	}
}

func TestInvalidModelDoesNotRequireRegistry(t *testing.T) {
	srv := &Server{}
	for _, model := range []string{"", "provider/", "model/daybreak-blue/daybreak-red"} {
		if _, err := srv.resolveModelAddress(context.Background(), model); err == nil {
			t.Errorf("malformed model accepted: %q", model)
		}
	}
}
