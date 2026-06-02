# 281 Codex Chat Reasoning Audit Alignment

## Goal

Align the compatibility audit row for direct Codex Chat reasoning with current
code evidence.

`docs/codex-compatibility-audit.md` currently says direct Codex Chat reasoning
is code-supported through `provider_options.codex.reasoning`, but needs separate
verification. Current code branches Codex Chat requests through the Codex
Responses request builder, where provider-options reasoning is represented as
upstream `reasoning` plus `include: ["reasoning.encrypted_content"]`.

## Scope

1. Add a temporary focused provider smoke, run it, and remove it before commit.
   It must prove:
   - OpenAI Chat request decoding and validation accept
     `provider_options.codex.reasoning`;
   - provider validation accepts Codex reasoning provider options;
   - `marshalCodexResponsesRequest` serializes reasoning `effort` and `summary`
     into the upstream Codex Responses body;
   - a locally allowed but model-unsupported reasoning effort maps through
     `codexResponsesModel.reasoningEffort` to a supported model effort;
   - the upstream body includes `reasoning.encrypted_content` only when
     reasoning is present;
   - unsupported top-level Chat reasoning remains rejected by request decoding;
   - unsupported Codex reasoning values remain rejected locally.
2. Update `docs/codex-compatibility-audit.md` direct Codex Chat reasoning row to
   reflect focused local validation and request-shaping evidence.
3. Do not change production code unless the temporary smoke exposes a real
   regression.

## Boundaries

- Documentation-only final diff unless the focused smoke exposes a code
  regression.
- No management API, DTO, storage, schema, TUI, provider credential, OAuth,
  model-discovery, logging policy, routing, config, or live credential changes.
- Do not claim live Codex Chat reasoning provider success, broad Codex switch
  readiness, or broader Codex tool-family parity.
- Do not move reasoning to a top-level Chat field; it stays in
  `provider_options.codex.reasoning`.
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

- decode a Chat Completions request with
  `provider_options.codex.reasoning.effort` and `summary`;
- assert request validation and provider validation succeed;
- marshal through `marshalCodexResponsesRequest` with a model that supports
  known reasoning efforts;
- assert upstream JSON contains `reasoning.effort`, `reasoning.summary`, and
  `include: ["reasoning.encrypted_content"]`;
- assert a locally allowed but model-unsupported effort serializes as the
  model-supported fallback effort;
- assert a request without reasoning omits both upstream `reasoning` and the
  encrypted-content include;
- assert an unsupported top-level `reasoning` field fails decoding as an
  unsupported Chat field;
- assert unsupported Codex reasoning values fail local validation.
- run it with:

```sh
go test ./internal/provider -run TestTmpCodexChatReasoningAudit -count=1
```

After removing the temporary smoke file, run
`find . -name '*_test.go' -type f -print` again and confirm no permanent test
files remain.

Run disposable daemon smokes:

1. Build a temporary `ilonasin` binary.
2. Start `serve` with temporary `ILONASIN_HOME`, temporary SQLite, IO capture
   disabled, keepalive disabled, and at least two provider instances.
3. Verify management health over the management socket.
4. Run `manage` under a short timeout and verify API, providers, usage, and
   logs chrome renders.
5. Remove all temporary artifacts.

## Acceptance

- Temporary smoke proves direct Codex Chat reasoning provider-options
  validation and Codex Responses request shaping.
- `docs/codex-compatibility-audit.md` no longer marks direct Codex Chat
  reasoning as needing separate local verification.
- The audit still distinguishes local request shaping from live provider
  success and broad switch-gate readiness.
- Final diff is docs-only, with no temporary smoke file left, unless a genuine
  code regression is fixed.
- No permanent test files remain.
- Compile, vet, serve smoke, manage smoke, senior plan review, and senior
  implementation review pass.
