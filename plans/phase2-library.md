# Phase 2 — Core Library

## Status

- [x] Go module initialized (`go mod init` in repo root)
- [x] `lib/types.go` — `Participant`, `Block`, `Problem`, `Options`, `Assignment`, `Cycle`, `Score`, `Solution`, `ErrInfeasible`
- [x] `lib/score.go` — `decomposeCycles`, `canonicalize`, `scoreOf`, `Score.Better`, `sortByScore`
- [x] `lib/graph.go` — `buildGraph`, `graph.isEdge`, `shuffled`
- [x] `lib/solver_test.go` — all tests written (TDD: written before implementation)
- [x] `lib/solver.go` — `validate`, `hamiltonianDFS`, `wouldClosePrematureCycle`, `constrainedBacktrack`, `collectSolutions`, `Solve`
- [x] All tests passing (`go test ./lib/...`)
- [x] `go vet ./...` and `staticcheck ./...` clean

> **Algorithm note**: The solver design was updated after Phase 1 experiments. The
> primary algorithm is **Hamiltonian DFS** (not backtracking + cycle-merge). See
> `plans/phase1-problem-exploration.md` section 3 for rationale.

## Goal

Implement the `lib/` package: a standalone, dependency-free Go library that takes a problem description (participants + blocks + options) and returns one or more ranked gift exchange solutions. This library is the only place algorithm logic lives.

---

## 1. High-Level Design

### 1.1 Package Structure

```
lib/
├── types.go        — public domain types (Participant, Block, Problem, Solution, Score)
├── graph.go        — valid-pairing graph construction (internal)
├── solver.go       — Hamiltonian DFS (primary) + constrained backtrack (fallback)
├── score.go        — objective function, cycle decomposition, canonicalization, ranking
└── solver_test.go  — all tests (unit + integration + property)
```

The package exports only the types and the `Solve` function. All graph and algorithm internals are unexported.

### 1.2 Public API

```go
package giftexchange

// Solve is the single entry point for the library.
// It returns up to opts.MaxSolutions solutions ranked best-first.
// Returns an error if the problem is infeasible or opts are invalid.
func Solve(ctx context.Context, p Problem, opts Options) ([]Solution, error)
```

Everything else is internal. The API is intentionally minimal — callers provide a `Problem`, get back `[]Solution`.

### 1.3 Core Types

**Input types:**

```go
type Participant struct {
    ID   string // unique identifier (used in blocks and output)
    Name string // display name
}

type Block struct {
    From string // participant ID: this person cannot give...
    To   string // ...to this person
    // Directed: Block{From: "alice", To: "bob"} does NOT block bob→alice
}

type Problem struct {
    Participants []Participant
    Blocks       []Block
    // No MinCycleLen — the solver automatically determines the best achievable
    // minimum cycle length by trying N, N/2, N/3, ... (see solver algorithm).
}

type Options struct {
    MaxSolutions int           // max solutions to return (default: 5)
    Seed         int64         // RNG seed for reproducibility (0 = random)
    Timeout      time.Duration // max solver wall time (0 = no limit)
}
```

**Output types:**

```go
type Assignment struct {
    GifterID    string
    RecipientID string
}

type Cycle []string // participant IDs in order: cycle[0]→cycle[1]→...→cycle[0]

type Score struct {
    MinCycleLen int // primary: maximize
    NumCycles   int // secondary: minimize
    MaxCycleLen int // tertiary: maximize
}

type Solution struct {
    Assignments []Assignment
    Cycles      []Cycle
    Score       Score
}
```

### 1.4 Validation

Before solving, the library validates the problem:

- No duplicate participant IDs
- All block participant IDs exist in the participant list
- `len(Participants) >= 2`
- Hall's condition check (necessary feasibility): every participant must have at least one valid recipient after blocks are applied (in-degree and out-degree ≥ 1)

### 1.5 Graph Construction

The internal `graph` type is an indexed, adjacency-list representation:

