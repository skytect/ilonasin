package provider

import "testing"

func TestCodexDaybreakModelsRequireCredentialCatalogAvailability(t *testing.T) {
	adapter := NewHTTPChatAdapter(nil)
	instance := Instance{Type: "codex"}
	if got := adapter.ModelAvailabilityScope(instance, "gpt-daybreak-blue-latest"); got != ModelAvailabilityCredentialCatalog {
		t.Fatalf("expected Daybreak alias to require credential catalog eligibility, got %v", got)
	}
	if got := adapter.ModelAvailabilityScope(instance, "gpt-5.6-sol"); got != ModelAvailabilityShared {
		t.Fatalf("expected ordinary Codex model to retain shared routing, got %v", got)
	}
	if !adapter.RequiresCredentialCatalogUnion(instance) {
		t.Fatal("expected Codex model discovery to union credential catalogs")
	}
}
