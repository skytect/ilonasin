# 099 Codex Custom Tool Items

## Context

The compatibility audit still blocks broad Codex use because a real
`codex exec` workspace edit exited successfully but did not modify the target
file.

Codex 0.135 source shows apply-patch editing is commonly represented as a
Responses `custom_tool_call` item named `apply_patch`, followed by a
`custom_tool_call_output` input item on the next turn. The current local
Responses route only handles message, `function_call`, and
`function_call_output` items. The Codex provider adapter also rejects streamed
or non-streamed upstream `custom_tool_call` output as an unsupported tool event.

That means ilonasin can pass text and flat function-call turns, but it cannot
faithfully relay the Codex custom tool protocol needed for normal workspace
edits.

## Goal

Preserve the Codex Responses custom-tool subset needed for apply-patch style
workspace edits while keeping ilonasin stateless and metadata-only.

This slice should move the local Codex path closer to Codex wire behavior. It
does not implement the tool locally; it relays the upstream Codex
`custom_tool_call` item to the Codex CLI client and accepts the client's
`custom_tool_call_output` follow-up for outbound Codex routing.

## Scope

1. Extend local Responses decoding for safe Codex custom tool transcript items.
   - Accept `custom_tool_call` input items with `call_id`, `name`, and string
     `input`.
   - Accept `custom_tool_call_output` input items with `call_id`, optional
     `name`, and string `output`.
   - Reject structured custom tool outputs in this slice. Structured outputs can
     contain arbitrary content items and need separate design.
   - Keep these item types unsupported for non-Codex provider conversion.
   - Keep validation errors generic and never echo custom tool input/output.
   - Validate transcript ordering for both `function_call` and
     `custom_tool_call` families: duplicate calls, duplicate outputs, orphan
     outputs, missing outputs before later messages, extra fields, malformed
     string fields, and structured custom outputs must fail locally before
     upstream dispatch.
2. Preserve raw local Responses input only for Codex provider routing.
   - Add an internal, non-JSON `CodexResponsesInput` field to the common chat
     request type.
   - Populate it from already-validated raw local Responses input when the
     requested provider type is `codex`.
   - In `marshalCodexResponsesRequest`, use preserved raw input for the Codex
     local Responses route when available.
   - Make this an explicit Codex-only bypass of Chat-message transcript
     conversion. The generic Chat conversion path should continue to reject
     custom tool items for DeepSeek/OpenRouter.
   - Continue using typed chat messages for normal Chat Completions callers.
3. Preserve upstream Codex custom tool calls for local Responses SSE.
   - Add an internal, non-JSON side channel for Responses-only output items on
     the provider result path. Consume it only in the local Responses route.
     Chat Completions routes must not expose or silently flatten these items.
   - Parse upstream Codex `response.output_item.done` items of type
     `custom_tool_call`.
   - Parse upstream Codex `response.output_item.added` items of type
     `custom_tool_call` and aggregate `response.custom_tool_call_input.delta`
     events by item ID or call ID until the final done item arrives.
   - If the final done item omits `input`, use the aggregated deltas. If both
     exist and conflict, fail with `upstream_invalid_response`.
   - Represent the parsed item in memory as allowlisted fields only:
     `type`, `call_id`, `name`, and `input`.
   - Extend the local Responses SSE writer to emit preserved Codex output items
     before `response.completed`.
   - Do not convert `custom_tool_call` into Chat Completions tool calls.
     Chat Completions callers still receive a clear unsupported/invalid
     response for unsupported Codex tool families.
4. Add smoke coverage without permanent tests.
   - Extend `serve --check` with a fake Codex upstream that returns an
     `apply_patch` `custom_tool_call`.
   - Include both final-only and added-plus-delta custom tool call wire shapes.
   - Assert the local `/v1/responses` SSE includes that custom tool item.
   - Send a follow-up local Responses request containing the
     `custom_tool_call_output`.
   - Assert fake upstream receives `custom_tool_call_output`, not
     `function_call_output`.
   - Assert DeepSeek/OpenRouter reject custom tool transcript items locally
     before upstream and without echoing custom input/output.
   - Assert malformed custom transcript items, structured outputs,
     duplicate/orphan call IDs, and missing outputs fail before upstream.
   - Assert sentinel strings from custom tool input/output do not appear in
     metadata or logs.
5. Run live Codex workspace-edit smoke if feasible.
   - Use real credentials in `~/.ilonasin`.
   - Use a temporary `CODEX_HOME`, temporary workspace, temporary logs, and a
     cleanup trap.
   - Create a fresh local client token and disable it during cleanup.
   - Prompt `codex exec` to make a small file edit.
   - Verify by reading the temporary workspace file.
   - Record only safe outcome fields in docs.
6. Update compatibility docs.
   - Update `docs/codex-client-red-team.md`.
   - Update `docs/codex-compatibility-audit.md`.
   - Distinguish "custom apply-patch relay works" from broader hosted,
     namespaced, MCP, shell, and tool-search parity.

## Non-Goals

- Do not implement local execution of apply-patch, shell, MCP, tool-search,
  web-search, or image-generation tools.
- Do not store Responses state or use `previous_response_id`.
- Do not persist raw custom tool input, raw custom tool output, prompts,
  completions, request bodies, response bodies, provider payloads, raw SSE
  chunks, bearer tokens, account IDs, or request IDs.
- Do not add permanent `_test.go` files.
- Do not change quota pooling behavior.
- Do not touch OAuth refresh-classification behavior in this slice.

## Acceptance

- `find . -name '*_test.go' -type f -print` prints nothing.
- `go test ./...` passes as a compile/package check.
- `go vet ./...` passes.
- A fresh binary builds.
- `serve --check` passes and covers the new custom-tool relay.
- `serve --check` proves non-Codex custom transcript requests and malformed
  custom transcript requests fail before upstream without leaking sentinels.
- `manage --check` is run. If it still fails only on the existing OAuth summary
  expectation, record that as unrelated and do not fold an OAuth fix into this
  slice.
- `git diff --check` passes.
- Fake and live smoke privacy scans show no sentinel hits in ilonasin logs or
  metadata.
- The compatibility docs reflect the exact new result.
