## Response Style

- Be very brief.
- Prefer concrete file paths, commands, and outcomes over long explanations.

## Context

- Read all markdown files in `docs/**` before making architecture-sensitive
  changes.
- Refer to `docs/ilonasin-architecture.md` as the target architecture.
- Write implementation plans in `docs/plans/` only when the active task requires
  a planned slice.

## Coding Style

- Do not write permanent tests unless explicitly told to.
- Use direct compile, vet, and CLI smoke checks instead of keeping test files.
- Never implement more than necessary; every line adds maintenance burden.
- Never hardcode or bodge. Start with the proper interfaces and stubs even for
  a vertical slice.
- Keep local API auth, upstream provider credentials, provider adapters,
  routing, HTTP transport, TUI, config, and SQLite storage as separate
  boundaries.
- The TUI may mutate SQLite, but it must not mutate `config.toml`.
- Do not store prompts, completions, request bodies, response bodies, raw
  provider payloads, raw SSE chunks, tool arguments, tool results, full bearer
  tokens, full provider request IDs, or full account IDs.

## Execution Style

- Use subagents when the active goal requires senior-engineer reviews or parallel
  review work.
- Run subagents in parallel when possible, but keep shared-state edits local
  unless deliberately delegated.
- Do not use `request_user_input` during goal pursuit.
- Do not push.

## Smoke Checks

- Prefer direct command checks:
  - `go test ./...` as a compile/package check only; do not add permanent tests.
  - `go vet ./...`
  - `go build -o "$tmpbin/ilonasin" ./cmd/ilonasin`
  - `ILONASIN_HOME="$tmp" "$tmpbin/ilonasin" serve --check`
  - `ILONASIN_HOME="$tmp" "$tmpbin/ilonasin" manage --check`

## Task Tips

### Writing a plan

- If the active goal requires a plan, document it in `docs/plans/`.
- Keep plans scoped to one coherent slice.

### Fixing a bug

Philosophy: bugs are not simply issues to patch. They are a view into how the
codebase design varies from the ideal. Use this view to refactor toward the
ideal.

When fixing a bug:

- Do not write a one-time bodge.
- Think about the robust long-term fix.
- Remove dead code made obsolete by the fix.
