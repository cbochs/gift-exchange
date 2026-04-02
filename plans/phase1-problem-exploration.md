# Phase 1 — Problem Exploration

## Status

- [x] Problem formalized as directed cycle cover optimization (Hamiltonian = optimal)
- [x] Algorithm candidates evaluated: randomized backtracking, greedy 2-opt merge, Hamiltonian DFS, bipartite matching
- [x] **Experiment**: greedy 2-opt merge proven incomplete — 50% failure rate on counterexample graph
- [x] **Experiment**: shuffle diversity measured — per-node marginally better; merge had zero effect on dense graphs
- [x] **Decision**: primary algorithm = Hamiltonian DFS with per-node shuffled adjacency lists
- [x] **Decision**: fallback = constrained backtracking when no Hamiltonian cycle exists
- [x] **Decision**: N/M progression (N, N/2, N/3, ...) replaces user-facing `minCycleLen` parameter
- [x] **Decision**: solutions ranked by Score; UI presents best-first with no special casing
- [x] Adaptive collision-rate stopping (≥5 consecutive duplicates → near-exhausted) validated

> **Status**: Revised after experiments in `experiments/merge_completeness/` and
> `experiments/shuffle_diversity/`. Key findings changed the architectural recommendation.

## Goal

Formalize the gift exchange problem as a graph theory problem. Identify which algorithms apply, analyze their complexity for realistic input sizes, and define the objective function the solver will optimize. Validate assumptions experimentally before committing to an implementation strategy.

---

## 1. High-Level Design

### 1.1 Problem Formulation

A gift exchange assigns each participant exactly one gift to give and exactly one gift to receive. This is equivalent to finding a **permutation** of the participant set with no fixed points (no self-gifting). Every permutation decomposes uniquely into disjoint directed cycles.

We model this as a directed graph problem:

- **Nodes**: participants
- **Directed edge (u → v)**: u is allowed to give to v (not blocked, not self)
- **Goal**: find a **cycle cover** — a set of directed cycles that together visit every node exactly once — that maximizes the objective function

A cycle cover where every edge exists in the valid-pairing graph corresponds exactly to a valid gift assignment.

**The ideal outcome is a Hamiltonian cycle** — a single cycle visiting all n nodes. It maximizes the minimum and maximum cycle lengths simultaneously and produces exactly one cycle. When a Hamiltonian cycle is achievable, it is the unique global optimum.

### 1.2 Cycle Coverage Spectrum

```
Best:   One n-cycle        [A→B→C→D→...→Z→A]           score: (n, 1, n)
Good:   Two large cycles   [A→B→C→A] + [D→E→F→G→D]     score: (3, 2, 4)
Okay:   Many medium cycles                               score: (k, m, ...)
Bad:    Contains 2-cycles  [A→B→A] + rest               score: (2, ...)
```

### 1.3 Objective Function

The objective function is lexicographic over three metrics:

```
Score = (MinCycleLen, -NumCycles, MaxCycleLen)
```

- **Primary**: maximize MinCycleLen (a 2-cycle is much worse than a 4-cycle)
- **Secondary**: minimize NumCycles (fewer cycles is better)
- **Tertiary**: maximize MaxCycleLen (prefer unbalanced large+small over many medium)

Higher MinCycleLen always wins, regardless of the other metrics.

The solver pursues this objective automatically via the **N/M progression**: it targets minCycleLen = N (Hamiltonian), then N/2, then N/3, and so on. The first level that yields any valid solution wins. Within that level, multiple solutions are collected and ranked by their actual Score — some may happen to score better than the minimum target. The user has no control over this progression; it is an internal detail of the solver.

### 1.4 Why NP-Hardness Doesn't Matter Here

Finding a Hamiltonian cycle in a general directed graph is NP-complete. However:

- Gift exchanges have n ≤ 30 participants in practice
- The valid-pairing graph is typically **dense** (most pairings are allowed — blocks are sparse)
- On dense graphs, backtracking DFS finds Hamiltonian cycles very quickly with no meaningful search tree explosion

---

## 2. Experimental Findings

Two experiments were conducted to validate the planned algorithm before committing to an implementation. Code is in `experiments/`.

### 2.1 Greedy 2-opt Cycle Merge Is Not Complete

**Hypothesis**: The planned post-processing step (greedily merge the shortest pair of cycles using a 2-opt edge swap) would find a Hamiltonian cycle whenever one exists.

**Result**: **Refuted.** A 6-node counterexample graph was constructed with exactly 2 valid cycle covers:

- One Hamiltonian cycle: `0→1→5→4→3→2→0`
- One set of three 2-cycles: `0↔1, 2↔3, 4↔5`

