# 361 Pooling Affinity Signal Grounding

1. Ground the current pooling affinity contract in the architecture docs.
   - Add a concise source-backed table under Credential Pooling describing what
     Codex Responses, Claude Code Anthropic, generic OpenAI Chat, generic
     Responses, and anonymous/minimal clients actually send.
   - Make `prompt_cache_key` the preferred Responses signal when present, with
     body metadata and safe session headers as fallbacks.
   - State that when clients send only model plus a local API key, ilonasin must
     rely on local token identity, provider/model route, least-in-flight
     pressure, and round-robin tie breaking.

2. Keep implementation scope narrow.
   - Do not add unsupported top-level fields.
   - Do not change current pooling selection unless the docs reveal a concrete
     mismatch.
   - Add only a small code comment if needed to make the signal order easier to
     maintain.

3. Verify.
   - Run `git diff --check`.
   - Run `go test ./...` as compile/package check only.
   - Run `go vet ./...`.
