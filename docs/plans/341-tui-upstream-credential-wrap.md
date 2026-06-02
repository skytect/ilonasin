# 341 TUI Upstream Credential Wrap

## Context

The architecture treats `ilonasin manage` as a polished first-class control
plane. Recent TUI slices moved request logs and subscription usage away from
ellipsized text toward wrapped, scan-friendly rows. The upstream credential pane
still renders credential labels, provider IDs, and fallback groups with
`safeDisplay` and plain `metricLine`, which truncates long but safe labels and
can push row content beyond the pane width.

This slice keeps the existing Providers information architecture and only makes
upstream credential rows use the same wrapping discipline already used by usage
and logs.

## Scope

1. Update `internal/tui/providers_upstreams.go` only.
2. Render upstream credential rows with existing wrapped helpers:
   - use full wrapped-safe display for credential labels;
   - use wrapped metric chips for provider and fallback group values;
   - wrap the final head/meta output to the pane width instead of relying on one
     long `metricLine`;
   - ensure every physical rendered line is bounded to the requested pane width,
     including long single-token provider or fallback group values;
   - preserve the secret fragment chip behavior.
3. Wrap the API-key entry line shown while adding a key so long provider IDs do
   not overflow and the typed key remains mask-only.
4. Preserve existing state counts, empty state, ordering, selected-row behavior,
   management data, storage, and actions.
5. Do not expose additional secret material or raw account IDs.

## Out Of Scope

- New TUI panes or layout changes.
- Management API or DTO changes.
- Credential metadata refresh.
- Storage/schema changes.
- Any provider/auth behavior.
- Permanent tests.

## Implementation Steps

1. Add a small local helper for upstream credential identity text if needed.
2. Replace `safeDisplay`/`metricLine` row construction with
   `safeFullWrappedDisplay`, `wrappedMetricChip`, and `wrapTargetedLines`.
3. Keep wide and narrow row behavior compact:
   - wide rows may still combine head and meta when they fit;
   - narrow rows should naturally wrap rather than truncate.
4. Review the diff for no new secret exposure and no accidental provider/action
   changes.

## Verification

Use a temporary focused check, then remove it before commit:

- a long safe credential label appears in wrapped output without `...`;
- long provider and fallback group values wrap instead of truncating;
- every physical output line is no wider than the requested test width;
- the API-key entry line wraps long provider IDs and renders only mask
  characters for the typed key;
- unsafe label/provider/group values still render as `[redacted]`;
- secret prefix/last4 rendering is unchanged.

Then run:

```sh
git diff --check
find . -name '*_test.go' -type f -print
go test ./internal/tui
go test ./...
go vet ./...
```

Run direct CLI smokes by building a temporary binary, starting
`ilonasin serve` with a temporary `ILONASIN_HOME`, checking management health
over the Unix socket, running bounded `ilonasin manage`, and cleaning up all
temporary files and processes.

## Acceptance

- Upstream credential rows no longer ellipsize long safe labels.
- Provider and fallback group values wrap inside the pane width.
- Secret fragments remain masked exactly as before.
- No management, storage, provider, or action behavior changes.
