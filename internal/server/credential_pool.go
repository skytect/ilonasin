package server

import (
	"context"
	"hash/fnv"
	"strconv"
	"strings"
	"sync"
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
	affinityKey = strings.TrimSpace(affinityKey)
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

type credentialPressureKey struct {
	providerInstanceID string
	providerModelID    string
	credentialID       int64
}

type credentialPressureTracker struct {
	mu       sync.Mutex
	inFlight map[credentialPressureKey]int
}

type credentialAttemptSlot struct {
	index      int
	credential provider.BearerCredential
}

func newCredentialPressureTracker() *credentialPressureTracker {
	return &credentialPressureTracker{inFlight: map[credentialPressureKey]int{}}
}

func (t *credentialPressureTracker) acquire(addr routing.ModelAddress, credential provider.BearerCredential) func() {
	if t == nil || credential.ID == 0 {
		return func() {}
	}
	key := credentialPressureKey{
		providerInstanceID: addr.ProviderInstanceID,
		providerModelID:    addr.ProviderModelID,
		credentialID:       credential.ID,
	}
	t.mu.Lock()
	t.inFlight[key]++
	t.mu.Unlock()
	return func() {
		t.release(key)
	}
}

func (t *credentialPressureTracker) reserveLeast(addr routing.ModelAddress, slots []credentialAttemptSlot) (int, provider.BearerCredential, func(), bool) {
	if t == nil || len(slots) == 0 {
		return 0, provider.BearerCredential{}, func() {}, false
	}
	t.mu.Lock()
	best := -1
	bestCount := 0
	for i, slot := range slots {
		credential := slot.credential
		count := 0
		if credential.ID != 0 {
			count = t.inFlight[credentialPressureKey{
				providerInstanceID: addr.ProviderInstanceID,
				providerModelID:    addr.ProviderModelID,
				credentialID:       credential.ID,
			}]
		}
		if best == -1 || count < bestCount {
			best = i
			bestCount = count
		}
	}
	if best == -1 {
		t.mu.Unlock()
		return 0, provider.BearerCredential{}, func() {}, false
	}
	slot := slots[best]
	credential := slot.credential
	key := credentialPressureKey{
		providerInstanceID: addr.ProviderInstanceID,
		providerModelID:    addr.ProviderModelID,
		credentialID:       credential.ID,
	}
	if credential.ID != 0 {
		t.inFlight[key]++
	}
	t.mu.Unlock()
	return slot.index, credential, func() {
		if credential.ID != 0 {
			t.release(key)
		}
	}, true
}

func (t *credentialPressureTracker) release(key credentialPressureKey) {
	t.mu.Lock()
	if count := t.inFlight[key]; count <= 1 {
		delete(t.inFlight, key)
	} else {
		t.inFlight[key] = count - 1
	}
	t.mu.Unlock()
}

func (s *Server) trackCredentialAttempt(addr routing.ModelAddress, credential provider.BearerCredential) func() {
	if s == nil || s.pressure == nil {
		return func() {}
	}
	return s.pressure.acquire(addr, credential)
}

func (s *Server) reserveCredentialAttempt(addr routing.ModelAddress, affinityKey string, slots []credentialAttemptSlot) (int, provider.BearerCredential, func(), bool) {
	if len(slots) == 0 {
		return 0, provider.BearerCredential{}, func() {}, false
	}
	if strings.TrimSpace(affinityKey) != "" || s == nil || s.pressure == nil {
		slot := slots[0]
		return slot.index, slot.credential, s.trackCredentialAttempt(addr, slot.credential), true
	}
	return s.pressure.reserveLeast(addr, slots)
}

func remainingCredentialAttemptSlots(credentials []provider.BearerCredential, used map[int]bool) []credentialAttemptSlot {
	if len(used) == 0 {
		out := make([]credentialAttemptSlot, 0, len(credentials))
		for i, credential := range credentials {
			out = append(out, credentialAttemptSlot{index: i, credential: credential})
		}
		return out
	}
	out := make([]credentialAttemptSlot, 0, len(credentials))
	for i, credential := range credentials {
		if used[i] {
			continue
		}
		out = append(out, credentialAttemptSlot{index: i, credential: credential})
	}
	return out
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
