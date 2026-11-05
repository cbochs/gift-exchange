# Phase 8 — Refactoring & Code Quality

## Status

- [x] **R1** — eliminate double graph construction in `validate()` + `Solve()`
- [x] **R2** — solver abstraction (`solverFunc`) + context cancellation
- [x] **R3** — define `ErrInvalid` sentinel; remove `isValidationErr` negative check
- [x] **R4** — `internal/dto`: shared wire types + lib mappings with tests
- [x] **R5** — move server entrypoint to `cmd/server/`; `server/` becomes `package server`
- [x] **R6** — define and export lib constants; pull infrastructure constants to one site
- [x] **R7** — centralize seed resolution (three independent sites today)
- [x] **R8** — add `http.Server` transport timeouts (slow-loris defense)
- [x] **R9** — adopt Go 1.22 method-based mux routing; drop empty `handler` struct
- [x] **R10** — standardize slice APIs (`sort.Slice` → `slices.SortFunc`, etc.)
- [x] **R11** — Dagger pipeline: `go vet`, `go test -race`, `-ldflags="-s -w" -trimpath`
- [x] **R12** — fuzz test `Solve`

______________________________________________________________________

## Goal

Address accumulated technical debt before Phase 8's full-stack feature lands. All
changes are non-functional (no observable behavior change for users) but improve
correctness, testability, and idiomatic Go. Each item is independently committable.

The issues below are organized by severity: **High** (correctness or safety risk),
**Medium** (structural debt that compounds as features are added), **Low**
(style / micro-optimization).

______________________________________________________________________

## High Priority

### R1 — Double graph construction (`lib/solver.go`)

**Problem.** `validate()` builds the constraint graph once to check Hall's condition,
then throws it away. `Solve()` calls `validate()` then immediately builds the graph
again.

```go
// validate() — current line 94
g := buildGraph(p.Participants, p.Blocks)  // ← allocated here, then discarded

// Solve() — current line 33
g := buildGraph(p.Participants, p.Blocks)  // ← allocated again
```

**Fix.** Split validation into two phases:

