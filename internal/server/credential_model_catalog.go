package server

import (
	"crypto/sha256"
	"strings"
	"sync"
	"time"

	"ilonasin/internal/provider"
)

const (
	credentialModelCatalogTTL        = 5 * time.Minute
	credentialModelCatalogFailureTTL = 30 * time.Second
	maxCredentialModelCatalogs       = 256
	maxCredentialCatalogModels       = 4096
	maxCredentialModelIDBytes        = 4096
	maxCredentialCatalogBytes        = 4 << 20
	maxCredentialCatalogAggregate    = 32 << 20
)

type credentialModelCatalogKey struct {
	providerInstanceID string
	credentialID       int64
	bearerGeneration   [sha256.Size]byte
}

func modelCatalogKey(providerInstanceID string, credential provider.BearerCredential) credentialModelCatalogKey {
	return credentialModelCatalogKey{
		providerInstanceID: providerInstanceID,
		credentialID:       credential.ID,
		bearerGeneration:   sha256.Sum256([]byte(credential.BearerToken)),
	}
}

type credentialModelCatalogEntry struct {
	models    map[string]struct{}
	known     bool
	expiresAt time.Time
	observed  time.Time
	bytes     int
}

// credentialModelCatalogCache retains only bounded, short-lived model IDs from
// live per-credential catalogs. Unknown and failed observations are cached
// briefly so account-scoped routes fail closed without hammering discovery.
type credentialModelCatalogCache struct {
	mu      sync.Mutex
	entries map[credentialModelCatalogKey]credentialModelCatalogEntry
	bytes   int
}

func newCredentialModelCatalogCache() credentialModelCatalogCache {
	return credentialModelCatalogCache{entries: make(map[credentialModelCatalogKey]credentialModelCatalogEntry)}
}

func (c *credentialModelCatalogCache) lookup(now time.Time, key credentialModelCatalogKey) (map[string]struct{}, bool, bool) {
	if c == nil {
		return nil, false, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[key]
	if !ok {
		return nil, false, false
	}
	if !now.Before(entry.expiresAt) {
		c.bytes -= entry.bytes
		delete(c.entries, key)
		return nil, false, false
	}
	return entry.models, entry.known, true
}

func (c *credentialModelCatalogCache) put(now time.Time, key credentialModelCatalogKey, modelIDs []string) {
	if c == nil || key.credentialID == 0 || len(modelIDs) == 0 || len(modelIDs) > maxCredentialCatalogModels {
		c.fail(now, key)
		return
	}
	models := make(map[string]struct{}, len(modelIDs))
	retainedBytes := 0
	for _, modelID := range modelIDs {
		if modelID == "" || len(modelID) > maxCredentialModelIDBytes {
			c.fail(now, key)
			return
		}
		if _, exists := models[modelID]; !exists {
			retainedBytes += len(modelID)
			if retainedBytes > maxCredentialCatalogBytes {
				c.fail(now, key)
				return
			}
			models[strings.Clone(modelID)] = struct{}{}
		}
	}
	if len(models) == 0 {
		c.fail(now, key)
		return
	}
	c.store(key, credentialModelCatalogEntry{
		models:    models,
		known:     true,
		expiresAt: now.Add(credentialModelCatalogTTL),
		observed:  now,
		bytes:     retainedBytes,
	})
}

func (c *credentialModelCatalogCache) fail(now time.Time, key credentialModelCatalogKey) {
	if c == nil || key.credentialID == 0 {
		return
	}
	c.store(key, credentialModelCatalogEntry{
		expiresAt: now.Add(credentialModelCatalogFailureTTL),
		observed:  now,
	})
}

func (c *credentialModelCatalogCache) store(key credentialModelCatalogKey, entry credentialModelCatalogEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entries == nil {
		c.entries = make(map[credentialModelCatalogKey]credentialModelCatalogEntry)
	}
	if previous, exists := c.entries[key]; exists {
		c.bytes -= previous.bytes
		delete(c.entries, key)
	}
	for len(c.entries) >= maxCredentialModelCatalogs || c.bytes+entry.bytes > maxCredentialCatalogAggregate {
		var oldestKey credentialModelCatalogKey
		var oldest time.Time
		for candidate, current := range c.entries {
			if oldest.IsZero() || current.observed.Before(oldest) {
				oldestKey = candidate
				oldest = current.observed
			}
		}
		if oldest.IsZero() {
			break
		}
		c.bytes -= c.entries[oldestKey].bytes
		delete(c.entries, oldestKey)
	}
	c.entries[key] = entry
	c.bytes += entry.bytes
}
