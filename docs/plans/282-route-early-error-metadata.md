# 282 Route Early Error Metadata

## Goal

Close the metadata-only observability gap for strict route failures that occur
after a request has been safely decoded.

The architecture expects request metadata to capture HTTP status and normalized
error class without storing prompts, completions, bodies, raw provider payloads,
tool arguments, or raw stream chunks. Current Chat, Responses, and Anthropic
routes record some early failures, but miss decoded validation and adapter
validation failures.

## Scope

1. Add safe early metadata recording for decoded Chat Completions failures:
   - `req.Validate()` failures;
   - invalid model address after decode;
   - provider not configured after decode;
   - provider adapter request-validation failures.
2. Add safe early metadata recording for decoded Responses failures:
   - invalid model address after decode;
   - provider not configured after decode;
   - Responses-to-Chat conversion failures;
   - converted Chat validation failures;
   - provider adapter request-validation failures.
3. Add safe early metadata recording for decoded Anthropic Messages failures:
   - invalid model address after decode;
   - provider not configured after decode;
   - Anthropic-to-Chat conversion failures;
   - converted Chat validation failures;
   - provider adapter request-validation failures.
4. Preserve invalid JSON/body-read failures as log-only in this slice because no
   safe request shape exists yet.
5. Preserve all current response statuses, error envelopes, and route ordering.
6. Introduce small safe early-recording helpers where existing metadata base
   helpers require a parsed address or configured provider. These helpers must
   fill only decoded-safe fields such as endpoint, local client token ID,
   stream flag, counts/options, status, error class, and latency. Model,
   provider, and resolved fields stay blank until safely resolved.
7. Expected source touch points are the three route files plus a narrowly scoped
   shared metadata helper if duplication would otherwise grow.

## Boundaries

- No provider, routing, credential, TUI, management, storage schema, config, IO
  logging, or public API shape changes.
- No raw prompts, completions, request bodies, response bodies, raw provider
  payloads, tool arguments, raw stream chunks, full account IDs, full request
  IDs, or bearer tokens in metadata.
- Do not record invalid JSON/body-read failures in this slice.
- No permanent tests.

## Verification

Run:

```sh
find . -name '*_test.go' -type f -print
git diff --check
go test ./...
go vet ./...
```

Run a temporary focused route smoke, then remove it before commit:

- instantiate a server with fake registry, local token verifier, metadata
  recorder, and adapters;
- send authenticated decoded requests that fail at:
  - Chat validation;
  - Chat invalid model;
  - Chat provider not configured;
  - Chat adapter validation;
  - Responses invalid model;
  - Responses provider not configured;
  - Responses conversion/validation;
  - Responses adapter validation;
  - Anthropic invalid model;
  - Anthropic provider not configured;
  - Anthropic conversion/validation;
  - Anthropic adapter validation;
- send an authenticated invalid JSON request and assert no metadata row is
  recorded;
- assert each response status and error envelope remains unchanged;
- assert one metadata row is recorded with the expected endpoint, status, and
  normalized error class;
- assert metadata contains only safe count/option/status fields before model
  resolution and no raw request text marker.

Run disposable daemon smokes:

1. Build a temporary `ilonasin` binary.
2. Start `serve` with temporary `ILONASIN_HOME`, temporary SQLite, IO capture
   disabled, keepalive disabled, and at least two provider instances.
3. Verify management health over the management socket.
4. Run `manage` under a short timeout and verify API, providers, usage, and
   logs chrome renders.
5. Remove all temporary artifacts.

## Acceptance

- Decoded early failures for Chat, Responses, and Anthropic Messages produce
  metadata-only request rows.
- Invalid JSON/body-read failures remain unchanged.
- Existing response statuses, response envelopes, and route ordering are
  preserved.
- Temporary smoke proves representative early failures record safe metadata and
  do not persist raw marker text.
- No permanent test files remain.
- Compile, vet, serve smoke, manage smoke, senior plan review, and senior
  implementation review pass.
