# 499 Whole Codebase Architecture Review

## Context

Plan 490 produced seven selected follow-up findings. Slices 491 through 498
addressed those findings across provider URL validation, management snapshot
privacy, pruning availability, TUI async mutations, downstream usage clarity,
permanent test cleanup, and CLI version build metadata.

The active goal requires regular whole-codebase senior-engineer review against
`docs/ilonasin-architecture.md`, not only local review of individual slices.
Run a fresh checkpoint before selecting another runtime change.

## Goal

Obtain three independent senior-engineer reviews of the current codebase and
docs, focused on fidelity to `docs/ilonasin-architecture.md`, and record
concrete findings for future implementation slices.

## Scope

1. Ask three senior subagents to review the entire current codebase and docs.
2. Reviewers should inspect at least:
   - `docs/ilonasin-architecture.md`;
   - the supporting markdown under `docs/**`;
   - server routing, provider adapters, credentials, storage, management, TUI,
     logging, metadata, privacy, config, app bootstrap, and CLI packages.
3. Ask reviewers to return only concrete findings, each with:
   - file and line reference;
   - severity;
   - architecture requirement violated or maintainability issue;
   - recommended next slice boundary.
4. Persist the three review outputs and selected follow-up findings in this
   plan before committing the slice.
5. Treat findings as input to future slices. Do not implement runtime fixes in
   this slice.
6. Do not add permanent tests.

## Worktree Isolation

This slice may run in a worktree that contains unrelated runtime edits. Those
edits are not part of slice 499, must not be modified by this slice, and must
not be staged or committed with this plan. Slice 499 may commit only:

- `docs/plans/499-whole-codebase-architecture-review.md`.

If verification observes failures caused by unrelated dirty runtime files, note
that in the review record instead of folding those changes into this slice.

## Verification

Run:

```sh
git diff --check
find . -name '*_test.go' -type f -print
go test ./...
go vet ./...
```

Run direct CLI smokes:

1. Build a temporary `ilonasin` binary.
2. Start `ilonasin serve` with isolated `ILONASIN_HOME`, temporary SQLite, IO
   capture disabled, keepalive disabled, and configured provider instances.
3. Verify management health and snapshot over the Unix management socket.
4. Run bounded `ilonasin manage` at narrow and wide terminal widths.
5. Remove all temporary files and terminate the daemon.

## Verification Record

- `git diff --check`: passed.
- `find . -name '*_test.go' -type f -print`: passed, no files found.
- `go test ./...`: passed as a compile/package check; all packages reported
  no test files.
- `go vet ./...`: passed.
- Temporary `go build -o "$tmpbin/ilonasin" ./cmd/ilonasin`: passed.
- `ilonasin serve` smoke: passed with isolated `ILONASIN_HOME`, temporary
  config, and management health plus snapshot checked over the Unix socket.
- `ilonasin manage` smoke: passed at 80 and 140 columns under a pseudo-terminal;
  both bounded runs exited by timeout with status `124` as expected.
- Cleanup: temporary home, binary, config, and daemon process were removed by
  the smoke script.
- Worktree isolation: unrelated dirty runtime files remained unstaged and are
  not part of this slice.

## Acceptance

- Three senior reviewers complete whole-codebase architecture reviews.
- Concrete findings are recorded in this plan.
- Selected follow-up findings are listed for future slices.
- No runtime behavior changes are made, staged, or committed by this slice.
- Compile, vet, serve smoke, manage smoke, senior plan review, and senior
  implementation review pass.

## Review Record

### Reviewer 1

Severity: High

- Finding: Codex upstream error `message` is copied into `error_reason` and
  written to normal application logs. That is upstream payload content outside
  explicit IO logging.
- References:
  - `internal/provider/codex_responses_parse.go:458`
  - `internal/provider/codex_responses.go:154`
  - `internal/provider/codex_responses_stream.go:165`
  - `docs/ilonasin-architecture.md:403`
- Requirement or issue: normal observability must not persist raw provider
  payloads, request IDs, account IDs, prompts, completions, or bodies unless IO
  logging is enabled.
- Recommended next slice: logging/privacy runtime slice. Normalize these logs
  to error class plus allowlisted sanitized fields only; put full provider
  reasons only behind IO logging with the scrubber.

Severity: Medium

- Finding: Server routing branches on `instance.Type == "codex"` to decide
  Codex-specific quota-pool exhaustion behavior. Provider-specific behavior is
  split between `internal/provider/route_policy.go` and server helpers.
