package server

import (
	"math"
	"math/bits"
	"sort"
	"strings"
	"sync"
	"time"
	"unsafe"

	"ilonasin/internal/provider"
)

// Prompt-bearing Codex model metadata is retained only as a short-lived,
// bounded fallback. The byte limits cover the cache's fixed storage and a
// conservative estimate of every uniquely retained heap allocation.
const (
	maxLastGoodCodexProviders      = 8
	maxLastGoodCodexModels         = 4096
	maxLastGoodCodexEntryBytes     = uint64(64 << 20)
	maxLastGoodCodexAggregateBytes = uint64(128 << 20)
	lastGoodCodexTTL               = 15 * time.Minute
	heapAllocationOverheadBytes    = uint64(16)
)

type ephemeralCodexModelCache struct {
	mu            sync.Mutex
	entries       [maxLastGoodCodexProviders]ephemeralCodexModelCacheEntry
	sequence      uint64
	retainedBytes uint64
	now           func() time.Time
}

type ephemeralCodexModelCacheEntry struct {
	providerInstanceID string
	models             []provider.ModelMetadata
	byteSize           uint64
	lastUsed           uint64
	expiresAt          time.Time
	used               bool
}

var ephemeralCodexCacheStructuralBytes = uint64(unsafe.Sizeof(ephemeralCodexModelCache{}))

func (c *ephemeralCodexModelCache) put(providerInstanceID string, models []provider.ModelMetadata) bool {
	if len(models) == 0 || len(models) > maxLastGoodCodexModels {
		c.remove(providerInstanceID)
		return false
	}

	byteSize := retainedCodexModelsBytes(providerInstanceID, models)
	if byteSize > maxLastGoodCodexEntryBytes ||
		ephemeralCodexCacheStructuralBytes > maxLastGoodCodexAggregateBytes ||
		byteSize > maxLastGoodCodexAggregateBytes-ephemeralCodexCacheStructuralBytes {
		c.remove(providerInstanceID)
		return false
	}
	// Clone only after checked/saturating accounting proves the planned retained
	// graph can be admitted. Cloning all strings avoids retaining larger source
	// backing allocations than the accounted payload.
	clonedID := strings.Clone(providerInstanceID)
	clonedModels := cloneProviderModels(models)
	now := c.currentTime()

	c.mu.Lock()
	defer c.mu.Unlock()
	c.purgeExpiredLocked(now)
	c.removeEntryLocked(providerInstanceID)
	for c.usedEntriesLocked() >= maxLastGoodCodexProviders ||
		c.aggregateWouldExceedLocked(byteSize) {
		if !c.removeOldestLocked() {
			return false
		}
	}
	index := c.freeEntryLocked()
	if index < 0 {
		return false
	}
	c.entries[index] = ephemeralCodexModelCacheEntry{
		providerInstanceID: clonedID,
		models:             clonedModels,
		byteSize:           byteSize,
		lastUsed:           c.nextSequenceLocked(),
		expiresAt:          now.Add(lastGoodCodexTTL),
		used:               true,
	}
	c.retainedBytes = saturatingAdd(c.retainedBytes, byteSize)
	return true
}

func (c *ephemeralCodexModelCache) get(providerInstanceID string) ([]provider.ModelMetadata, bool) {
	now := c.currentTime()
	c.mu.Lock()
	defer c.mu.Unlock()
	c.purgeExpiredLocked(now)
	index := c.findEntryLocked(providerInstanceID)
	if index < 0 {
		return nil, false
	}
	c.entries[index].lastUsed = c.nextSequenceLocked()
	return cloneProviderModels(c.entries[index].models), true
}

func (c *ephemeralCodexModelCache) remove(providerInstanceID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.removeEntryLocked(providerInstanceID)
}

func (c *ephemeralCodexModelCache) currentTime() time.Time {
	if c.now != nil {
		return c.now()
	}
	return time.Now()
}

func (c *ephemeralCodexModelCache) aggregateWouldExceedLocked(additional uint64) bool {
	limit := maxLastGoodCodexAggregateBytes - ephemeralCodexCacheStructuralBytes
	return additional > limit || c.retainedBytes > limit-additional
}

func (c *ephemeralCodexModelCache) purgeExpiredLocked(now time.Time) {
	for i := range c.entries {
		if c.entries[i].used && !now.Before(c.entries[i].expiresAt) {
			c.removeEntryAtLocked(i)
		}
	}
}

