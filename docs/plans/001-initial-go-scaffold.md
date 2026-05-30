# Plan 001: Initial Go Scaffold

## Goal

Create the first production-shaped Go implementation for `ilonasin` that matches
the architecture's locked decisions closely enough to support further slices.
This slice establishes the one-binary/two-command shape, local home/config
bootstrap, SQLite state foundation, and smoke-testable command behavior.

## Architecture Inputs

- `docs/ilonasin-architecture.md`
- `docs/deepseek-api.md`
- `docs/openrouter-api.md`
- `docs/deepseek-openrouter-comparison.md`
- `docs/codex-auth.md`
- `docs/codex-endpoints.md`

## Scope

1. Initialize a Go module and command entrypoint for one binary named
   `ilonasin`.
2. Implement subcommands:
   - `ilonasin serve`
   - `ilonasin manage`
3. Add shared app bootstrap modules:
   - command parsing,
   - local home path resolution,
   - static TOML config loading,
   - built-in provider type defaults for `deepseek` and `openrouter`,
   - a safe `codex` provider type placeholder only,
   - SQLite opening and schema migration.
4. Implement architecture-compatible defaults:
   - default home `~/.ilonasin`,
   - `ILONASIN_HOME` override,
   - `--config` override,
   - Unix directory mode `0700` where practical,
   - Unix config/database mode `0600` where practical,
   - default server bind `127.0.0.1:11435`.
5. Add an initial SQLite schema for the architecture's mutable state concepts:
   - client tokens,
   - provider credentials,
   - OAuth tokens,
   - provider accounts,
   - model cache,
   - request metadata,
   - stream metrics,
   - health events,
   - fallback events,
   - migrations.
6. Classify SQLite columns and storage boundaries:
   - `secret_material`: upstream API keys and OAuth access/refresh tokens may
     exist only in credential secret tables.
   - `credential_metadata`: labels, disabled state, account surrogates, hashes,
     token expiry, and health metadata.
   - `telemetry`: request, stream, health, and fallback metadata only.
   Local ilonasin client tokens must be one-way hashed; the reusable bearer
   value itself must never be stored.
7. Ensure telemetry, health, fallback, model cache, account metadata, TUI state,
   logs, and errors never store raw prompts, completions, request bodies,
   response bodies, raw provider payloads, tool arguments, tool results, raw SSE
   chunks, full bearer tokens, full provider request IDs, full generation IDs,
   full account IDs, balances, credit totals, OAuth callback URLs, token endpoint
   bodies, provider command stdout, cookies, or raw provider error bodies.
8. Make `serve` start an OpenAI-compatible HTTP daemon skeleton with:
   - authenticated `/v1/models`,
   - authenticated `/v1/chat/completions`,
   - clear unsupported/not-configured errors,
   - no body persistence.
9. Make `manage` open a minimal Bubble Tea/Lipgloss TUI shell by default, with a
   non-interactive `--check` mode for smoke testing.
10. Add focused tests for config, path resolution, migrations, model addressing,
    HTTP auth, strict JSON decoding, unsupported request validation, and no body
    persistence.
11. Run smoke tests directly against:
    - `ilonasin serve --check`
    - `ilonasin manage --check`

## Out of Scope

- Real upstream provider calls.
- OAuth browser/device login flows.
- API-key entry UX in the TUI.
- Complete provider adapter implementations.
- Full streaming translation.
- Credential fallback execution.
- Rich TUI views beyond the first shell.
- Real Codex authentication, Codex credential import, Codex keyring/file/cookie
  inspection, agent identity handling, or account rotation behavior.

## Design Constraints

- Provider instances remain config-defined; the TUI must not mutate
  `config.toml`.
- SQLite is the mutable source of truth for credentials, local client tokens,
  usage metadata, model cache, health, and fallback events.
- Local client tokens are distinct from upstream provider credentials.
- Normal operation must persist metadata only.
- Unsupported OpenAI-compatible fields must be rejected clearly instead of
  silently forwarded.
- Provider-specific behavior must live behind adapter boundaries as subsequent
  slices fill them in.
