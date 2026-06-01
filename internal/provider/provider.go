package provider

import (
	"fmt"
	"net/url"
	"sort"
	"strings"
	"unicode"

	"ilonasin/internal/config"
)

type Defaults struct {
	Type           string
	BaseURL        string
	AuthIssuer     string
	AuthStyle      string
	APIKey         bool
	OAuth          bool
	OAuthRefresh   bool
	Chat           bool
	ModelDiscovery bool
}

var builtIns = map[string]Defaults{
	"deepseek": {
		Type:           "deepseek",
		BaseURL:        "https://api.deepseek.com",
		AuthStyle:      "bearer_api_key",
		APIKey:         true,
		Chat:           true,
		ModelDiscovery: true,
	},
	"openrouter": {
		Type:           "openrouter",
		BaseURL:        "https://openrouter.ai/api/v1",
		AuthStyle:      "bearer_api_key",
		APIKey:         true,
		Chat:           true,
		ModelDiscovery: true,
	},
	"codex": {
		Type:           "codex",
		BaseURL:        "https://chatgpt.com/backend-api/codex",
		AuthIssuer:     "https://auth.openai.com",
		AuthStyle:      "deferred",
		OAuth:          true,
		OAuthRefresh:   true,
		Chat:           true,
		ModelDiscovery: true,
	},
}

func Lookup(providerType string) (Defaults, error) {
	d, ok := builtIns[providerType]
	if !ok {
		return Defaults{}, fmt.Errorf("unknown provider type %q", providerType)
	}
	return d, nil
}

type Registry struct {
	instances map[string]Instance
	ordered   []Instance
}

type Instance struct {
	ID             string
	Type           string
	BaseURL        string
	AuthIssuer     string
	AuthStyle      string
	APIKey         bool
	OAuth          bool
	OAuthRefresh   bool
	Chat           bool
	ModelDiscovery bool
}

func NewRegistry(cfg config.Config) (Registry, error) {
	ids := make([]string, 0, len(cfg.Providers))
	for id := range cfg.Providers {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	reg := Registry{instances: map[string]Instance{}}
	for _, id := range ids {
		if err := validateInstanceID(id); err != nil {
			return Registry{}, err
		}
		providerCfg := cfg.Providers[id]
		def, err := Lookup(providerCfg.Type)
		if err != nil {
			return Registry{}, err
		}
		baseURL := def.BaseURL
		if providerCfg.BaseURL != "" {
			baseURL = providerCfg.BaseURL
			if err := validateHTTPSBaseURL(baseURL); err != nil {
				return Registry{}, fmt.Errorf("provider %q base_url: %w", id, err)
			}
		}
		authIssuer := def.AuthIssuer
		if providerCfg.AuthIssuer != "" {
			if !def.OAuthRefresh {
				return Registry{}, fmt.Errorf("provider %q auth_issuer: provider type does not support oauth refresh", id)
			}
			authIssuer = providerCfg.AuthIssuer
			if err := validateHTTPSAuthIssuer(authIssuer); err != nil {
				return Registry{}, fmt.Errorf("provider %q auth_issuer: %w", id, err)
			}
		}
		instance := Instance{
			ID:             id,
			Type:           providerCfg.Type,
			BaseURL:        baseURL,
			AuthIssuer:     authIssuer,
			AuthStyle:      def.AuthStyle,
			APIKey:         def.APIKey,
			OAuth:          def.OAuth,
			OAuthRefresh:   def.OAuthRefresh,
			Chat:           def.Chat,
			ModelDiscovery: def.ModelDiscovery,
		}
		reg.instances[id] = instance
		reg.ordered = append(reg.ordered, instance)
	}
	return reg, nil
}

func (r Registry) Get(id string) (Instance, bool) {
	instance, ok := r.instances[id]
	return instance, ok
}

func (r Registry) List() []Instance {
	out := make([]Instance, len(r.ordered))
	copy(out, r.ordered)
	return out
}

func validateInstanceID(id string) error {
	if id == "" {
		return fmt.Errorf("provider instance id must not be empty")
	}
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= '0' && r <= '9':
		case r == '_' || r == '-':
		default:
			if r == '/' || unicode.IsSpace(r) || unicode.IsControl(r) {
				return fmt.Errorf("invalid provider instance id %q", id)
			}
			return fmt.Errorf("provider instance id %q must use lowercase ASCII letters, digits, '_' or '-'", id)
		}
	}
	if strings.ToLower(id) != id {
		return fmt.Errorf("provider instance id %q must be lowercase", id)
	}
	return nil
}

func validateHTTPSBaseURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return err
	}
	if u.Scheme != "https" || u.Host == "" {
		return fmt.Errorf("must be an https URL")
	}
	return nil
}

func validateHTTPSAuthIssuer(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return err
	}
	if u.Scheme != "https" || u.Host == "" {
		return fmt.Errorf("must be an https URL")
	}
	if u.User != nil {
		return fmt.Errorf("must not include userinfo")
	}
	if u.RawQuery != "" {
		return fmt.Errorf("must not include query")
	}
	if u.Fragment != "" {
		return fmt.Errorf("must not include fragment")
	}
	return nil
}
