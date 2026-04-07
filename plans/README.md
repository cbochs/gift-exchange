# Gift Exchange Web Application — High-Level Plan

## Status

- [x] **Phase 1 — Problem Exploration**: Complete. Algorithm chosen (Hamiltonian DFS + N/M progression). All design questions resolved. Experiments in `experiments/`.
- [x] **Phase 2 — Core Library**: Complete. All tests pass; `go vet` and `staticcheck` clean.
- [x] **Phase 3 — CLI**: Complete. `solve`, `validate`, `analyze` subcommands; round-trip verified.
- [x] **Phase 4 — Web Backend**: Complete. `POST /api/v1/solve`, `GET /api/v1/health`, CORS middleware, 10 handler tests passing.
- [x] **Phase 5 — Web Frontend**: Complete. Vanilla JS/D3 two-panel UI; force-directed graph; solution tabs; JSON import/export.
- [x] **Phase 6 — UI Polish**: Bug fixes (solution display, collapsible sections), symmetric relationships, "Add as history blocks", mobile layout.
- [x] **Phase 7 — Deployment**: `go:embed` static assets, Docker multi-stage build, Helm chart, Forward Auth proxy.
- [x] **Phase 8 — Refactoring & Code Quality**: Structural improvements before feature work: solver abstraction, shared DTOs, project layout, error types, constants, context cancellation.
- [x] **Phase 10 — State Persistence & Link Sharing**: LocalStorage save/load, reset button, compact URL hash encoding, "Copy Link" button, shared-link banner. Frontend-only.
- [x] **Phase 11 — UI Polish**: Solutions panel two-row layout, D3 zoom/pan, disconnected-graph gravity fix, 20-person participant cap. Frontend-only.

## Future Work

- **Required Assignments** (`plans/phase9-required.md`): Force specific gifter→recipient pairs in every solution. Full-stack feature spanning lib, server, and frontend.
- **UI Polish** (`plans/phase11-ui-polish.md`): Solutions panel two-row layout, D3 zoom/pan, disconnected-graph gravity fix, 20-person participant cap. Frontend-only.
- [x] **Phase 12 — Rename & Block Groups**: Participant rename (stable IDs), block groups with collapsible UI, history blocks create named groups, URL hash v2 format. Frontend-only.

---

## Vision

A stateless web application for creating optimized gift exchanges. Given a list of participants and optional constraints (blocked pairings), the system finds one or more valid assignments that maximize cycle size in the participant network — ideally a single Hamiltonian cycle visiting every participant, or the closest approximation possible under the given constraints.

## The Problem in One Sentence

Find a permutation of participants (everyone gives exactly one gift, everyone receives exactly one gift) where no constraint is violated, all cycles are as long as possible, and the number of cycles is minimized.

---

## Architecture

The system is organized as three distinct tiers sharing a well-defined contract:

```
┌─────────────────────────────────────────────────────┐
│                   Frontend (HTML/JS)                 │
│   Form → Graph Visualization → JSON Import/Export    │
└────────────────────┬────────────────────────────────┘
                     │ HTTP JSON API
┌────────────────────▼────────────────────────────────┐
│               Web Backend (Go/net/http)              │
│         Stateless handler, no DB dependency          │
└────────────────────┬────────────────────────────────┘
                     │ function call
┌────────────────────▼────────────────────────────────┐
│           Gift Exchange Library (Go package)         │
│   Graph construction, solver, cycle optimization     │
└─────────────────────────────────────────────────────┘
```

The library is independently usable from both the CLI and the web backend with no coupling between the two consumers.

---

## Technology Stack

| Layer    | Technology                    | Rationale                                 |
| -------- | ----------------------------- | ----------------------------------------- |
| Library  | Go (stdlib only)              | Fast, testable, no runtime deps           |
| CLI      | Go (`cobra` or stdlib `flag`) | Direct library consumer, no HTTP overhead |
| Backend  | Go `net/http`                 | Stateless, no framework needed            |
| Frontend | Vanilla HTML + CSS + JS       | Minimal dependency surface                |
| Graph UI | D3.js (CDN)                   | Mature force-directed graph visualization |

---

## Directory Structure

```
gift-exchange/
├── plans/                        # This planning document and phase plans
├── lib/                          # Core gift exchange library (standalone package)
│   ├── types.go                  # Participant, Block, Assignment, Solution, Score
│   ├── graph.go                  # Valid-pairing graph construction
│   ├── solver.go                 # Backtracking solver with cycle optimization
│   ├── score.go                  # Objective function and solution ranking
│   └── solver_test.go            # All library tests
├── cmd/
│   └── giftexchange/
│       └── main.go               # CLI entry point
├── server/
│   ├── main.go                   # HTTP server entry point
│   ├── handlers.go               # Request handlers
│   └── api.go                    # Request/response types (shared API contract)
├── web/
│   ├── index.html
│   ├── app.js
│   └── style.css
├── go.mod
└── go.sum
```

---

## API Contract (Summary)

```
POST /api/v1/solve     — Submit participants + constraints, receive ranked solutions
GET  /api/v1/health    — Liveness check
```

Full schema defined in `server/api.go` and detailed in Phase 4.

---

## Phases

| Phase | Title                | Goal                                                                    |
| ----- | -------------------- | ----------------------------------------------------------------------- |
| 1     | Problem Exploration  | Formalize the problem; identify algorithms and objective fn             |
| 2     | Core Library         | Implement and test the solver library                                   |
| 3     | CLI                  | Expose the library via a usable command-line interface                  |
| 4     | Web Backend          | Wrap the library in a stateless HTTP API                                |
| 5     | Web Frontend         | Build the presentation layer with graph visualization                   |
| 6     | UI Polish            | Bug fixes, symmetric relationships, history blocks, mobile layout       |
| 7     | Deployment           | Containerize and deploy to Kubernetes via Helm behind Forward Auth      |
| 8     | Refactoring          | Solver abstraction, shared DTOs, project layout, error types, constants |
| 9     | Required Assignments | Force specific gifter→recipient pairs in every solution                 |

Each phase builds on the previous. Phases 3 and 4 can overlap once the library API stabilizes.

---

## Working Norms

1. **Documentation stays current.** When experiments or implementation reveal insights that contradict or refine the plan, update the relevant plan document before moving on. Plans are living documents, not historical artifacts.
2. **Tripwire on design decisions.** If a design choice is required during experimentation or implementation that isn't already resolved in the plan, stop and ask rather than improvise. Undocumented decisions become silent technical debt.

---

## Key Design Principles

1. **Library first** — All algorithm logic lives in `lib/`. The CLI and server are thin wrappers.
2. **Stateless** — No database, no session state. Each `/solve` request is self-contained.
3. **Test-driven** — Library tests are written alongside (or before) implementation. The solver must be verifiably correct before the CLI or server is built.
4. **Reproducible** — A solution is fully reproducible from the JSON export (participants + blocks + seed + selected solution index).
5. **Multiple solutions** — The solver returns the top-k solutions ranked by score, giving users a choice.