func (c *ephemeralCodexModelCache) findEntryLocked(providerInstanceID string) int {
	for i := range c.entries {
		if c.entries[i].used && c.entries[i].providerInstanceID == providerInstanceID {
			return i
		}
	}
	return -1
}

func (c *ephemeralCodexModelCache) freeEntryLocked() int {
	for i := range c.entries {
		if !c.entries[i].used {
			return i
		}
	}
	return -1
}

func (c *ephemeralCodexModelCache) usedEntriesLocked() int {
	count := 0
	for i := range c.entries {
		if c.entries[i].used {
			count++
		}
	}
	return count
}

func (c *ephemeralCodexModelCache) removeEntryLocked(providerInstanceID string) {
	if index := c.findEntryLocked(providerInstanceID); index >= 0 {
		c.removeEntryAtLocked(index)
	}
}

func (c *ephemeralCodexModelCache) removeEntryAtLocked(index int) {
	entry := &c.entries[index]
	if !entry.used {
		return
	}
	if entry.byteSize > c.retainedBytes {
		c.retainedBytes = 0
	} else {
		c.retainedBytes -= entry.byteSize
	}
	*entry = ephemeralCodexModelCacheEntry{}
}

func (c *ephemeralCodexModelCache) removeOldestLocked() bool {
	oldest := -1
	for i := range c.entries {
		if c.entries[i].used && (oldest < 0 || c.entries[i].lastUsed < c.entries[oldest].lastUsed) {
			oldest = i
		}
	}
	if oldest < 0 {
		return false
	}
	c.removeEntryAtLocked(oldest)
	return true
}

func (c *ephemeralCodexModelCache) nextSequenceLocked() uint64 {
	if c.sequence == math.MaxUint64 {
		indices := make([]int, 0, maxLastGoodCodexProviders)
		for i := range c.entries {
			if c.entries[i].used {
				indices = append(indices, i)
			}
		}
		sort.Slice(indices, func(i, j int) bool {
			return c.entries[indices[i]].lastUsed < c.entries[indices[j]].lastUsed
		})
		for rank, index := range indices {
			c.entries[index].lastUsed = uint64(rank + 1)
		}
		c.sequence = uint64(len(indices))
	}
	c.sequence++
	return c.sequence
}

type retainedMemoryCounter struct {
	total uint64
}

func (c *retainedMemoryCounter) add(value uint64) {
	c.total = saturatingAdd(c.total, value)
}

func (c *retainedMemoryCounter) addAllocation(requested uint64) {
	if requested == 0 {
		return
	}
	c.add(conservativeHeapAllocationBytes(requested))
}

func (c *retainedMemoryCounter) addString(value string) {
	c.addAllocation(uint64(len(value)))
}

func (c *retainedMemoryCounter) addStringPointer(value *string) {
	if value == nil {
		return
	}
	c.addAllocation(uint64(unsafe.Sizeof(string(""))))
	c.addString(*value)
}

func addSliceBacking[T any](counter *retainedMemoryCounter, length int) {
	if length == 0 {
		return
	}
	counter.addAllocation(saturatingMultiply(uint64(length), uint64(unsafe.Sizeof(*new(T)))))
}

func addPointedObject[T any](counter *retainedMemoryCounter, value *T) {
	if value != nil {
		counter.addAllocation(uint64(unsafe.Sizeof(*value)))
	}
}

func retainedCodexModelsBytes(providerInstanceID string, models []provider.ModelMetadata) uint64 {
	var counter retainedMemoryCounter
	counter.addString(providerInstanceID)
	addSliceBacking[provider.ModelMetadata](&counter, len(models))
	for i := range models {
		model := &models[i]
		counter.addString(model.ProviderInstanceID)
		counter.addString(model.ModelID)
		counter.addString(model.DisplayName)
		counter.addString(model.CapabilityFlags)
		counter.addStringPointer(model.DefaultReasoningLevel)
		counter.addStringPointer(model.DefaultServiceTier)
		addPointedObject(&counter, model.ContextLength)
		addPointedObject(&counter, model.MaxContextWindow)

		addSliceBacking[provider.ModelReasoningLevel](&counter, len(model.SupportedReasoningLevels))
		for _, level := range model.SupportedReasoningLevels {
			counter.addString(level.Effort)
			counter.addString(level.Description)
		}
		addSliceBacking[provider.ModelServiceTier](&counter, len(model.ServiceTiers))
		for _, tier := range model.ServiceTiers {
			counter.addString(tier.ID)
			counter.addString(tier.Name)
			counter.addString(tier.Description)
		}
		addSliceBacking[string](&counter, len(model.InputModalities))
		for _, modality := range model.InputModalities {
			counter.addString(modality)
		}
		addRetainedCodexMetadataBytes(&counter, model.Codex)
	}
	return counter.total
}

