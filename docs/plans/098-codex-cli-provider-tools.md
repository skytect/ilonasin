# 098 Codex CLI Provider Tools

## Context

Plan 096 showed that direct local Responses calls pass for Codex, DeepSeek, and
OpenRouter, but `codex exec` through DeepSeek/OpenRouter fails before inference
with `tools[5].type is unsupported`.

Current Codex CLI 0.135.0 custom providers use the Responses API. Source in
`/tmp/codex-src-0.135.0/codex-rs` shows Codex serializes flat `function`
tools plus non-chat tool families such as `namespace`, `tool_search`,
`image_generation`, `web_search`, `custom`, and `freeform`. DeepSeek and
OpenRouter currently receive Chat Completions requests from ilonasin, so the
local implementation can only forward flat OpenAI-style Chat function tools
through this route. The local conversion currently rejects every non-`function`
Responses tool for non-Codex providers.

There are unrelated OAuth refresh-classification edits in the worktree. This
slice must not modify or stage those files.

## Goal

Make `codex exec` routed through ilonasin to DeepSeek and OpenRouter get past
Codex's default Responses request shape by preserving representable flat
function tools and filtering non-chat-representable or provider-unsupported
Responses fields.

This is not full Codex tool parity. It is a compatibility step that removes the
local `tools[n].type is unsupported` blocker while preserving privacy and
provider-boundary validation.

## Scope

1. Update Responses tool conversion.
   - In `internal/openai/responses.go`, keep strict validation for tool arrays
     and flat `type: "function"` tools.
   - For non-Codex providers, skip non-`function` Responses tool families
     rather than failing. This includes every unknown or known non-function
     type, including `namespace`, `tool_search`, `image_generation`,
     `web_search`, `custom`, hosted, and freeform tool shapes that cannot be
     represented as Chat Completions tools.
   - For non-Codex providers, skip deferred function tools
     (`defer_loading: true`) rather than failing.
   - For non-Codex providers, omit the Responses-style top-level `strict:false`
     flag from forwarded Chat tools.
   - For non-Codex providers, skip `strict:true` function tools rather than
     silently downgrading their schema contract.
   - Preserve flat function names, descriptions, and parameter schemas when
     they are representable.
   - Keep duplicate-name checks among forwarded function tools only. A skipped
     deferred or strict tool must not collide with a forwarded same-name tool.
   - If no tools remain after filtering, omit both `tools` and `tool_choice`
     from the Chat Completions request.
2. Filter Responses-to-Chat fields by provider.
   - Do not forward `parallel_tool_calls` to DeepSeek, because the current
     provider validator and DeepSeek docs treat it as unsupported.
   - Continue forwarding `parallel_tool_calls` to OpenRouter when present.
   - Keep provider-specific request validation as the final boundary. The
     conversion should avoid known Codex CLI shape mismatches, not bypass
     provider validators.
3. Preserve Codex-provider behavior.
   - Do not weaken the Codex path that preserves raw Responses function tools
     for the Codex provider adapter.
   - Keep unsupported non-function tool families filtered for Codex, as today.
   - Keep malformed flat function tools as request errors.
4. Add direct smoke coverage without permanent tests.
   - Extend existing `serve --check` exercises so a synthetic Responses request
     with mixed Codex-style tools succeeds through DeepSeek and OpenRouter.
   - Assert the fake upstream receives only flat Chat function tools.
   - Assert non-representable tool families, deferred tools, and strict flags
     are not forwarded.
   - Assert DeepSeek does not receive `parallel_tool_calls` from a Responses
     request and OpenRouter still can.
   - Assert an all-filtered tool set forwards no `tools`, no `tool_choice`,
     and no DeepSeek-rejected `parallel_tool_calls`.
   - Assert a deferred duplicate plus forwarded same-name function succeeds.
   - Assert two forwarded same-name function tools still fail locally.
   - Keep sentinel strings out of logs and metadata.
5. Run real Codex CLI smokes when feasible.
   - Use real credentials in `~/.ilonasin`.
   - Use a temporary `CODEX_HOME`, temporary workspace, temporary logs, and a
     local client token that is disabled during cleanup.
   - Run at least text `codex exec` routes for DeepSeek and OpenRouter through
     ilonasin and verify the failure is no longer the local unsupported tool
     type.
   - If provider/model behavior still fails upstream or agent behavior is
     incomplete, record the exact safe failure class in docs.
   - After live smokes, scan the relevant ilonasin logs and metadata for the
     smoke sentinels and secret-shaped values. Do not persist raw Codex JSONL,
     raw HTTP bodies, prompts, completions, image bytes, tool arguments, tool
     results, bearer tokens, account IDs, or request IDs in docs.
6. Update compatibility docs.
   - Update `docs/codex-client-red-team.md`.
   - Update `docs/codex-compatibility-audit.md`.
   - Distinguish "unsupported local tool type blocker fixed" from "full
     coding-agent tool parity proven".

## Non-Goals

- Do not implement Responses `namespace` semantics for Chat providers.
- Do not implement freeform `apply_patch` translation in this slice.
- Do not claim workspace edit/tool-loop compatibility is fixed.
- Do not add permanent `_test.go` files.
- Do not touch or stage the unrelated OAuth refresh-classification work.
- Do not add support for custom, MCP, tool-search, or freeform transcript item
  types. Existing unsupported input item types remain rejected. Existing
  `function_call` and `function_call_output` transcript handling remains the
  intentional flat-function tool-loop subset and must stay metadata/log safe.

## Acceptance

- `find . -name '*_test.go' -type f -print` prints nothing.
- `go test ./...` passes as a compile/package check.
- `go vet ./...` passes.
- A fresh binary builds.
- `serve --check` passes with the new mixed-tool DeepSeek/OpenRouter exercise.
- `manage --check` passes.
- `git diff --check` passes.
- Real-smoke results, if run, include a privacy/log/metadata scan result.
- `docs/codex-client-red-team.md` and
  `docs/codex-compatibility-audit.md` reflect the new result.
- Only files for this slice are staged and committed. If `internal/app/app.go`
  needs a smoke hook, stage only the Plan 098 hunk and leave existing OAuth
  refresh-classification hunks unstaged.
