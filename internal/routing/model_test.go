package routing

import "testing"

func TestParseModelAddress(t *testing.T) {
	tests := []struct {
		name     string
		model    string
		provider string
		upstream string
		wantErr  bool
	}{
		{name: "deepseek", model: "deepseek/deepseek-v4-pro", provider: "deepseek", upstream: "deepseek-v4-pro"},
		{name: "openrouter nested", model: "openrouter/deepseek/deepseek-v4-pro", provider: "openrouter", upstream: "deepseek/deepseek-v4-pro"},
		{name: "codex", model: "codex/gpt-5.5", provider: "codex", upstream: "gpt-5.5"},
		{name: "missing separator", model: "deepseek-v4-pro", wantErr: true},
		{name: "empty provider", model: "/deepseek-v4-pro", wantErr: true},
		{name: "empty model", model: "deepseek/", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseModelAddress(tt.model)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got.ProviderInstanceID != tt.provider || got.ProviderModelID != tt.upstream {
				t.Fatalf("got %#v", got)
			}
		})
	}
}
