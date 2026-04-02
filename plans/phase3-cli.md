# Phase 3 — CLI

## Status

- [x] `cmd/giftexchange/main.go` — `run(args, stdin, stdout, stderr) int` entry point
- [x] `solve` subcommand — reads input JSON, calls library, formats output
- [x] `validate` subcommand — validates input without solving
- [x] `analyze` subcommand — prints graph statistics
- [x] `cmd/giftexchange/main_test.go` — all CLI integration tests passing
- [x] Round-trip verified: `solve --json` output can be re-used as input

## Goal

Expose the `lib/` package through a command-line interface. The CLI serves two purposes: (1) provide a practical, scriptable tool for generating gift exchanges, and (2) act as the first integration test of the library's public API, surfacing any ergonomic or design issues before the web backend is built.

---

## 1. High-Level Design

### 1.1 Command Structure

The CLI uses subcommands to group distinct operations:

```
giftexchange <command> [flags]

Commands:
  solve     Generate one or more gift exchange solutions
  validate  Check that a problem JSON is well-formed and feasible
  analyze   Show graph statistics (node count, edge density, Hamiltonian cycle possible?)
```

The primary command is `solve`. `validate` and `analyze` are diagnostic helpers that exercise library internals without running the full solver.

### 1.2 Input Format

All commands read from a single JSON file (`--input`) or stdin (`-`). The input schema mirrors the library's `Problem` type plus solver `Options`:

```json
{
  "participants": [
    { "id": "alice", "name": "Alice Smith" },
    { "id": "bob", "name": "Bob Jones" }
  ],
  "blocks": [{ "from": "alice", "to": "bob" }],
  "options": {
    "max_solutions": 5,
    "seed": 42
  }
}
```

The `options` field is optional; `max_solutions` defaults to 5, `seed` defaults to 0 (random). There is no `min_cycle_len` — the solver automatically determines the best achievable minimum via the N/M progression.

### 1.3 Output Format

**Default (human-readable):**

```
Seed: 42
Solutions found: 2

=== Solution 1 ===
Score: min_cycle=4  cycles=1  max_cycle=4
Cycles:
  [1] alice → charlie → dave → bob → alice

  Assignments:
    alice    →  charlie
    charlie  →  dave
    dave     →  bob
    bob      →  alice

=== Solution 2 ===
...
```

**JSON output (`--json`):**
Emits the full response body, identical in schema to what the web backend returns. This enables piping output into other tools and makes the CLI a testbed for the API contract.

```json
{
  "solutions": [...],
  "feasible": true
}
```

### 1.4 Flags

```
giftexchange solve
  --input  FILE     Input JSON file (or - for stdin). Required.
  --json            Output as JSON instead of human-readable text.
  --seed   INT      Override the seed in the input file.
  --n      INT      Override max_solutions in the input file.
  # Note: no --min-cycle flag. The solver determines cycle targets automatically.

giftexchange validate
  --input  FILE     Input JSON file (or - for stdin). Required.

giftexchange analyze
  --input  FILE     Input JSON file (or - for stdin). Required.
```

### 1.5 Exit Codes

| Code | Meaning                                         |
| ---- | ----------------------------------------------- |
| 0    | Success                                         |
| 1    | Invalid input (parse error, validation failure) |
| 2    | No solution found (infeasible)                  |
| 3    | Timeout expired before first solution found     |

### 1.6 JSON Import/Export Round-Trip

The `solve --json` output can be saved and re-fed to `solve` as input:

```bash
# Generate and save
giftexchange solve --input exchange.json --json > result.json

# Re-run with same parameters (seed is embedded in result)
giftexchange solve --input result.json --json
```

This establishes the same import/export contract the frontend will use.

### 1.7 Testing Strategy

CLI tests are integration tests that invoke the `main` function or the top-level `run(args)` function with controlled inputs and capture stdout/stderr/exit code. No HTTP is involved.

**Tests:**

- `TestCLI_Solve_Basic`: valid 4-participant JSON → exits 0, output contains ≥1 solution
- `TestCLI_Solve_JSON`: `--json` flag → output is valid JSON with correct schema
- `TestCLI_Solve_Infeasible`: all pairs blocked → exits 2
- `TestCLI_Validate_Valid`: well-formed input → exits 0 with "valid" message
- `TestCLI_Validate_Invalid`: unknown participant in block → exits 1 with error
- `TestCLI_Analyze`: outputs node count, edge count, and density
- `TestCLI_Stdin`: pipe JSON via stdin → same output as file input
- `TestCLI_SeedOverride`: `--seed` flag overrides input seed, result is reproducible

---

## 2. Implementation Plan