1. `validateStructural(p Problem) error` — checks participant count, duplicate IDs,
   and block participant references. No graph needed. This is what the exported
   `Validate` (CLI's `validate` subcommand) actually needs.
1. `checkHall(g *graph) error` — Hall's condition check on an already-built graph.
   Called from `Solve()` after `buildGraph`, on the same graph the solver will use.

```
Validate (exported):  validateStructural → buildGraph → checkHall
Solve (exported):     validateStructural → buildGraph → [applyRequired] → checkHall → solve
```

This split also cleanly accommodates Phase 8's design: `applyRequiredConstraints`
runs between `buildGraph` and `checkHall` inside `Solve`, with no changes to either
validation function.

______________________________________________________________________

### R2 — Solver abstraction + context cancellation (`lib/solver.go`, `lib/analyze.go`)

#### 2a — Define a `solverFunc` type

**Problem.** `hamiltonianDFS` and `constrainedBacktrack` have incompatible signatures
(the latter takes `minCycleLen`), so `collectSolutions` selects between them with an
inline `if`. This is a closed dispatch: adding a third solver requires editing
`collectSolutions` directly.

**Fix.** Introduce a `solverFunc` type that both solvers satisfy:

```go
// solverFunc attempts to find a valid assignment for the given graph.
// Returns (assign, true) on success, (nil, false) if no solution exists at
// this constraint level. Must respect ctx cancellation.
type solverFunc func(ctx context.Context, g *graph, rng *rand.Rand) ([]int, bool)
```

`hamiltonianDFS` is adapted to this signature directly:

```go
func hamiltonianSolver(ctx context.Context, g *graph, rng *rand.Rand) ([]int, bool)
```

`constrainedBacktrack` takes an extra parameter (`minCycleLen`), so it is wrapped by
a factory function:

```go
func constrainedSolver(minCycleLen int) solverFunc {
    return func(ctx context.Context, g *graph, rng *rand.Rand) ([]int, bool) {
        return constrainedBacktrack(ctx, g, rng, minCycleLen)
    }
}
```

`collectSolutions` becomes:

```go
func collectSolutions(ctx context.Context, solver solverFunc, g *graph, seed int64, max int) []Solution
```

`Solve` selects the solver before calling `collectSolutions`:

```go
var solver solverFunc
if target == g.n {
    solver = hamiltonianSolver
} else {
    solver = constrainedSolver(target)
}
solutions := collectSolutions(ctx, solver, g, seed, opts.MaxSolutions)
```

`solverFunc` is unexported — it is an internal algorithm contract, not part of the
public API.

#### 2b — Context cancellation inside DFS

**Problem.** Neither `hamiltonianDFS` nor `constrainedBacktrack` check for context
cancellation. A single recursive call can run far past the request timeout, preventing
clean shutdown. `Analyze` runs `hamiltonianDFS` in a goroutine and leaks it when the
context is canceled.

**Fix.** Add a periodic context check inside the recursive DFS using a bitmask
counter — no channel, no atomic, negligible branch cost:

```go
func hamiltonianSolver(ctx context.Context, g *graph, rng *rand.Rand) ([]int, bool) {
    // ... existing setup ...
    var calls int
    var dfs func() bool
    dfs = func() bool {
        calls++
        if calls&0xFF == 0 && ctx.Err() != nil { // check every 256 calls
            return false
        }
        // ... existing DFS logic unchanged ...
    }
    // ...
}
```

The same pattern applies to `constrainedBacktrack`. With this change, `Analyze` no
longer needs a goroutine: it can call `hamiltonianSolver` directly on the calling
goroutine, and the solver will return promptly when `ctx` fires. The goroutine launch
and `select` in `Analyze` can be removed.

______________________________________________________________________

### R3 — Error types in `lib/` (`lib/types.go`)

**Problem.** The lib currently defines one sentinel:

```go
var ErrInfeasible = errors.New("no valid gift exchange is possible under the given constraints")
```

All other errors from `validate()` are bare `fmt.Errorf` strings. The server's
`isValidationErr` must infer "this is a validation error" by negation — any error
that is not `ErrInfeasible` is treated as a 400. This is fragile: a future context
cancellation error or internal error would be misclassified as 400 Bad Request.

**Fix.** Define a second sentinel in the lib:

```go
// ErrInvalid is returned when the Problem definition is structurally malformed:
// too few participants, duplicate IDs, or constraint references to unknown IDs.
// It is distinct from ErrInfeasible: an invalid problem has a definition error;
// an infeasible problem is well-formed but has no valid assignment.
var ErrInvalid = errors.New("invalid problem")
```

All errors from `validateStructural` wrap `ErrInvalid`:

```go
return fmt.Errorf("%w: duplicate participant ID: %q", ErrInvalid, part.ID)
```

`ErrInfeasible` is returned as-is (not wrapped) from `checkHall` and `Solve`. It
remains a pure sentinel.

The server's handler then has a correct three-way dispatch:

```go
switch {
case errors.Is(err, ge.ErrInvalid):
    writeError(w, http.StatusBadRequest, err.Error())
case errors.Is(err, ge.ErrInfeasible):
    writeError(w, http.StatusUnprocessableEntity, err.Error())
default:
    writeError(w, http.StatusInternalServerError, "internal error")
}
```

`isValidationErr` is deleted.

**Summary of lib error contract:**

| Error | Meaning | HTTP status |
| -- | -- | -- |
| `ErrInvalid` (wrapped) | Problem definition is malformed | 400 |
| `ErrInfeasible` (sentinel) | No valid assignment exists | 422 |
| `context.DeadlineExceeded` | Solver timed out | 504 |
| `context.Canceled` | Request canceled | 499 / drop |

No additional custom error types are needed. Context errors are stdlib; all other
outcomes are covered by the two sentinels.

______________________________________________________________________

## Medium Priority

### R4 — `internal/dto`: shared wire types + lib mappings (`internal/dto/`)

**Problem.** The lib types (`ge.Participant`, `ge.Block`, `ge.Solution`, etc.) carry
JSON tags and are used directly as the CLI's JSON schema. The server adds a separate
DTO layer (`ParticipantDTO`, `BlockDTO`, etc.) that mirrors these types
field-for-field, with mechanical mapping code in `dtoToProblem` and
`solutionsToDTOs`. Adding Phase 8's `Required` field requires changes in 4 places
instead of 2.

**Fix.** Create `internal/dto/` as the single authoritative location for wire types
and their mappings:

```
internal/
  dto/
    types.go       ← shared DTO types (ParticipantDTO, BlockDTO, AssignmentDTO,
                     ScoreDTO, SolutionDTO)
    mapping.go     ← ToLib / FromLib conversion functions
    mapping_test.go ← roundtrip tests
```

The lib types lose their JSON tags (they become pure domain/algorithm types with no
serialization concerns).

**`internal/dto/types.go`** — types shared across CLI and server:

```go
package dto

type ParticipantDTO struct {
    ID   string `json:"id"`
    Name string `json:"name"`
}

type BlockDTO struct {
    From string `json:"from"`
    To   string `json:"to"`
}

type AssignmentDTO struct {
    GifterID    string `json:"gifter_id"`
    RecipientID string `json:"recipient_id"`
}

type ScoreDTO struct {
    MinCycleLen int `json:"min_cycle_len"`
    NumCycles   int `json:"num_cycles"`
    MaxCycleLen int `json:"max_cycle_len"`
}

type SolutionDTO struct {
    Assignments []AssignmentDTO `json:"assignments"`
    Cycles      [][]string      `json:"cycles"`
    Score       ScoreDTO        `json:"score"`
}
```

**`internal/dto/mapping.go`** — conversion functions:

```go
package dto

import ge "github.com/cbochs/gift-exchange/lib"

func ParticipantToLib(d ParticipantDTO) ge.Participant { ... }
func ParticipantsToLib(ds []ParticipantDTO) []ge.Participant { ... }
func BlockToLib(d BlockDTO) ge.Block { ... }
func BlocksToLib(ds []BlockDTO) []ge.Block { ... }
func SolutionFromLib(s ge.Solution) SolutionDTO { ... }
func SolutionsFromLib(ss []ge.Solution) []SolutionDTO { ... }
```

**`server/api.go`** retains the HTTP-specific envelope types (not shared):

```go
package server

import "github.com/cbochs/gift-exchange/internal/dto"

type SolveRequest struct {
    Participants []dto.ParticipantDTO `json:"participants"`
    Blocks       []dto.BlockDTO       `json:"blocks,omitempty"`
    Required     []dto.BlockDTO       `json:"required,omitempty"`
    Options      OptionsDTO           `json:"options,omitempty"`
}

type OptionsDTO struct {
    MaxSolutions int   `json:"max_solutions,omitempty"`
    Seed         int64 `json:"seed,omitempty"`
    TimeoutMs    int   `json:"timeout_ms,omitempty"`
}

type SolveResponse struct {
    Solutions []dto.SolutionDTO `json:"solutions"`
    Feasible  bool              `json:"feasible"`
    SeedUsed  int64             `json:"seed_used"`
}

type ErrorResponse struct {
    Error string `json:"error"`
}
```

**`cmd/giftexchange/main.go`** imports from `internal/dto` for participant/block
types; keeps its own `inputDoc` struct for round-trip fields (`_solutions`,
`feasible`):

```go
type inputDoc struct {
    Participants []dto.ParticipantDTO `json:"participants"`
    Blocks       []dto.BlockDTO       `json:"blocks,omitempty"`
    Options      inputOptions         `json:"options"`
    // Round-trip only:
    Solutions []dto.SolutionDTO `json:"solutions,omitempty"`
    Feasible  *bool             `json:"feasible,omitempty"`
}
```

**`internal/dto/mapping_test.go`** — roundtrip tests:

```go
func TestParticipantRoundtrip(t *testing.T) {
    orig := ge.Participant{ID: "a", Name: "Alice"}
    if got := ParticipantToLib(ParticipantFromLib(orig)); got != orig {
        t.Errorf("roundtrip: got %v, want %v", got, orig)
    }
}
// ... similar for Block, Solution, Score
```

______________________________________________________________________

### R5 — Move server entrypoint to `cmd/server/` (`cmd/server/`, `server/`)

**Problem.** The server lives at `server/` as `package main`. This is non-standard
for a repo that already uses `cmd/<name>/` for binary entrypoints (the CLI). A
`package main` in `server/` can't be imported, which limits testability (though the
current tests work by being `package main` themselves).