- References:
  - `internal/server/provider_policy.go:40`
  - `internal/server/chat_nonstream.go:222`
  - `internal/server/chat_stream.go:306`
  - `internal/server/responses_route.go:173`
  - `docs/ilonasin-architecture.md:548`
- Requirement or issue: provider adapters own provider-specific behavior, and
  router/server core should consume typed route policy.
- Recommended next slice: provider-policy runtime slice. Move quota-pool
  exhausted error-envelope behavior into `provider.RoutePolicy` or
  adapter-owned policy, then make server handlers consume only that policy.
- Isolation note: the referenced server files are dirty runtime edits outside
  slice 499 and are not committed by this review checkpoint.

### Reviewer 2

Severity: High

- Finding: Responses tools for non-Codex chat-adapter routes are silently
  dropped instead of rejected. `responsesToolsToChatTools` skips non-`function`
  tools, `defer_loading` tools, and `strict` tools, then
  `ToChatCompletionRequest` only forwards tools if any remain, so a request can
  dispatch without requested tool capabilities.
- References:
  - `internal/openai/responses_tools.go:55`
  - `internal/openai/responses_tools.go:69`
  - `internal/openai/responses_tools.go:77`
  - `internal/openai/responses.go:718`
  - `docs/ilonasin-architecture.md:214`
- Requirement or issue: Responses routes must convert into the strict local
  model or reject unsupported features before provider dispatch. Chat-adapter
  paths must not silently drop hosted, deferred, namespaced, tool-search, or
  other unrepresentable tool families.
- Recommended next slice: Responses tool conversion rejection boundary. For
  non-Codex providers, reject any unrepresentable Responses tool declaration
  instead of filtering it out; keep Codex-native tool preservation unchanged.

### Reviewer 3

Severity: Medium

- Finding: `parallel_tool_calls` is silently dropped when the route policy
  disallows it, so DeepSeek Responses requests lose an unsupported client field
  instead of failing locally.
- References:
  - `internal/openai/responses.go:731`
  - `docs/ilonasin-architecture.md:214`
- Requirement or issue: compatibility routes should reject unsupported fields
  before provider dispatch.
- Recommended next slice: Responses provider-field validation, starting with
  `parallel_tool_calls`.

Severity: Medium

- Finding: Codex request building always sends a generated `ids.ThreadID` as
  upstream `prompt_cache_key`; the client key parsed from the Responses request
  is used only for local affinity and never reaches upstream.
- References:
  - `internal/provider/codex_responses_request.go:121`
  - `internal/openai/responses.go:120`
  - `docs/ilonasin-architecture.md:370`
  - `docs/codex-compatibility-audit.md:104`
- Requirement or issue: the documented Codex cache-locality contract treats
  safe client `prompt_cache_key` as the preferred upstream cache signal.
- Recommended next slice: carry safe client `prompt_cache_key` through
  conversion and forward it to Codex, with generated fallback only when absent.

Severity: Low

- Finding: request metadata persistence writes `outputTPSTotal` into both
  `output_tokens_per_second` and `output_tokens_per_second_total`, discarding
  `m.OutputTokensPerSecond`.
- References:
  - `internal/storage/sqlite/request_metadata.go:35`
- Requirement or issue: metadata fidelity is weakened and throughput fields are
  ambiguous.
- Recommended next slice: request metadata persistence cleanup.

## Selected Follow-Up Findings

1. Codex upstream error logging privacy boundary. This is high severity because
   upstream error messages may contain provider payload content outside explicit
   IO logging.
2. Responses tool conversion rejection boundary. Non-Codex chat-adapter
   Responses routes should reject unrepresentable tool declarations instead of
   silently dropping them.
3. Responses provider-field validation. Start with rejecting unsupported
   `parallel_tool_calls` instead of silently omitting it for providers whose
   route policy disallows it.
4. Codex prompt-cache-key propagation. Preserve a safe client
   `prompt_cache_key` for upstream Codex requests, using generated thread IDs
   only as a fallback.
5. Provider-policy quota error boundary. Move Codex-specific quota-pool
   response behavior out of raw server type checks and into typed provider
   policy. This follow-up belongs to the separate dirty runtime work currently
   present in the worktree, not to slice 499.
6. Request metadata throughput persistence cleanup. Store instantaneous and
   aggregate output tokens per second in their intended columns.
