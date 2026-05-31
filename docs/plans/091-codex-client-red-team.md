# 091 codex client red team

## Goal

Red-team `ilonasin` as a local backend for the Codex CLI non-interactive
client, using `codex exec` as the Codex equivalent of `claude -p`.

The output of this slice is an evidence-backed compatibility map: what works,
what fails, what leaks or misroutes, and which fixes are needed before Codex can
reliably use `ilonasin` as its base URL.

## Ground Truth

- `codex exec` is the non-interactive CLI entry point.
- The installed Codex CLI supports config overrides with `-c key=value`,
  profile layering, model selection, image attachments with `--image`, sandbox
  selection, and JSON event output.
- Current Codex model providers support `wire_api = "responses"` only.
  `wire_api = "chat"` is rejected by Codex itself.
- Codex appends relative endpoint paths to the configured provider `base_url`.
  A base URL without `/v1` targets `/responses`; a base URL with `/v1` targets
  `/v1/responses`.
- `docs/ilonasin-architecture.md` requires a strict OpenAI-compatible local
  API, provider-specific adapters, no raw prompt/completion/body/payload
  persistence, daemon-owned metadata, and provider/model routing by local model
  strings such as `codex/gpt-5.5`.
- Plan `090` added local support for multimodal chat content, Codex reasoning
  effort, verbosity, fast service tier, and function-tool translation, but that
  was smoke coverage through direct HTTP requests, not through the real Codex
  CLI client.
- Codex itself may use Responses API shapes, agent tool protocols, model
  provider config, session state, approval and sandbox fields, and image
  attachment paths that differ from plain chat-completions smoke requests.
- Current `ilonasin` exposes OpenAI-style `GET /v1/models` and
  `POST /v1/chat/completions`. It does not currently expose a local
  `POST /v1/responses`, bare `POST /responses`, `/files`, or Codex-compatible
  `{ "models": [...] }` model catalog.

## In Scope

1. Discover the Codex CLI provider configuration surface.
   - Use `codex --help`, `codex exec --help`, and local config inspection.
   - Use a mandatory temporary `CODEX_HOME` with mode `700`.
   - Run `codex exec --ephemeral --ignore-user-config` with explicit `-c`
     overrides.
   - Configure a custom provider id such as `ilonasin`.
   - Use `model_providers.ilonasin.env_key = "ILONASIN_CLIENT_TOKEN"` and an
     environment variable for auth. Do not use `experimental_bearer_token` or
     literal token values in command-line config.
   - Test both base URL suffixes:
     - `http://127.0.0.1:<port>`
     - `http://127.0.0.1:<port>/v1`
   - Do not print tokens, refresh tokens, account IDs, raw prompts, raw
     completions, or provider payloads.

2. Run a dumb local HTTP-recorder preflight.
   - Before starting `ilonasin`, point Codex at a local recorder.
   - Capture only allowlisted derived metadata:
     - endpoint family: `models`, `responses`, `files`, or `other`,
     - method,
     - status,
     - auth header presence,
     - provider config variant,
     - pass/fail.
   - Do not store request bodies, response bodies, raw paths with query strings,
     headers other than auth presence, prompts, completions, tool arguments,
     tool results, file IDs, callback URLs, or image data.
   - Use the preflight to prove whether Codex can target a local base URL
     independently of `ilonasin` API compatibility.

3. Start `ilonasin serve` in an isolated smoke setup.
   - Build the current worktree binary.
   - Use a temporary `ILONASIN_HOME` with mode `700`.
   - Prefer disposable credentials loaded through supported CLI or management
     APIs. Do not copy logs, cache, request metadata, WAL/SHM files, or normal
     production account state from `~/.ilonasin`.
   - Create a local client token through supported CLI or management APIs.
   - Run `ilonasin serve` in the background and track its PID.
   - Use a concrete cleanup trap that kills the daemon, waits for it, and
     removes temp homes, binaries, workspaces, and captures.

4. Run Codex CLI probes against `ilonasin`.
   - Text-only simple prompt.
   - Image prompt using `codex exec --image`.
   - A task that should require tool calls, such as reading a file in `/tmp`,
     listing files, or computing from local inputs.
   - Different reasoning efforts where Codex config or CLI supports them.
   - Fast or low-latency mode where Codex config or CLI supports it.
   - JSON event mode only when immediately reduced to allowlisted derived
     fields. Do not save raw JSONL or terminal transcripts.

5. Cover provider variety through `ilonasin`.
   - Try Codex-provider routing first, because this is closest to the real
     Codex binary behavior.
   - Try OpenRouter and DeepSeek routes when the Codex CLI can be configured
     to send compatible OpenAI-style requests through `ilonasin`.
   - Record whether failures are Codex CLI config limitations, local API
     shape mismatches, provider-adapter rejections, or upstream provider
     limitations.

