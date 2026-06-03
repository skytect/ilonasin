# 443 TUI API Surface Inventory

## Context

Plan 442 recorded two independent whole-codebase review findings that the TUI
API pane under-represents the implemented local API surface. The active
architecture lists `/models`, `/v1/models`, `/responses`, `/v1/responses`,
`/v1/chat/completions`, `/v1/messages`, and `/v1/messages/count_tokens`.
`internal/server/handler.go` registers those routes, but
`internal/tui/control_sections.go` currently reports `surfaces 3` and renders
only grouped route families.

## Goal

Align the TUI API pane with the documented and registered local compatibility
routes without changing HTTP routing or management data.

## Scope

1. Update `internal/tui/control_sections.go`.
2. Render every concrete compatibility route registered in
   `internal/server/handler.go`:
   - `GET /models`;
   - `GET /v1/models`;
   - `POST /responses`;
   - `POST /v1/responses`;
   - `POST /v1/chat/completions`;
   - `POST /v1/messages`;
   - `POST /v1/messages/count_tokens`.
3. Keep the three API family groupings visible:
   - OpenAI Chat/Models;
   - OpenAI Responses;
   - Anthropic Messages.
4. Keep route paths visible and wrapped at narrow and wide widths.
5. Preserve local-token count/status rendering and downstream key management.
6. Do not change server routes, auth, provider behavior, DTOs, storage, config,
   logging policy, pane layout, key handling, or TUI mutation behavior.
7. Do not add permanent tests.

## Verification

Run:

```sh
gofmt -w internal/tui/control_sections.go
git diff --check
find . -name '*_test.go' -type f -print
go test ./internal/tui
go test ./...
go vet ./...
```

Run a temporary focused render check, then remove it before commit:

- render the API summary at narrow and wide widths;
- assert all seven concrete routes are visible;
- assert the three family labels remain visible;
- assert rendered lines fit the target widths.

Run disposable daemon smokes:

1. Build a temporary `ilonasin` binary.
2. Start `ilonasin serve` with temporary `ILONASIN_HOME`, temporary SQLite, IO
   capture disabled, keepalive disabled, and configured DeepSeek/Codex provider
   instances.
3. Verify management health and snapshot over the management socket.
4. Run bounded `ilonasin manage` at narrow and wide terminal widths.
5. Remove all temporary files and terminate the daemon.

## Acceptance

- The API pane accurately inventories the documented and registered routes.
- The change is TUI-only.
- Compile, vet, focused render smoke, serve smoke, manage smoke, senior plan
  review, and senior implementation review pass.
