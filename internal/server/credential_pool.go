package server

import (
	"context"
	"hash/fnv"
	"strconv"
	"strings"
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

func (s *Server) planCredentialAttempts(ctx context.Context, addr routing.ModelAddress, tokenID int64, affinityKey string, credentials []provider.BearerCredential) credentialAttemptPlan {
	ordered := affinityCredentialOrder(addr, tokenID, affinityKey, credentials)
	plan := credentialAttemptPlan{attempts: ordered}
	if len(credentials) == 0 {
		return plan
	}
	plan.modelCredential = plan.attempts[0]
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
	for _, credential := range ordered {
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
	plan.modelCredential = plan.attempts[0]
	return plan
}

func affinityCredentialOrder(addr routing.ModelAddress, tokenID int64, affinityKey string, credentials []provider.BearerCredential) []provider.BearerCredential {
	if len(credentials) < 2 {
		return credentials
	}
	start := credentialAffinityStart(addr, tokenID, affinityKey, len(credentials))
	if start == 0 {
		return credentials
	}
	out := make([]provider.BearerCredential, 0, len(credentials))
	out = append(out, credentials[start:]...)
	out = append(out, credentials[:start]...)
	return out
}

func credentialAffinityStart(addr routing.ModelAddress, tokenID int64, affinityKey string, size int) int {
	if size <= 1 {
		return 0
	}
	affinityKey = strings.TrimSpace(affinityKey)
	h := fnv.New64a()
	_, _ = h.Write([]byte("ilonasin-credential-affinity-v1\x00"))
	_, _ = h.Write([]byte(strconv.FormatInt(tokenID, 10)))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(addr.ProviderInstanceID))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(addr.ProviderModelID))
	if affinityKey != "" {
		_, _ = h.Write([]byte{0})
		_, _ = h.Write([]byte(affinityKey))
	}
	return int(h.Sum64() % uint64(size))
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
