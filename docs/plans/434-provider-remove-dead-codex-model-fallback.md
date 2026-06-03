# 434 Provider Remove Dead Codex Model Fallback

## Context

Plan 426 found an unreachable `case "codex"` branch inside
`normalizeModels` in `internal/provider/http_models.go`.

Current code returns through `normalizeCodexModels` before entering the generic
OpenAI-style model normalization path:

```go
if instance.Type == "codex" {
    return normalizeCodexModels(instance, body)
}
```

That makes the later generic switch branch for `case "codex"` dead policy.
Provider adapter behavior is easier to audit when Codex model discovery lives
only in the explicit Codex normalizer.

## Goal

Remove the unreachable generic Codex model capability fallback without changing
runtime model-discovery behavior.

## Scope

1. Update `internal/provider/http_models.go`.
2. Remove only the unreachable `case "codex"` branch from the generic
   `normalizeModels` switch.
3. Keep `normalizeCodexModels`, `codexCapabilityFlags`, Codex `/models` URL
   shaping, OpenRouter normalization, DeepSeek normalization, model cache
   conversion, management DTOs, server routes, logging, config, and TUI
   behavior unchanged.
4. Do not add permanent tests.

## Verification

Use temporary focused checks, then remove them before commit:

- Codex still calls `normalizeCodexModels` and reads Codex `models`/`slug`
  payloads;
- generic OpenAI-style payloads for Codex do not accidentally become accepted
  by the generic fallback path;
- DeepSeek generic model normalization still assigns the same static
  capability flags;
- OpenRouter generic model normalization still uses OpenRouter capability
  extraction.

Then run:

```sh
rg -n 'case "codex"|normalizeCodexModels|normalizeModels' internal/provider/http_models.go
git diff --check
find . -name '*_test.go' -type f -print
go test ./internal/provider
go test ./...
go vet ./...
```

Run direct CLI smokes by building a temporary binary, starting `ilonasin serve`
with an isolated temporary home and config, checking management health and
snapshot over the Unix socket, running bounded `ilonasin manage` at narrow and
wide terminal widths, and cleaning up all temporary files and processes.

## Acceptance

- Generic model normalization has no unreachable Codex branch.
- Codex model discovery remains explicit in `normalizeCodexModels`.
- Existing runtime behavior is unchanged.