1. **Create `cmd/giftexchange/main.go`** with a `run(args []string) int` function returning exit code — allows testing without `os.Exit`
2. **Implement `solve` subcommand**: parse input → call `giftexchange.Solve` → format output
3. **Implement `validate` subcommand**: parse input → call library validation → report
4. **Implement `analyze` subcommand**: parse input → call `buildGraph` → report statistics
5. **Write CLI integration tests** in `cmd/giftexchange/main_test.go`
6. **Address any library API friction** discovered during CLI implementation (API changes go back to the library, not workarounds in the CLI)
7. **Verify round-trip**: JSON output can be re-used as input without changes

---

## 3. Implementation Snippets

### `cmd/giftexchange/main.go` — Testable entry point

```go
package main

import (
    "context"
    "encoding/json"
    "flag"
    "fmt"
    "io"
    "os"

    ge "github.com/[user]/gift-exchange/lib"
)

func main() {
    os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
    if len(args) == 0 {
        fmt.Fprintln(stderr, "usage: giftexchange <solve|validate|analyze> [flags]")
        return 1
    }
    switch args[0] {
    case "solve":
        return cmdSolve(args[1:], stdout, stderr)
    case "validate":
        return cmdValidate(args[1:], stdout, stderr)
    case "analyze":
        return cmdAnalyze(args[1:], stdout, stderr)
    default:
        fmt.Fprintf(stderr, "unknown command: %s\n", args[0])
        return 1
    }
}
```

### `cmd/giftexchange/main.go` — Solve command

```go
func cmdSolve(args []string, stdout, stderr io.Writer) int {
    fs := flag.NewFlagSet("solve", flag.ContinueOnError)
    inputPath := fs.String("input", "", "input JSON file (or - for stdin)")
    asJSON    := fs.Bool("json", false, "output as JSON")
    seedOver  := fs.Int64("seed", 0, "override seed (0 = use input seed)")
    nOver     := fs.Int("n", 0, "override max_solutions (0 = use input value)")
    fs.Parse(args)

    req, err := readInput(*inputPath)
    if err != nil {
        fmt.Fprintln(stderr, "input error:", err)
        return 1
    }

    if *seedOver != 0 { req.Options.Seed = *seedOver }
    if *nOver > 0    { req.Options.MaxSolutions = *nOver }

    solutions, err := ge.Solve(context.Background(), req.Problem, req.Options)
    if err != nil {
        fmt.Fprintln(stderr, "solve error:", err)
        if isInfeasible(err) { return 2 }
        return 1
    }

    if *asJSON {
        return writeJSON(stdout, stderr, solutions)
    }
    return writeHuman(stdout, solutions, req)
}
```

### `cmd/giftexchange/main_test.go` — Integration test helper

```go
func runCLI(t *testing.T, input string, extraArgs ...string) (stdout, stderr string, code int) {
    t.Helper()
    f := writeTemp(t, input)
    args := append([]string{"solve", "--input", f}, extraArgs...)
    var outBuf, errBuf strings.Builder
    code = run(args, &outBuf, &errBuf)
    return outBuf.String(), errBuf.String(), code
}

func TestCLI_Solve_Basic(t *testing.T) {
    input := `{
        "participants": [
            {"id":"a","name":"Alice"}, {"id":"b","name":"Bob"},
            {"id":"c","name":"Carol"}, {"id":"d","name":"Dave"}
        ],
        "blocks": [],
        "options": {"seed": 1, "max_solutions": 3}
    }`
    out, _, code := runCLI(t, input)
    require.Equal(t, 0, code)
    require.Contains(t, out, "Solution 1")
}

func TestCLI_Solve_JSON(t *testing.T) {
    // ... same input ...
    out, _, code := runCLI(t, input, "--json")
    require.Equal(t, 0, code)

    var resp struct {
        Solutions []ge.Solution `json:"solutions"`
        Feasible  bool          `json:"feasible"`
    }
    require.NoError(t, json.Unmarshal([]byte(out), &resp))
    require.True(t, resp.Feasible)
    require.NotEmpty(t, resp.Solutions)
}
```

### `cmd/giftexchange/main.go` — Human-readable output

```go
func writeHuman(w io.Writer, solutions []ge.Solution, req *inputDoc) int {
    fmt.Fprintf(w, "Seed: %d\nSolutions found: %d\n\n", req.Options.Seed, len(solutions))
    for i, s := range solutions {
        fmt.Fprintf(w, "=== Solution %d ===\n", i+1)
        fmt.Fprintf(w, "Score: min_cycle=%d  cycles=%d  max_cycle=%d\n",
            s.Score.MinCycleLen, s.Score.NumCycles, s.Score.MaxCycleLen)
        fmt.Fprintf(w, "Cycles:\n")
        for j, c := range s.Cycles {
            fmt.Fprintf(w, "  [%d] %s → %s\n", j+1, strings.Join(c, " → "), c[0])
        }
        fmt.Fprintln(w)
    }
    return 0
}
```
