## Response Style

- Be very brief.
- Prefer concrete file paths, commands, and outcomes over long explanations.

## Context

- Read all markdown files in `docs/**` before making architecture-sensitive
  changes.
- Refer to `docs/ilonasin-architecture.md` as the target architecture.
- Refer to `docs/deepseek-api.md`, `docs/openrouter-api.md`,
  `docs/deepseek-openrouter-comparison.md`, `docs/codex-auth.md`, and
  `docs/codex-endpoints.md` when touching provider behavior.
- Only if explicitly told to save an implementation plan, add it to
  `docs/plans/`.

## Coding Style

- Do not write tests by yourself unless explicitly told to. Tests are not fixes,
  they are false assurances.
- Do not keep permanent test files. Use direct compile, vet, and CLI smoke
  checks instead.
- Never implement more than necessary. Every piece of code you write adds to
  maintenance burden.
- Never hardcode or bodge, always start with the proper interfaces with stubs,
  even if just implementing a vertical slice.
- Keep local API auth, upstream provider credentials, provider adapters,
  routing, HTTP transport, TUI, config, and SQLite storage as separate
  boundaries.
- The TUI may mutate SQLite, but it must not mutate `config.toml`.
- Only store if IO logging is enabled for the request: prompts, completions, request bodies, response bodies, raw
  provider payloads, raw SSE chunks, tool arguments, tool result.

## Execution Style

- Prefer delegating tasks to subagents rather than doing them in the main agent,
  to keep the main agent focused on orchestration and reduce complexity.
- Run subagents in parallel or in the background whenever possible to save time,
  but be mindful of synchronization and shared state issues.
- DO NOT use `request_user_input` in the middle of a goal pursuit.
- Do not push.

## Smoke Checks

- Prefer direct command checks:
  - `find . -name '*_test.go' -type f -print`
  - `go test ./...` as a compile/package check only; do not add permanent tests.
  - `go vet ./...`
  - `go build -o "$tmpbin/ilonasin" ./cmd/ilonasin`
  - start `ILONASIN_HOME="$tmp" "$tmpbin/ilonasin" serve` in the background,
    wait for the HTTP API or management socket, then terminate it.
  - run `ILONASIN_HOME="$tmp" "$tmpbin/ilonasin" manage` against that daemon
    with a short timeout or PTY when a TUI smoke is useful.

## Task Tips

### Writing a plan

1. Discuss grey-areas using the `request_user_input` tool if not in the middle
   of a goal pursuit.
2. Document the plan in `docs/plans/` only when explicitly told to save one.

### Fixing a bug

Philosophy: bugs are not simply issues to patch. They are a view into how the
codebase design varies from the ideal. Use this view to refactor toward the
ideal.

When fixing a bug, you MUST:

- Do not write a one-time bodge. Think about the root cause and the robust
  long-term fix.

After fixing a bug, you MUST think:

- Is there any refactoring to make the fix more robust?
- Is there any refactoring to make sure adjacent issues never happen again?
- Any dead code to clean up?
