package management

import (
	"net/url"
	"regexp"
	"strings"
	"unicode"
)

var unsafeSnapshotStringPattern = regexp.MustCompile(`(?i)(bearer|sk-|iln_|oauth|token|secret|authorization|raw|payload|prompt|completion|body|account|acct[-_]|request[_ -]?id|requestid|req[-_]|balance|credit|sse[_ -]?chunk|tool[_ -]?(argument|result)|eyj[a-z0-9_-]*\.[a-z0-9_-]*\.)`)
var unsafeAccountDisplayPattern = regexp.MustCompile(`(?i)(bearer|sk-|iln_|token|secret|authorization|raw|payload|prompt|completion|body|acct[-_]|request[_ -]?id|requestid|req[-_]|balance|credit|sse[_ -]?chunk|tool[_ -]?(argument|result)|eyj[a-z0-9_-]*\.[a-z0-9_-]*\.)`)

func sanitizeSnapshot(out *ManagementSnapshotResponse) {
	for i := range out.Providers {
		out.Providers[i].ID = safeMachineString(out.Providers[i].ID)
		out.Providers[i].Type = safeSnapshotString(out.Providers[i].Type)
	}
	for i := range out.LocalTokens {
		out.LocalTokens[i].Label = safeSnapshotString(out.LocalTokens[i].Label)
		out.LocalTokens[i].TokenPrefix = safeTokenFragment(out.LocalTokens[i].TokenPrefix, 8)
		out.LocalTokens[i].TokenLast4 = safeTokenFragment(out.LocalTokens[i].TokenLast4, 4)
	}
	for i := range out.UpstreamCredentials {
		out.UpstreamCredentials[i].ProviderInstanceID = safeMachineString(out.UpstreamCredentials[i].ProviderInstanceID)
		out.UpstreamCredentials[i].Label = safeSnapshotString(out.UpstreamCredentials[i].Label)
		out.UpstreamCredentials[i].SecretPrefix = safeSecretFragment(out.UpstreamCredentials[i].SecretPrefix, 8, "sk-")
		out.UpstreamCredentials[i].SecretLast4 = safeSecretFragment(out.UpstreamCredentials[i].SecretLast4, 4)
		out.UpstreamCredentials[i].FallbackGroup = safeSnapshotString(out.UpstreamCredentials[i].FallbackGroup)
	}
	for i := range out.FallbackPolicies {
		out.FallbackPolicies[i].ProviderInstanceID = safeMachineString(out.FallbackPolicies[i].ProviderInstanceID)
		out.FallbackPolicies[i].GroupLabel = safeSnapshotString(out.FallbackPolicies[i].GroupLabel)
	}
	for i := range out.OAuthCredentials {
		out.OAuthCredentials[i].ProviderInstanceID = safeMachineString(out.OAuthCredentials[i].ProviderInstanceID)
		out.OAuthCredentials[i].Label = safeSnapshotString(out.OAuthCredentials[i].Label)
		out.OAuthCredentials[i].AccountDisplayLabel = safeAccountDisplayString(out.OAuthCredentials[i].AccountDisplayLabel)
		out.OAuthCredentials[i].PlanLabel = safeSnapshotString(out.OAuthCredentials[i].PlanLabel)
		out.OAuthCredentials[i].Scopes = safeSnapshotString(out.OAuthCredentials[i].Scopes)
		out.OAuthCredentials[i].RefreshFailureClass = safeRefreshFailureClass(out.OAuthCredentials[i].RefreshFailureClass)
		out.OAuthCredentials[i].RefreshFailureDescription = safeRefreshFailureDescription(out.OAuthCredentials[i].RefreshFailureDescription)
	}
	for i := range out.ProviderAccounts {
		out.ProviderAccounts[i].ProviderInstanceID = safeMachineString(out.ProviderAccounts[i].ProviderInstanceID)
		out.ProviderAccounts[i].DisplayLabel = safeAccountDisplayString(out.ProviderAccounts[i].DisplayLabel)
		out.ProviderAccounts[i].PlanLabel = safeSnapshotString(out.ProviderAccounts[i].PlanLabel)
	}
	for i := range out.ModelCache {
		out.ModelCache[i].ProviderInstanceID = safeMachineString(out.ModelCache[i].ProviderInstanceID)
		out.ModelCache[i].ModelID = safeSnapshotString(out.ModelCache[i].ModelID)
		out.ModelCache[i].DisplayName = safeSnapshotString(out.ModelCache[i].DisplayName)
		out.ModelCache[i].Capabilities = safeSnapshotString(out.ModelCache[i].Capabilities)
	}
	for i := range out.RecentRequests {
		out.RecentRequests[i].ProviderInstanceID = safeMachineString(out.RecentRequests[i].ProviderInstanceID)
		out.RecentRequests[i].ModelID = safeSnapshotString(out.RecentRequests[i].ModelID)
		out.RecentRequests[i].Endpoint = safeEndpointString(out.RecentRequests[i].Endpoint)
		out.RecentRequests[i].ProviderType = safeSnapshotString(out.RecentRequests[i].ProviderType)
		out.RecentRequests[i].RequestedServiceTier = safeSnapshotString(out.RecentRequests[i].RequestedServiceTier)
		out.RecentRequests[i].EffectiveServiceTier = safeSnapshotString(out.RecentRequests[i].EffectiveServiceTier)
		out.RecentRequests[i].ReasoningEffort = safeSnapshotString(out.RecentRequests[i].ReasoningEffort)
		out.RecentRequests[i].ReasoningSummary = safeSnapshotString(out.RecentRequests[i].ReasoningSummary)
		out.RecentRequests[i].ThinkingType = safeSnapshotString(out.RecentRequests[i].ThinkingType)
		out.RecentRequests[i].RequestedProviderID = safeMachineString(out.RecentRequests[i].RequestedProviderID)
		out.RecentRequests[i].RequestedModelID = safeSnapshotString(out.RecentRequests[i].RequestedModelID)
		out.RecentRequests[i].ResolvedProviderID = safeMachineString(out.RecentRequests[i].ResolvedProviderID)
		out.RecentRequests[i].ResolvedModelID = safeSnapshotString(out.RecentRequests[i].ResolvedModelID)
		out.RecentRequests[i].CredentialLabel = safeSnapshotString(out.RecentRequests[i].CredentialLabel)
		out.RecentRequests[i].ErrorClass = safeSnapshotString(out.RecentRequests[i].ErrorClass)
		out.RecentRequests[i].FallbackReason = safeSnapshotString(out.RecentRequests[i].FallbackReason)
		out.RecentRequests[i].StreamCompletionStatus = safeSnapshotString(out.RecentRequests[i].StreamCompletionStatus)
	}
	for i := range out.Usage {
		out.Usage[i].ProviderInstanceID = safeMachineString(out.Usage[i].ProviderInstanceID)
	}
	for i := range out.Latency {
		out.Latency[i].ProviderInstanceID = safeMachineString(out.Latency[i].ProviderInstanceID)
	}
	for i := range out.Streams {
		out.Streams[i].CompletionStatus = safeSnapshotString(out.Streams[i].CompletionStatus)
	}
	for i := range out.Health {
		out.Health[i].ProviderInstanceID = safeMachineString(out.Health[i].ProviderInstanceID)
		out.Health[i].ModelID = safeSnapshotString(out.Health[i].ModelID)
		out.Health[i].CredentialLabel = safeSnapshotString(out.Health[i].CredentialLabel)
		out.Health[i].EventClass = safeSnapshotString(out.Health[i].EventClass)
		out.Health[i].ErrorClass = safeSnapshotString(out.Health[i].ErrorClass)
	}
	for i := range out.Fallbacks {
		out.Fallbacks[i].ProviderInstanceID = safeMachineString(out.Fallbacks[i].ProviderInstanceID)
		out.Fallbacks[i].ModelID = safeSnapshotString(out.Fallbacks[i].ModelID)
		out.Fallbacks[i].FromCredentialLabel = safeSnapshotString(out.Fallbacks[i].FromCredentialLabel)
		out.Fallbacks[i].ToCredentialLabel = safeSnapshotString(out.Fallbacks[i].ToCredentialLabel)
		out.Fallbacks[i].Reason = safeSnapshotString(out.Fallbacks[i].Reason)
	}
	for i := range out.Quotas {
		out.Quotas[i].ProviderInstanceID = safeMachineString(out.Quotas[i].ProviderInstanceID)
		out.Quotas[i].ModelID = safeSnapshotString(out.Quotas[i].ModelID)
		out.Quotas[i].CredentialLabel = safeSnapshotString(out.Quotas[i].CredentialLabel)
		out.Quotas[i].Source = safeSnapshotString(out.Quotas[i].Source)
		out.Quotas[i].ErrorClass = safeSnapshotString(out.Quotas[i].ErrorClass)
	}
	sanitizeSubscriptionUsageResponse(&out.SubscriptionUsage)
}

