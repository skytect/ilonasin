# 504 Codex Error Log Privacy

## Context

Plan 499 found that Codex upstream error `message` values can be copied into
`error_reason` and written to normal `provider_http` logs. That violates the
architecture rule that normal logs and metadata must not persist raw provider
payloads unless IO logging is explicitly enabled.

The risky path is Codex Responses parsing:

- `codexFailureFromError` reads upstream `response.failed` or
  `response.incomplete` error `message`.
- `codexErrorReason` builds `error_reason` with that message.
- `CompleteCodexResponses` and `StreamCodexResponses` write `error_reason` to
  normal structured logs.

The code already exposes structured `upstream_error_code`,
`upstream_error_type`, and `upstream_error_param` attributes for
`codexEventFailure`. Those are safer metadata fields than a raw provider
message.

## Goal

Prevent Codex upstream error messages and generic Codex read error strings from
being persisted in normal logs, while preserving useful metadata-only error
classification.

## Scope

1. Change Codex upstream failed-event reasons so they include only safe
   structured metadata such as code and type, not raw upstream `message`.
2. Preserve use of upstream `message` only for in-memory error classification
   where needed, not for normal log output.
3. Keep `upstream_error_code`, `upstream_error_type`, and safe
   `upstream_error_param` logging from `codexFailureLogAttrs`.
4. Replace normal Codex read `error_reason` logging with an allowlisted safe
   reason helper. Parser-owned local reasons must be fixed tokens and must not
   interpolate upstream-controlled event types, item types, or payload values.
   Raw generic `err.Error()` values must not be written to normal logs.
5. Preserve IO logging behavior, which remains the explicit local debugging
   exception controlled by `capture_io`.
6. Keep routing, provider requests, response conversion, health events,
   storage schema, management APIs, TUI, and config unchanged.
7. Do not add permanent tests.

## Out Of Scope

- Changing provider error classes or retry/fallback decisions.
- Adding new storage fields or tables.
- Scrubbing IO logs beyond existing IO logging boundaries.
- Broad log-policy refactors outside the Codex Responses read-error path.

## Verification

Use a temporary focused harness, then remove it before commit, to verify:

- A Codex `response.failed` event with a distinctive upstream message does not
  place that message in `codexSafeReadErrorReason`.
- The same event still classifies with the expected error class.
- Structured error metadata still contains safe code and type fields.
- A generic read error does not produce an `error_reason` for normal logs.
- Parser-owned local validation reasons are fixed tokens and do not include
  upstream-controlled event types, item types, or payload values.

Run:

```sh
rg -n 'error_reason|codexReadErrorReason|codexSafeReadErrorReason|message=' internal/provider/codex_responses*.go
git diff --check
find . -name '*_test.go' -type f -print
go test ./...
go vet ./...
```

Run direct CLI smoke:

1. Build a temporary `ilonasin` binary.
2. Start `ilonasin serve` with isolated `ILONASIN_HOME`, temporary config,
   temporary SQLite, IO capture disabled, and keepalive disabled.
3. Verify management health and snapshot over the Unix management socket.
4. Run bounded `ilonasin manage` at 80 and 140 columns under a pseudo-terminal.
5. Remove all temporary files and terminate the daemon.

## Acceptance

- Normal Codex provider logs no longer persist raw upstream error messages or
  generic read error strings.
- Normal Codex provider logs no longer persist upstream-controlled event or item
  type values inside parser-owned validation reasons.
- Safe structured upstream error metadata remains available.
- Codex error classification and route behavior are preserved.
- No permanent tests are added.
- Compile, vet, serve smoke, manage smoke, senior plan review, and senior
  implementation review pass.

## Implementation Record

- Added `codexSafeReadErrorReason` for normal log `error_reason` output.
- Removed the old `codexReadErrorReason` helper so raw `err.Error()` is not
  available through a stale Codex Responses helper.
- Changed Codex upstream failed-event reasons to include only code and type,
  while keeping upstream message available only for in-memory error
  classification.
- Changed parser-owned validation reasons to fixed tokens, without embedding
  upstream-controlled event or item type values.
- Kept safe structured upstream error metadata logging through
  `upstream_error_code`, `upstream_error_type`, and `upstream_error_param`.
- Replaced the adjacent Codex response marshal `err.Error()` log reason with a
  fixed `codex response marshal failed` token.

## Verification Record

- Senior plan review: two reviewers found that parser-owned reasons still
  embedded upstream-controlled event/item type values. The plan was tightened to
  require fixed local reason tokens. One reviewer reported no findings.
- Temporary focused harness: passed for upstream failed-event message privacy,
  preserved classification, safe code/type attributes, generic read-error
  omission, and malformed item type privacy. Temporary harness was removed
  before commit.
- `rg -n 'error_reason|codexReadErrorReason|codexSafeReadErrorReason|message=' internal/provider/codex_responses*.go`:
  passed; normal Codex Responses error reasons now use safe fixed-token paths.
- `git diff --check`: passed.
- `find . -name '*_test.go' -type f -print`: passed, no files found.
- `go test ./...`: passed as a compile/package check; all packages reported
  no test files.
- `go vet ./...`: passed.
- Temporary `go build -o "$tmpbin/ilonasin" ./cmd/ilonasin`: passed.
- `ilonasin serve` smoke: passed with isolated `ILONASIN_HOME`, temporary
  config, and management health plus snapshot checked over the Unix socket.
- `ilonasin manage` smoke: passed at 80 and 140 columns under a pseudo-terminal.
- Senior implementation review: three reviewers reported no findings; one
  reviewer noted the old unsafe helper as residual stale-code risk, and it was
  removed before final verification.
- Cleanup: temporary home, binary, config, harness, captures, and daemon process
  were removed.