**Recommended layout** (standard Go multi-binary structure):

```
cmd/
  giftexchange/       ← CLI entrypoint (unchanged)
    main.go
  server/             ← HTTP server entrypoint (new, thin wrapper)
    main.go           ← flags, os.Exit, http.ListenAndServe
server/               ← HTTP server package (package server, was package main)
  server.go           ← NewServer(cfg Config) http.Handler (was newServer, now exported)
  handlers.go         ← handlers (unchanged logic, no longer package main)
  api.go              ← SolveRequest, SolveResponse, OptionsDTO, ErrorResponse
  handlers_test.go    ← unchanged (stays in package server)
internal/
  dto/                ← (from R4)
lib/                  ← unchanged
```

`cmd/server/main.go` contains only what can't be tested — flag parsing, `os.Exit`,
`http.Server` construction, `ListenAndServe`:

```go
package main

import (
    "flag"
    "fmt"
    "net/http"
    "os"
    "time"

    "github.com/cbochs/gift-exchange/server"
)

func main() {
    var cfg server.Config
    flag.StringVar(&cfg.Addr, "addr", envOrDefault("ADDR", server.DefaultAddr), "listen address")
    // ... flags ...
    flag.Parse()

    srv := &http.Server{
        Addr:         cfg.Addr,
        Handler:      server.NewServer(cfg),
        ReadTimeout:  server.ReadTimeout,
        WriteTimeout: cfg.Timeout + server.WriteTimeoutBuffer,
        IdleTimeout:  server.IdleTimeout,
    }
    fmt.Fprintf(os.Stderr, "listening on %s\n", cfg.Addr)
    if err := srv.ListenAndServe(); err != nil {
        fmt.Fprintf(os.Stderr, "server error: %v\n", err)
        os.Exit(1)
    }
}
```

