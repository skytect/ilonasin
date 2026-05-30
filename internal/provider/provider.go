package provider

import "fmt"

type Defaults struct {
	Type        string
	BaseURL     string
	AuthStyle   string
	Placeholder bool
}

var builtIns = map[string]Defaults{
	"deepseek": {
		Type:      "deepseek",
		BaseURL:   "https://api.deepseek.com",
		AuthStyle: "bearer_api_key",
	},
	"openrouter": {
		Type:      "openrouter",
		BaseURL:   "https://openrouter.ai/api/v1",
		AuthStyle: "bearer_api_key",
	},
	"codex": {
		Type:        "codex",
		BaseURL:     "https://chatgpt.com/backend-api/codex",
		AuthStyle:   "deferred",
		Placeholder: true,
	},
}

func Lookup(providerType string) (Defaults, error) {
	d, ok := builtIns[providerType]
	if !ok {
		return Defaults{}, fmt.Errorf("unknown provider type %q", providerType)
	}
	return d, nil
}
