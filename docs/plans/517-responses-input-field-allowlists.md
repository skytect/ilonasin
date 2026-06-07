# 517 Responses Input Field Allowlists

## Context

Plan 516 found that known Responses `input[]` item parsers validate required
fields but do not reject extra top-level fields. On Codex routes, known
non-message and non-function-call input families can be preserved and forwarded
as original raw items through `CodexResponsesInput`.

`docs/ilonasin-architecture.md` requires unsupported or unknown compatibility
fields to fail locally before provider dispatch. Extra input item fields are not
an explicit namespaced provider escape hatch.

## Goal

Reject unknown top-level fields on known Responses `input[]` item families
before Codex raw preservation or non-Codex conversion can dispatch the request.

## Scope

1. Update `internal/openai/responses.go`.
2. Add per-family top-level field allowlists for:
   - `message`: `type`, `role`, `content`, `id`, `status`;
   - `function_call`: `type`, `id`, `call_id`, `name`, `namespace`,
     `arguments`;
   - `function_call_output`: `type`, `call_id`, `output`;
   - `tool_search_call`: `type`, `id`, `call_id`, `execution`, `status`,
     `arguments`;
   - `tool_search_output`: `type`, `call_id`, `status`, `execution`, `tools`;
   - `custom_tool_call`: `type`, `id`, `call_id`, `name`, `input`;
   - `custom_tool_call_output`: `type`, `call_id`, `name`, `output`.
3. Preserve existing accepted fields and validations:
   - normalized message items with `type`, `role`, and `content`;
   - message `id` and `status`, which are currently tolerated; explicit typed
     message items strip them before Codex message preservation, while
     normalized messages without an explicit `type` preserve the original raw
     item shape;
   - optional `id`, `namespace`, `status`, `arguments`, `tools`, and `name`
     where currently accepted by the specific families above;
   - required call IDs, names, execution values, output parsing, function name
     validation, call-pair validation, and content item allowlists.
4. Return clear local errors shaped as `input[n].<field> is unsupported`.
5. Preserve Codex raw input preservation for known validated families after
   allowlist validation.
6. Preserve provider adapters, route policy, storage, management APIs, TUI,
   config, logging, IO logging, and SQLite behavior.
7. Do not add permanent tests.

## Out Of Scope

- Adding support for new Responses item families or extra fields.
- Rebuilding Codex preserved items from typed structs instead of validated raw
  input.
- Changing Responses content item allowlists.
- Changing Responses tool declaration validation.
- Changing provider, server, management, TUI, config, storage, logging, or IO
  logging behavior.

## Verification

Use a temporary focused harness, then remove it before commit, to verify:

- each known input item family rejects an unknown extra top-level field with
  `input[n].<field> is unsupported`;
- normalized message input without explicit `type` still decodes;
- representative valid item families still decode;
- Codex-style conversion still preserves known validated raw input;
- non-Codex conversion behavior for representable and unrepresentable known
  families is unchanged.

Run:

```sh
rg -n 'parseResponses.*Item|firstUnsupportedRawField|input\\[%d\\]\\.%s is unsupported|CodexResponsesInput' internal/openai/responses.go docs/plans/517-responses-input-field-allowlists.md
gofmt -w internal/openai/responses.go
git diff --check
git diff --no-index --check "$tmpempty" docs/plans/517-responses-input-field-allowlists.md
find . -name '*_test.go' -type f -print
go test ./...
go vet ./...
```

Run direct CLI smoke:

1. Build a temporary `ilonasin` binary.
2. Start `ilonasin serve` with isolated `ILONASIN_HOME`, temporary config,
   temporary SQLite, IO capture disabled, keepalive disabled, and configured
   provider instances.
3. Verify management health and snapshot over the Unix management socket.
4. Run bounded `ilonasin manage` at 80 and 140 columns under a
   pseudo-terminal.
5. Confirm TUI output includes ANSI color sequences.
6. Remove all temporary files and terminate the daemon.

## Acceptance

- Unknown top-level fields on known Responses input item families are rejected
  locally before provider dispatch.
- Existing supported fields and known-family behavior are preserved.
- Codex raw input preservation remains limited to known validated fields and
  families.
- No permanent tests are added.
- Compile, vet, serve smoke, manage smoke, senior plan review, and senior
  implementation review pass.

## Implementation Record

- Updated `internal/openai/responses.go` only.
- Added `rejectUnsupportedResponsesInputFields` as a shared item-level
  allowlist helper.
- Added allowlist checks to all known Responses input item family parsers.
- Preserved current accepted fields, including message `id` and `status`.
  Explicit typed message items continue to strip those fields before Codex
  message preservation, while normalized messages without an explicit `type`
  continue to preserve the original raw item shape.
- Preserved existing required-field, type, call-pair, content, and conversion
  validation.

## Verification Record

- Senior plan review: two reviewers reported no findings; one reviewer found a
  low-risk plan ambiguity around exact per-family allowed fields. The plan was
  updated to list each family allowlist explicitly, including message `id` and
  `status`.
- Temporary focused harness: passed. It verified extra-field rejection for
  `message`, `function_call`, `function_call_output`, `tool_search_call`,
  `tool_search_output`, `custom_tool_call`, and `custom_tool_call_output`;
  verified normalized message decode with accepted `id` and `status`; verified
  Codex-style raw preservation for known validated input; and verified
  non-Codex rejection of an unrepresentable known family. Temporary harness was
  removed before commit.
- `rg -n 'parseResponses.*Item|firstUnsupportedRawField|rejectUnsupportedResponsesInputFields|CodexResponsesInput' internal/openai/responses.go docs/plans/517-responses-input-field-allowlists.md`:
  passed.
- `gofmt -w internal/openai/responses.go`: passed.
- `git diff --check`: passed.
- `git diff --no-index --check "$tmpempty" docs/plans/517-responses-input-field-allowlists.md`:
  passed for the new untracked plan file. Git returned status `1` only because
  the files differ, with no whitespace findings.
- `find . -name '*_test.go' -type f -print`: passed, no files found.
- `go test ./...`: passed as a compile/package check; all packages reported no
  test files.
- `go vet ./...`: passed.
- Temporary `go build -o "$tmpbin/ilonasin" ./cmd/ilonasin`: passed.
- `ilonasin serve` smoke: passed with isolated `ILONASIN_HOME`, temporary
  config, free local bind port, IO capture disabled, keepalive disabled, and
  management health plus snapshot checked over the Unix socket.
- `ilonasin manage` smoke: passed at 80 and 140 columns under a
  pseudo-terminal. Both bounded runs exited by timeout with status `124` as
  expected.
- TUI color capture: passed. The 80-column capture contained 108 256-color SGR
  foreground sequences, and the 140-column capture contained 175.
- Cleanup: temporary home, binary, config, terminal captures, temporary harness,
  and daemon process were removed.
- Senior implementation review: two reviewers reported no findings; one
  reviewer found a low-risk plan wording issue around explicit typed messages
  versus normalized messages. The plan text was corrected without changing
  runtime behavior.
