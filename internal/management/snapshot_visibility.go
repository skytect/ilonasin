package management

import (
	"ilonasin/internal/credentials"
)

func visibleUpstreamCredentials(rows []credentials.UpstreamCredentialMetadata, providers []ProviderInstance) []credentials.UpstreamCredentialMetadata {
	allowed := apiKeyProviderIDs(providers)
	out := make([]credentials.UpstreamCredentialMetadata, 0, len(rows))
	for _, row := range rows {
		if allowed[row.ProviderInstanceID] {
			out = append(out, row)
		}
	}
	return out
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
	out := make([]credentials.OAuthCredentialMetadata, 0, len(rows))
	for _, row := range rows {
		if allowed[row.ProviderInstanceID] {
			out = append(out, row)
		}
	}
	return out
}

func visibleCredentialPoolGroupMetadata(rows []credentials.CredentialPoolGroupMetadata, providers []ProviderInstance) []credentials.CredentialPoolGroupMetadata {
	allowed := allowedPoolGroupCredentialKindsByProvider(providers)
	out := make([]credentials.CredentialPoolGroupMetadata, 0, len(rows))
	for _, row := range rows {
		if allowed[row.ProviderInstanceID][row.CredentialKind] && row.CredentialCount >= 2 {
			out = append(out, row)
		}
	}
	return out
}

func allowedPoolGroupCredentialKindsByProvider(providers []ProviderInstance) map[string]map[string]bool {
	allowed := map[string]map[string]bool{}
	for _, instance := range providers {
		kinds := poolGroupCredentialKinds(instance)
		if len(kinds) > 0 {
			allowed[instance.ID] = kinds
		}
	}
	return allowed
}

func poolGroupCredentialKinds(instance ProviderInstance) map[string]bool {
	out := map[string]bool{}
	if instance.APIKey {
		out[credentials.CredentialKindAPIKey] = true
	}
	if SupportsCodexOAuth(instance) {
		out[credentials.CredentialKindOAuth] = true
	}
	return out
}

func visibleProviderAccounts(rows []credentials.ProviderAccountMetadata, providers []ProviderInstance) []credentials.ProviderAccountMetadata {
	allowed := oauthProviderIDs(providers)
	out := make([]credentials.ProviderAccountMetadata, 0, len(rows))
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
