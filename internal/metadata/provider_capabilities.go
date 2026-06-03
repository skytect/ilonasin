package metadata

func SupportsCodexOAuth(providerType string, oauth bool) bool {
	return providerType == "codex" && oauth
}

func SupportsCodexOAuthRefresh(providerType string, oauth, oauthRefresh bool) bool {
	return SupportsCodexOAuth(providerType, oauth) && oauthRefresh
}