func addRetainedCodexMetadataBytes(counter *retainedMemoryCounter, model *provider.CodexModelMetadata) {
	if model == nil {
		return
	}
	addPointedObject(counter, model)
	counter.addString(model.ShellType)
	counter.addString(model.Visibility)
	counter.addString(model.BaseInstructions)
	counter.addString(model.DefaultReasoningSummary)
	counter.addStringPointer(model.DefaultVerbosity)
	counter.addStringPointer(model.ApplyPatchToolType)
	counter.addString(model.WebSearchToolType)
	counter.addString(model.TruncationPolicy.Mode)
	counter.addStringPointer(model.CompHash)
	counter.addStringPointer(model.AutoReviewModelOverride)
	counter.addStringPointer(model.ToolMode)
	counter.addStringPointer(model.MultiAgentVersion)
	counter.addStringPointer(model.Description)
	addPointedObject(counter, model.AutoCompactTokenLimit)

	addSliceBacking[string](counter, len(model.AdditionalSpeedTiers))
	for _, tier := range model.AdditionalSpeedTiers {
		counter.addString(tier)
	}
	addSliceBacking[string](counter, len(model.ExperimentalSupportedTools))
	for _, tool := range model.ExperimentalSupportedTools {
		counter.addString(tool)
	}
	if model.AvailabilityNUX != nil {
		addPointedObject(counter, model.AvailabilityNUX)
		counter.addString(model.AvailabilityNUX.Message)
	}
	if model.Upgrade != nil {
		addPointedObject(counter, model.Upgrade)
		counter.addString(model.Upgrade.Model)
		counter.addString(model.Upgrade.MigrationMarkdown)
	}
	if model.ModelMessages != nil {
		addPointedObject(counter, model.ModelMessages)
		counter.addStringPointer(model.ModelMessages.InstructionsTemplate)
		if variables := model.ModelMessages.InstructionsVariables; variables != nil {
			addPointedObject(counter, variables)
			counter.addStringPointer(variables.PersonalityDefault)
			counter.addStringPointer(variables.PersonalityFriendly)
			counter.addStringPointer(variables.PersonalityPragmatic)
		}
		if approvals := model.ModelMessages.Approvals; approvals != nil {
			addPointedObject(counter, approvals)
			counter.addStringPointer(approvals.OnRequest)
			counter.addStringPointer(approvals.OnRequestAutoReview)
		}
	}
}

func conservativeHeapAllocationBytes(requested uint64) uint64 {
	requested = saturatingAdd(requested, heapAllocationOverheadBytes)
	if requested == math.MaxUint64 {
		return requested
	}
	if requested <= 1 {
		return 1
	}
	width := bits.Len64(requested - 1)
	if width >= 64 {
		return math.MaxUint64
	}
	return uint64(1) << width
}

func saturatingAdd(left, right uint64) uint64 {
	if right > math.MaxUint64-left {
		return math.MaxUint64
	}
	return left + right
}

func saturatingMultiply(left, right uint64) uint64 {
	if left != 0 && right > math.MaxUint64/left {
		return math.MaxUint64
	}
	return left * right
}