func safeSnapshotString(value string) string {
	value = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, strings.TrimSpace(value))
	if value == "" {
		return ""
	}
	if unsafeSnapshotStringPattern.MatchString(value) {
		return "[redacted]"
	}
	const maxRunes = 128
	runes := []rune(value)
	if len(runes) > maxRunes {
		return string(runes[:maxRunes]) + "..."
	}
	return value
}

func safeAccountDisplayString(value string) string {
	value = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, strings.TrimSpace(value))
	if value == "" {
		return ""
	}
	if unsafeAccountDisplayPattern.MatchString(value) {
		return "[redacted]"
	}
	const maxRunes = 128
	runes := []rune(value)
	if len(runes) > maxRunes {
		return string(runes[:maxRunes]) + "..."
	}
	return value
}

func safeEndpointString(value string) string {
	value = strings.TrimSpace(value)
	switch value {
	case "chat_completions", "responses":
		return value
	default:
		return ""
	}
}

func safeRefreshFailureDescription(value string) string {
	value = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, strings.TrimSpace(value))
	value = strings.Join(strings.Fields(value), " ")
	if value == "" {
		return ""
	}
	const maxRunes = 1024
	runes := []rune(value)
	if len(runes) > maxRunes {
		return string(runes[:maxRunes])
	}
	return value
}