When merge starts from the three 2-cycles, all three pairwise merge attempts fail (the required cross-cycle edges don't exist). The merge is completely stuck despite the Hamiltonian cycle existing. **50% of all valid starting assignments for this graph cause the merge to fail.**

The direct Hamiltonian DFS finds the solution immediately.

**Implication**: Greedy 2-opt merge is a local search heuristic — it finds a local optimum under 2-opt neighborhood moves, not the global optimum. The solver cannot rely on it as a correctness mechanism.

### 2.2 Does Shuffling Provide Diverse Solutions?

**Hypothesis to test**: Both global shuffle (all recipients in one shared order) and per-node shuffle (each gifter independently shuffles its own neighbors) produce diverse solutions when run repeatedly.

**Results** (1000 runs, seed=42):

| Graph                           | Out-degree | Global distinct | Per-node distinct | Collision rate |
| ------------------------------- | ---------- | --------------- | ----------------- | -------------- |
| Dense (n=8, all pairs valid)    | 7          | 891             | **904**           | 10–11%         |
| Moderate (n=8, ~4 valid each)   | 4          | 108             | **117**           | 88–89%         |
| Very sparse (n=6, 2 valid each) | 2          | 3               | 3                 | 97–100%        |

Key observations:

1. **Per-node shuffle is marginally better** (904 vs 891 distinct on dense, 117 vs 108 on moderate). The difference is real but small — the global-order correlation creates mild bias.
2. **Merge had zero effect in all cases.** The backtracker with cycle-length pruning already finds Hamiltonian cycles on dense graphs. The merge step adds overhead with no benefit. This was surprising and changes the design.
3. **Very sparse graphs have non-uniform sampling.** With only 3 valid solutions, the global shuffle sampled them at (502, 277, 221) frequency — far from the ideal (333, 333, 333). Per-node was (514, 247, 239) — marginally better but still skewed. For near-exhaustible solution spaces, systematic enumeration is needed.
4. **Collision rate is a reliable exhaustion signal.** The adaptive strategy (stop after 5 consecutive collisions) found 1831/56/3 distinct solutions on dense/moderate/sparse graphs respectively, all with minimal wasted attempts.

### 2.3 Core Insight: Merge Is Redundant in the Common Case

The most important finding from the experiments: **on dense graphs, the backtracker with cycle-pruning constraints already finds Hamiltonian cycles directly.** The post-merge step never fires because the initial solution is already optimal.

This occurs because:

- On a dense graph, the backtracker has many valid choices at each step
- The cycle-premature-close pruning prevents short cycles from forming
- As a result, the backtracker naturally "falls into" Hamiltonian cycles when they are plentiful

Merge is only needed for sparse graphs where the backtracker exhausts some branches and settles for a multi-cycle solution. But merge is incomplete on exactly those graphs (as shown in 2.1). This makes merge a poor fit as a correctness mechanism.

---

## 3. Revised Algorithm Recommendation

The experimental results change the recommended architecture substantially.

### 3.1 Primary Algorithm: Hamiltonian DFS

Instead of "find any valid cycle cover, then post-process to improve it," the primary algorithm directly searches for a Hamiltonian cycle using depth-first search with backtracking. The cycle-length constraint is applied **during** the search, not after.

```
HamiltonianDFS(graph, minCycleLen, rng):
  start = 0  (Hamiltonian cycle is rotation-invariant; fix the start)
  path = [0]
  visited = {0}

  DFS():
    if len(path) == n:
      if edge(path[-1] → path[0]) exists:
        if len(path) >= minCycleLen:  // only meaningful if minCycleLen == n
          return path as assignment
      return FAIL

    last = path[-1]
    for neighbor in shuffle(graph.adj[last], rng):  // per-node shuffle
      if neighbor not in visited:
        path.append(neighbor)
        visited.add(neighbor)
        if DFS() succeeds: return SUCCESS
        path.pop()
        visited.remove(neighbor)

    return FAIL
```

**Why this is better than backtracking + merge**:

- It searches for the global optimum directly, not through local search
- No merge step → no incompleteness risk
- If it succeeds, the result is guaranteed Hamiltonian (optimal)
- If it fails (no Hamiltonian cycle), we know definitively and can fall back

**Performance**: For n ≤ 30 on dense graphs, this is extremely fast in practice. The DFS with pruning visits a small fraction of possible paths before finding a Hamiltonian cycle. The per-node shuffle ensures different runs explore different paths.

### 3.2 Fallback Algorithm: Constrained Backtracking

If no Hamiltonian cycle exists (DFS exhausts the search space), fall back to finding the best achievable cycle cover. This uses the same backtracking structure but relaxes the cycle requirement:

```
ConstrainedBacktrack(graph, minCycleLen, rng):
  assign = [-1] * n
  gifterOrder = [0..n-1]  // fixed order (not shuffled — no benefit here)

  Recurse(gifter):
    if gifter == n: return SUCCESS (all assigned)

    for recipient in shuffle(graph.adj[gifter], rng):
      if recipient already used: continue
      if wouldClosePrematureCycle(assign, gifter, recipient, minCycleLen, n): continue
      assign[gifter] = recipient
      if Recurse(gifter + 1): return SUCCESS
      assign[gifter] = -1

    return FAIL
```

`wouldClosePrematureCycle`: Follow the chain from `recipient` through `assign`. If it loops back to `gifter` before all n participants are assigned, it would close a premature cycle — reject it if the cycle length is less than `minCycleLen` OR if not all participants are included.

The result is a valid cycle cover where all cycles have length ≥ `minCycleLen`. It won't be Hamiltonian (by definition, since the primary DFS already confirmed that's impossible), but it's the best achievable structure.

