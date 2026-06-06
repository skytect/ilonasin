# 497 Responses Test Cleanup

## Context

Whole-codebase review in plan 490 found one remaining permanent Go test file:
`internal/openai/responses_test.go`. The repository workflow asks for direct
compile, vet, and CLI smoke checks, with temporary focused checks removed before
commit.

The current tests cover `DecodeResponses` function-call ordering behavior. This
slice removes the permanent test file only. Runtime request parsing and
validation behavior must not change.

## Goal

Remove the permanent Responses test file while preserving confidence through
temporary focused verification that is deleted before commit.

## Scope

1. Delete `internal/openai/responses_test.go`.
2. Do not change `DecodeResponses`, request conversion, provider adapters,
   server routes, storage, management DTOs, TUI, config, logging, routing, or
   public API behavior.
3. Do not add permanent tests.

## Verification

Use a temporary focused check, then remove it before commit:

- temporarily add `internal/openai/responses_tmp_test.go` with equivalent checks
  for interleaved function outputs, parallel interleaved function outputs,
  unmatched function output rejection, and missing function output rejection;
- run `go test ./internal/openai -run TestDecodeResponses`;
- delete `internal/openai/responses_tmp_test.go`.

Then run:

```sh
git diff --check
find . -name '*_test.go' -type f -print
go test ./...
go vet ./...
```

Run direct CLI smokes by building a temporary `ilonasin` binary, starting
`ilonasin serve` with isolated `ILONASIN_HOME` and a temporary config, checking
management health and snapshot over the Unix management socket, running bounded
`ilonasin manage` at narrow and wide terminal widths, and cleaning up all
temporary files and processes.

## Acceptance

- No permanent `_test.go` files remain from this slice.
- The focused `DecodeResponses` checks pass before deletion.
- Runtime behavior is unchanged.
- Compile, vet, serve/manage smoke, and three implementation reviews pass.
