package server

import (
	"context"
	"time"

	"ilonasin/internal/metadata"
	"ilonasin/internal/provider"
	"ilonasin/internal/routing"
)

type credentialAttemptPlan struct {
	modelCredential provider.BearerCredential
	attempts        []provider.BearerCredential
	exhausted       bool
	retryAfter      *time.Time
}

func (s *Server) planCredentialAttempts(ctx context.Context, addr routing.ModelAddress, credentials []provider.BearerCredential) credentialAttemptPlan {
	plan := credentialAttemptPlan{attempts: credentials}
	if len(credentials) == 0 {
		return plan
	}
	plan.modelCredential = credentials[0]
	if s.quota == nil {
		return plan
	}
	blocks, err := s.quota.ActiveQuotaBlocks(ctx, addr.ProviderInstanceID, addr.ProviderModelID, s.now().UTC())
	if err != nil || len(blocks) == 0 {
		return plan
	}
	blocked := make(map[int64]metadata.ActiveQuotaBlock, len(blocks))
	for _, block := range blocks {
		blocked[block.CredentialID] = block
	}
	attempts := make([]provider.BearerCredential, 0, len(credentials))
	var retryAfter *time.Time
	for _, credential := range credentials {
		block, ok := blocked[credential.ID]
		if !ok {
			attempts = append(attempts, credential)
			continue
		}
		if retryAfter == nil || block.ActiveUntil.Before(*retryAfter) {
			next := block.ActiveUntil
			retryAfter = &next
		}
	}
	if len(attempts) == 0 {
		plan.attempts = nil
		plan.exhausted = true
		plan.retryAfter = retryAfter
		return plan
	}
	plan.attempts = attempts
	return plan
}

func quotaRetryAfter(a, b *time.Time) *time.Time {
	if a == nil {
		return b
	}
	if b == nil || a.Before(*b) {
		return a
	}
	return b
}