**Note**: Cycle-merge can still be applied as an optional improvement step here, with the understanding that it may not find the global optimum. The LOCAL_OPTIMUM result should trigger a retry with a new seed rather than being returned as-is.

### 3.3 Solution Enumeration

To collect multiple diverse solutions:

```
Solve(problem, options):
  graph = buildGraph(problem)
  rng = newRNG(options.seed)
  results = []
  seen = {}
  consecutiveCollisions = 0
  COLLISION_THRESHOLD = 5

  while len(results) < options.maxSolutions:
    // Try primary algorithm
    assign = HamiltonianDFS(graph, minCycleLen, rng)

    if assign == nil:
      // No Hamiltonian cycle: use fallback once and break the loop
      assign = ConstrainedBacktrack(graph, minCycleLen, rng)
      if assign != nil:
        results = [solutionFromAssign(assign)]
      break  // Can't enumerate multiple Hamiltonians if none exist

    canon = canonicalize(assign)
    if seen[canon]:
      consecutiveCollisions++
      if consecutiveCollisions >= COLLISION_THRESHOLD:
        break  // solution space near-exhausted; stop
    else:
      seen[canon] = true
      consecutiveCollisions = 0
      results.append(solutionFromAssign(assign))

    // Reseed RNG to get a different DFS path next call
    rng.advance()

  return rankByScore(results)
```

**Canonicalization** (for deduplication): normalize each cycle to start at its lexicographically smallest element, then sort cycles by first element. Two permutations are identical iff their canonical forms match. Critically, this is **directed** — `A→B→C→A` and `B→C→A→B` are the same, but `A→C→B→A` is different.

**Adaptive stopping**: the 5-consecutive-collision threshold was validated experimentally:

- Dense graph: found 1831 distinct solutions before triggering (2255 total attempts)
- Moderate graph: found 56 distinct solutions (82 attempts)
- Very sparse graph: found all 3 solutions (8 attempts)

---

## 4. Complexity Analysis

For n ≤ 30 (typical gift exchange):

| Operation                 | Complexity       | Practical timing (est.) |
| ------------------------- | ---------------- | ----------------------- |
| Build valid-pairing graph | O(n²)            | < 1ms                   |
| Hamiltonian DFS (dense)   | O(n) amortized   | < 1ms per call          |
| Hamiltonian DFS (sparse)  | O(n!) worst case | < 100ms for n=30        |
| Constrained backtrack     | O(n!) worst case | same                    |
| Canonicalize              | O(n log n)       | negligible              |
| Full solve (k solutions)  | O(k · n) typical | < 10ms                  |

Worst-case behavior only manifests on pathologically sparse graphs. For all known real gift exchange instances (the existing participant/relationship data), the dense-graph fast path applies.

---

## 5. Implementation Plan

Phase 1 produces no production code. Its deliverables are:

1. **This document** (finalized after experiments)
2. **`experiments/merge_completeness/`**: confirms merge is incomplete — done
3. **`experiments/shuffle_diversity/`**: confirms per-node shuffle is marginally better, validates adaptive stopping — done
4. **Open design questions answered** (see below)

### Resolved Design Questions

