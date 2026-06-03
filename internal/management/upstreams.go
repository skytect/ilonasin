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

type FallbackPolicyRequest struct {
	ProviderInstanceID string `json:"provider_instance_id"`
	CredentialKind     string `json:"credential_kind"`
	GroupLabel         string `json:"group_label"`
}

type FallbackPolicyResponse struct {
	Policy FallbackPolicy `json:"policy"`
}

const (
	CredentialKindAPIKey = "api_key"
	CredentialKindOAuth  = "oauth"
)

func ProviderAllowsFallbackCredentialKind(instance ProviderInstance, credentialKind string) bool {
	return fallbackCredentialKinds(instance)[credentialKind]
}

func fallbackCredentialKinds(instance ProviderInstance) map[string]bool {
	out := map[string]bool{}
	if instance.APIKey {
		out[CredentialKindAPIKey] = true
	}
	if instance.OAuth && instance.Type == "codex" {
		out[CredentialKindOAuth] = true
	}
	return out
}

func allowedFallbackCredentialKindsByProvider(providers []ProviderInstance) map[string]map[string]bool {
	allowed := map[string]map[string]bool{}
	for _, instance := range providers {
		kinds := fallbackCredentialKinds(instance)
		if len(kinds) > 0 {
			allowed[instance.ID] = kinds
		}
	}
	return allowed
}

func VisibleFallbackPolicies(rows []FallbackPolicy, providers []ProviderInstance) []FallbackPolicy {
	allowed := allowedFallbackCredentialKindsByProvider(providers)
	out := make([]FallbackPolicy, 0, len(rows))
	for _, row := range rows {
		if allowed[row.ProviderInstanceID][row.CredentialKind] && row.CredentialCount >= 2 {
			out = append(out, row)
		}
	}
	return out
}

func visibleFallbackPolicyMetadata(rows []credentials.FallbackPolicyMetadata, providers []ProviderInstance) []credentials.FallbackPolicyMetadata {
	allowed := allowedFallbackCredentialKindsByProvider(providers)
	out := rows[:0]
	for _, row := range rows {
		if allowed[row.ProviderInstanceID][row.CredentialKind] && row.CredentialCount >= 2 {
			out = append(out, row)
		}
	}
	return out
}

type UpstreamCredentialClient interface {
	AddUpstreamAPIKey(ctx context.Context, req AddUpstreamAPIKeyRequest) (AddUpstreamAPIKeyResponse, error)
	DisableUpstreamCredential(ctx context.Context, req DisableUpstreamCredentialRequest) (DisableUpstreamCredentialResponse, error)
	EnableFallbackPolicy(ctx context.Context, req FallbackPolicyRequest) (FallbackPolicyResponse, error)
	DisableFallbackPolicy(ctx context.Context, req FallbackPolicyRequest) (FallbackPolicyResponse, error)
}

type UpstreamMutationManager interface {
	AddAPIKey(ctx context.Context, providerInstanceID, label, apiKey string) (credentials.UpstreamCredentialMetadata, error)
	Disable(ctx context.Context, id int64) error
	EnableFallbackGroup(ctx context.Context, providerInstanceID, credentialKind, groupLabel string) error
	DisableFallbackGroup(ctx context.Context, providerInstanceID, credentialKind, groupLabel string) error
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

func (s Service) EnableFallbackPolicy(ctx context.Context, req FallbackPolicyRequest) (FallbackPolicyResponse, error) {
	if s.UpstreamMutations == nil {
		return FallbackPolicyResponse{}, fmt.Errorf("upstream mutations unavailable")
	}
	groupLabel, err := safeManagementGroupLabel(req.GroupLabel)
	if err != nil {
		return FallbackPolicyResponse{}, err
	}
	req.GroupLabel = groupLabel
	if err := s.UpstreamMutations.EnableFallbackGroup(ctx, req.ProviderInstanceID, req.CredentialKind, req.GroupLabel); err != nil {
		return FallbackPolicyResponse{}, err
	}
	return s.fallbackPolicyResponse(ctx, req, true)
}

func (s Service) DisableFallbackPolicy(ctx context.Context, req FallbackPolicyRequest) (FallbackPolicyResponse, error) {
	if s.UpstreamMutations == nil {
		return FallbackPolicyResponse{}, fmt.Errorf("upstream mutations unavailable")
	}
	groupLabel, err := safeManagementGroupLabel(req.GroupLabel)
	if err != nil {
		return FallbackPolicyResponse{}, err
	}
	req.GroupLabel = groupLabel
	if err := s.UpstreamMutations.DisableFallbackGroup(ctx, req.ProviderInstanceID, req.CredentialKind, req.GroupLabel); err != nil {
		return FallbackPolicyResponse{}, err
	}
	return s.fallbackPolicyResponse(ctx, req, false)
}

func (s Service) fallbackPolicyResponse(ctx context.Context, req FallbackPolicyRequest, enabled bool) (FallbackPolicyResponse, error) {
	if s.Upstreams != nil {
		rows, err := s.Upstreams.ListFallbackPolicies(ctx)
		if err != nil {
			return FallbackPolicyResponse{}, err
		}
		for _, row := range rows {
			if row.ProviderInstanceID == req.ProviderInstanceID &&
				row.CredentialKind == req.CredentialKind &&
				row.GroupLabel == req.GroupLabel {
				return FallbackPolicyResponse{Policy: fallbackPolicyFromCredentials(row)}, nil
			}
		}
	}
	return FallbackPolicyResponse{Policy: FallbackPolicy{
		ProviderInstanceID: safeMachineString(req.ProviderInstanceID),
		CredentialKind:     req.CredentialKind,
		GroupLabel:         safeSnapshotString(req.GroupLabel),
		Enabled:            enabled,
		Explicit:           true,
	}}, nil
}

func upstreamCredentialFromCredentials(row credentials.UpstreamCredentialMetadata) UpstreamCredential {
	return UpstreamCredential{
		ID:                 row.ID,
		ProviderInstanceID: safeMachineString(row.ProviderInstanceID),
		Kind:               row.Kind,
		Label:              safeSnapshotString(row.Label),
		SecretPrefix:       safeSecretFragment(row.SecretPrefix, 8, "sk-"),
		SecretLast4:        safeSecretFragment(row.SecretLast4, 4),
		FallbackGroup:      safeSnapshotString(row.FallbackGroup),
		CreatedAt:          row.CreatedAt,
		DisabledAt:         row.DisabledAt,
		Disabled:           row.Disabled,
	}
}

func fallbackPolicyFromCredentials(row credentials.FallbackPolicyMetadata) FallbackPolicy {
	return FallbackPolicy{
		ProviderInstanceID: safeMachineString(row.ProviderInstanceID),
		CredentialKind:     row.CredentialKind,
		GroupLabel:         safeSnapshotString(row.GroupLabel),
		Enabled:            row.Enabled,
		CredentialCount:    row.CredentialCount,
		Explicit:           row.Explicit,
	}
}

func safeManagementGroupLabel(label string) (string, error) {
	trimmed := strings.TrimSpace(label)
	safe := safeSnapshotString(label)
	if safe == "" || safe != trimmed {
		return "", credentials.ErrInvalidSecretDomain
	}
	return safe, nil
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
