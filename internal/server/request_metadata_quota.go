package server

import (
	"time"

	"ilonasin/internal/metadata"
	"ilonasin/internal/provider"
	"ilonasin/internal/routing"
)

func chatQuotaObservation(observedAt time.Time, addr routing.ModelAddress, credential provider.BearerCredential, source string, status int, errorClass string, retryAfter *time.Time) metadata.QuotaObservation {
	return metadata.QuotaObservation{
		ObservedAt:         observedAt,
		ProviderInstanceID: addr.ProviderInstanceID,
		CredentialID:       credential.ID,
		ModelID:            addr.ProviderModelID,
		Source:             source,
		HTTPStatus:         status,
		ErrorClass:         errorClass,
		RetryAfter:         retryAfter,
	}
}
