# 508 Request Throughput Persistence

## Context

Plan 499 found that request metadata persistence writes
`OutputTokensPerSecondTotal` into both `output_tokens_per_second` and
`output_tokens_per_second_total`. That discards `OutputTokensPerSecond` at the
SQLite boundary and weakens metadata fidelity.

`docs/ilonasin-architecture.md` requires metadata-only observability to remain
accurate and useful. Throughput metrics are safe metadata, but the instantaneous
and total fields should be stored in their intended columns.

## Goal

Persist request throughput fields without conflating instantaneous output TPS
and aggregate output TPS.

## Scope

1. Update `internal/server/chat_stream.go` so streaming request metadata keeps
   `summary.OutputTokensPerSecond` in `OutputTokensPerSecond`, while preserving
   total-latency TPS in `OutputTokensPerSecondTotal` with the existing fallback
   only when total is absent.
2. Update `internal/storage/sqlite/request_metadata.go` so:
   - `output_tokens_per_second` stores `m.OutputTokensPerSecond`;
   - `output_tokens_per_second_total` stores `m.OutputTokensPerSecondTotal`,
     falling back to `m.OutputTokensPerSecond` only when the total value is
     absent.
3. Preserve existing read-side fallback behavior for old rows whose total TPS is
   zero.
4. Preserve request metadata schema, migrations, summaries, TUI rendering,
   server metadata producers, management APIs, provider behavior, routing,
   logging structure, config, and IO logging behavior.
5. Do not add permanent tests.

## Out Of Scope

- Changing throughput formulas.
- Backfilling existing SQLite rows.
- Renaming metadata fields or TUI labels.
- Changing stream metrics persistence.

## Verification

Use a temporary focused harness, then remove it before commit, to verify:

- A request with distinct `OutputTokensPerSecond` and
  `OutputTokensPerSecondTotal` stores distinct values in the two SQLite columns.
- A request with only `OutputTokensPerSecond` still stores that value in both
  columns for compatibility.
- Streaming request metadata preserves `summary.OutputTokensPerSecond` as
  instantaneous TPS instead of overwriting it with total TPS.

Run:

```sh
rg -n 'OutputTokensPerSecond|output_tokens_per_second|outputTPSTotal' internal/storage/sqlite/request_metadata.go internal/storage/sqlite/summaries.go internal/server/request_metadata_finalizer.go internal/server/chat_stream.go
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

- SQLite request metadata preserves distinct instantaneous and total output TPS.
- Existing fallback behavior for absent total TPS is preserved.
- No permanent tests are added.
- Compile, vet, serve smoke, manage smoke, senior plan review, and senior
  implementation review pass.

## Implementation Record

- Updated streaming chat request metadata to preserve
  `summary.OutputTokensPerSecond` in `OutputTokensPerSecond`.
- Preserved the existing total-latency TPS field and fallback for streaming
  request metadata.
- Updated SQLite request metadata persistence so
  `output_tokens_per_second` stores instantaneous TPS and
  `output_tokens_per_second_total` stores aggregate TPS with the existing
  fallback when total is absent.
- Preserved schema, migrations, summaries, TUI rendering, management APIs,
  provider behavior, routing, logging structure, config, and IO logging
  behavior.

## Verification Record

- Senior plan review: two reviewers reported no findings; one reviewer found
  that streaming metadata producer scope was required, and the plan was updated
  before implementation.
- Temporary focused harnesses: passed for streaming metadata producer
  preservation, distinct SQLite column persistence, and fallback persistence.
  Temporary harnesses were removed before commit.
- `rg -n 'OutputTokensPerSecond|output_tokens_per_second|outputTPSTotal' internal/storage/sqlite/request_metadata.go internal/storage/sqlite/summaries.go internal/server/request_metadata_finalizer.go internal/server/chat_stream.go`:
  passed.
- `git diff --check`: passed.
- `find . -name '*_test.go' -type f -print`: passed, no files found.
- `go test ./...`: passed as a compile/package check; all packages reported no
  test files.
- `go vet ./...`: passed.
- Temporary `go build -o "$tmpbin/ilonasin" ./cmd/ilonasin`: passed.
- `ilonasin serve` smoke: passed with isolated `ILONASIN_HOME`, temporary
  config, free local bind port, IO capture disabled, keepalive disabled, and
  management health plus snapshot checked over the Unix socket.
- `ilonasin manage` smoke: passed at 80 and 140 columns under a pseudo-terminal.
- Senior implementation review: initial review found an unintended normal log
  attribute addition and incomplete verification wording; both were corrected.
  Three corrected implementation reviewers then reported no findings.
- Cleanup: temporary home, binary, config, harness, terminal captures, marker
  files, and daemon process were removed.
