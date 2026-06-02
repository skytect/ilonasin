package management

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strings"

	"ilonasin/internal/credentials"
)

type StartOAuthDeviceLoginRequest struct {
	ProviderInstanceID string `json:"provider_instance_id"`
}

type StartOAuthDeviceLoginResponse struct {
	Challenge OAuthDeviceLoginChallenge `json:"challenge"`
}

type CompleteOAuthDeviceLoginRequest struct {
	Handle string `json:"handle"`
}

type CompleteOAuthDeviceLoginResponse struct {
	Credential OAuthCredential `json:"credential"`
}

type RefreshOAuthCredentialRequest struct {
	ID int64 `json:"id"`
}

type RefreshOAuthCredentialResponse struct {
	Refreshed bool `json:"refreshed"`
}

type OAuthDeviceLoginChallenge struct {
	ProviderInstanceID string `json:"provider_instance_id"`
	VerificationURL    string `json:"verification_url"`
	UserCode           string `json:"user_code"`
	Handle             string `json:"handle"`
}

type OAuthClient interface {
	StartOAuthDeviceLogin(ctx context.Context, req StartOAuthDeviceLoginRequest) (StartOAuthDeviceLoginResponse, error)
	CompleteOAuthDeviceLogin(ctx context.Context, req CompleteOAuthDeviceLoginRequest) (CompleteOAuthDeviceLoginResponse, error)
	RefreshOAuthCredential(ctx context.Context, req RefreshOAuthCredentialRequest) (RefreshOAuthCredentialResponse, error)
}

type OAuthMutationManager interface {
	StartOAuthDeviceLogin(ctx context.Context, providerInstanceID string) (credentials.OAuthDeviceLoginChallenge, error)
	CompleteOAuthDeviceLogin(ctx context.Context, handle string) (credentials.OAuthCredentialMetadata, error)
	RefreshOAuthCredential(ctx context.Context, credentialID int64) error
}

type managementErrorResponse struct {
	Error   string `json:"error"`
	Class   string `json:"class,omitempty"`
	EventID string `json:"event_id,omitempty"`
}

func (s Service) StartOAuthDeviceLogin(ctx context.Context, req StartOAuthDeviceLoginRequest) (StartOAuthDeviceLoginResponse, error) {
	if s.OAuthMutations == nil {
		return StartOAuthDeviceLoginResponse{}, credentials.ErrUnsupportedCredential
	}
	challenge, err := s.OAuthMutations.StartOAuthDeviceLogin(ctx, req.ProviderInstanceID)
	if err != nil {
		return StartOAuthDeviceLoginResponse{}, err
	}
	return StartOAuthDeviceLoginResponse{Challenge: oauthChallengeFromCredentials(challenge)}, nil
}

func (s Service) CompleteOAuthDeviceLogin(ctx context.Context, req CompleteOAuthDeviceLoginRequest) (CompleteOAuthDeviceLoginResponse, error) {
	if s.OAuthMutations == nil {
		return CompleteOAuthDeviceLoginResponse{}, credentials.ErrUnsupportedCredential
	}
	row, err := s.OAuthMutations.CompleteOAuthDeviceLogin(ctx, req.Handle)
	if err != nil {
		return CompleteOAuthDeviceLoginResponse{}, err
	}
	return CompleteOAuthDeviceLoginResponse{Credential: oauthCredentialFromCredentials(row)}, nil
}

func (s Service) RefreshOAuthCredential(ctx context.Context, req RefreshOAuthCredentialRequest) (RefreshOAuthCredentialResponse, error) {
	if s.OAuthMutations == nil {
		return RefreshOAuthCredentialResponse{}, credentials.ErrUnsupportedCredential
	}
	if err := s.OAuthMutations.RefreshOAuthCredential(ctx, req.ID); err != nil {
		return RefreshOAuthCredentialResponse{}, err
	}
	return RefreshOAuthCredentialResponse{Refreshed: true}, nil
}

func oauthChallengeFromCredentials(row credentials.OAuthDeviceLoginChallenge) OAuthDeviceLoginChallenge {
	return OAuthDeviceLoginChallenge{
		ProviderInstanceID: safeSnapshotString(row.ProviderInstanceID),
		VerificationURL:    safeBaseURL(row.VerificationURL),
		UserCode:           safeSnapshotString(row.UserCode),
		Handle:             safeOAuthHandle(row.Handle),
	}
}

