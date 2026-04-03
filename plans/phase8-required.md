# Phase 8 — Required Assignments

## Status

- [ ] `lib/types.go` — add `Required []Block` to `Problem`
- [ ] `lib/solver.go` — validation + required-constraint graph injection
- [ ] `lib/solver_test.go` — required assignment test cases
- [ ] `server/api.go` — add `Required []BlockDTO` to `SolveRequest`
- [ ] `server/handlers.go` — extend `dtoToProblem()`, add handler tests
- [ ] `web/app.js` — state, rendering, stateToRequest, import/export
- [ ] `web/index.html` — Required sidebar section
- [ ] End-to-end smoke test

## Goal

Allow users to specify assignments that must appear in every solution — e.g. guaranteeing a new participant has a familiar partner, or honoring a special request. This is a full-stack feature: library, API, and frontend.

---

## 1. Library Changes (`lib/`)

### 1.1 `lib/types.go`

Add `Required` to `Problem`:

```go
type Problem struct {
    Participants []Participant
    Blocks       []Block
    Required     []Block  // directed assignments that MUST appear in every solution
    // No MinCycleLen — determined automatically via N/M progression.
}
```

### 1.2 `lib/solver.go` — Validation

New checks added to `validate()` after the existing structural checks:

1. Each required pair's `From` and `To` must reference participant IDs in `p.Participants`.
2. No required pair `{G, R}` may also appear in `p.Blocks` (forbidden and required simultaneously is a contradiction).
3. No participant ID may appear more than once as `From` across `p.Required` (two outgoing required assignments from the same gifter is impossible in a permutation).
4. No participant ID may appear more than once as `To` across `p.Required` (same logic for recipients).

### 1.3 `lib/solver.go` — Constraint Injection

Required assignments are encoded entirely in the graph before the solver runs — zero changes to `hamiltonianDFS` or `constrainedBacktrack`.

**New helper `applyRequiredConstraints(g *graph, required []Block)`:**

For each required assignment `(G → R)`:
- Set `g.adj[gi] = []int{ri}` — G can only give to R.
- Remove `ri` from `g.adj[j]` for all `j ≠ gi` — no one else can give to R.

```go
func applyRequiredConstraints(g *graph, required []Block) {
    for _, req := range required {
        gi := g.idx[req.From]
        ri := g.idx[req.To]
        g.adj[gi] = []int{ri}
        for j := range g.n {
            if j != gi {
                g.adj[j] = removeInt(g.adj[j], ri)
            }
        }
    }
}
```

**Calling sequence in `Solve()`:**

```go
g := buildGraph(p.Participants, p.Blocks)
applyRequiredConstraints(g, p.Required)
// Hall's condition check (out-degree ≥ 1, in-degree ≥ 1) runs on the constrained graph
if err := checkHall(g); err != nil {
    return nil, err
}
```

The existing `validate()` runs structural checks first; Hall's condition is re-checked post-constraint-injection in `Solve()` to catch infeasibility introduced by required assignments.

### 1.4 New Tests (`lib/solver_test.go`)

| Test | Expected result |
|---|---|
| Required pair in 4-person problem | Every solution includes that assignment |
| Multiple compatible required pairs | All solutions honor all of them |
| Required pair also in Blocks | Validation error |
| Required pair — unknown participant | Validation error |
| Same gifter in two required entries | Validation error |
| Same recipient in two required entries | Validation error |
| Required assignment creates Hall's violation | `ErrInfeasible` |

---

## 2. Server Changes

### 2.1 `server/api.go`

```go
type SolveRequest struct {
    Participants []ParticipantDTO `json:"participants"`
    Blocks       []BlockDTO       `json:"blocks,omitempty"`
    Required     []BlockDTO       `json:"required,omitempty"`
    Options      OptionsDTO       `json:"options,omitempty"`
}
```

`BlockDTO` is reused for `Required` — it is the same directed-pair structure.

### 2.2 `server/handlers.go`

Extend `dtoToProblem()` with a required mapping loop (mirrors the existing blocks loop):

```go
required := make([]ge.Block, len(req.Required))
for i, r := range req.Required {
    required[i] = ge.Block{From: r.From, To: r.To}
}
prob.Required = required
```

### 2.3 New Handler Tests

| Test | Expected HTTP status |
|---|---|
| Valid required pair present in solution | 200 |
| Required references unknown participant ID | 400 |
| Required pair conflicts with block | 422 (ErrInfeasible) |
| Required gifter appears twice | 400 |
| Required recipient appears twice | 400 |

---

## 3. Frontend Changes

### 3.1 State

```js
const state = {
  participants: [],
  relationships: [], // Phase 6
  blocks: [],
  required: [],      // [{from, to}]
  options: { ... },
  solutions: [],
  ...
};
```

### 3.2 Sidebar Section

"Required" section between Blocks and Options (same position as planned in Phase 6). Structurally identical to Blocks: two dropdowns (From → To) with add-row, list with remove buttons, displayed as `Alice → Bob`. The directionality is meaningful (required assignments are directed).

**Self-assignment and duplicate prevention:** same checks as the Blocks add-row.

**Cascade delete:** when a participant is removed:
```js
state.required = state.required.filter(r => r.from !== id && r.to !== id);
```

### 3.3 `stateToRequest()`

```js
export function stateToRequest(state) {
  const req = {
    participants: state.participants,
    blocks: effectiveBlocks(state),
    options: { ... },
  };
  if (state.required.length) req.required = state.required;
  return req;
}
```

### 3.4 Import/Export

```json
{
  "participants": [...],
  "relationships": [...],
  "blocks": [...],
  "required": [{"from": "alice", "to": "bob"}],
  "options": {...},
  "_selected_solution": 0,
  "_solutions": [...]
}
```

Emit `required` only if non-empty. `applyImport()` reads `doc.required ?? []`. Pre-Phase-8 exports default to empty.

---

## 4. Implementation Plan

1. **Library** — add `Required []Block` to `Problem`; implement `applyRequiredConstraints()`; extend `validate()` and `Solve()`; write all test cases and verify they pass.
2. **Server** — extend `SolveRequest` and `dtoToProblem()`; add handler tests.
3. **Frontend** — add `required: []` to state; add Required sidebar section; update `stateToRequest()`, cascade delete, `applyImport()`, `onDownload()`; add HTML section.
4. **End-to-end smoke test** — add 4 participants, specify a required assignment, Generate → verify every solution includes that pair; verify infeasible required assignment shows error.

Each layer is independently committable.
