# Phase 17 — Dead Edge Analysis

## Status

> Complete.

---

## Motivation

The `Analyze` output tells you who each participant can give to and whether any
Hall violation exists. What it does not tell you is whether a specific valid
edge can actually appear in a solution. An edge (u→v) might be formally valid
(not blocked) yet structurally dead: including it in the assignment makes
completing the rest impossible.

Two distinct failure modes are useful to distinguish:

- **Solution-dead**: fixing u→v makes any valid complete assignment impossible.
- **Hamiltonian-dead**: fixing u→v allows some multi-cycle solution but rules
  out any single Hamiltonian cycle.

This phase adds both categories to `GraphInfo`, removes the now-redundant
global `HamiltonianPossible` field, and fixes the analyze output ordering.

---

## Definitions

**Solution-dead edge (u→v)**: after fixing gifter u → recipient v, the
remaining n-1 participants cannot be bipartite-matched. No valid assignment of
any cycle structure can include u→v.

Check: remove u from the available gifter pool and v from the available
recipient pool; run max bipartite matching on the remaining (n-1)×(n-1)
subgraph; dead iff matching size < n-1. Cost: O(n²) per edge, O(n³) total.

**Hamiltonian-dead edge (u→v)**: fixing u→v allows a complete valid
assignment (bipartite check passes) but no Hamiltonian cycle includes it —
i.e., no Hamiltonian path from v to u exists through all n participants.

Check: DFS from v, visiting all n nodes, ending at u. Hamiltonian-dead iff
no such path exists. Cost: O(n!) worst case per edge with pruning; practical
for n ≤ 20.

Edges that are solution-dead are implicitly also Hamiltonian-dead. The
Hamiltonian-dead check only runs for edges that are NOT solution-dead.

---

## Removing `HamiltonianPossible`

The previous global Hamiltonian DFS (`HamiltonianPossible bool` in `GraphInfo`)
answered "does any Hamiltonian cycle exist?" The per-edge Hamiltonian-dead
analysis is strictly more informative and subsumes this question. A global
Hamiltonian cycle exists iff at least one edge is not Hamiltonian-dead (and a
valid set of non-Hamiltonian-dead edges forms a cycle covering all participants,
which the existing `solve` command answers directly).

`HamiltonianPossible` is removed from `GraphInfo`. The Hamiltonian DFS is
removed from `Analyze`.

---

## Behavior when Hall violations exist

If Hall violations exist, the base problem is infeasible. Every edge (u→v) is
trivially solution-dead: any partial assignment inherits the global infeasibility.
Computing dead edges in this case would list every valid edge as dead — correct
but uninformative.

Dead edge analysis is **skipped when Hall violations exist**. The CLI notes this
in the output: `Dead edges: skipped (Hall condition violated)`.

---

## Changes

### `lib/types.go`

Remove `HamiltonianPossible bool` from `GraphInfo`.

Add:

```go
// DeadEdge is a valid (non-blocked) edge that cannot be used under some
// or all solution types.
type DeadEdge struct {
    Gifter    string // participant ID
    Recipient string // participant ID
}
```

Add to `GraphInfo`:

```go
// SolutionDeadEdges: valid edges where fixing u→v makes any valid complete
// assignment impossible. nil when Hall violations exist (analysis skipped).
SolutionDeadEdges []DeadEdge

// HamiltonianDeadEdges: valid edges where fixing u→v allows some multi-cycle
// solution but rules out any Hamiltonian cycle. Excludes SolutionDeadEdges.
// nil when Hall violations exist (analysis skipped).
HamiltonianDeadEdges []DeadEdge
```

### `lib/analyze.go`

Remove Hamiltonian DFS block.

Add two private functions:

```go
// canCompleteMatching reports whether the remaining n-1 participants can be
// bipartite-matched after fixing gifter excludeGifter → recipient excludeRecipient.
func canCompleteMatching(g *graph, excludeGifter, excludeRecipient int) bool

// hamiltonianPathExists reports whether a Hamiltonian path from start to end
// exists, visiting all n participants exactly once.
// end is reserved (not visitable as an intermediate node).
func hamiltonianPathExists(ctx context.Context, g *graph, start, end int) bool
```

Update `Analyze`:

```
if Hall violations exist:
    SolutionDeadEdges = nil, HamiltonianDeadEdges = nil
else:
    for each valid edge (i, j):
        if !canCompleteMatching(g, i, j):
            append to SolutionDeadEdges
        else if !hamiltonianPathExists(ctx, g, j, i):
            append to HamiltonianDeadEdges
```

### `lib/solver_test.go`

Remove references to `HamiltonianPossible`. Add:

- `TestAnalyze_DeadEdges_None`: complete graph — no dead edges of either kind
- `TestAnalyze_DeadEdges_SolutionDead`: an edge whose fixing creates a Hall
  violation in the remaining subproblem
- `TestAnalyze_DeadEdges_HamiltonianDead`: an edge that passes the matching
  check but whose v→u Hamiltonian path doesn't exist
- `TestAnalyze_DeadEdges_SkippedWhenHallViolated`: Hall violations present →
  both dead edge slices are nil

### `cmd/giftexchange/main.go`

Fix output ordering and remove Hamiltonian line. New order:

```
Participants:  N
Edges:         E of M possible (D% density)

Hall condition: satisfied | violated
  [violation details if any]

Dead edges: none | skipped (Hall condition violated)
  [grouped by gifter if any]

Recipients:
  Name  (N): R1, R2, ...
```

Dead edge output grouped by gifter:

```
Dead edges:
  Alice cannot gift to (no solution):        Bob
  Alice cannot gift to (Hamiltonian cycle):  Carol, Dave
  Bob   cannot gift to (no solution):        Carol
```

---

## Files to change

| File                            | Changes                                                                                               |
| ------------------------------- | ----------------------------------------------------------------------------------------------------- |
| `lib/types.go`                  | Remove `HamiltonianPossible`; add `DeadEdge`, `SolutionDeadEdges`, `HamiltonianDeadEdges`             |
| `lib/analyze.go`                | Remove Hamiltonian DFS; add `canCompleteMatching`, `hamiltonianPathExists`; populate dead edge fields |
| `lib/solver_test.go`            | Remove `HamiltonianPossible` test refs; add dead edge tests                                           |
| `cmd/giftexchange/main.go`      | Reorder output; remove Hamiltonian line; add dead edge section                                        |
| `cmd/giftexchange/main_test.go` | Update `TestCLI_Analyze` for new output                                                               |

---

## Acceptance criteria

- Solution-dead edge correctly identified: an edge whose fixing leaves some
  gifter with no reachable recipients (transitively through the matching).
- Hamiltonian-dead edge correctly identified: an edge where fixing it allows
  a complete assignment but no Hamiltonian path from recipient→gifter exists.
- Solution-dead edges do not appear in HamiltonianDeadEdges.
- When Hall violations exist, both dead edge slices are nil.
- All existing lib, CLI, server tests pass.
- `go vet` clean.