`server.NewServer` (exported) is the current `newServer` with `serverConfig` renamed
to `server.Config` (exported fields). The handler tests in `server/handlers_test.go`
require no changes — they call `server.NewServer` or the handler functions directly
within the package.

The Dagger build target changes from `./server` to `./cmd/server`.

**Alternative (pragmatic):** Keep `server/` as `package main`. Acceptable for a
project of this size; the `cmd/` convention is a preference, not a requirement. The
cost is the non-standard layout and a minor inconsistency with `cmd/giftexchange/`.
Document the choice in `CLAUDE.md` if kept.

______________________________________________________________________

### R6 — Define and export lib constants; pull infrastructure constants to one site

**Problem.** Magic numbers appear in multiple files with no shared source of truth:

| Value | Files |
| -- | -- |
| `MaxSolutions` default of `5` | `lib/solver.go`, `server/handlers.go`, `cmd/giftexchange/main.go` |
| `collisionThreshold` of `5` | `lib/solver.go` (already a `const`, but unexported and inline) |
| Body size limit `1<<20` (1 MB) | `server/handlers.go` |
| Default timeout `10 * time.Second` | `server/handlers.go` |

**Fix — lib constants (exported, because callers need to reference defaults):**

```go
// lib/types.go or lib/defaults.go

// DefaultMaxSolutions is the number of solutions Solve returns when
// Options.MaxSolutions is 0.
const DefaultMaxSolutions = 5
```

Callers use `ge.DefaultMaxSolutions` instead of their own magic `5`.

**Fix — algorithm constants (unexported, internal tuning):**

```go
// lib/solver.go
const collisionThreshold = 5  // already exists, keep unexported
```

**Fix — server infrastructure constants (unexported, one location):**

```go
// server/server.go (or server/config.go)

const (
    DefaultAddr         = ":8080"
    DefaultCORSOrigin   = "*"
    DefaultTimeout      = 15 * time.Second

    // http.Server transport layer constants
    ReadTimeout         = 5 * time.Second
    WriteTimeoutBuffer  = 5 * time.Second  // added to solver timeout for WriteTimeout
    IdleTimeout         = 60 * time.Second

    // Request body size limit
    maxRequestBodyBytes = 1 << 20 // 1 MB
)
```

