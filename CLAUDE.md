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

**Docs to keep current after every session:**

- `CLAUDE.md` — active phase, last session summary, next action, codebase tree
- `plans/README.md` — phase checklist (`[ ]` → `[x]` when complete)
- `plans/phase<N>-*.md` — status checklist at the top of the active phase
- `README.md` — user-facing feature list (update when UI features are added)

---

## Current State (update this each session)

**Active phase**: Phase 8 — Required Assignments.
**Last session**: Completed Phase 7. Moved `web/` to `server/web/`; added `go:embed` (`server/static.go`); server now serves embedded assets by default and reads config from `GIFT_EXCHANGE_*` env vars with flag override. Build and publish via Dagger (Dang SDK): `Container`/`Serve`/`Publish` functions in `.dagger/main.dang`; multi-arch (amd64 + arm64) publish to `ghcr.io/cbochs/gift-exchange`. Dockerfile and helm chart were drafted then removed in favor of Dagger.
**Next action**: Phase 8 — read phase8-required.md plan before starting.

---

## Codebase State

```
gift-exchange/
├── CLAUDE.md                  ← this file
├── README.md                  ← user-facing readme (brief)
├── go.mod                     ← module: github.com/cbochs/gift-exchange
├── giftexchange.py            ← original Python implementation (reference only)
├── participants.json          ← sample data: 22 participants, two groups
├── relationships.json         ← sample data: relationship blocks
├── history.json               ← sample data: 10 years of historical pairings
├── lib/                       ← COMPLETE — core solver library (stdlib only)
│   ├── types.go               ← public types + ErrInfeasible
│   ├── graph.go               ← buildGraph, isEdge, shuffled
│   ├── score.go               ← decomposeCycles, canonicalize, scoreOf, Score.Better
│   ├── analyze.go             ← Analyze (exported) — graph stats + Hamiltonian check
│   ├── solver.go              ← Validate (exported), hamiltonianDFS, constrainedBacktrack, Solve
│   └── solver_test.go         ← unit + integration + property tests (all passing)
├── cmd/
│   └── giftexchange/          ← COMPLETE — CLI thin wrapper around lib
│       ├── main.go            ← run(args, stdin, stdout, stderr); solve/validate/analyze subcommands
│       └── main_test.go       ← integration tests (all passing)
├── server/                    ← COMPLETE — HTTP server (package main)
│   ├── api.go                 ← DTO types: SolveRequest, SolveResponse, ErrorResponse, etc.
│   ├── handlers.go            ← solveHandler, healthHandler, corsMiddleware, dtoToProblem
│   ├── main.go                ← flags + GIFT_EXCHANGE_* env vars; embedded asset serving
│   ├── static.go              ← go:embed web
│   ├── handlers_test.go       ← 10 handler tests using httptest (all passing)
│   └── web/                   ← embedded frontend assets (moved from root web/)
├── dagger.json                ← Dagger module root; declares "go" local dependency
├── .dagger/                   ← COMPLETE — Dagger build pipeline (Dang SDK)
│   ├── config.toml
│   ├── main.dang              ← GiftExchange type: Container, Serve, Publish
│   └── modules/go/
│       └── main.dang          ← Go type: Build (cross-compile, pinned golang:1.26 digest)
├── plans/
│   ├── README.md              ← high-level plan + phase status checklist
│   ├── phase1-problem-exploration.md  ← COMPLETE
│   ├── phase2-library.md      ← COMPLETE
│   ├── phase3-cli.md          ← COMPLETE
│   ├── phase4-web-backend.md  ← COMPLETE
│   ├── phase5-web-frontend.md ← COMPLETE
│   ├── phase6-polish.md       ← COMPLETE
│   ├── phase7-deployment.md   ← COMPLETE
│   └── phase8-required.md     ← PLANNED — required assignments (full-stack)
└── experiments/
    ├── go.mod                 ← imports root module via replace directive
    ├── merge_completeness/    ← proves greedy 2-opt merge is incomplete
    ├── shuffle_diversity/     ← compares global vs per-node shuffle strategies
    └── cousins_2026/          ← real-data run: 15 cousins, 2019–2025 history blocks
        ├── main.go            ← Go experiment
        ├── cousins_2026.json  ← web-importable input (105 blocks: partners+siblings+2019-2025 history)
        └── verify.py          ← verification script: checks solutions against relationships+history
```

**The Python file is reference only — do not modify it.**

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
