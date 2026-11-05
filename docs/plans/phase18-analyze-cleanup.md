# Phase 18 — Analyze Cleanup

## Status

> Not started.

---

## Motivation

Phase 17 introduced dead edge analysis (`canCompleteMatching`,
`hamiltonianPathExists`) alongside the pre-existing `hallViolations`. The three
functions are correct and all tests pass, but a review identified three categories
of technical debt worth fixing before the codebase grows further:

1. **Bipartite matching is copy-pasted.** The augmenting-path `augment` closure
   appears nearly verbatim in both `hallViolations` and `canCompleteMatching`.
   These two functions differ only in how they initialize the matching (one
   excludes a gifter/recipient pair, one does not) and what they do with the
   result. The shared logic should live in one place.

2. **Test coverage is incomplete.** The dead edge tests verify membership of
   specific edges but do not verify the *complete* dead edge sets. A partial
   implementation that silently misclassifies some edges would still pass.
   Two cases are also missing entirely: the all-Hamiltonian-dead scenario and
   context cancellation.

3. **Two minor documentation/style issues.** The `SolutionDeadEdges` doc comment
   is misleading about the semantics of `nil`. A local helper function in the CLI
   is a two-call abstraction that should be inlined.

---

## Changes

### `lib/analyze.go` — extract shared bipartite matching

Extract a private function:

```go
// bipartiteMatch returns a maximum bipartite matching for g, optionally
// excluding one gifter and one recipient from consideration.
// Pass -1 for either parameter to include all gifters / all recipients.
// Returns matchLeft[i] = matched recipient for gifter i (or -1 if unmatched),
// and matchRight[j] = matched gifter for recipient j (or -1 if unmatched).
func bipartiteMatch(g *graph, excludeGifter, excludeRecipient int) (matchLeft, matchRight []int)
```

Refactor `hallViolations` to call `bipartiteMatch(g, -1, -1)` and use the
returned arrays for the BFS alternating-tree pass.

Refactor `canCompleteMatching` to call `bipartiteMatch(g, excludeGifter,
excludeRecipient)` and return `matchedCount == g.n-1`.

The augment closure and outer loop live in exactly one place.

**Boundary condition**: when `excludeRecipient == -1`, the `r == excludeRecipient`
guard in the augment inner loop is a false comparison against -1, which can never
equal a valid index. No special-casing needed; the logic is uniform.

### `lib/solver_test.go` — improve dead edge test coverage

**Fix `TestAnalyze_DeadEdges_SolutionDead`**: assert the *complete* dead edge
set, not just a subset. The 3-participant graph (`c→b` blocked) has exactly 5
valid edges. Assert that `a→c` and `b→a` are dead and that `a→b`, `b→c`, `c→a`
are not. This prevents false-positive dead edge classifications from going
undetected.

**Add `TestAnalyze_DeadEdges_AllHamiltonianDead`**: use `twoGroupProblem(4)`.
In this graph every valid edge passes `canCompleteMatching` (a 2+2 cycle cover
remains achievable regardless of which single within-group edge is fixed) but
fails `hamiltonianPathExists` (no Hamiltonian path exists since the graph is
split into two disconnected components under the block structure). Expected
result: `SolutionDeadEdges` is empty, `HamiltonianDeadEdges` contains all 4
valid edges (`0→1`, `1→0`, `2→3`, `3→2`). This is the important case where
the graph is feasible but no Hamiltonian cycle is possible.

**Add `TestAnalyze_DeadEdges_CtxCancelled`**: create an already-cancelled context
and call `Analyze`. Verify the error returned wraps `context.Canceled`. The test
validates the cancellation path through the dead edge loop (lines 55–57 in the
current implementation).

### `lib/types.go` — fix doc comments

```go
// SolutionDeadEdges lists valid edges where fixing u→v makes any valid complete
// assignment impossible. nil when Hall violations exist (analysis skipped).
```

`nil` is returned in two distinct cases: (a) Hall violations exist (analysis
skipped), and (b) analysis ran and found no solution-dead edges. The current
comment implies case (a) only, which would mislead a caller testing `== nil` to
detect whether analysis ran.

Fix both field comments to document both nil cases:

```go
// SolutionDeadEdges lists valid edges where fixing u→v makes any valid complete
// assignment impossible. nil when Hall violations exist (analysis skipped) or
// when no solution-dead edges were found.
HamiltonianDeadEdges []DeadEdge
```

Apply the same fix to `HamiltonianDeadEdges`.

### `cmd/giftexchange/main.go` — two minor CLI cleanups

**Inline `ensureGifter`**: the helper is 3 lines and called in exactly 2 places
within the same block. Per the project norm (no abstraction for aesthetics),
inline it.

**Fix dead edge output ordering**: gifters currently appear in `gifterOrder` in
the order they were first seen when iterating `SolutionDeadEdges` then
`HamiltonianDeadEdges`. A gifter with only Hamiltonian-dead edges appears after
all solution-dead gifters regardless of ID order. Since `Analyze` emits both
slices in sorted-ID order, the CLI should build a single sorted-gifter output
loop. Simplest fix: collect all dead edges into one slice of `(gifter, recipient,
kind)` tuples in iteration order (already sorted by gifter ID since
`SolutionDeadEdges` and `HamiltonianDeadEdges` iterate in graph index order),
then group by gifter. This matches the plan's example output where all of Alice's
lines are adjacent regardless of category.

---

## Files to change

| File | Changes |
| --- | --- |
| `lib/analyze.go` | Extract `bipartiteMatch`; refactor `hallViolations` and `canCompleteMatching` to call it |
| `lib/solver_test.go` | Fix `TestAnalyze_DeadEdges_SolutionDead` for completeness; add `_AllHamiltonianDead` and `_CtxCancelled` |
| `lib/types.go` | Fix doc comments for `SolutionDeadEdges` and `HamiltonianDeadEdges` |
| `cmd/giftexchange/main.go` | Inline `ensureGifter`; fix output ordering |

---

## Acceptance criteria

- `bipartiteMatch` is the single implementation of augmenting-path matching;
  `hallViolations` and `canCompleteMatching` contain no augment closure.
- `TestAnalyze_DeadEdges_SolutionDead` asserts the complete dead edge set
  (both present and absent edges checked).
- `TestAnalyze_DeadEdges_AllHamiltonianDead` passes: `twoGroupProblem(4)`
  produces empty `SolutionDeadEdges` and 4-element `HamiltonianDeadEdges`.
- `TestAnalyze_DeadEdges_CtxCancelled` passes: cancelled ctx returns a
  `context.Canceled`-wrapping error.
- Dead edge CLI output lists gifters in consistent sorted-ID order across
  both dead edge categories.
- All existing tests continue to pass; `go vet` clean.
