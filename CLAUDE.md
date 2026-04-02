# Claude Session Orientation

> **Keep this file up-to-date.** After each session, update the "Current State" and
> "Last Completed" sections before closing. This is the primary document for
> re-orienting in a new session.

---

## What This Project Is

A stateless web application for creating optimized gift exchanges. Given a list of
participants and optional blocked pairings, the solver finds one or more valid
assignments where everyone gives and receives exactly one gift, maximizing the
length of cycles in the participant graph (ideally one Hamiltonian cycle covering
everyone). Returns multiple ranked solutions.

**Stack**: Go library + Go HTTP server + Vanilla JS/D3 frontend.

---

## How to Get Up to Speed

1. **Read `plans/README.md`** — architecture overview, tech stack, phase status checklist, working norms (documentation currency + tripwire policy)
2. **Read the plan for the current/next phase** (see "Current State" below)
3. **Read the plan for the most recently completed phase** for context
4. Skim `experiments/` to understand empirical basis for algorithm decisions

Do not start any implementation work before reading the relevant phase plan.

---

## Current State (update this each session)

**Active phase**: Phase 3 — CLI (not yet started)
**Last session**: Completed Phase 2. Implemented full Go library (`lib/`): types, graph, score, solver. All tests pass; `go vet` and `staticcheck` clean. One plan correction: `wouldClosePrematureCycle` uses `length < minLen` not `|| assigned < total` (the `||` form blocks multi-cycle solutions).
**Next action**: Read `plans/phase3-cli.md` and begin Phase 3.

---

## Codebase State

```
gift-exchange/
├── CLAUDE.md                  ← this file
├── README.md                  ← user-facing readme (brief)
├── giftexchange.py            ← original Python implementation (reference only)
├── participants.json          ← sample data: 22 participants, two groups
├── relationships.json         ← sample data: relationship blocks
├── history.json               ← sample data: 10 years of historical pairings
├── plans/
│   ├── README.md              ← high-level plan + phase status checklist
│   ├── phase1-problem-exploration.md  ← COMPLETE
│   ├── phase2-library.md      ← ready to implement (TDD)
│   ├── phase3-cli.md          ← planned
│   ├── phase4-web-backend.md  ← planned
│   └── phase5-web-frontend.md ← planned
└── experiments/
    ├── go.mod
    ├── merge_completeness/main.go   ← proves greedy 2-opt merge is incomplete
    └── shuffle_diversity/main.go    ← compares global vs per-node shuffle strategies
```

**No Go library, CLI, server, or frontend code exists yet.** The Python file is
reference only — do not modify it.

---

## Key Algorithm Decisions (Phase 1 output)

These are settled. Do not re-open them without flagging to the user.

| Decision                 | Choice                                                        | Rationale                                         |
| ------------------------ | ------------------------------------------------------------- | ------------------------------------------------- |
| Primary solver           | Hamiltonian DFS (fix start node, per-node shuffled adj lists) | Direct, complete, no merge needed                 |
| Fallback solver          | Constrained backtracking with `wouldClosePrematureCycle`      | Used only when no Hamiltonian cycle exists        |
| Cycle target progression | N/M: try N, N/2, N/3, ... until target ≤ 1 (infeasible)       | Automatic; no user-facing `minCycleLen` parameter |
| Multiple solutions       | Random restarts with directed canonical deduplication         | Stop at ≥5 consecutive collisions                 |
| Shuffle strategy         | Per-node (each gifter shuffles its own adjacency list)        | Marginally more diverse than global shuffle       |
| Greedy cycle-merge       | Removed from primary path (proven incomplete)                 | Kept as optional safety net only                  |
| Score ranking            | Lexicographic: MaxMinCycleLen → MinNumCycles → MaxMaxCycleLen | Hamiltonian cycles always score highest           |
| User-facing API          | No `minCycleLen` input anywhere (lib, CLI, HTTP, frontend)    | N/M progression is internal                       |

---

## Working Norms

1. **Documentation stays current.** When implementation reveals something that
   contradicts or refines a plan, update the plan document before proceeding.
2. **Tripwire on design decisions.** If a design choice is required that isn't already
   resolved in the plan, stop and ask rather than improvise.
3. **Test-driven.** Write `solver_test.go` before `solver.go`. Tests must pass before
   moving to the next phase.
4. **Library first.** All algorithm logic lives in `lib/`. CLI and server are thin
   wrappers. Do not put business logic in handlers or CLI code.
5. **Commit early and often.** Use conventional commit format (`feat:`, `fix:`,
   `test:`, `chore:`, etc.) with a body that explains what changed and why. Each
   logical unit of work (a new file, a passing test suite, a bug fix) warrants its
   own commit.

---

## Phase 2 Starting Checklist

When beginning Phase 2, complete in this order:

1. `go mod init github.com/[user]/gift-exchange` in repo root
2. Write `lib/types.go` (types + `Solve` stub that returns `ErrInfeasible`)
3. Write `lib/score.go` (`decomposeCycles`, `canonicalize`, `scoreOf`, `Score.Better`)
4. Write `lib/graph.go` (`buildGraph`, `isEdge`, `shuffled`)
5. Write `lib/solver_test.go` (all tests — they should fail at this point)
6. Write `lib/solver.go` until all tests pass
7. Run `go vet ./...` and `staticcheck ./...` — zero warnings
8. Verify `TestSolve_MergeCounterexample` passes (the 6-node graph from experiments)

The counterexample graph (from `experiments/merge_completeness/main.go`):

```
adj = [[1,5], [0,5], [3,0,4], [2,0,1], [5,3], [4]]
Only one Hamiltonian cycle: 0→1→5→4→3→2→0
```

The solver must find it via Hamiltonian DFS, not via cycle-merge.