| Question                                | Decision                                                                                                                                                                   |
| --------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Primary algorithm                       | Hamiltonian DFS (not backtrack + merge)                                                                                                                                    |
| Merge step                              | Optional safety net on fallback path only, not primary                                                                                                                     |
| Shuffle strategy                        | Per-node (each gifter shuffles its own adjacency list)                                                                                                                     |
| Multiple solutions                      | Random restarts with canonicalized deduplication                                                                                                                           |
| Sparse graph fallback                   | Constrained backtracking + collision-rate adaptive stop                                                                                                                    |
| Systematic enumeration                  | Triggered when ≥5 consecutive collisions                                                                                                                                   |
| MinCycleLen as user parameter           | **Removed.** Solver uses automatic N/M progression: try N, N/2, N/3, ... until a solution is found or the target drops to 1 (infeasible). No user-facing parameter needed. |
| Score ranking for mixed-quality results | **Always rank by Score.** Hamiltonian cycles naturally float to the top (Score.MinCycleLen = n). The UI presents solutions ranked best-first without special-casing.       |

---

## 6. Key Snippets

### Hamiltonian DFS with per-node shuffle

```go
func hamiltonianDFS(g *graph, rng *rand.Rand) ([]int, bool) {
    assign := make([]int, g.n)
    for i := range assign {
        assign[i] = -1
    }
    visited := make([]bool, g.n)

    // Fix start node — Hamiltonian cycle is rotation-invariant
    path := []int{0}
    visited[0] = true

    var dfs func() bool
    dfs = func() bool {
        if len(path) == g.n {
            last := path[len(path)-1]
            if g.isEdge(last, 0) {
                for i := 0; i < len(path)-1; i++ {
                    assign[path[i]] = path[i+1]
                }
                assign[path[len(path)-1]] = 0
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

### wouldClosePrematureCycle (for constrained backtracking fallback)

```go
func wouldClosePrematureCycle(assign []int, gifter, recipient, minLen, total int) bool {
    // Count currently assigned gifters (+1 for the edge we're about to add)
    assigned := 1
    for _, v := range assign {
        if v >= 0 {
            assigned++
        }
    }

    length := 1
    cur := recipient
    for {
        next := assign[cur]
        if next < 0 {
            return false // open chain — no cycle yet
        }
        length++
        if next == gifter {
            // Would close a cycle of `length`.
            // Premature if: cycle is too short OR not all participants are included.
            return length < minLen || assigned < total
        }
        cur = next
    }
}
```

### Canonical form for deduplication

```go
func canonicalize(assign []int) string {
    cycles := decomposeCycles(assign)
    parts := make([]string, len(cycles))
    for i, c := range cycles {
        // Rotate to start at smallest element
        minIdx := 0
        for j, v := range c {
            if v < c[minIdx] {
                minIdx = j
            }
        }
        rotated := append(append([]int{}, c[minIdx:]...), c[:minIdx]...)
        strs := make([]string, len(rotated))
        for j, v := range rotated {
            strs[j] = fmt.Sprintf("%d", v)
        }
        parts[i] = strings.Join(strs, "→")
    }
    sort.Strings(parts)
    return strings.Join(parts, "|")
}
```

### Score comparison

```go
type Score struct {
    MinCycleLen int // primary: maximize
    NumCycles   int // secondary: minimize
    MaxCycleLen int // tertiary: maximize
}

func (s Score) Better(other Score) bool {
    if s.MinCycleLen != other.MinCycleLen {
        return s.MinCycleLen > other.MinCycleLen
    }
    if s.NumCycles != other.NumCycles {
        return s.NumCycles < other.NumCycles
    }
    return s.MaxCycleLen > other.MaxCycleLen
}

func scoreOf(assign []int) Score {
    cycles := decomposeCycles(assign)
    min, max := len(assign), 0
    for _, c := range cycles {
        if len(c) < min { min = len(c) }
        if len(c) > max { max = len(c) }
    }
    return Score{MinCycleLen: min, NumCycles: len(cycles), MaxCycleLen: max}
}
```

---

## Summary

The gift exchange problem is a **directed cycle cover optimization** problem on a typically dense graph. Empirical experiments revised the planned algorithm:

| Aspect             | Originally planned         | Revised after experiments               |
| ------------------ | -------------------------- | --------------------------------------- |
| Primary algorithm  | Backtracking + cycle-merge | **Hamiltonian DFS**                     |
| Merge step         | Core optimization          | Safety net only (incomplete)            |
| Shuffle            | Global order               | **Per-node** (marginally better)        |
| Multiple solutions | Random restarts            | Random restarts + collision-rate cutoff |
| Fallback           | Bipartite matching         | Constrained backtracking                |

The Hamiltonian DFS approach is simpler, more direct, and avoids the incompleteness problem of greedy 2-opt merge entirely. On dense graphs (the typical case), it finds Hamiltonian cycles with negligible search effort.
