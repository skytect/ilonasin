# 500 Codex Cyber Mitigation Observations

## Context

Plan 495 researched Codex cyber-use verification. The research found no stable
positive machine-readable signal that proves an account has Trusted Access for
Cyber. It did find three safe request-level observation classes:

- `verification_recommended`;
- `mitigated_rerouted`;
- `policy_blocked`.

The architecture requires metadata-only observability, daemon-owned management
surfaces, and provider-specific behavior behind provider boundaries. This slice
implements only conservative observed mitigation metadata. It must not route by
"verified account" and must not label unknown accounts as verified.

The current worktree also contains unrelated dirty server changes around Codex
quota-pool error formatting. Those are out of scope for this slice unless they
must be adjusted for compile safety.

## Goal

Record and surface source-backed Codex cyber mitigation observations as
metadata-only credential health state.

## Scope

1. Parse Codex Responses events for:
   - `response.metadata.metadata.openai_verification_recommendation` containing
     `trusted_access_for_cyber`;
   - a Codex server-reported model that differs from the requested upstream
     model, when that can be observed from safe response metadata;
   - `response.failed` with error code `cyber_policy`.
2. Map those observations to safe health event metadata:
   - `codex_verification_recommended`;
   - `codex_mitigated_rerouted`;
   - `codex_policy_blocked`.
3. Reuse the existing `health_events` table and management health snapshot
   rather than adding a new table in this slice.
4. Surface the event classes in `ilonasin manage` health rows using existing
   metadata-only provider, model, credential, time, and status fields.
5. Keep these event classes display and audit metadata only. They must not
   alter credential eligibility, credential ordering, fallback decisions, model
   routing, or quota pooling in this slice.
6. Preserve the research boundary:
   - no positive `trusted_access_observed` state;
   - no account verification routing;
   - no account probes;
   - no account emails or full account IDs;
   - no raw provider payloads outside IO logging.
7. Do not add permanent tests.

## Out Of Scope

- Verified-account routing.
- Config flags such as `require_trusted_access`.
- Background account checks or private account endpoint probes.
- A new cyber-specific table or retention policy.
- TUI color restoration; that follows in the next slice.
- The existing dirty Codex quota-pool error-response edits, except for keeping
  the worktree buildable while this slice is verified.

## Verification

Use temporary focused compile-only probes or local fake-upstream harnesses as
needed, then remove them before commit.

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
4. Run bounded `ilonasin manage` at narrow and wide terminal widths under a
   pseudo-terminal.
5. Remove all temporary files and terminate the daemon.

## Acceptance

- Codex cyber mitigation observations are represented only as metadata health
  events.
- `verification_recommended`, `mitigated_rerouted`, and `policy_blocked` are
  all covered when the source-backed signal is present.
- Cyber mitigation health events do not affect routing or credential
  eligibility in this slice.
- Unknown credentials are never rendered or routed as verified.
- The management API and TUI expose only safe event class metadata.
- No permanent tests are added.
- Compile, vet, serve smoke, manage smoke, senior plan review, and senior
  implementation review pass.

## Implementation Record

- Added `HealthEventClasses` to provider chat and stream result summaries as
  provider-owned metadata output.
- Parsed Codex `response.metadata` verification recommendations and
  `response.failed` `cyber_policy` errors into safe health event classes.
- Detected Codex mitigated reroutes when a safe server-reported model differs
  from the requested upstream model, including safe model signals from HTTP
  headers and SSE header payloads.
- Recorded cyber observations through existing `health_events` metadata rows.
- Rendered cyber health rows in the TUI as `verify`, `reroute`, or `cyber`
  badges without changing routing, eligibility, or credential ordering.

## Verification Record

- Temporary focused parser harness covered:
  - `codex_verification_recommended`;
  - `codex_mitigated_rerouted`;
  - `codex_policy_blocked`.
- Temporary harness was deleted before commit.
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
- Implementation re-review: three senior subagents reported no findings after
  fixes for parser state preservation, exact `cyber_policy` code matching,
  top-level metadata events, model header sources, array-valued SSE headers,
  and stream/non-stream error-path reroute derivation.