func safeRefreshFailureClass(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	return safeErrorToken(value)
}

func safeMachineString(value string) string {
	value = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.IsSpace(r) {
			return -1
		}
		return r
	}, strings.TrimSpace(value))
	if value == "" {
		return ""
	}
	return value
}

func safeTokenFragment(value string, maxLen int) string {
	return safeSecretFragment(value, maxLen, "iln_")
}

func safeSecretFragment(value string, maxLen int, allowedUnsafePrefixes ...string) string {
	value = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.IsSpace(r) {
			return -1
		}
		return r
	}, strings.TrimSpace(value))
	if value == "" {
		return ""
	}
	if len([]rune(value)) > maxLen {
		return "[redacted]"
	}
	if unsafeSnapshotStringPattern.MatchString(value) && !hasAllowedUnsafePrefix(value, allowedUnsafePrefixes) {
		return "[redacted]"
	}
	return value
}

func hasAllowedUnsafePrefix(value string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(value, prefix) {
			return true
		}
	}
	return false
}

func safeBaseURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	if unsafeSnapshotStringPattern.MatchString(u.Host) {
		return "[redacted]"
	}
	u.User = nil
	u.RawQuery = ""
	u.ForceQuery = false
	u.Fragment = ""
	if unsafeSnapshotStringPattern.MatchString(u.Path) {
		u.Path = ""
		u.RawPath = ""
	}
	return u.String()
}
