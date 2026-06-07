# 529 Codex Switch-Gate Evidence Refresh

## Context

Plan 528 failed the strict zero-tech-debt completion gate. All three senior
reviewers identified stale or incomplete Codex compatibility evidence as a
blocker. The shared themes were:

- `docs/codex-compatibility-audit.md` still warns against broad Codex switching;
- root and `/v1` model discovery need refreshed live switch-gate evidence after
  a historical primary-credential discovery hazard;
- Codex CLI through OpenRouter remains partial;
- hosted, deferred, namespaced, MCP, shell, tool-search, and other tool-family
  parity remains unproven;
- privacy and health evidence must remain metadata-only and current.

## Goal

Refresh the Codex switch-gate evidence against the current codebase and update
`docs/codex-compatibility-audit.md` with current results, remaining blockers,
and next slices.

## Scope

1. Read all provider-related docs before running the evidence refresh:
   - `docs/ilonasin-architecture.md`;
   - `docs/codex-compatibility-audit.md`;
   - `docs/codex-auth.md`;
   - `docs/codex-endpoints.md`;
   - `docs/deepseek-api.md`;
   - `docs/openrouter-api.md`;
   - `docs/deepseek-openrouter-comparison.md`.
2. Build a temporary `ilonasin` binary from the current worktree.
3. Start `ilonasin serve` with isolated temp runtime paths and IO capture
   disabled.
4. Use either:
   - an isolated config whose provider credentials come only from existing
     environment variables or existing non-secret credential references; or
   - the real configured SQLite database in place only when a live Codex OAuth
     probe cannot be performed otherwise, with no copying of the database and
     with durable metadata-only rows explicitly documented.
5. If required live credentials are unavailable without unsafe copying or
   mutation, record the affected probe as unavailable and keep it as a concrete
   switch-gate blocker.
6. Use management APIs to create any temporary local client token required for
   probes, then disable it during cleanup.
7. Refresh local evidence for:
   - root and `/v1` model discovery routes;
   - root and `/v1` Responses text turns where provider credentials are
     available;
   - multi-credential model-discovery behavior and per-attempt health metadata
     where available;
   - the historical primary-credential discovery regression, without
     destructively changing real credential state;
   - Codex CLI routing through DeepSeek and OpenRouter where credentials and
     installed `codex` CLI are available;
   - upstream 401, 429, retryable 5xx, and `Retry-After` error-path behavior
     using fake upstreams where live-provider probes would be unsafe or
     unavailable;
   - privacy scans proving no raw prompts, completions, request bodies, response
     bodies, provider payloads, tool arguments, tool results, bearer tokens,
     account IDs, or request IDs are persisted in checked local logs or metadata.
8. Update `docs/codex-compatibility-audit.md` with:
   - current date;
   - current worktree or commit context;
   - which checks passed, failed, were unavailable, or remained partial;
   - sanitized evidence only;
   - next blockers and next slices.
9. Update this plan with implementation and verification records.

## Worktree Isolation

- This slice may change:
  - `docs/plans/529-codex-switch-gate-evidence-refresh.md`;
  - `docs/codex-compatibility-audit.md`;
- Do not push.
- Do not add permanent tests.
- Do not keep probe logs, temp workspaces, temp Codex homes, raw HTTP captures,
  local tokens, or temp SQLite files after verification.
- Do not store secrets, raw prompts, raw completions, raw provider payloads,
  raw tool arguments, or raw tool results in docs or committed files.
- If the evidence refresh reveals a runtime bug, stop this slice after
  recording the evidence, create a separate implementation plan for the fix,
  then rerun this switch-gate refresh after that fix.

## Out Of Scope

- Full implementation of hosted, deferred, namespaced, MCP, shell, or
  tool-search parity.
- Runtime bug fixes discovered by the evidence refresh.
- Broad TUI redesign.
- Provider-term policy decisions for subscription account fallback.
- Non-Unix management transport implementation.
- XDG path support.

## Verification

Run:

```sh
rg -n 'Date:|Switch Gate|Blockers|OpenRouter|Model discovery|tool' docs/codex-compatibility-audit.md docs/plans/529-codex-switch-gate-evidence-refresh.md
git diff --check
git diff --no-index --check "$tmpempty" docs/plans/529-codex-switch-gate-evidence-refresh.md
find . -name '*_test.go' -type f -print
go test ./...
go vet ./...
```

Run direct CLI smoke:

1. Build a temporary `ilonasin` binary.
2. Start `ilonasin serve` with isolated `ILONASIN_HOME`, temporary config,
   temporary SQLite, IO capture disabled, keepalive disabled, and configured
   provider instances.
3. Verify management health and snapshot over the Unix management socket.
4. Run bounded `ilonasin manage` at 80 and 140 columns under a pseudo-terminal.
5. Confirm TUI output includes ANSI color sequences and multiple 256-color
   codes.
6. Remove all temporary files and terminate the daemon.

## Acceptance

