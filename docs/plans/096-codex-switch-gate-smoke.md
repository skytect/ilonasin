# Plan 096: Codex switch-gate smoke

## Goal

Run a broad, real-credential Codex compatibility smoke before recommending
normal Codex use through ilonasin, then update the stale compatibility audit
with current evidence.

## Scope

- Use the real credentials already configured in `~/.ilonasin`.
- Keep the smoke isolated with a temporary `ILONASIN_HOME`, temporary
  `CODEX_HOME`, temporary workspace, temporary logs, and a fresh local client
  token that is disabled during cleanup.
- Point only the worktree server at the existing real SQLite database. Do not
  copy credential state, OAuth state, database files, WAL/SHM files, Codex auth
  state, logs, or cache into the temporary home.
- Refresh the real model cache with the current worktree binary before selecting
  models, because plan 095 changed Codex capability flags.
- Smoke Codex through ilonasin for:
  - root and `/v1` base URL text turns,
  - image input,
  - developer/system instruction behavior,
  - workspace edit/tool loop behavior,
  - direct function-call follow-up,
  - available reasoning efforts,
  - fast or priority service-tier behavior.
- Include DeepSeek and OpenRouter text probes through ilonasin when eligible
  models are present, to confirm all three provider types still route.
- Probe fake or direct error paths for `401`, retryable `5xx`, `429`, and
  `Retry-After` without intentionally exhausting live quota.
- Confirm outbound Codex backend requests use `store: false`.
- Run privacy/log scans after the live smoke for forbidden local persistence:
  prompts, completions, raw bodies, raw provider payloads, raw SSE chunks, image
  bytes, tool arguments, tool results, bearer tokens, account IDs, and request
  IDs.

## Non-goals

- Do not implement quota routing or quota pooling in this plan.
- Do not add permanent test files.
- Do not persist raw smoke captures in the repository.
- Do not switch user Codex defaults to ilonasin.

## Validation

- Record pass, fail, or partial results in `docs/codex-compatibility-audit.md`.
- Keep only sanitized structural evidence in docs.
- Run:
  - `git diff --check`
  - `find . -name '*_test.go' -type f -print`
  - `go test ./...`
  - `go vet ./...`
  - `go build -o "$tmpbin/ilonasin" ./cmd/ilonasin`
  - `ILONASIN_HOME="$tmp" "$tmpbin/ilonasin" serve --check`
  - `ILONASIN_HOME="$tmp" "$tmpbin/ilonasin" manage --check`

## Cleanup

- Disable the smoke local client token.
- Stop the worktree server.
- Remove all temporary smoke directories.
- Verify no smoke `codex exec` or worktree `ilonasin serve` process remains.
