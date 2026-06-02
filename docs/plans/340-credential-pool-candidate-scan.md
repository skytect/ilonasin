# 340 Credential Pool Candidate Scan

## Context

Plan 339 added no-affinity round-robin tie-breaking for equal-pressure
credential candidates. The implementation is correct, but
`firstCandidateAtOrAfter` builds a temporary map while the pressure tracker
mutex is held. This is small, but it is avoidable work in the hot credential
reservation path.

The architecture expects routing and credential pooling to stay modular,
auditable, and efficient while preserving same-provider-instance and same-model
constraints. This slice removes that avoidable allocation without changing the
pooling policy.

## Scope

1. Replace the map-backed candidate membership check in
   `firstCandidateAtOrAfter` with a small linear scan helper.
2. Keep the existing selection semantics unchanged:
   - find the cursor credential in the current slot order;
   - scan forward through the current slot order;
   - choose the first slot index that is in the lowest-pressure candidate list;
   - fall back to the first candidate if the cursor credential is absent.
3. Keep all locking, cursor state, explicit-affinity behavior, quota filtering,
   retry behavior, metadata, logging, storage, management API, provider adapter,
   and TUI behavior unchanged.

## Out Of Scope

- Changing no-affinity rotation policy.
- Adding persistent routing state.
- Adding permanent tests.
- Any TUI, management, storage, request parsing, or provider changes.

## Implementation Steps

1. Add a tiny package-private helper, for example
   `containsCandidateIndex(candidates []int, index int) bool`.
2. Use that helper from `firstCandidateAtOrAfter` instead of allocating a map.
3. Review the diff for exact behavior preservation and readability.

## Verification

Use a temporary focused check, then remove it before commit:

- cursor present and itself a candidate still selects the cursor slot;
- cursor present but higher-pressure/non-candidate selects the next candidate
  after the cursor;
- cursor absent returns no match so the caller keeps the first candidate;
- retry-filtered slots with non-contiguous original attempt indexes still treat
  candidates as current slice positions, not original attempt indexes;
- wraparound after an end-of-list cursor still finds an earlier candidate;
- no allocation-oriented helper changes affect ordinary round-robin sequence.

Then run:

```sh
git diff --check
find . -name '*_test.go' -type f -print
go test ./internal/server
go test ./...
go vet ./...
```

Run direct CLI smokes by building a temporary binary, starting
`ilonasin serve` with a temporary `ILONASIN_HOME`, checking management health
over the Unix socket, running bounded `ilonasin manage`, and cleaning up all
temporary files and processes.

## Acceptance

- No-affinity candidate scanning avoids temporary map allocation under the
  pressure tracker mutex.
- Slice 339 routing behavior is preserved.
- No new persistent state, metadata fields, logs, config, or TUI surfaces are
  added.