6. Capture failure modes.
   - Status code and safe error class.
   - Endpoint family used by Codex, without raw query strings, file IDs, hosts,
     callback URLs, or route parameters.
   - Whether the failure occurs before reaching `ilonasin`, at local routing,
     at request validation, at provider translation, or at upstream response
     translation.
   - Whether any local metadata, logs, or CLI output leak forbidden data.
   - Whether the request shape depends on Responses API, chat completions,
     tools, images, reasoning effort, sandbox approvals, or stream handling.
   - Classify each failure as:
     - endpoint mismatch,
     - model metadata mismatch,
     - request translation mismatch,
     - upstream provider limitation,
     - Codex CLI configuration limitation.

7. Define the compatibility map as allowlisted metadata only.
   - Feature: text, image, tool, reasoning effort, fast mode, model discovery.
   - Provider route: `codex`, `openrouter`, `deepseek`, or recorder.
   - Base URL variant: bare or `/v1`.
   - Endpoint family.
   - Reached local server: yes/no.
   - Result class.
   - Safe status and safe local error class.
   - Required next fix.
   - Do not include raw Codex traces, raw stdout JSONL, raw prompts, model
     output, request bodies, response bodies, provider payloads, image bytes,
     tool arguments, tool results, tokens, account IDs, or raw paths.

8. Run leak checks after probes.
   - Search captured stdout/stderr summaries, temporary `CODEX_HOME`,
     temporary `ILONASIN_HOME/logs`, and SQLite metadata for sentinel prompt
     markers, completion markers, image filename/content markers, tool argument
     markers, bearer-token prefixes, account IDs, raw request bodies, raw
     response bodies, and provider payload markers.

9. Fix only small, clearly justified defects found during probing.
   - A fix is allowed only if the failure is local, narrow, and architecture
     aligned.
   - Larger compatibility gaps should be documented for the next numbered plan.

## Out of Scope

- Quota tracking or quota pooling.
- Any overlap with the separate quota-tracking worktree.
- Reworking account pooling.
- Broad provider refactors without direct evidence from Codex CLI probes.
- Persisting raw Codex traces, raw provider payloads, prompts, completions,
  images, tool arguments, or tool results.
- Mutating the user's normal Codex config or normal `~/.ilonasin` state.
- Saving raw Codex JSONL, HTTP request bodies, HTTP response bodies, terminal
  transcripts, or image bytes.
- Treating direct Chat Completions success as proof that `codex exec` works.

## Implementation Steps

1. Replace the old quota-oriented `091` plan with this plan.
2. Get three senior plan reviews:
   - Codex CLI/provider configuration review.
   - `ilonasin` local API and adapter compatibility review.
   - Privacy, logging, and smoke isolation review.
3. Run CLI discovery commands and summarize the provider configuration knobs.
4. Run the recorder preflight against bare and `/v1` base URL variants.
5. Build and start `ilonasin serve` from this worktree in `/tmp`.
6. Configure a temporary Codex home and explicit `-c` overrides for the local
   `ilonasin` base URL and client token through `ILONASIN_CLIENT_TOKEN`.
7. Run the probe matrix and collect safe evidence.
8. Apply any narrow local fixes that are directly supported by the evidence.
9. Re-run relevant probes and direct smoke checks:

```sh
find . -name '*_test.go' -type f -print
go test ./...
go vet ./...
tmpbin=$(mktemp -d)
tmp=$(mktemp -d)
go build -o "$tmpbin/ilonasin" ./cmd/ilonasin
ILONASIN_HOME="$tmp" "$tmpbin/ilonasin" serve --check
ILONASIN_HOME="$tmp" "$tmpbin/ilonasin" manage --check
rm -rf "$tmp" "$tmpbin"
```

10. Get three senior code or findings reviews.
11. Commit the plan plus any findings or code changes with a
    `Co-Authored-By` line.

## Findings

The red-team results are recorded in `docs/codex-client-red-team.md`.

The core finding is that Codex CLI can target a local custom provider with
environment-variable auth, but it sends Responses API traffic. Current
`ilonasin` exposes Chat Completions, so every useful `codex exec` probe is
blocked before inference by the missing local Responses API surface.

The next numbered plan should implement local Responses API compatibility. Do
not overlap with quota tracking work in the separate quota-tracking worktree.

## Review Questions for Subagents

1. Is this probe matrix sufficient to reveal real Codex CLI compatibility
   failures instead of only direct HTTP smoke behavior?
2. Does the plan avoid mutating or leaking the user's normal Codex and
   `ilonasin` credential state?
3. Are the allowed fixes scoped tightly enough for a red-team slice?
