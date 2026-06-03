# 441 TUI API Provider Summary Visuals

## Context

The API and provider panes are functional but still have several chip-heavy
summary rows. The user asked for less plain text and more visual structure while
keeping the screen compact.

## Goal

Make API routes and provider runtime summaries more structured and visual
without changing management data, credentials, OAuth accounts, or pane layout.

## Scope

1. Update `internal/tui/control_sections.go` and
   `internal/tui/providers_instances.go`.
2. Render the API surface list as a compact route grid:
   - status;
   - API family;
   - route path, wrapped at narrow and wide widths rather than hidden;
   - capability list.
3. Add a compact provider runtime summary meter or structured row showing:
   - enabled provider count;
   - chat-capable provider count;
   - model-discovery-capable provider count;
   - API-key-capable provider count;
   - OAuth-capable provider count.
4. Preserve individual provider instance rows, model cache rows, upstream
   credential rows, OAuth/account rows, local token rows, key handling, pane
   layout, and scrolling.
5. Do not change management DTOs, storage, provider behavior, routes, config,
   auth, or logging policy.
6. Do not add permanent tests.

## Verification

Run:

```sh
gofmt -w internal/tui/control_sections.go internal/tui/providers_instances.go
git diff --check
find . -name '*_test.go' -type f -print
go test ./internal/tui
go test ./...
go vet ./...
```

Run a temporary focused render check, then remove it before commit:

- render API summary at narrow and wide widths;
- assert all three API surfaces and their route/capability metadata remain
  visible at narrow and wide widths;
- render provider runtime with providers covering chat, model discovery,
  API-key, and OAuth capabilities;
- assert the runtime summary shows provider, chat, model, key, and OAuth counts;
- assert individual provider instance rows and model cache summary rows still
  render;
- assert rendered lines fit target widths.

Run disposable daemon smokes:

1. Build a temporary `ilonasin` binary.
2. Start `ilonasin serve` with temporary `ILONASIN_HOME`, temporary SQLite, IO
   capture disabled, keepalive disabled, and configured DeepSeek/Codex provider
   instances.
3. Verify management health and snapshot over the management socket.
4. Run bounded `ilonasin manage` at narrow and wide terminal widths.
5. Remove all temporary files and terminate the daemon.

## Acceptance

- API routes read as a compact structured grid.
- Provider runtime summary has visual/structured counts.
- Existing provider, credential, OAuth, token, and model rows remain visible.
- No runtime behavior outside TUI rendering changes.
- Compile, vet, focused render smoke, serve smoke, manage smoke, senior plan
  review, and senior implementation review pass.
