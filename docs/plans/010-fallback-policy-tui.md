# Plan 010: Fallback Policy TUI

## Goal

Add explicit `ilonasin manage` controls for enabling and disabling same-provider
credential fallback groups.

The server already supports constrained credential fallback, and SQLite already
stores `credential_fallback_policies`. This slice makes the policy visible and
controllable from the TUI without editing `config.toml`.

## Architecture Inputs

- `docs/ilonasin-architecture.md`
- `docs/deepseek-openrouter-comparison.md`
- `docs/deepseek-api.md`
- `docs/openrouter-api.md`
- `docs/codex-auth.md`
- `docs/codex-endpoints.md`
- prior plans `001` through `009`
- `AGENTS.md`

## Scope

1. Add a narrow fallback-policy read boundary:
   - storage can list fallback policy rows,
   - TUI receives it through the existing upstream credential manager boundary
     or a small adjacent interface,
   - listing reads metadata only from `provider_credentials` and
     `credential_fallback_policies`, filtered to `kind = 'api_key'`,
   - listing must never join or select `credential_secrets.secret_material`,
   - server/provider adapters do not receive TUI methods.
2. Show fallback policy state in `manage`:
   - list provider instance ID, group label, and enabled/disabled state,
   - include a default policy row for API-key provider groups that have
     credentials but no explicit row yet,
   - display only configured, non-placeholder API-key provider instances,
   - ignore stale DB provider rows, OAuth-only providers, Codex placeholder
     rows, and non-API-key providers,
   - do not show token material, account IDs, raw provider payloads, balances,
     credits, request IDs, prompts, or completions.
3. Add TUI commands:
   - `f` enables the first eligible fallback group,
   - `F` disables the first enabled fallback group,
   - both mutate SQLite only,
   - no config mutation.
   Eligibility and order are deterministic:
   - API-key provider instance only,
   - configured provider instance only,
   - enabled credentials only,
   - same fallback group,
   - at least two enabled credentials in the group,
   - ordered by provider instance ID, then group label.
4. Extend `manage --check`:
   - seed two API-key credentials in an isolated DB,
   - seed unsafe credential labels, unsafe group labels, and API-key marker
     material,
   - prove the default policy is displayed as disabled,
   - exercise `f` and `F`,
   - prove the policy row toggles in SQLite,
   - prove resolver behavior changes: `ResolveAPIKeys` returns one credential
     before `f`, two after `f`, and one again after `F`,
   - prove stale and placeholder policy rows do not render or toggle,
   - prove no unsafe labels or secret material leaks,
   - prove TUI output and failure messages are marker-free or redacted,
   - prove selected-home DB and config snapshots remain unchanged,
   - prove isolated DB snapshots show only `credential_fallback_policies`
     changes and no mutation to `provider_credentials`, `credential_secrets`
     (hashed), OAuth tables, model cache, telemetry, migrations, or config.

## Out of Scope

- Cross-provider fallback.
- Cross-model fallback.
- Account cycling policy beyond the existing same-group switch.
- Per-request fallback controls.
- Permanent tests.

## Design Constraints

- No permanent `*_test.go` files.
- The TUI must not mutate `config.toml`.
- Fallback controls must be explicit and auditable.
- API-key fallback remains constrained by provider instance and fallback group.
- Do not introduce provider-specific fallback behavior in the TUI.
- Do not store or display full bearer tokens, full account IDs, request bodies,
  response bodies, raw provider payloads, prompts, completions, raw SSE chunks,
  balances, or credits.

## Proposed Package Changes

```text
internal/credentials/
  upstream.go      # fallback policy metadata and manager method
internal/storage/sqlite/
  db.go            # list fallback policies
internal/tui/
  tui.go           # fallback policy view and toggle keys
internal/app/
  app.go           # isolated manage smoke check
```

The view should stay compact and match the existing management screen rather
than becoming a separate route.

## Verification

Run:

```text
find . -name '*_test.go' -type f -print
go test ./...
go vet ./...
tmpbin="$(mktemp -d)"
go build -o "$tmpbin/ilonasin" ./cmd/ilonasin
tmp="$(mktemp -d)"
ILONASIN_HOME="$tmp" "$tmpbin/ilonasin" serve --check
ILONASIN_HOME="$tmp" "$tmpbin/ilonasin" manage --check
git diff --check
```

Smoke checks must prove:

- no permanent test files exist,
- fallback policy defaults display as disabled before toggle,
- `f` creates/enables the policy row,
- `F` disables it,
- policy state reloads correctly from SQLite,
- stale/unconfigured/placeholder policy rows do not render,
- selected-home DB and config snapshots remain unchanged across
  `manage --check`,
- TUI output and check failures do not leak secret material or unsafe markers.
- isolated fallback-policy toggles mutate only `credential_fallback_policies`,
- resolver behavior follows the toggled policy without printing API keys.

## Review Questions

1. Is using `f`/`F` acceptable for first fallback policy controls?
2. Should default policy rows be synthesized from existing credentials or only
   shown after a row exists?
3. Are the storage and TUI boundaries narrow enough?
