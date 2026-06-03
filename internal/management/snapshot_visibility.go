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

func visibleCredentialPoolGroups(rows []credentials.CredentialPoolGroupMetadata, providers []ProviderInstance) []credentials.CredentialPoolGroupMetadata {
	return visibleCredentialPoolGroupMetadata(rows, providers)
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
