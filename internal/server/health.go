package server

import (
	"net/http"
	"time"

	"ilonasin/internal/metadata"
	"ilonasin/internal/provider"
	"ilonasin/internal/routing"
)

func healthFromChatAttempt(addr routing.ModelAddress, attempt chatAttempt) metadata.HealthEvent {
	status := normalizedChatStatus(attempt.result)
	errorClass := normalizedChatErrorClass(attempt.result, status)
	eventClass := "upstream_failure"
	if attempt.err == nil && status >= 200 && status < 300 {
		eventClass = "upstream_success"
		errorClass = ""
	}
	retryAfter := attempt.result.RetryAfter
	if eventClass == "upstream_success" {
		retryAfter = nil
	}
	return metadata.HealthEvent{
		OccurredAt:         time.Now(),
		ProviderInstanceID: addr.ProviderInstanceID,
		CredentialID:       attempt.credential.ID,
		ModelID:            addr.ProviderModelID,
		EventClass:         eventClass,
		HTTPStatus:         status,
		ErrorClass:         errorClass,
		RetryAfter:         retryAfter,
	}
}

func healthFromModelDiscovery(instance provider.Instance, credential provider.BearerCredential, result provider.ModelResult, err error) metadata.HealthEvent {
	status := result.StatusCode
	if status == 0 {
		status = http.StatusBadGateway
	}
	errorClass := result.ErrorClass
	if errorClass == "" && status >= 400 {
		errorClass = "upstream_http_error"
	}
	eventClass := "upstream_failure"
	if err == nil && len(result.Models) > 0 && status >= 200 && status < 300 {
		eventClass = "upstream_success"
		errorClass = ""
	}
	retryAfter := result.RetryAfter
	if eventClass == "upstream_success" {
		retryAfter = nil
	}
	return metadata.HealthEvent{
		OccurredAt:         time.Now(),
		ProviderInstanceID: instance.ID,
		CredentialID:       credential.ID,
		ModelID:            "",
		EventClass:         eventClass,
		HTTPStatus:         status,
		ErrorClass:         errorClass,
		RetryAfter:         retryAfter,
	}
}

func cyberHealthEventsFromChat(addr routing.ModelAddress, credential provider.BearerCredential, result provider.ChatResult) []metadata.HealthEvent {
	return cyberHealthEvents(addr, credential, result.StatusCode, result.HealthEventClasses)
}

func cyberHealthEventsFromStream(addr routing.ModelAddress, credential provider.BearerCredential, summary provider.ChatStreamSummary) []metadata.HealthEvent {
	return cyberHealthEvents(addr, credential, summary.StatusCode, summary.HealthEventClasses)
}

func cyberHealthEvents(addr routing.ModelAddress, credential provider.BearerCredential, status int, classes []string) []metadata.HealthEvent {
	if len(classes) == 0 {
		return nil
	}
	if status == 0 {
		status = http.StatusOK
	}
	now := time.Now()
	out := make([]metadata.HealthEvent, 0, len(classes))
	for _, class := range classes {
		if !isCodexCyberHealthEvent(class) {
			continue
		}
		out = append(out, metadata.HealthEvent{
			OccurredAt:         now,
			ProviderInstanceID: addr.ProviderInstanceID,
			CredentialID:       credential.ID,
			ModelID:            addr.ProviderModelID,
			EventClass:         class,
			HTTPStatus:         status,
			ErrorClass:         class,
		})
	}
	return out
}

func isCodexCyberHealthEvent(class string) bool {
	switch class {
	case "codex_verification_recommended", "codex_mitigated_rerouted", "codex_policy_blocked":
		return true
	default:
		return false
	}
}
