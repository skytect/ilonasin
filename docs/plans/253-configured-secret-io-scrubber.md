# 253 Configured Secret IO Scrubber

## Context

`docs/ilonasin-architecture.md` and plan 300 require IO logs to preserve useful
payloads only when `[logging].capture_io = true`, while never writing local
client tokens, upstream API keys, OAuth tokens, cookies, codes, code verifiers,
provider command stdout, or configured credential secret values.

Plan 231 improved field-name and marker scrubbing, but explicitly left
configured-secret exact-value redaction incomplete. The current IO scrubber is
package-level and key-based: `logging.ScrubIOBody` can redact `access_token`,
`refreshToken`, `Authorization: Bearer ...`, and similar carriers, but it
cannot remove a stored credential secret if that exact value appears in an
ordinary payload field.

## Goal

Make opt-in IO logging redact exact configured credential secret values without
loosening normal structured logging or changing route/provider behavior.

## Scope

1. Add a small IO scrubber object in `internal/logging`:
   - it keeps the existing key, form, header, and marker scrubbing behavior;
   - it also redacts exact configured secret values;
   - it redacts configured secret values even when embedded inside larger JSON,
     form, or plain string values;
   - it applies marker scrubbing inside JSON string values, so `Bearer ...`,
     `iln_...`, and configured exact values cannot leak through non-secret JSON
     fields;
   - it ignores empty or very short values to avoid redacting ordinary text.
2. Keep the existing package-level `ScrubIOBody` as the default scrubber for
   callers that do not have configured secrets.
3. Let `IOLogger` own the scrubber and expose methods to replace its configured
   secret set and scrub bodies through that scrubber. Updates and reads must be
   thread-safe because IO records can be written while credentials are added or
   refreshed.
4. Add a storage method that lists current credential secret material from
   SQLite without exposing it through management DTOs, TUI, or normal logs.
5. Wire configured secrets into `IOLogger` during `serve` startup and through
   the shared credential mutation boundary. Runtime refresh must cover
   management-triggered API-key creation, OAuth device-login completion,
   management-triggered OAuth refresh, server-triggered OAuth refresh,
   subscription-usage-triggered OAuth refresh, and keepalive-adjacent refresh
   paths because they all mutate through the same credential service.
6. Register newly created local client token values with the IO scrubber at the
   only point where the raw value exists, without storing or exposing them in
   SQLite, management snapshots, TUI state, or normal logs. Existing local
   tokens cannot be recovered from hashes and remain covered by recursive
   `iln_...` marker scrubbing.
7. Change server and provider IO logging to call the `IOLogger` scrubber method
   instead of package-level `logging.ScrubIOBody`.
8. Keep normal `slog` behavior unchanged.
9. Do not add new public API routes, config fields, database schema, provider
   behavior, TUI behavior, or permanent tests.

## Non-Goals

- No full plan-300 rewrite of normal logging.
- No remote log shipping.
- No IO log viewer.
- No arbitrary secret detection in prompts or completions.
- No storage encryption changes.
- No raw credential exposure in management snapshots or TUI.

## Verification

Run a temporary focused logging smoke and remove it before commit. It should
prove:

- default `ScrubIOBody` still redacts known JSON/form/header/plain secret
  carriers;
- a configured-secret scrubber redacts exact API-key, OAuth access, and OAuth
  refresh values even when those values appear in non-secret-looking fields;
- configured secret values are also redacted when embedded inside larger string
  values such as `before <secret> after`, URL-encoded form values, and plain
  text;
- local client token markers such as `iln_...` are redacted inside JSON string
  fields, not only in non-JSON text bodies;
- non-secret fields such as `prompt_tokens`, `completion_tokens`,
  `reasoning_tokens`, `cache_hit`, `account_hash`, `token_count`,
  `account_summary`, prompt text, and completion text remain intact;
- very short configured values are ignored.

Run:

```sh
find . -name '*_test.go' -type f -print
git diff --check
go test ./...
go vet ./...
```

Run disposable daemon smokes:

1. Build a temporary `ilonasin` binary.
2. Start `serve` with temporary `ILONASIN_HOME`, temporary SQLite,
   `[logging].capture_io = true`, and no upstream credentials.
3. Create a local client token through the management socket.
4. Add a disposable upstream API key through the management socket.
5. Send a local request body containing:
   - a visible prompt marker;
   - a non-secret token count field;
   - the local client token in a payload field;
   - the disposable upstream API-key value in a payload field.
6. Verify `ilonasin-io.log` contains the visible prompt marker and non-secret
   metric field but does not contain the local client token or upstream API key.
7. Run a focused temporary in-package smoke for the runtime refresh hook:
   - seed API-key and OAuth-like secret values in a fake secret source;
   - refresh the IO logger from that source;
   - mutate the source through the same callback used by the shared credential
     service to simulate API-key creation, OAuth device-login completion,
     direct OAuth refresh, and server retry OAuth refresh;
   - refresh again and verify old and new configured secrets are scrubbed from
     embedded JSON/form/plain payloads.
8. Run `ilonasin manage` against the daemon under a short timeout and verify
   the TUI renders API, providers, usage, and logs.
9. Start a second disposable daemon with `[logging].capture_io = false` and
   verify it does not create `ilonasin-io.log`.

During code review, explicitly check:

- normal application logs still use the existing `secretGuardHandler`;
- exact configured secrets are only loaded into the IO scrubber, not management
  snapshots or TUI state;
- newly created local client token values are registered only with the
  in-memory IO scrubber and are still never stored outside the normal hashed
  local-token storage path;
- runtime-added and refreshed upstream secrets refresh the IO scrubber;
- server-triggered OAuth refresh paths refresh the IO scrubber through the same
  shared mutation callback as management-triggered refresh paths;
- scrubber secret-set replacement and body scrubbing are race-safe;
- server and provider IO paths use the logger-owned scrubber;
- no permanent tests or smoke files remain.

## Acceptance

- IO logs redact exact configured credential secret values.
- IO logs still preserve useful non-secret payload fields when capture is
  enabled.
- Normal logging behavior is unchanged.
- Runtime credential mutations refresh the IO scrubber's configured secret set.
- Compile, vet, focused smoke, serve/manage smoke, capture-off smoke, and three
  implementation reviews pass.
