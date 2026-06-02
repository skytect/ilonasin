# 284 Route Address Metadata Sanitizer

## Goal

Prevent client-controlled provider/model address strings from reaching durable
request metadata when they contain secret-shaped, account-shaped, request-ID, or
raw-payload markers.

The architecture allows metadata-only requested and resolved model/provider
fields, but forbids durable bearer tokens, local tokens, upstream API keys,
OAuth tokens, full account IDs, full request IDs, raw payload markers, prompts,
completions, bodies, tool arguments, tool results, and raw SSE chunks.

Three fresh whole-codebase reviews identified the same gap: normal Chat,
Responses, Anthropic Messages, and especially Anthropic Count Tokens still copy
parsed model address fields directly into `request_metadata`. Management and
TUI snapshot sanitization protects rendered output, but it does not protect the
durable SQLite ledger.

## Scope

1. Add a shared server-side metadata address sanitizer for:
   - requested provider instance;
   - requested model;
   - resolved provider instance;
   - resolved model.
2. Preserve normal explicit model IDs, including OpenRouter-style slash and
   suffix model IDs such as `deepseek/deepseek-v4-pro` and
   `deepseek/deepseek-v4-flash:free`.
3. Omit unsafe marker-shaped values before persistence. Unsafe markers include
   secret/token/bearer/oauth words, `sk-`, `iln_`, account markers, request ID
   markers, raw/payload/body/prompt/completion markers, tool argument/result
   markers, and JWT-like values.
4. Apply the sanitizer at the final server metadata recording boundary before
   `RecordRequestMetadata`, so future or missed route helpers cannot bypass it.
5. Apply the sanitizer consistently to existing construction/finalization sites
   where address fields are assigned:
   - Chat metadata base construction;
   - Responses metadata base construction;
   - Anthropic Messages configured-provider early/final metadata paths;
   - Anthropic Count Tokens success, invalid-model, provider-not-configured,
     and invalid-json metadata paths;
   - chat metadata finalization when an upstream resolved model is returned.
6. Keep the decoded early-error behavior from slice 282 unchanged:
   pre-address and unconfigured-provider rows still omit provider/model fields
   even when the parsed address looks safe, because no configured provider has
   been resolved.
7. Keep route status codes, response envelopes, IO logging behavior, management
   DTO shape, storage schema, TUI layout, provider behavior, and routing behavior
   unchanged.

## Boundaries

- No routing validation change. Requests with unusual but syntactically valid
  model IDs should still route or fail exactly as before.
- No storage schema, management API, TUI, provider, config, or credential
  changes.
- Do not sanitize or alter outbound upstream model IDs. This slice only affects
  metadata values before they are recorded.
- No permanent tests.

## Verification

Run:

```sh
find . -name '*_test.go' -type f -print
git diff --check
go test ./...
go vet ./...
```

Run temporary focused smokes, then remove them before commit:

- direct sanitizer smoke:
  - preserves safe values such as `codex`, `gpt-5.5`,
    `deepseek/deepseek-v4-pro`, `openrouter/openai/gpt-4o-mini`, and
    `deepseek/deepseek-v4-flash:free`, and `openrouter`;
  - preserves safe slugs that contain ordinary words only when they are not
    marker-shaped secrets, such as a normal `completion-model` or
    `prompt-tuned-model` slug if accepted by the sanitizer design;
  - omits marker-shaped values such as `sk-secret`, `iln_token`,
    `acct_123`, `request_id_123`, `raw_body`, `tool_argument`, and JWT-shaped
    strings.
- route metadata smoke with an in-process server and fake metadata recorder:
  - Chat configured-provider adapter failure with unsafe model segment records
    no unsafe model/provider marker;
  - Responses configured-provider conversion/adapter failure with unsafe model
    segment records no unsafe marker;
  - Anthropic Messages configured-provider conversion/adapter failure with
    unsafe model segment records no unsafe marker;
  - Anthropic Messages final success or credential-unavailable path that uses
    `nonStreamContext.clientModel` records no unsafe marker;
  - Anthropic Count Tokens invalid model, provider-not-configured, and success
    cases record safe endpoint/status/count metadata and no unsafe marker;
  - provider-not-configured rows omit requested/resolved provider/model fields
    even when the parsed model address is otherwise safe;
  - safe model IDs still record requested and resolved model/provider metadata.
- SQLite smoke:
  - record a request metadata row containing unsafe address markers through the
    normal server recorder path and assert persisted summaries do not expose the
    unsafe markers;
  - assert the raw `request_metadata` columns `requested_provider_instance`,
    `requested_model`, `resolved_provider_instance`, and `resolved_model` do not
    contain unsafe markers after recording;
  - prove direct `s.record`/`s.recordWithID` callers are covered by the final
    record-boundary sanitizer.

Run disposable daemon smokes:

1. Build a temporary `ilonasin` binary.
2. Start `serve` with temporary `ILONASIN_HOME`, temporary SQLite, IO capture
   disabled, keepalive disabled, and at least two provider instances.
3. Verify management health over the management socket.
4. Run `manage` under a short timeout and verify API, providers, usage, and
   logs chrome renders.
5. Remove all temporary artifacts.

## Acceptance

- Durable request metadata cannot store unsafe marker-shaped provider/model
  address values.
- Safe provider/model identifiers still appear in metadata.
- Routing, response shapes, IO logging policy, management DTOs, TUI layout, and
  storage schema are unchanged.
- Temporary smokes prove Chat, Responses, Anthropic Messages, and Anthropic
  Count Tokens metadata paths are covered.
- Compile, vet, serve smoke, manage smoke, senior plan review, and senior
  implementation review pass.