- Three senior reviewers approve this plan.
- The evidence refresh is run or a specific unavailable prerequisite is
  recorded as a blocker.
- `docs/codex-compatibility-audit.md` reflects current evidence, not stale plan
  099-era state.
- Privacy posture is documented with sanitized evidence only.
- Any remaining switch blocker is explicit and mapped to a next slice.
- Temporary tokens, probe files, logs, workspaces, and daemon processes are
  cleaned up.
- No permanent tests are added.
- Compile, vet, serve smoke, manage smoke, senior implementation review, and
  plan-review checks pass.

## Plan Review Record

- Euclid approved the initial plan.
- Ampere approved the initial plan.
- Avicenna found two plan issues:
  - credential sourcing was undefined for live Codex OAuth probes while the plan
    also required isolated temp runtime paths;
  - allowing runtime fixes inside the evidence-refresh slice blurred the slice
    boundary.
- The plan was revised to allow either env-backed credentials or the real
  SQLite database in place for Codex OAuth only when needed, to record
  unavailable credentials as blockers, and to stop this slice if runtime bugs
  are discovered.
- Euclid, Avicenna, and Ampere approved the revised plan.

## Implementation Record

- Read the required provider docs:
  - `docs/ilonasin-architecture.md`;
  - `docs/codex-compatibility-audit.md`;
  - `docs/codex-auth.md`;
  - `docs/codex-endpoints.md`;
  - `docs/deepseek-api.md`;
  - `docs/openrouter-api.md`;
  - `docs/deepseek-openrouter-comparison.md`.
- Confirmed installed `codex` is `codex-cli 0.137.0`.
- Ran an isolated switch-gate evidence refresh with a temporary binary,
  temporary runtime, temporary `CODEX_HOME`, temporary workspace, IO capture
  disabled, keepalive disabled, and the real configured SQLite database used in
  place for upstream credentials.
- Created a temporary local client token through the daemon management API and
  disabled it during cleanup.
- Updated `docs/codex-compatibility-audit.md` with current sanitized evidence.

## Evidence Record

- Management health returned `ok`.
- Safe management metadata showed provider instances:
  `pragnition-codex`, `pragnition-deepseek`, and `pragnition-openrouter`.
- Safe management metadata showed six active Codex OAuth credentials for
  `pragnition-codex` and active API-key credentials for `pragnition-deepseek`
  and `pragnition-openrouter`.
- `GET /models` returned HTTP 200 with `object: "list"` and 348 model rows.
- `GET /v1/models` returned HTTP 200 with `object: "list"` and 348 model rows.
- Direct streaming Responses returned HTTP 200 for:
  - `pragnition-codex/gpt-5.4-mini`;
  - `pragnition-deepseek/deepseek-v4-flash`;
  - `pragnition-openrouter/openai/gpt-3.5-turbo`.
- Non-streaming direct Responses probes were not counted as compatibility
  evidence because local validation requires `stream: true` for this surface.
- A real `codex exec` text probe with Codex CLI 0.137.0 reached the local
  Responses route but exited nonzero. Sanitized terminal output showed the
  local validation error `tools[0].strict is unsupported`.
- Because that is a runtime compatibility bug, this evidence slice stops here.

## Selected Next Slice

Plan 530 should implement bounded Codex Responses function-tool `strict`
handling.

The slice boundary is:

- update `internal/openai/responses_tools.go`;
- for Codex-preserved `function` tool declarations, accept a boolean `strict`
  field only when it is representable and safe to preserve upstream;
- preserve rejection of non-boolean `strict`, unknown fields, and all unproven
  non-function tool families;
- preserve non-Codex behavior where `strict:true` function tools are skipped
  rather than silently downgraded;
- use a temporary focused harness and rerun the Codex switch-gate smoke after
  the fix.

## Verification Record

- Temporary live probe cleanup: temporary binary, temporary runtime,
  `CODEX_HOME`, workspace, token files, probe logs, and daemon process were
  removed. The temporary local client token was disabled through the management
  API.
- Privacy scan: checked one temporary server log file and found zero
  probe-marker hits, zero bearer-shaped hits, and zero secret-key-name hits.
- Documentation checks passed:
  - `rg -n 'Date:|Switch Gate|Blockers|OpenRouter|Model discovery|tool' docs/codex-compatibility-audit.md docs/plans/529-codex-switch-gate-evidence-refresh.md`;
  - `git diff --check`;
  - `git diff --no-index --check "$tmpempty" docs/plans/529-codex-switch-gate-evidence-refresh.md`.
- Compile and static checks passed:
  - `find . -name '*_test.go' -type f -print` returned no permanent test
    files;
  - `go test ./...`;
  - `go vet ./...`.
- Direct CLI smoke passed with a temporary binary, temporary runtime, and
  terminated daemon:
  - management health and snapshot over the Unix management socket;
  - bounded `ilonasin manage` under a pseudo-terminal at 80 and 140 columns;
  - TUI capture included ANSI SGR output with multiple color codes.
