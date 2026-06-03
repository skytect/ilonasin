package management

import (
	"context"
	"fmt"
	"strings"
	"unicode"

	"ilonasin/internal/credentials"
)

type AddUpstreamAPIKeyRequest struct {
	ProviderInstanceID string `json:"provider_instance_id"`
	Label              string `json:"label"`
	APIKey             string `json:"api_key"`
}

type AddUpstreamAPIKeyResponse struct {
	Credential UpstreamCredential `json:"credential"`
}

type DisableUpstreamCredentialRequest struct {
	ID int64 `json:"id"`
}

type DisableUpstreamCredentialResponse struct {
	Disabled bool `json:"disabled"`
}

type UpstreamCredentialClient interface {
	AddUpstreamAPIKey(ctx context.Context, req AddUpstreamAPIKeyRequest) (AddUpstreamAPIKeyResponse, error)
	DisableUpstreamCredential(ctx context.Context, req DisableUpstreamCredentialRequest) (DisableUpstreamCredentialResponse, error)
}

type UpstreamMutationManager interface {
	AddAPIKey(ctx context.Context, providerInstanceID, label, apiKey string) (credentials.UpstreamCredentialMetadata, error)
	Disable(ctx context.Context, id int64) error
}

func (s Service) AddUpstreamAPIKey(ctx context.Context, req AddUpstreamAPIKeyRequest) (AddUpstreamAPIKeyResponse, error) {
	if s.UpstreamMutations == nil {
		return AddUpstreamAPIKeyResponse{}, fmt.Errorf("upstream mutations unavailable")
	}
	apiKey := strings.TrimSpace(req.APIKey)
	comparableLabel := comparableManagementText(req.Label)
	label := safeSnapshotString(comparableLabel)
	if label == "" || labelContainsSecret(comparableLabel, apiKey) {
		label = "api key"
	}
	row, err := s.UpstreamMutations.AddAPIKey(ctx, req.ProviderInstanceID, label, req.APIKey)
	if err != nil {
		return AddUpstreamAPIKeyResponse{}, err
	}
	return AddUpstreamAPIKeyResponse{Credential: upstreamCredentialFromCredentials(row)}, nil
}

func (s Service) DisableUpstreamCredential(ctx context.Context, req DisableUpstreamCredentialRequest) (DisableUpstreamCredentialResponse, error) {
	if s.UpstreamMutations == nil {
		return DisableUpstreamCredentialResponse{}, fmt.Errorf("upstream mutations unavailable")
	}
	if err := s.UpstreamMutations.Disable(ctx, req.ID); err != nil {
		return DisableUpstreamCredentialResponse{}, err
	}
	return DisableUpstreamCredentialResponse{Disabled: true}, nil
}

func upstreamCredentialFromCredentials(row credentials.UpstreamCredentialMetadata) UpstreamCredential {
	return UpstreamCredential{
		ID:                 row.ID,
		ProviderInstanceID: safeMachineString(row.ProviderInstanceID),
		Kind:               row.Kind,
		Label:              safeSnapshotString(row.Label),
		SecretPrefix:       safeSecretFragment(row.SecretPrefix, 8, "sk-"),
		SecretLast4:        safeSecretFragment(row.SecretLast4, 4),
		PoolGroup:          safeSnapshotString(row.PoolGroup),
		CreatedAt:          row.CreatedAt,
		DisabledAt:         row.DisabledAt,
		Disabled:           row.Disabled,
	}
}

func credentialPoolGroupFromCredentials(row credentials.CredentialPoolGroupMetadata) CredentialPoolGroup {
	return CredentialPoolGroup{
		ProviderInstanceID: safeMachineString(row.ProviderInstanceID),
		CredentialKind:     row.CredentialKind,
		GroupLabel:         safeSnapshotString(row.GroupLabel),
		CredentialCount:    row.CredentialCount,
	}
}

func labelContainsSecret(label, secret string) bool {
	label = comparableManagementText(label)
	secret = comparableManagementText(secret)
	if label == "" || secret == "" {
		return false
	}
	if strings.Contains(label, secret) {
		return true
	}
	return len(label) >= 4 && strings.Contains(secret, label)
}

func comparableManagementText(value string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, strings.TrimSpace(value))
}
