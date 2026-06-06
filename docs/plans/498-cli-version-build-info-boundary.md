# 498 CLI Version Build Info Boundary

## Context

Plan 490 found that `internal/cli/version.go` shells out to `git` from the
process current working directory. That makes `ilonasin --version` depend on
ambient filesystem state instead of build metadata. The packaging path already
injects `Version`, `Commit`, and `CommitSubject` through ldflags, and Go embeds
VCS revision and dirty-state metadata in normal module builds.

## Goal

Make CLI version identity deterministic from explicit build metadata:

- prefer `Version`, `Commit`, and `CommitSubject` ldflags;
- otherwise use Go build-info `vcs.revision` and `vcs.modified`;
- do not run `git` or read repository files at runtime.

## Scope

1. Update `internal/cli/version.go`.
2. Remove runtime `git` probing helpers and their imports.
3. Preserve current version formatting as far as possible:
   - commit identity remains the primary output when a commit is known;
   - commit hashes remain shortened to 12 characters;
   - `CommitSubject` still appears when supplied by ldflags;
   - dirty state still appears when Go build info reports `vcs.modified=true`;
   - version remains the fallback when no commit is known.
4. Do not change CLI command dispatch, package ldflags, config, server, TUI,
   provider adapters, storage, management APIs, routing, or logging.
5. Do not add permanent tests.

## Verification

Use direct temporary binary checks:

- build normally and run `ilonasin --version` from inside and outside the repo;
- build with ldflags for `Commit` and `CommitSubject` and verify the output uses
  those values even when run outside the repo;
- build with `-buildvcs=false` and an explicit `Version` ldflag to verify the
  no-commit version fallback path;
- verify no runtime `git`, `exec`, `CommandContext`, `ReadFile`, or `go.mod`
  probing remains in `internal/cli/version.go`.

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

- `VersionString` no longer depends on the current working directory, local
  `go.mod`, or a local `git` executable.
- Build-info and ldflag version identity still render useful output.
- Compile, vet, serve/manage smoke, and three implementation reviews pass.