```go
type graph struct {
    n    int
    ids  []string        // ids[i] = participant ID at index i
    idx  map[string]int  // idx[id] = index
    adj  [][]int         // adj[i] = sorted list of valid recipient indices for gifter i
}
```

A directed edge `i → j` exists if:

1. `i != j` (no self-gifting)
2. `Block{From: ids[i], To: ids[j]}` is not in the block set

### 1.6 Solver Algorithm

The solver automatically determines the best achievable minimum cycle length without any caller-supplied parameter, using an N/M progression. See Phase 1 sections 3 and 5 for experimental basis.

**Progression: N/M search**

The solver tries a decreasing sequence of minimum cycle length targets — N, N/2, N/3, N/4, ... — stopping as soon as any solution is found. For each target `minCycleLen = N/M` (integer division):

- If `minCycleLen <= 1`: the problem is infeasible (no valid exchange is possible under any constraint) — return `ErrInfeasible`
- If `minCycleLen == previous target` (integer division can produce duplicates): skip, try M+1
- Otherwise: attempt to find `MaxSolutions` valid solutions under this target

The sequence of unique targets for various N:

| N   | Targets tried (until infeasible at 1) |
| --- | ------------------------------------- |
| 4   | 4, 2                                  |
| 6   | 6, 3, 2                               |
| 8   | 8, 4, 2                               |
| 12  | 12, 6, 4, 3, 2                        |
| 20  | 20, 10, 6, 5, 4, 3, 2                 |

Since minCycleLen=N is the Hamiltonian case (the highest achievable score), the solver always tries the optimal objective first and only relaxes when forced to by the constraint graph.

**Algorithm A — Hamiltonian DFS (M=1, minCycleLen=N):**

