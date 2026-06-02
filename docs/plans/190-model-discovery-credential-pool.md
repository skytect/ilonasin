# 190 Model Discovery Credential Pool

## Context

`docs/ilonasin-architecture.md` defines credential pooling as default
same-provider-instance behavior and requires model discovery, routing,
credential health, and metadata to remain auditable without storing raw
payloads or full account IDs.

`docs/codex-compatibility-audit.md` calls out a current deployment hazard:
Codex model discovery can fail on one unhealthy primary OAuth credential even
when another configured Codex credential is valid. The serving path already has
`resolveModelCredentials` and per-request attempt planning, but `/v1/models`
still calls `resolveModelCredential` and therefore uses only one credential.

The SQLite OAuth pool resolver also materializes the first OAuth candidate as a
mandatory primary. If the first row has missing or expired bearer material, the
resolver returns `ErrNoEligibleCredential` instead of skipping to later valid
credentials.

## Goal

Make model discovery use the same resolved credential-pool availability and
auth semantics as serving, and make the OAuth bearer pool resolver skip
ineligible candidates consistently.

After this slice:

- `/v1/models` attempts live discovery with every eligible credential for a
  provider instance until one returns usable model metadata;
- Codex OAuth model discovery refreshes a 401 credential once, then falls back
  to later eligible credentials if refresh does not recover it;
- model discovery health rows are recorded per attempted credential;
- cached model rows remain the fallback when every live attempt fails;
- OAuth bearer pool resolution skips expired, missing, terminal refresh-failed,
  or malformed bearer candidates instead of letting the first bad row hide
  later valid rows;
- chat and Responses routing may benefit from the shared resolver fix when an
  earlier OAuth row is ineligible, but route policy, quota planning, streaming,
  subscription usage, TUI, config, and logging behavior are otherwise
  unchanged.

## Scope

1. Update OAuth bearer pool resolution in `internal/storage/sqlite`.
   - `ResolveOAuthBearerCredentials` should iterate all candidate rows and
     return every materializable bearer.
   - Skip candidates with missing access-secret references, expired
     `expires_at`, missing access-secret rows, or terminal refresh failure
     classes such as `refresh_token_reused`.
   - Do not skip a candidate only because best-effort ChatGPT routing claims
     cannot derive an account ID.
   - If no candidate materializes, return `ErrNoEligibleCredential`.
   - Preserve ordering by credential ID for eligible rows.
   - Do not change single-credential or by-ID resolver behavior.
2. Update model discovery in `internal/server/models.go`.
   - Use `resolveModelCredentials` instead of `resolveModelCredential`.
   - For each provider instance, try each eligible credential in order.
   - Record `health_events` for each live discovery attempt.
   - On Codex OAuth 401, refresh that credential once and retry it if refresh
     succeeds.
   - If a credential still fails with auth or other upstream error, try the
     next eligible credential before falling back to cache.
   - Replace the provider cache only after a successful non-empty live result.
   - Stop after the first successful non-empty result for a provider instance.
     Do not merge model lists across multiple credentials for the same
     provider instance.
   - Use cached rows only when all live attempts for that provider fail.
   - Do not consult `ActiveQuotaBlocks` or model-specific quota planning in
     model discovery. Model discovery has no requested provider model ID.
3. Keep the response shape unchanged.
   - `GET /models` and `GET /v1/models` still return `object`, `data`, and
     Codex-compatible `models` metadata.
   - No new public route, management DTO, migration, or TUI field.
4. Preserve privacy boundaries.
   - Do not log or store bearer tokens, raw provider model payloads, full
     account IDs, full provider request IDs, prompts, completions, request
     bodies, response bodies, raw SSE chunks, or tool payloads.
   - Existing health rows may include local credential IDs, provider instance
     IDs, status, error class, and retry-after metadata.
5. Do not add dependencies or permanent tests.

## Out of Scope

- Changing chat fallback policy or quota pooling.
- Changing API-key credential resolution order.
- Removing single-credential resolver methods.
- Changing model cache schema or management model-cache DTOs.
- Running real provider credentials as part of this slice.

## Implementation Steps

1. Refactor OAuth bearer pool materialization to scan all candidates.
2. Extract a small model-discovery helper that tries one credential, including
   the Codex 401 refresh retry.
3. Update `handleModels` to use pooled credential attempts and cache fallback.
4. Review the diff for privacy, response shape, and unchanged non-model routes.
5. Run compile, vet, whitespace, direct route smoke, and direct TUI smoke.

## Smoke Checks

Run:

```sh
set -euo pipefail
tmpbin="$(mktemp -d)"
tmp="$(mktemp -d)"
pid=""
cleanup() {
  if [ -n "$pid" ]; then
    kill "$pid" 2>/dev/null || true
    wait "$pid" 2>/dev/null || true
  fi
  rm -rf "$tmp" "$tmpbin"
}
trap cleanup EXIT
find . -name '*_test.go' -type f -print
go test ./...
go vet ./...
git diff --check
go build -o "$tmpbin/ilonasin" ./cmd/ilonasin
```

Then start `ilonasin serve` with a temporary `ILONASIN_HOME`, explicit config,
temporary SQLite database, fake upstream server, and `[logging].capture_io =
false`. Seed one local client token plus multiple credentials for the same
provider instance.

Model route smoke must prove:

- both `GET /models` and `GET /v1/models` are covered;
- when the first credential gets a fake upstream 401 and a refresh retry still
  fails or is unavailable, the second credential gets a successful `/models`
  response and the local route returns `200`;
- health metadata records the failed first credential, the single refresh retry
  attempt when refresh succeeds, and the successful later credential by local
  credential ID only;
- the returned model ID uses the same `<provider_instance>/<provider_model>`
  shape;
- model cache contains only normalized model metadata, not raw upstream body
  text.

Resolver and cache smokes must prove:

- a seeded OAuth pool where credential 1 has missing or expired access material
  and credential 2 is valid returns credential 2 through
  `ResolveOAuthBearerCredentials` or reaches credential 2 through a model route;
- if all live model-discovery attempts fail and cached rows already exist, the
  model route returns cached normalized rows and records failed health attempts;
- if any live attempt succeeds, the live result wins over cache and replaces
  the cache only for that provider instance.

Run a minimal chat or Responses route smoke against an OAuth pool with an
ineligible first row and eligible second row to prove the shared resolver
change does not regress serving.

Also run `ilonasin manage` against the same daemon in a short PTY capture and
verify the TUI can load the management snapshot without mutating config.

Secret scan the temporary normal logs, database text dumps, and TUI capture for:
local token text, fake upstream credential text, fake account IDs, raw upstream
model body sentinels, `bearer`, `sk-`, `iln_`, `access_token`,
`refresh_token`, `id_token`, `raw`, `payload`, `prompt body`,
`completion body`, `request_id`, `tool argument`, and `tool result`.

Clean up all temporary files and terminate the temporary daemon.

## Acceptance

- `/v1/models` no longer lets one bad model-discovery credential hide a later
  eligible credential for the same provider instance.
- OAuth bearer pool resolution returns later eligible credentials even when
  earlier rows are ineligible.
- Model discovery remains metadata-only and cache-backed.
- Compile, vet, route smoke, manage smoke, secret scan, and whitespace checks
  pass.