func cloneProviderModels(models []provider.ModelMetadata) []provider.ModelMetadata {
	out := make([]provider.ModelMetadata, len(models))
	for i, model := range models {
		out[i] = model
		out[i].ProviderInstanceID = strings.Clone(model.ProviderInstanceID)
		out[i].ModelID = strings.Clone(model.ModelID)
		out[i].DisplayName = strings.Clone(model.DisplayName)
		out[i].CapabilityFlags = strings.Clone(model.CapabilityFlags)
		out[i].ContextLength = cloneInt64Pointer(model.ContextLength)
		out[i].MaxContextWindow = cloneInt64Pointer(model.MaxContextWindow)
		out[i].DefaultReasoningLevel = cloneStringPointer(model.DefaultReasoningLevel)
		out[i].DefaultServiceTier = cloneStringPointer(model.DefaultServiceTier)
		out[i].SupportedReasoningLevels = make([]provider.ModelReasoningLevel, len(model.SupportedReasoningLevels))
		for j, level := range model.SupportedReasoningLevels {
			out[i].SupportedReasoningLevels[j] = provider.ModelReasoningLevel{
				Effort:      strings.Clone(level.Effort),
				Description: strings.Clone(level.Description),
			}
		}
		out[i].ServiceTiers = make([]provider.ModelServiceTier, len(model.ServiceTiers))
		for j, tier := range model.ServiceTiers {
			out[i].ServiceTiers[j] = provider.ModelServiceTier{
				ID:          strings.Clone(tier.ID),
				Name:        strings.Clone(tier.Name),
				Description: strings.Clone(tier.Description),
			}
		}
		out[i].InputModalities = cloneStrings(model.InputModalities)
		out[i].Codex = cloneCodexModelMetadata(model.Codex)
		out[i].UpdatedAt = model.UpdatedAt.UTC()
	}
	return out
}

func cloneCodexModelMetadata(model *provider.CodexModelMetadata) *provider.CodexModelMetadata {
	if model == nil {
		return nil
	}
	out := *model
	out.ShellType = strings.Clone(model.ShellType)
	out.Visibility = strings.Clone(model.Visibility)
	out.Description = cloneStringPointer(model.Description)
	out.AdditionalSpeedTiers = cloneStrings(model.AdditionalSpeedTiers)
	out.BaseInstructions = strings.Clone(model.BaseInstructions)
	out.DefaultReasoningSummary = strings.Clone(model.DefaultReasoningSummary)
	out.DefaultVerbosity = cloneStringPointer(model.DefaultVerbosity)
	out.ApplyPatchToolType = cloneStringPointer(model.ApplyPatchToolType)
	out.WebSearchToolType = strings.Clone(model.WebSearchToolType)
	out.TruncationPolicy.Mode = strings.Clone(model.TruncationPolicy.Mode)
	out.AutoCompactTokenLimit = cloneInt64Pointer(model.AutoCompactTokenLimit)
	out.CompHash = cloneStringPointer(model.CompHash)
	out.AutoReviewModelOverride = cloneStringPointer(model.AutoReviewModelOverride)
	out.ToolMode = cloneStringPointer(model.ToolMode)
	out.MultiAgentVersion = cloneStringPointer(model.MultiAgentVersion)
	out.ExperimentalSupportedTools = cloneStrings(model.ExperimentalSupportedTools)
	if model.AvailabilityNUX != nil {
		out.AvailabilityNUX = &provider.CodexModelAvailabilityNUX{
			Message: strings.Clone(model.AvailabilityNUX.Message),
		}
	}
	if model.Upgrade != nil {
		out.Upgrade = &provider.CodexModelUpgrade{
			Model:             strings.Clone(model.Upgrade.Model),
			MigrationMarkdown: strings.Clone(model.Upgrade.MigrationMarkdown),
		}
	}
	if model.ModelMessages != nil {
		messages := &provider.CodexModelMessages{
			InstructionsTemplate: cloneStringPointer(model.ModelMessages.InstructionsTemplate),
		}
		if variables := model.ModelMessages.InstructionsVariables; variables != nil {
			messages.InstructionsVariables = &provider.CodexModelInstructionVariables{
				PersonalityDefault:   cloneStringPointer(variables.PersonalityDefault),
				PersonalityFriendly:  cloneStringPointer(variables.PersonalityFriendly),
				PersonalityPragmatic: cloneStringPointer(variables.PersonalityPragmatic),
			}
		}
		if approvals := model.ModelMessages.Approvals; approvals != nil {
			messages.Approvals = &provider.CodexModelApprovalMessages{
				OnRequest:           cloneStringPointer(approvals.OnRequest),
				OnRequestAutoReview: cloneStringPointer(approvals.OnRequestAutoReview),
			}
		}
		out.ModelMessages = messages
	}
	return &out
}

func cloneStrings(values []string) []string {
	if values == nil {
		return nil
	}
	out := make([]string, len(values))
	for i, value := range values {
		out[i] = strings.Clone(value)
	}
	return out
}

func cloneStringPointer(value *string) *string {
	if value == nil {
		return nil
	}
	cloned := strings.Clone(*value)
	return &cloned
}

func cloneInt64Pointer(value *int64) *int64 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
