# 348 TUI Dead Fallback Helper

## Context

Recent TUI/management boundary slices moved fallback-policy visibility ownership
to the daemon management API and made `applySnapshot` trust management snapshot
DTOs directly.

`internal/tui/provider_fallback_actions.go` still contains
`fallbackPolicyEnabled`, a package-private helper that scans fallback policy DTOs
for a specific enabled row. It has no remaining call sites after the TUI
snapshot trust change.

Keeping dead fallback-policy helpers in the TUI makes it look like the TUI still
owns fallback-policy interpretation. Removing the helper keeps the TUI closer to
the architecture where it consumes management DTOs and sends management actions.

## Scope

1. Keep this slice limited to:
   - `internal/tui/provider_fallback_actions.go`;
   - this plan.
2. Remove the unused `fallbackPolicyEnabled` helper.
3. Preserve all fallback action behavior:
   - enable action still picks the first disabled row in `m.fallbackPolicies`;
   - disable action still picks the first enabled row in `m.fallbackPolicies`;
   - management API remains responsible for accepting or rejecting policy
     changes.
4. Do not change management DTOs, management routes, storage schema, provider
   behavior, credential mutation, config, logging, affinity, quota behavior,
   rendering, or permanent tests.

## Verification

Before implementation review:

1. Review the diff manually for scope and behavior.
2. Run:

```sh
find . -name '*_test.go' -type f -print
git diff --check
go test ./internal/tui ./internal/management
go test ./...
go vet ./...
```

3. Build `ilonasin`, start `ilonasin serve` with an isolated temporary
   `ILONASIN_HOME`, verify the management health route over the Unix socket,
   run a short `ilonasin manage` TUI smoke, then terminate and clean up.

## Expected Outcome

- The unused TUI fallback-policy helper is removed.
- TUI fallback actions and management behavior are unchanged.
- The TUI contains one less stale policy helper after moving snapshot visibility
  ownership to management.