Exported server constants (`DefaultAddr`, `DefaultTimeout`, `ReadTimeout`, etc.) are
referenced by `cmd/server/main.go` for flag defaults and `http.Server` construction.
`maxRequestBodyBytes` stays unexported (callers don't need it).

**Decision rationale:**

- Export a constant if a caller (outside the package) needs to reference it for
  defaults, display, or validation.
- Keep unexported if it is purely an internal implementation detail (algorithm
  tuning, one-use magic numbers that callers never see).

______________________________________________________________________

### R7 — Centralize seed resolution (three independent sites)

**Problem.** "If seed == 0, generate one" logic appears independently in:

1. `lib/solver.go:Solve()` — resolves seed if 0
1. `server/handlers.go:dtoToProblem()` — resolves seed before calling `Solve`
1. `cmd/giftexchange/main.go:cmdSolve()` — resolves seed before calling `Solve`

The handler and CLI pre-resolve so they can echo the seed back to the caller. `Solve`
also resolves defensively — making its internal resolution dead code on the server
and CLI paths.

**Fix.** `Solve` requires a non-zero seed. Document it:

```go
// Options controls solver behavior.
type Options struct {
    MaxSolutions int           // default: DefaultMaxSolutions
    Seed         int64         // required non-zero; use NewSeed() if random
    Timeout      time.Duration // 0 = no limit
}
```

Add a one-liner helper in the lib:

```go
// NewSeed returns a non-deterministic seed suitable for Options.Seed.
func NewSeed() int64 { return time.Now().UnixNano() }
```

Remove the seed-resolution block from `Solve`. CLI and server both call `ge.NewSeed()`
when the user provides no seed, then pass the resolved value to `Solve` and echo it
back.

______________________________________________________________________

### R8 — `http.Server` transport timeouts (`cmd/server/main.go`)

**Problem.** The server uses `http.TimeoutHandler` for per-request timeouts but sets
no `ReadTimeout`, `WriteTimeout`, or `IdleTimeout` on `http.Server`. A client that
sends headers slowly holds a connection open indefinitely.

**Fix** (see constants in R6):

```go
srv := &http.Server{
    Addr:         cfg.Addr,
    Handler:      server.NewServer(cfg),
    ReadTimeout:  server.ReadTimeout,                       // 5s
    WriteTimeout: cfg.Timeout + server.WriteTimeoutBuffer,  // solver timeout + 5s buffer
    IdleTimeout:  server.IdleTimeout,                       // 60s
}
```

`WriteTimeout` is intentionally larger than `cfg.Timeout` so `http.TimeoutHandler`
fires first and returns a clean JSON error body, rather than the server closing the
connection mid-response.

______________________________________________________________________

### R9 — Go 1.22 method-based mux routing; drop empty `handler` struct (`server/`)

**Problem 1.** Handlers manually check `r.Method` and return 405. Go 1.22 mux
handles this automatically with the `"METHOD /path"` syntax.

**Problem 2.** The `handler` struct is empty — it has no fields and exists only to
give the handler functions a receiver. This adds two lines of boilerplate per handler
with no benefit.

**Fix.** In `NewServer`:

```go
mux.HandleFunc("POST /api/v1/solve", solveHandler)
mux.HandleFunc("GET /api/v1/health", healthHandler)
```

`solveHandler` and `healthHandler` become package-level functions (no receiver). The
`handler` struct and `newHandler()` constructor are deleted. The `r.Method` check in
`solveHandler` is deleted (the mux enforces it).

______________________________________________________________________

## Low Priority

### R10 — Standardize slice APIs (`lib/score.go`)

**Problem.** `graph.go` uses the 1.21+ `slices` package; `score.go` uses the older
`sort` package. `canonicalize` uses `fmt.Sprintf` for integer-to-string conversion.

**Fix:**

```go
// score.go: sort.Slice → slices.SortFunc
slices.SortFunc(solutions, func(a, b Solution) int {
    if a.Score.Better(b.Score) { return -1 }
    if b.Score.Better(a.Score) { return 1 }
    return 0
})

// canonicalize: sort.Strings → slices.Sort
slices.Sort(cycleStrs)

// canonicalize: fmt.Sprintf("%d", ...) → strconv.Itoa
parts[i] = strconv.Itoa(indices[(minPos+i)%len(indices)])
```

After this, `score.go` no longer imports `fmt` (only `sort` was using it for the
int→string case; `strings.Join` remains from `strings`). Update the import block.

______________________________________________________________________

### R11 — Dagger pipeline hardening (`.dagger/`)

1. **Build flags.** Change `go build -o binary ./server` to
   `./cmd/server` (post-R5) with `-ldflags="-s -w" -trimpath`. Smaller binary,
   no local filesystem paths embedded.

1. **Test + vet step.** Add a `Test` function to the Dagger module:

   ```go
   pub Test(): String! {
       container.from(defaultImage)
           .withMountedDirectory("/work", source)
           .withWorkdir("/work")
           .withExec(["go", "vet", "./..."])
           .withExec(["go", "test", "-race", "./..."])
           .stdout()
   }
   ```

1. **Output naming.** Rename `-o binary` to `-o gift-exchange` in the Go build step
   so the `withFile("gift-exchange", binary, ...)` rename in the container is
   unnecessary.

______________________________________________________________________

### R12 — Fuzz testing (`lib/solver_test.go`)

```go
func FuzzSolve(f *testing.F) {
    f.Add(2, 0)
    f.Add(10, 15)

    f.Fuzz(func(t *testing.T, n int, blockSeed int) {
        if n < 2 || n > 20 { return }
        participants := makeParticipants(n)
        rng := rand.New(rand.NewSource(int64(blockSeed)))
        var blocks []Block
        for i := range n {
            for j := range n {
                if i != j && rng.Float64() < 0.15 {
                    blocks = append(blocks, Block{From: participants[i].ID, To: participants[j].ID})
                }
            }
        }
        p := Problem{Participants: participants, Blocks: blocks}
        ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
        defer cancel()
        sols, err := Solve(ctx, p, Options{Seed: 1})
        if err != nil { return } // ErrInfeasible, timeout — both acceptable
        for _, s := range sols {
            assertValidSolution(t, p, s)
        }
    })
}
```

Run with: `go test -fuzz=FuzzSolve -fuzztime=60s ./lib/`

______________________________________________________________________

## Resulting Directory Layout

After all items above are complete:

```
gift-exchange/
├── cmd/
│   ├── giftexchange/         ← CLI (package main, unchanged except imports)
│   │   ├── main.go
│   │   └── main_test.go
│   └── server/               ← HTTP server entrypoint (package main, new)
│       └── main.go           ← flags, http.Server, ListenAndServe
├── internal/
│   └── dto/                  ← shared wire types + lib mappings (package dto)
│       ├── types.go
│       ├── mapping.go
│       └── mapping_test.go
├── lib/                      ← core solver library (package giftexchange)
│   ├── types.go              ← Participant, Block, Problem, Options, Solution, Score
│   │                            ErrInvalid, ErrInfeasible, DefaultMaxSolutions, NewSeed
│   ├── graph.go              ← graph, buildGraph, checkHall, shuffled
│   ├── score.go              ← decomposeCycles, canonicalize, scoreOf, Score.Better
│   ├── analyze.go            ← Analyze (no goroutine; solver takes ctx directly)
│   ├── solver.go             ← validateStructural, Validate, Solve, solverFunc,
│   │                            hamiltonianSolver, constrainedSolver, collectSolutions
│   └── solver_test.go        ← unit + integration + property + fuzz tests
├── server/                   ← HTTP server package (package server, not main)
│   ├── server.go             ← Config, constants, NewServer (exported)
│   ├── api.go                ← SolveRequest, SolveResponse, OptionsDTO, ErrorResponse
│   ├── handlers.go           ← solveHandler, healthHandler, corsMiddleware, dtoToProblem
│   └── handlers_test.go      ← unchanged
├── dagger.json
└── .dagger/
    └── main.dang             ← build target updated to ./cmd/server
```

______________________________________________________________________

## Implementation Order

The following sequence minimizes merge conflicts with Phase 8:

1. **R10** — slice/strconv cleanup. Pure style, zero risk.
1. **R3** — add `ErrInvalid`. Phase 8 handler tests depend on error classification.
1. **R6** — define constants. Unblocks R7 (seed) and R8 (timeouts).
1. **R1** — split `validate`. Phase 8 plan already assumes `validateStructural` + `checkHall` split.
1. **R2** — `solverFunc` abstraction + ctx cancellation. Most invasive lib change; do after R1.
1. **R4** — `internal/dto`. Mechanical but touches many files; do before Phase 8 adds `Required`.
1. **R5** — `cmd/server/` layout. Once `server/` is a package, R8 and R9 follow naturally.
1. **R7** — seed centralization. Depends on R5 (one fewer site).
1. **R8** — transport timeouts. One-liner after R5.
1. **R9** — mux routing. After R5 (`server/` package, not `package main`).
1. **R11** — Dagger. After R5 (build target changes to `./cmd/server`).
1. **R12** — fuzz tests. Additive, any time after R1/R2 stabilize.
