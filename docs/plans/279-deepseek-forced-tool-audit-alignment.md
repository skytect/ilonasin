# 279 DeepSeek Forced Tool Audit Alignment

## Goal

Align stale compatibility-audit language for direct DeepSeek forced tool calls
with current docs and code evidence.

`docs/deepseek-openrouter-comparison.md` records a live DeepSeek forced tool
call returning `200`, and current code accepts OpenAI-style `tools` plus
`tool_choice` for DeepSeek:

- `internal/openai/chat_validation.go` validates named function `tool_choice`
  against the supplied tool list;
- `internal/provider/chat_validation.go` does not reject non-strict DeepSeek
  tools or `tool_choice`;
- `internal/openai/chat_request.go` forwards `tools` and `tool_choice` when
  present.

`docs/codex-compatibility-audit.md` still reports “DeepSeek forced tool” as a
current fail with required next fix. That row conflicts with later DeepSeek
comparison evidence and likely misdirects architecture work.

## Scope

1. Add a temporary focused provider smoke, run it, and remove it before commit.
   It must prove:
   - the request passes the OpenAI Chat request validation path, including the
     named `tool_choice` guard;
   - a DeepSeek Chat Completions request with one function tool and matching
     named `tool_choice` validates at the provider boundary;
   - the marshaled upstream body preserves `tools` and the named
     `tool_choice`;
   - a mismatched named `tool_choice` still fails local request validation;
   - DeepSeek strict tool mode still fails locally, preserving the documented
     beta-boundary behavior.
2. Update `docs/codex-compatibility-audit.md` DeepSeek forced-tool row to match
   current evidence:
   - direct DeepSeek forced tool is no longer a current local/request-shaping
     capability failure in this audit, citing later live provider evidence
     separately from the temporary local validation and marshaling smoke;
   - strict DeepSeek tool mode remains a separate beta feature boundary;
   - broader Codex/OpenRouter/tool-family blockers remain unchanged.
3. Do not change production code unless the temporary smoke contradicts current
   code inspection.

## Boundaries

- Documentation-only final diff unless the focused smoke exposes a real code
  regression.
- No management API, storage, TUI, subscription keepalive, model discovery,
  logging policy, route shape, OpenRouter behavior, Codex behavior, or live
  credential behavior changes.
- Do not claim all DeepSeek tool schemas are portable or strict mode is
  supported.
- Do not claim broad Codex switch-gate readiness.
- No permanent tests.

## Verification

Run:

```sh
find . -name '*_test.go' -type f -print
git diff --check
go test ./...
go vet ./...
```

Run a temporary focused provider smoke, then remove it before commit:

- construct a DeepSeek `ChatCompletionRequest` with a function tool and named
  `tool_choice`;
- assert OpenAI Chat request validation succeeds;
- assert provider validation succeeds;
- assert `marshalChatCompletionsRequest("deepseek", ...)` preserves `tools` and
  the named `tool_choice`;
- assert a mismatched named `tool_choice` fails local request validation;
- assert a strict tool still fails local validation.

Run disposable daemon smokes:

1. Build a temporary `ilonasin` binary.
2. Start `serve` with temporary `ILONASIN_HOME`, temporary SQLite, IO capture
   disabled, keepalive disabled, and at least two provider instances.
3. Verify management health over the management socket.
4. Run `manage` under a short timeout and verify API, providers, usage, and
   logs chrome renders.
5. Remove all temporary artifacts.

## Acceptance

- Temporary smoke proves current DeepSeek forced-tool validation and marshaling.
- `docs/codex-compatibility-audit.md` no longer reports non-strict DeepSeek
  forced tool as a current failure.
- Strict DeepSeek tool mode remains explicitly not broadened.
- Remaining compatibility blockers remain intact.
- Final diff is docs-only, with no temporary smoke file left.
- No permanent test files remain.
- Compile, vet, serve smoke, manage smoke, senior plan review, and senior
  implementation review pass.
