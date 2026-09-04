package routing

import "testing"

func TestAccountSelectorParsing(t *testing.T) {
	for _, model := range []string{"gpt-5.6-sol/daybreak-blue/daybreak-red", "codex/gpt-6-astra/daybreak-blue/daybreak-red", "/daybreak-blue", "   /daybreak-blue", "codex/gpt-6-astra/daybreak-", "codex/gpt-6-astra/daybreak-BLUE"} {
		if _, _, err := SplitAccountSelector(model); err == nil {
			t.Errorf("invalid selector accepted: %q", model)
		}
	}
	addr, err := ParseModelAddress("codex/gpt-5.6-sol/daybreak-blue")
	if err != nil || addr.ProviderInstanceID != "codex" || addr.ProviderModelID != "gpt-5.6-sol" || addr.AccountSelector != "daybreak-blue" || addr.RequestedModel() != "gpt-5.6-sol/daybreak-blue" {
		t.Fatalf("selector changed base model: %+v err=%v", addr, err)
	}
	addr, err = ParseModelAddress("openrouter/vendor/model")
	if err != nil || addr.ProviderModelID != "vendor/model" || addr.AccountSelector != "" {
		t.Fatalf("nested ordinary provider model changed: %+v err=%v", addr, err)
	}
}