- No default reusable local API token, auth bypass, or magic development token
  may be created. Tests and `serve --check` may seed temporary isolated tokens
  in temporary databases only. Real `serve` returns `401` until a client token
  exists in SQLite.
- `codex` is a placeholder provider type in this slice. The scaffold must not
  inspect `CODEX_HOME`, Codex keyrings, cookies, `auth.json`, or
  `.credentials.json`.
- HTTP auth should happen before body parsing where practical. Request bodies
  must be bounded. JSON decoding must reject unknown fields.
- Errors should use clear OpenAI-style error envelopes without raw provider
  payloads or sensitive IDs.
- Logging/redaction code must treat `Authorization`, cookies, bearer-like
  strings, OAuth URLs, provider request/generation IDs, account IDs, raw bodies,
  raw SSE chunks, raw provider errors, tool arguments/results, and secret
  presence inventory as sensitive unless explicitly allowlisted.
- First-run bootstrap may create a minimal `config.toml` only when no explicit
  `--config` path was provided. The TUI must not edit `config.toml`.

## Slice 001 Library Decisions

- HTTP stack: Go standard library `net/http`.
- SQLite driver: `github.com/mattn/go-sqlite3` through `database/sql`, with WAL,
  foreign keys, busy timeout, and UTC timestamps.
- Migration strategy: embedded, ordered SQL migrations applied by
  `internal/storage/sqlite`.
- TOML parser: `github.com/BurntSushi/toml`.
- TUI libraries: Bubble Tea and Lipgloss.
- Token hashing: SHA-256 for the initial local client token verifier; future
  slices may introduce a stronger keyed or memory-hard token storage strategy if
  review requires it.
- Test approach: Go `testing`, `httptest`, temp homes, and isolated SQLite DBs.
- Provider adapters: handwritten typed interfaces; no generated clients in this
  slice.

## Proposed Package Layout

```text
cmd/ilonasin/
internal/app/
internal/cli/
internal/config/
internal/credentials/
internal/home/
internal/openai/
internal/provider/
internal/routing/
internal/server/
internal/storage/sqlite/
internal/tui/
```

Import direction:

```text
cmd -> cli -> app -> {config, home, storage/sqlite, server, tui}
server -> {credentials, openai, provider, routing}
provider must not import server or tui
tui must not mutate config files
app is composition only
```

Package responsibilities:

- `internal/openai`: request/response DTOs, strict validation, error envelopes.
- `internal/routing`: model address parsing and routing policy primitives.
- `internal/credentials`: local API token hashing/verification, secret redaction
  helpers, credential resolver interfaces.
- `internal/provider`: provider adapter interfaces and built-in provider default
  registry.
- `internal/storage/sqlite`: SQL migrations and repositories.
- `internal/server`: HTTP transport and handler wiring only.

## Verification

Run:

```text
go test ./...
tmp="$(mktemp -d)"
ILONASIN_HOME="$tmp" go run ./cmd/ilonasin serve --check
ILONASIN_HOME="$tmp" go run ./cmd/ilonasin manage --check
```

`serve --check` must initialize config and SQLite in the selected home, seed a
temporary isolated hashed client token, bind `127.0.0.1:0`, construct the real
handler, issue an unauthenticated `/v1/models` request, issue an authenticated
`/v1/models` request, issue an authenticated `/v1/chat/completions` request with
an unsupported field, verify the expected statuses, then exit.

`serve --check` must never seed a token into the real default home. If no
explicit isolated `ILONASIN_HOME` or `--config` is provided, it must create and
use its own temporary home for the check run.

`manage --check` must run the same bootstrap path and render the real Bubble
Tea root model in headless mode once. It must not use a separate fake TUI path.

## Review Questions

1. Is this slice small enough to complete safely while still moving toward the
   full architecture?
2. Does the proposed package layout create clean boundaries for later provider,
   routing, credential, and TUI work?
3. Are any schema concepts likely to encode forbidden raw request/response data?
4. Is `--check` acceptable for repeatable CLI smoke tests without compromising
   the real `serve` and `manage` commands?
5. What must change before implementation?
