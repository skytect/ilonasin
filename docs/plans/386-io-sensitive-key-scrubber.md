# 386 IO Sensitive Key Scrubber

## Context

`docs/ilonasin-architecture.md` allows `[logging].capture_io = true` as a
local debugging exception, but still forbids persisting bearer tokens, local
client tokens, upstream API keys, OAuth tokens, cookies, authorization codes,
device codes, code verifiers, provider command stdout, configured credential
secret values, full provider request IDs, and full account IDs.

Current IO body scrubbing in `internal/logging/io.go` redacts configured secret
values and JSON/form/header keys recognized by `IsCredentialKey`. Recent work
added affinity metadata keys there, but the key policy still misses documented
non-token sensitive fields such as account IDs, request IDs, balances, credits,
provider command stdout, raw payload/body markers, and tool argument/result
markers.

## Goal

Make IO capture key redaction match the documented local debugging privacy
boundary without changing normal structured logging behavior or route logic.

## Scope

1. Split logging key classification into:
   - credential/secret keys that should stay narrowly credential-focused;
   - IO-sensitive keys that also include documented non-token privacy markers.
2. Move IO-only affinity keys added by slice 385 out of `IsCredentialKey`:
   - `metadata`;
   - `client_metadata`;
   - `session_id`;
   - `thread_id`;
   - `conversation_id`.
   These keys should be redacted by the IO-sensitive classifier in IO capture,
   but should not be treated as credential keys or ordinary structured-log
   sensitive keys by themselves.
3. Use the broader IO-sensitive key classifier in:
   - JSON IO body scrubbing;
   - form body scrubbing;
   - header-line scrubbing;
   - key/value marker scrubbing.
4. Include these IO-sensitive marker families:
   - full account IDs and account UUIDs;
   - request IDs and request-id-shaped names;
   - balances, credits, and billing-like totals;
   - raw payload/body, prompt/completion body, and response body markers;
   - provider command stdout;
   - tool argument and tool result markers, including context-aware JSON
     redaction for tool-shaped `arguments`, `input`, `output`, and
     `tool_search_output.tools`, `tool_result.content`, and OpenAI Chat
     `role: "tool"` `content` fields without redacting top-level Responses
     `input` or ordinary message content;
   - SSE chunk markers;
   - existing affinity/session metadata keys.
5. Keep `IsSensitiveLogKey` conservative for structured logs by continuing to
   redact credential keys and explicit raw/body/payload/stdout attributes.
6. Do not add durable test files. Use temporary direct smokes for the scrubber.

## Out Of Scope

- No route, provider adapter, request parsing, storage, management API, TUI, or
  config changes.
- No change to whether `capture_io` records request/response bodies when it is
  explicitly enabled.
- No attempt to redact all possible IDs, timestamps, safe labels, model IDs, or
  provider instance IDs.
- No broad rewrite of normal logging or metadata sanitizers.

## Verification

Run:

```sh
rg -n "IsCredentialKey|IsIOSensitiveKey|ScrubIOBody|capture_io|account|request" internal/logging docs/ilonasin-architecture.md
git diff --check
find . -name '*_test.go' -type f -print
go test ./...
go vet ./...
```

Run a temporary direct scrubber smoke that proves JSON, form, header-line, and
free-text key/value IO bodies redact representative values for:

- `account_id`;
- `account_uuid`;
- `request_id`;
- `x-request-id`;
- `balance`;
- `credits`;
- `stdout`;
- `raw_body`;
- `tool_arguments`;
- tool-shaped JSON `arguments`, `input`, `output`, `tool_search_output.tools`,
  `tool_result.content`, and OpenAI Chat `role: "tool"` `content`;
- `sse_chunk`;
- `metadata.session_id`.
- flattened or dotted metadata/session/cache keys in form, header-line, and
  free-text key/value inputs, such as `metadata.session_id` and
  `metadata.prompt_cache_key`.

The same smoke should prove an ordinary non-sensitive field remains visible and
that top-level Responses `input` remains visible in IO capture while tool-shaped
`input` is redacted. It should also prove
that `metadata`, `client_metadata`, `session_id`, `thread_id`, and
`conversation_id` are not credential keys or normal structured-log sensitive
keys, while real credential keys remain protected. The smoke should
should clean up all temporary files.

Run direct CLI smokes by building a temporary binary, starting `ilonasin serve`
with an isolated temporary home and config, checking management health over the
Unix socket, running bounded `ilonasin manage` at narrow and wide terminal
widths, and cleaning up all temporary files and processes.

## Acceptance

- IO capture redacts documented non-token sensitive key families.
- Existing credential key redaction remains intact.
- Normal structured logging remains conservative: real credential keys and
  explicit raw/body/payload/stdout keys are redacted, while IO-only affinity
  keys are not treated as structured-log secrets by name alone.
- No new persistent tests or generated files remain.
- Compile/package checks, vet, direct scrubber smoke, and serve/manage smoke
  pass.
