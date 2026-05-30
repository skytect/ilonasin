# Plan 021: Model Discovery Health Metadata

## Goal

Record metadata-only health events for provider model discovery attempts.

The architecture treats provider and credential health as separate from request
usage. Chat attempts already record health events, but `/v1/models` refreshes
currently do not. This slice makes model discovery successes and failures
visible in the same safe health ledger without changing model cache fallback,
credential fallback, OAuth refresh, rate-limit behavior, or the local
OpenAI-compatible `/v1/models` response.

## Architecture Inputs

- `AGENTS.md`
- `docs/ilonasin-architecture.md`
- `docs/deepseek-api.md`
- `docs/openrouter-api.md`
- `docs/deepseek-openrouter-comparison.md`
- `docs/codex-auth.md`
- `docs/codex-endpoints.md`
- prior plans `001` through `020`

## Scope

1. Record model-discovery health events from the server layer:
   - one `upstream_success` event for each successful live provider model
     refresh,
   - one `upstream_failure` event for each failed live provider model refresh,
   - include provider instance ID, credential ID, HTTP status when known,
     normalized error class, and retry-after timestamp when safely parsed,
   - use empty `model_id` for provider-level discovery health because model
     discovery is not scoped to one model.
2. Preserve model-discovery behavior:
   - do not change `/v1/models` response shape,
   - do not change cache fallback semantics,
   - do not expose stale cache for no-eligible Codex OAuth credentials,
   - do not change which providers are skipped,
   - do not make model discovery failures affect chat routing.
3. Preserve retry and auth behavior:
   - Codex model discovery may still refresh expired OAuth before request and
     retry once after upstream `401`,
   - record the pre-refresh `401` discovery failure and the retry result when
     an upstream request was made,
   - do not refresh on `429`, `5xx`, malformed responses, timeout, or cache
     fallback,
   - do not add credential fallback, account cycling, cross-provider routing,
     cross-model routing, queueing, or waiting based on health events.
4. Keep privacy boundaries:
   - health events store only typed metadata,
   - no raw model discovery response body, provider payload, provider error
     body, bearer token, refresh token, account ID, provider request ID,
     generation ID, balance, credit, prompt, completion, or request body is
     stored or displayed,
   - provider adapters still own HTTP response parsing and safe result fields,
   - storage still performs no HTTP.
5. Make TUI output clearer for provider-level health:
   - render empty health `model_id` as a provider-level label such as
     `models`,
   - display only safe status, error class, credential label/ID, occurred time,
     and retry-after timestamp when present.
6. Extend smoke checks without permanent tests:
   - successful DeepSeek/OpenRouter model refresh records provider-level
     `upstream_success` health,
   - failed model refresh records provider-level `upstream_failure` health and
     safe retry-after when the fake upstream sends it,
   - Codex upstream `401` during model discovery records failure and retry
     success after refresh,
   - OpenRouter/DeepSeek `429` model discovery records retry-after and does not
     trigger fallback or OAuth refresh,
   - cache fallback behavior remains unchanged after recording health,
   - TUI health output renders provider-level model discovery rows safely,
   - existing privacy scans cover health rows and continue to reject forbidden
     markers.

## Out of Scope

- New SQLite columns or migrations.
- Provider health scoring.
- Avoiding credentials based on prior model-discovery health.
- Model-discovery background jobs.
- Rate-limit buckets, queueing, sleeping, or retry based on retry-after.
- Fallback on `429`.
- OAuth refresh on non-`401`.
- Recording raw model payloads, pricing, descriptions, account data, request
  IDs, or provider error bodies.
- Permanent tests.

## Design Constraints

- No permanent `*_test.go` files.
- Do not push.
- Storage must not perform HTTP.
- Provider adapters must not import SQLite, TUI, config loaders, or credential
  storage.
- TUI must not mutate `config.toml`.
- Health event classes remain exactly `upstream_success` and
  `upstream_failure`.
- Model discovery health must not create request metadata rows, stream metrics,
  or fallback events.
- Model discovery health must not record local auth failures, no-eligible
  credential skips, local cache read/write failures, or unsupported provider
  skips.

## Proposed Package Changes

```text
internal/server/
  server.go      # record health events around model discovery calls
internal/tui/
  tui.go         # display empty model_id health rows as provider-level models
internal/app/
  app.go         # serve/manage smoke assertions for model discovery health
```

`provider.ModelResult` already carries safe status, error class, and
retry-after metadata from slice 020, so this slice should not require provider
adapter changes except for smoke-only fake upstream behavior if needed.

## Smoke Checks

Run:

```sh
find . -name '*_test.go' -type f -print
go test ./...
go vet ./...
tmpbin="$(mktemp -d)"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp" "$tmpbin"' EXIT
go build -o "$tmpbin/ilonasin" ./cmd/ilonasin
ILONASIN_HOME="$tmp" "$tmpbin/ilonasin" serve --check
ILONASIN_HOME="$tmp" "$tmpbin/ilonasin" manage --check
git diff --check
```

Acceptance:

- no permanent tests exist,
- compile/package, vet, build, `serve --check`, `manage --check`, and diff
  whitespace checks pass,
- live model discovery success records provider-level health success,
- live model discovery failure records provider-level health failure,
- model discovery `429` records retry-after but does not trigger fallback or
  OAuth refresh,
- Codex model discovery `401` refresh/retry records safe health events,
- no-credential and unsupported-provider skips do not record health,
- TUI displays provider-level model discovery health without unsafe markers,
- no raw prompts, completions, request/response bodies, provider payloads,
  model payloads, bearer tokens, refresh tokens, provider request IDs, account
  IDs, balances, or credits appear in SQLite metadata, TUI output, CLI output,
  or local errors.
