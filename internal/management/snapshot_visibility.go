package management

import (
	"ilonasin/internal/credentials"
)

func visibleUpstreamCredentials(rows []credentials.UpstreamCredentialMetadata, providers []ProviderInstance) []credentials.UpstreamCredentialMetadata {
	allowed := apiKeyProviderIDs(providers)
	out := rows[:0]
	for _, row := range rows {
		if allowed[row.ProviderInstanceID] {
			out = append(out, row)
		}
	}
	return out
}

func visibleFallbackPolicies(rows []credentials.FallbackPolicyMetadata, providers []ProviderInstance) []credentials.FallbackPolicyMetadata {
	allowed := fallbackPolicyProviderKinds(providers)
	out := rows[:0]
	for _, row := range rows {
		if allowed[row.ProviderInstanceID][row.CredentialKind] && row.CredentialCount >= 2 {
			out = append(out, row)
		}
	}
	return out
}

func fallbackPolicyProviderKinds(providers []ProviderInstance) map[string]map[string]bool {
	allowed := map[string]map[string]bool{}
	for _, instance := range providers {
		if instance.APIKey {
			allowed[instance.ID] = map[string]bool{credentials.CredentialKindAPIKey: true}
		}
		if instance.OAuth && instance.Type == "codex" {
			if allowed[instance.ID] == nil {
				allowed[instance.ID] = map[string]bool{}
			}
			allowed[instance.ID][credentials.CredentialKindOAuth] = true
		}
	}
	return allowed
}

func apiKeyProviderIDs(providers []ProviderInstance) map[string]bool {
	allowed := map[string]bool{}
	for _, instance := range providers {
		if instance.APIKey {
			allowed[instance.ID] = true
		}
	}
	return allowed
}

func visibleOAuthCredentials(rows []credentials.OAuthCredentialMetadata, providers []ProviderInstance) []credentials.OAuthCredentialMetadata {
	allowed := oauthProviderIDs(providers)
	out := rows[:0]
	for _, row := range rows {
		if allowed[row.ProviderInstanceID] {
			out = append(out, row)
		}
	}
	return out
}

func visibleProviderAccounts(rows []credentials.ProviderAccountMetadata, providers []ProviderInstance) []credentials.ProviderAccountMetadata {
	allowed := oauthProviderIDs(providers)
	out := rows[:0]
	for _, row := range rows {
		if allowed[row.ProviderInstanceID] {
			out = append(out, row)
		}
	}
	return out
}

func oauthProviderIDs(providers []ProviderInstance) map[string]bool {
	allowed := map[string]bool{}
	for _, instance := range providers {
		if instance.OAuth {
			allowed[instance.ID] = true
		}
	}
	return allowed
}