func oauthCredentialFromCredentials(row credentials.OAuthCredentialMetadata) OAuthCredential {
	return OAuthCredential{
		ID:                        row.ID,
		ProviderInstanceID:        safeSnapshotString(row.ProviderInstanceID),
		Label:                     safeSnapshotString(row.Label),
		AccountDisplayLabel:       safeSnapshotString(row.AccountDisplayLabel),
		PlanLabel:                 safeSnapshotString(row.PlanLabel),
		Scopes:                    safeSnapshotString(row.Scopes),
		ExpiresAt:                 row.ExpiresAt,
		LastRefreshAt:             row.LastRefreshAt,
		RefreshFailureClass:       safeRefreshFailureClass(row.RefreshFailureClass),
		RefreshFailureDescription: safeRefreshFailureDescription(row.RefreshFailureDescription),
		CreatedAt:                 row.CreatedAt,
		DisabledAt:                row.DisabledAt,
		Disabled:                  row.Disabled,
	}
}

func writeOAuthManagementError(w http.ResponseWriter, err error) {
	status := http.StatusBadGateway
	switch {
	case errors.Is(err, credentials.ErrCredentialNotFound):
		status = http.StatusNotFound
	case errors.Is(err, credentials.ErrNoEligibleCredential),
		errors.Is(err, credentials.ErrUnsupportedCredential),
		errors.Is(err, credentials.ErrInvalidOAuthInput):
		status = http.StatusBadRequest
	}
	class, eventID := safeOAuthErrorClass(err)
	writeJSON(w, status, managementErrorResponse{Error: http.StatusText(status), Class: class, EventID: eventID})
}

func safeOAuthErrorClass(err error) (string, string) {
	if err == nil {
		return "", ""
	}
	if class := credentials.OAuthDeviceLoginErrorClass(err); class != "" {
		return safeManagementErrorClass(class), safeEventID(credentials.OAuthDeviceLoginErrorEventID(err))
	}
	if errors.Is(err, credentials.ErrOAuthRefreshFailed) {
		return "oauth_refresh_failed", ""
	}
	if class := credentials.OAuthRefreshErrorClass(err); class != "" {
		return safeManagementErrorClass(class), ""
	}
	switch {
	case errors.Is(err, credentials.ErrCredentialNotFound):
		return "credential_not_found", ""
	case errors.Is(err, credentials.ErrNoEligibleCredential):
		return "oauth_login_expired", ""
	case errors.Is(err, credentials.ErrUnsupportedCredential):
		return "unsupported_credential", ""
	case errors.Is(err, credentials.ErrInvalidOAuthInput):
		return "invalid_oauth_input", ""
	default:
		return "oauth_unavailable", ""
	}
}

var safeManagementTokenPattern = regexp.MustCompile(`^[A-Za-z0-9_.:-]{1,128}$`)

func safeErrorToken(value string) string {
	value = strings.TrimSpace(value)
	if safeManagementTokenPattern.MatchString(value) {
		return value
	}
	return "details_redacted"
}

var safeManagementErrorClassWithMarkerPattern = regexp.MustCompile(`^refresh_token_(expired|invalidated|reused)$`)
var unsafeManagementErrorClassPattern = regexp.MustCompile(`(?i)(^|[_.:-])(bearer|sk|iln|token|secret|authorization|raw|payload|prompt|completion|account|acct|requestid|request[-_.:]id|req|balance|credit|sse|argument|result)($|[_.:-])`)

func safeManagementErrorClass(value string) string {
	value = safeErrorToken(value)
	if value == "details_redacted" {
		return value
	}
	if safeManagementErrorClassWithMarkerPattern.MatchString(value) || !unsafeManagementErrorClassPattern.MatchString(value) {
		return value
	}
	return "details_redacted"
}

func safeEventID(value string) string {
	value = strings.TrimSpace(value)
	if safeManagementTokenPattern.MatchString(value) && !unsafeSnapshotStringPattern.MatchString(value) {
		return value
	}
	return ""
}

func safeOAuthHandle(value string) string {
	value = strings.TrimSpace(value)
	if safeManagementTokenPattern.MatchString(value) {
		return value
	}
	return ""
}
