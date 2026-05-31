# 094 Local Responses Function Tool Loop

## Goal

Make ilonasin's local `/responses` and `/v1/responses` routes compatible with representable Codex function tool turns while staying stateless and privacy-preserving.

## Current Gap

- `internal/provider/codex_responses.go` already translates Chat Completions tool calls to Codex's upstream Responses API.
- `internal/openai/responses.go` currently drops local Responses `tools`, rejects `function_call` and `function_call_output` input items, and ignores `parallel_tool_calls`.
- `internal/server/responses_route.go` currently returns `responses_tool_calls_unsupported` when the upstream chat result contains tool calls.
- Codex source confirms follow-up turns send full-context `input` with matching `function_call` and `function_call_output` items. Orphan outputs are invalid.

## Scope

1. Parse local Responses function tools.
   - Accept Codex Responses-style function tools:
     `{"type":"function","name":"...","description":"...","parameters":{...},"strict":false}`.
   - Convert them to the internal Chat Completions tool shape.
   - For Codex provider routes, preserve raw local Responses function tools in memory for the outbound Codex request when this slice can round trip them.
   - Derive internal Chat Completions tools only for function declarations this contract can represent. Drop unsupported non-function and deferred declarations from the Codex outbound request because this slice cannot round trip their calls, and reject them for non-Codex providers.
   - Reject duplicate function names, invalid function names, and non-object function schemas at the local Responses boundary.
   - Reject `strict:true` at the local Responses compatibility boundary because none of the current provider paths support it.
   - Preserve JSON numbers with `UseNumber`.

2. Parse local Responses tool transcript items.
   - Accept `function_call` items with `call_id`, `name`, and string `arguments`.
   - Accept `function_call_output` items with `call_id` and string `output`.
   - Reject structured content-item-array `output` with a stable compatibility error. The internal Chat Completions contract cannot preserve those Codex wire semantics without a direct Responses path.
   - Preserve user `input_image` items by translating them into internal multimodal Chat content arrays before the Codex adapter converts them back to upstream Responses format.
   - Reject duplicate `call_id` values, outputs without a prior call, duplicate outputs for the same `call_id`, and ordering that cannot be represented as assistant/tool Chat Completions messages.
   - Translate them into assistant `tool_calls` messages and `role:"tool"` messages before provider validation.
   - Keep Codex's matching-call invariant enforced locally and through existing provider validation.
   - Return a structured compatibility error for unsupported valid Codex item families rather than echoing payloads.
   - Mirror Codex model metadata behavior by remapping unsupported requested reasoning efforts to the middle supported effort for the selected model.

3. Return local Responses function calls.
   - Extend `ExtractChatCompletionMessageResult` to return validated tool call payloads, not just `HasToolCalls`.
   - Emit `response.output_item.done` items of type `function_call` from local Responses SSE.
   - Keep emitting assistant message output when content is present.
   - Remove the local 501 for tool-call results.

4. Keep envelope compatibility unchanged.
   - Continue requiring `stream:true`.
   - Continue rejecting `store:true`.
   - Continue accepting `prompt_cache_key` and `client_metadata` structurally without storing raw values.
   - Map `parallel_tool_calls` into the internal request when a provider path supports it, and otherwise reject or ignore only through explicit provider validation. For Codex, do not forward it as a Chat Completions field because the Codex adapter derives upstream `parallel_tool_calls` from model capabilities.

## Unsupported Codex Item Families

Codex source also has `custom_tool_call`, `custom_tool_call_output`, `local_shell_call`, MCP outputs, tool-search outputs, and namespaced dynamic function calls. Plan 094 does not translate those transcript items through the Chat Completions contract. The implementation must reject unsupported transcript items before upstream with a stable compatibility error and no payload echo. Unsupported non-function tool declarations are not forwarded to Codex upstream in this slice, because forwarding them advertises calls that local Responses cannot return correctly. If upstream still emits an unsupported item family, ilonasin must fail the response instead of returning an empty success. A later compatibility slice must decide whether to extend the internal contract beyond Chat Completions or add a direct local Responses provider path.

Structured `function_call_output.output` arrays are also out of scope for this slice for the same reason. They must not be flattened to text because that would silently change the Codex transcript.

## Privacy Boundaries

- Do not store or log tool definitions, tool names, tool arguments, tool results, request bodies, response bodies, raw SSE chunks, full provider request IDs, full account IDs, prompts, or completions.
- Raw `arguments` and `output` values may exist only in memory while validating, translating, sending the selected upstream request, or emitting the client SSE response.
- Raw `arguments` and `output` values must never be copied into metadata, health rows, fallback rows, logs, errors, smoke summaries, or report artifacts.
- Generated local Responses SSE events may include function-call arguments for the client, but raw SSE chunks must not be stored.
- Error messages must identify bad fields without echoing field values.
- Smoke checks must include private markers for tool arguments and results and verify those markers do not land in SQLite metadata or logs.

## Smoke Checks

- `find . -name '*_test.go' -type f -print`
- `go test ./...` as compile/package check only
- `go vet ./...`
- `git diff --check`
- `go build -o "$tmpbin/ilonasin" ./cmd/ilonasin`
- `ILONASIN_HOME="$tmp" "$tmpbin/ilonasin" serve --check`
- `ILONASIN_HOME="$tmp" "$tmpbin/ilonasin" manage --check`
- Focused real-Codex compatibility checks through ilonasin using real `~/.ilonasin` credentials and a temporary `CODEX_HOME`, temporary workspace, sanitized environment, cleanup trap, `--ephemeral`, and `--ignore-user-config` where supported:
  - simple text prompt
  - tool-call task that requires command execution and records only sanitized structural summaries
  - follow-up after a tool result, asserting the outbound request structurally contains matching call and output items without retaining raw bodies
  - multimodal request as a compatibility check, not a Plan 094 pass gate
  - varied reasoning effort request as a compatibility check, not a Plan 094 pass gate
- Real-smoke marker scans must at minimum cover ilonasin SQLite metadata and ilonasin logs. Temporary `CODEX_HOME`, temp workspace, stdout, and stderr can contain the user's prompts, tool outputs, and Codex session/client artifacts by design, so those locations are reviewed for command failure evidence and cleaned up rather than treated as ilonasin privacy storage.

## Non-Goals

- No quota tracking.
- No storage-backed Responses state.
- No hosted web-search/file-search/computer-use support in this slice.
- No permanent tests.
