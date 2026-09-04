package provider

import "testing"

func TestCodexModelsRequireCredentialCatalogAvailability(t *testing.T) {
	adapter := NewHTTPChatAdapter(nil)
	instance := Instance{Type: "codex"}
	for _, model := range []string{"gpt-daybreak-blue-latest", "gpt-5.6-sol", "gpt-6-astra", "future-model"} {
		if got := adapter.ModelAvailabilityScope(instance, model); got != ModelAvailabilityCredentialCatalog {
			t.Fatalf("expected Codex model %q to require credential catalog eligibility, got %v", model, got)
		}
	}
	if !adapter.RequiresCredentialCatalogUnion(instance) {
		t.Fatal("expected Codex model discovery to union credential catalogs")
	}
}