Search directly for a Hamiltonian cycle. Fix node 0 as the start (cycle rotation-invariance means this doesn't miss solutions). Depth-first search with per-node shuffled adjacency lists:

1. Maintain `path` and `visited`
2. At each step, try each unvisited neighbor of `path[last]` (in shuffled order)
3. When `len(path) == n`: check if an edge back to node 0 exists; if yes, record the solution
4. If DFS exhausts the search space: no Hamiltonian cycle exists — try M=2

**Algorithm B — Constrained Backtracking (M>1, minCycleLen=N/M):**

For each target `minCycleLen = N/M`:

1. Assign gifters to recipients one at a time (fixed gifter order, per-node shuffled adjacency lists)
2. For each candidate `gifter → recipient`, apply `wouldClosePrematureCycle`: follow the partial assignment chain from `recipient`; if it loops back to `gifter` before all n participants are assigned AND the would-be cycle length < minCycleLen, reject
3. Recurse; backtrack if no valid recipient found
4. If backtracking succeeds, the result is a valid cycle cover — record it

**Solution enumeration (both algorithms):**

Once a valid target `minCycleLen` is found, collect up to `MaxSolutions` distinct solutions at that level:

1. Run the solver with a seeded RNG; canonicalize and record the result
2. Re-seed and run again; if the result is distinct (new canonical form), add to results
3. Track consecutive collisions; if ≥ 5 consecutive duplicates, the solution space is near-exhausted — stop early
4. Return results ranked by `Score` (best-first)

All returned solutions use the same `minCycleLen` constraint, but their actual Scores may differ (some might have longer minimum cycles by chance). Ranking by Score naturally surfaces the best ones first.

### 1.7 Testing Strategy

Tests are in `solver_test.go`. All tests are table-driven.

**Unit tests:**

- `TestBuildGraph`: verify edge existence / absence for given blocks
- `TestWouldClosePrematureCycle`: verify correct detection for open chains, premature closes, and valid final-edge closes
- `TestDecomposeCycles`: verify cycle decomposition for known permutations (single cycle, multi-cycle, degenerate)
- `TestCanonicalForm`: verify two rotations of the same cycle produce the same canonical string; verify a reversed cycle is distinct
- `TestScoreBetter`: verify Score comparison logic (all three tie-break levels)

**Integration tests (solver correctness):**

- `TestSolve_SmallNoBlocks`: n=4, no blocks → must find Hamiltonian 4-cycle (MinCycleLen=4 in score)
- `TestSolve_SmallWithBlocks`: n=4, one block → verify solution respects block and is still valid
- `TestSolve_Infeasible`: participant with all recipients blocked → must return `ErrInfeasible`
- `TestSolve_Reproducible`: same seed → identical solution set and ordering
- `TestSolve_MultipleSolutions`: `MaxSolutions=5` → verify all 5 are distinct under canonical form
- `TestSolve_ScoreRanking`: verify returned solutions are ordered best-first by Score
- `TestSolve_MergeCounterexample`: use the 6-node counterexample from Phase 1 experiments → solver finds the Hamiltonian cycle (not the three 2-cycles)
- `TestSolve_FallbackProgression`: construct graph with no Hamiltonian cycle but valid 2-cycles → solver returns valid solutions at the next achievable minCycleLen; verify the actual minCycleLen in Score matches what was achievable
- `TestSolve_ProgressionN6`: n=6 graph where only minCycleLen=3 is achievable → verify Score.MinCycleLen=3 in all returned solutions

**Property-based tests (using a simple fuzzer):**
For random problem instances, verify all returned solutions satisfy:

- Every participant appears exactly once as gifter and once as recipient
- No assignment violates a block
- Score computed from assignments exactly matches `solution.Score`
- All solutions in one response have the same Score.MinCycleLen (they were found at the same progression level)

---

## 2. Implementation Plan

1. **Initialize Go module**: `go mod init github.com/[user]/gift-exchange` in the repo root
2. **Write `lib/types.go`**: all public types and `Solve` signature (stub)
3. **Write `lib/score.go`**: `decomposeCycles`, `canonicalize`, `scoreOf`, `Score.Better`
4. **Write `lib/graph.go`**: `buildGraph`, `graph.isEdge`, `shuffled`
5. **Write `lib/solver_test.go`**: write all tests first (they will fail)
6. **Write `lib/solver.go`**: `validate`, `hamiltonianDFS`, `wouldClosePrematureCycle`, `constrainedBacktrack`, `Solve` — iterate until all tests pass
7. **Run `go vet` and `staticcheck`** — zero warnings required before Phase 3
8. **Verify with the Phase 1 counterexample graph** — `TestSolve_MergeCounterexample` must pass

---

## 3. Implementation Snippets

### `lib/types.go` — Core types and Solve signature

```go
package giftexchange

import (
    "context"
    "errors"
    "time"
)

// ErrInfeasible is returned when no valid gift exchange assignment exists
// under the given constraints (all N/M progression levels exhausted).
var ErrInfeasible = errors.New("no valid gift exchange is possible under the given constraints")

type Participant struct {
    ID   string
    Name string
}

type Block struct {
    From string // this participant cannot give...
    To   string // ...to this participant (directed constraint)
}

type Problem struct {
    Participants []Participant
    Blocks       []Block
    // No MinCycleLen: the solver automatically finds the best achievable
    // cycle structure via the N/M progression (N, N/2, N/3, ...).
}

type Options struct {
    MaxSolutions int           // max solutions to return (default: 5)
    Seed         int64         // RNG seed; 0 = random (non-reproducible)
    Timeout      time.Duration // max solver wall time; 0 = no limit
}

type Assignment struct {
    GifterID    string
    RecipientID string
}

type Cycle []string

type Score struct {
    MinCycleLen int
    NumCycles   int
    MaxCycleLen int
}

type Solution struct {
    Assignments []Assignment
    Cycles      []Cycle
    Score       Score
}

func Solve(ctx context.Context, p Problem, opts Options) ([]Solution, error)
```

### `lib/graph.go` — Graph construction

```go
package giftexchange

type graph struct {
    n   int
    ids []string
    idx map[string]int
    adj [][]int
}

func buildGraph(participants []Participant, blocks []Block) *graph {
    n := len(participants)
    g := &graph{n: n, ids: make([]string, n), idx: make(map[string]int), adj: make([][]int, n)}
    for i, p := range participants {
        g.ids[i] = p.ID
        g.idx[p.ID] = i
    }

    blocked := make(map[[2]int]bool)
    for _, b := range blocks {
        fi, ti := g.idx[b.From], g.idx[b.To]
        blocked[[2]int{fi, ti}] = true
    }

    for i := range n {
        for j := range n {
            if i != j && !blocked[[2]int{i, j}] {
                g.adj[i] = append(g.adj[i], j)
            }
        }
    }
    return g
}
```

### `lib/solver.go` — Hamiltonian DFS (primary algorithm)

```go
// hamiltonianDFS attempts to find a Hamiltonian cycle in g using depth-first
// search starting from node 0. Per-node adjacency list shuffling (via rng)
// ensures different calls produce different solutions.
// Returns (assignment, true) on success, (nil, false) if no Hamiltonian cycle exists.
func hamiltonianDFS(g *graph, rng *rand.Rand) ([]int, bool) {
    assign := make([]int, g.n)
    for i := range assign {
        assign[i] = -1
    }
    visited := make([]bool, g.n)
    path := make([]int, 0, g.n)
    path = append(path, 0) // fix start node; cycle is rotation-invariant
    visited[0] = true

    var dfs func() bool
    dfs = func() bool {
        if len(path) == g.n {
            last := path[len(path)-1]
            if g.isEdge(last, 0) {
                for i := 0; i < len(path)-1; i++ {
                    assign[path[i]] = path[i+1]
                }
                assign[last] = 0
                return true
            }
            return false
        }
        cur := path[len(path)-1]
        for _, next := range shuffled(g.adj[cur], rng) { // per-node shuffle
            if !visited[next] {
                path = append(path, next)
                visited[next] = true
                if dfs() {
                    return true
                }
                path = path[:len(path)-1]
                visited[next] = false
            }
        }
        return false
    }

    if dfs() {
        return assign, true
    }
    return nil, false
}
```

### `lib/solver.go` — Constrained backtracking (fallback when no Hamiltonian exists)

```go
// constrainedBacktrack finds a valid cycle cover where all cycles have length
// >= minCycleLen. Used only when hamiltonianDFS has confirmed no Hamiltonian
// cycle exists in the graph.
func constrainedBacktrack(g *graph, rng *rand.Rand, minCycleLen int) ([]int, bool) {
    assign := make([]int, g.n)
    for i := range assign {
        assign[i] = -1
    }
    usedRecipient := make([]bool, g.n)

    var backtrack func(gifter int) bool
    backtrack = func(gifter int) bool {
        if gifter == g.n {
            return true
        }
        for _, recipient := range shuffled(g.adj[gifter], rng) {
            if usedRecipient[recipient] {
                continue
            }
            if wouldClosePrematureCycle(assign, gifter, recipient, minCycleLen) {
                continue
            }
            assign[gifter] = recipient
            usedRecipient[recipient] = true
            if backtrack(gifter + 1) {
                return true
            }
            assign[gifter] = -1
            usedRecipient[recipient] = false
        }
        return false
    }

    if backtrack(0) {
        return assign, true
    }
    return nil, false
}

// CORRECTED during implementation: the original used `length < minLen || assigned < total`
// but || is wrong — it blocks all intermediate cycle closures, preventing multi-cycle
// solutions. The `total` parameter is also unnecessary. The correct check is:
func wouldClosePrematureCycle(assign []int, gifter, recipient, minLen int) bool {
    length := 1
    cur := recipient
    for {
        next := assign[cur]
        if next < 0 {
            return false // open chain, no cycle
        }
        length++
        if next == gifter {
            return length < minLen
        }
        cur = next
    }
}
```

````

### `lib/solver.go` — Top-level Solve function with N/M progression

```go
func Solve(ctx context.Context, p Problem, opts Options) ([]Solution, error) {
    if err := validate(p); err != nil {
        return nil, err
    }
    g := buildGraph(p.Participants, p.Blocks)
    n := g.n

    seed := opts.Seed
    if seed == 0 {
        seed = time.Now().UnixNano()
    }

    // N/M progression: try minCycleLen = N, N/2, N/3, ... until a solution is
    // found or the target drops to 1 (infeasible).
    lastTarget := -1
    for M := 1; ; M++ {
        if ctx.Err() != nil {
            return nil, ctx.Err()
        }
        target := n / M
        if target <= 1 {
            return nil, ErrInfeasible
        }
        if target == lastTarget {
            continue // integer division produced a duplicate target; skip
        }
        lastTarget = target

        solutions := collectSolutions(ctx, g, target, seed, opts.MaxSolutions)
        if len(solutions) > 0 {
            sortByScore(solutions) // best Score first
            return solutions, nil
        }
        // No solution found at this target — try the next (more relaxed) level
    }
}

// collectSolutions attempts to find up to maxSolutions distinct valid assignments
// where every cycle has length >= minCycleLen. Returns whatever was found (may be
// fewer than maxSolutions if the solution space is small).
func collectSolutions(ctx context.Context, g *graph, minCycleLen int, seed int64, max int) []Solution {
    const collisionThreshold = 5

    seen := map[string]bool{}
    results := []Solution{}
    consecutive := 0

    // Use a separate RNG per attempt so seeds are deterministic across call sites
    masterRNG := rand.New(rand.NewSource(seed))

    for len(results) < max {
        if ctx.Err() != nil {
            break
        }
        attemptSeed := masterRNG.Int63()
        rng := rand.New(rand.NewSource(attemptSeed))

        var assign []int
        var ok bool
        if minCycleLen == g.n {
            assign, ok = hamiltonianDFS(g, rng)
        } else {
            assign, ok = constrainedBacktrack(g, rng, minCycleLen)
        }
        if !ok {
            // This target is infeasible — signal to the caller via empty results
            return nil
        }

        canon := canonicalize(assign)
        if seen[canon] {
            consecutive++
            if consecutive >= collisionThreshold {
                break
            }
        } else {
            seen[canon] = true
            consecutive = 0
            results = append(results, makeSolution(assign, g))
        }
    }
    return results
}
````

### `lib/solver_test.go` — Property-based solution validator

```go
func assertValidSolution(t *testing.T, p Problem, s Solution) {
    t.Helper()
    n := len(p.Participants)

    // Every participant gives exactly once
    givers := make(map[string]int)
    receivers := make(map[string]int)
    for _, a := range s.Assignments {
        givers[a.GifterID]++
        receivers[a.RecipientID]++
    }
    require.Len(t, givers, n)
    require.Len(t, receivers, n)

    // No self-assignments, no blocked pairs
    blocked := make(map[[2]string]bool)
    for _, b := range p.Blocks {
        blocked[[2]string{b.From, b.To}] = true
    }
    for _, a := range s.Assignments {
        require.NotEqual(t, a.GifterID, a.RecipientID)
        require.False(t, blocked[[2]string{a.GifterID, a.RecipientID}])
    }

    // All cycles meet the achieved minimum (recorded in the Score)
    for _, c := range s.Cycles {
        require.GreaterOrEqual(t, len(c), s.Score.MinCycleLen)
    }

    // Score.MinCycleLen matches what's actually in the cycles
    actualMin := len(p.Participants)
    for _, c := range s.Cycles {
        if len(c) < actualMin { actualMin = len(c) }
    }
    require.Equal(t, actualMin, s.Score.MinCycleLen)
}
```
