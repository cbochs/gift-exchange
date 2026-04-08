# Phase 16 — Backend Enrichment (Full-stack)

## Status

> Complete.

---

## Motivation

The frontend currently owns three pieces of business logic that belong in the shared
Go layer:

1. **Relationship expansion** — symmetric relationships are expanded into two
   directed blocks in `effectiveBlocks()` before the API call.
2. **Block group filtering** — block groups are a UI concept; their members are
   flattened before the API call.
3. **Enable/disable filtering** — disabled participants, blocks, relationships,
   and groups are filtered out in `effectiveBlocks()` / `stateToRequest()`.

Moving this logic into `internal/dto`:

- Lets the frontend send its data model directly (no expansion/filtering code)
- Gives the **CLI the same richer input format** — it can accept `relationships`
  and `block_groups` in its JSON input with no additional work
- Creates a single authoritative conversion from the rich domain model to the flat
  lib input (`ge.Problem`), shared by all consumers

The lib (`lib/`) is **not changed** — it still receives a flat list of participants
and blocks. The solver knows nothing about relationships, groups, or disabled state.

---

## Design

### Why `internal/dto`, not `server/api.go`

Both the CLI (`cmd/giftexchange/main.go`) and the server (`server/handlers.go`)
convert a JSON-described problem into a `ge.Problem`. That conversion is identical
and must stay in sync. The right place is the package they both already share:
`internal/dto`.

`server/api.go` defines HTTP-specific types (`OptionsDTO`, `SolveResponse`,
`ErrorResponse`). It references dto types but does not define domain types. No
new domain types go in `server/api.go`.

### New types in `internal/dto/types.go`

```go
type ParticipantDTO struct {
    ID       string `json:"id"`
    Name     string `json:"name"`
    Disabled bool   `json:"disabled,omitempty"`
}

type BlockDTO struct {
    From     string `json:"from"`
    To       string `json:"to"`
    Disabled bool   `json:"disabled,omitempty"`
    Group    string `json:"group,omitempty"`
}

type RelationshipDTO struct {
    A        string `json:"a"`
    B        string `json:"b"`
    Disabled bool   `json:"disabled,omitempty"`
}

// BlockGroupDTO carries the server-relevant state for a block group.
// The `collapsed` field (frontend-only UI state) is intentionally absent.
type BlockGroupDTO struct {
    ID       string `json:"id"`
    Label    string `json:"label"`
    Disabled bool   `json:"disabled,omitempty"`
}
```

### New function in `internal/dto/mapping.go`

```go
// BuildProblem converts the rich domain model into the flat ge.Problem the
// solver expects. It filters disabled participants, blocks, and relationships,
// expands relationships into directed block pairs, and filters blocks belonging
// to disabled groups or involving disabled participants.
func BuildProblem(
    participants []ParticipantDTO,
    blocks []BlockDTO,
    relationships []RelationshipDTO,
    blockGroups []BlockGroupDTO,
) ge.Problem {
    disabledParticipants := make(map[string]bool)
    var activeParticipants []ge.Participant
    for _, p := range participants {
        if p.Disabled {
            disabledParticipants[p.ID] = true
        } else {
            activeParticipants = append(activeParticipants, ge.Participant{ID: p.ID, Name: p.Name})
        }
    }

    disabledGroups := make(map[string]bool)
    for _, g := range blockGroups {
        if g.Disabled {
            disabledGroups[g.ID] = true
        }
    }

    var activeBlocks []ge.Block
    for _, b := range blocks {
        if b.Disabled || disabledGroups[b.Group] || disabledParticipants[b.From] || disabledParticipants[b.To] {
            continue
        }
        activeBlocks = append(activeBlocks, ge.Block{From: b.From, To: b.To})
    }
    for _, r := range relationships {
        if r.Disabled || disabledParticipants[r.A] || disabledParticipants[r.B] {
            continue
        }
        activeBlocks = append(activeBlocks, ge.Block{From: r.A, To: r.B})
        activeBlocks = append(activeBlocks, ge.Block{From: r.B, To: r.A})
    }

    return ge.Problem{Participants: activeParticipants, Blocks: activeBlocks}
}
```

The existing `ParticipantsToLib`, `BlocksToLib`, and their singular forms remain
unchanged — they are still used by `SolutionsFromLib` and tests.

---

## Changes by file

### `internal/dto/types.go`

- Add `Disabled bool` to `ParticipantDTO` and `BlockDTO`
- Add `Group string` to `BlockDTO`
- Add `RelationshipDTO` and `BlockGroupDTO` (new types)

### `internal/dto/mapping.go`

- Add `BuildProblem()` (see above)

### `internal/dto/mapping_test.go`

- Tests for `BuildProblem`:
  - Disabled participant excluded; their blocks and relationships excluded
  - Disabled block excluded
  - Disabled group → all blocks in that group excluded
  - Disabled relationship excluded
  - Enabled relationship expands to two directed blocks
  - Old-style call (no relationships, no groups, no disabled) still works

### `cmd/giftexchange/main.go`

`inputDoc` gains two new optional fields:

```go
type inputDoc struct {
    Participants  []dto.ParticipantDTO  `json:"participants"`
    Blocks        []dto.BlockDTO        `json:"blocks,omitempty"`
    Relationships []dto.RelationshipDTO `json:"relationships,omitempty"`
    BlockGroups   []dto.BlockGroupDTO   `json:"block_groups,omitempty"`
    Options       inputOptions          `json:"options"`
    // Round-trip fields (written on output, ignored when re-used as input).
    Solutions []dto.SolutionDTO `json:"solutions,omitempty"`
    Feasible  *bool             `json:"feasible,omitempty"`
}
```

`inputDoc.problem()` becomes:

```go
func (d *inputDoc) problem() ge.Problem {
    return dto.BuildProblem(d.Participants, d.Blocks, d.Relationships, d.BlockGroups)
}
```

The `validate` output line changes from `Blocks: N` to `Blocks: N  Relationships: M`
(or similar) to reflect the enriched input. This is a minor cosmetic update.

### `server/api.go`

`SolveRequest` gains the new fields:

```go
type SolveRequest struct {
    Participants  []dto.ParticipantDTO  `json:"participants"`
    Blocks        []dto.BlockDTO        `json:"blocks,omitempty"`
    Relationships []dto.RelationshipDTO `json:"relationships,omitempty"`
    BlockGroups   []dto.BlockGroupDTO   `json:"block_groups,omitempty"`
    Options       OptionsDTO            `json:"options,omitempty"`
}
```

No types are defined in `server/api.go` that duplicate what is in `internal/dto`.

### `server/handlers.go`

`dtoToProblem` calls `BuildProblem`:

```go
func dtoToProblem(req SolveRequest) (ge.Problem, ge.Options, int64) {
    seed := req.Options.Seed
    if seed == 0 {
        seed = ge.NewSeed()
    }
    maxSolutions := req.Options.MaxSolutions
    if maxSolutions <= 0 {
        maxSolutions = ge.DefaultMaxSolutions
    }
    timeout := time.Duration(req.Options.TimeoutMs) * time.Millisecond
    if timeout <= 0 {
        timeout = defaultTimeout
    }
    return dto.BuildProblem(req.Participants, req.Blocks, req.Relationships, req.BlockGroups),
        ge.Options{MaxSolutions: maxSolutions, Seed: seed, Timeout: timeout}, seed
}
```

### `server/handlers_test.go`

Add tests:

- Request with `relationships` produces same result as equivalent two-block request
- Request with `disabled: true` on a participant excludes them + their blocks
- Request with `disabled: true` on a block group excludes its blocks
- Old-style request (no new fields) continues to work identically

### `server/web/app.js`

`stateToRequest` is simplified:

```js
export function stateToRequest(state) {
  const opts = { max_solutions: state.options.maxSolutions };
  if (state.options.seed != null) opts.seed = Number(state.options.seed);
  return {
    participants: state.participants,
    relationships: state.relationships,
    blocks: state.blocks,
    block_groups: state.blockGroups,
    options: opts,
  };
}
```

`effectiveBlocks()` and `activeParticipants()` are removed (dead code after this
change). `restartGraph()` calls a trimmed local helper instead — see below.

### `server/web/app.js` — graph still needs local filtering

`buildValidEdges` is called from `restartGraph` to drive the D3 layout. It must
compute the active participant set and effective block set locally for graph
rendering. After this phase it is no longer exported or named `effectiveBlocks`;
it becomes a private implementation detail of `restartGraph`:

The logic that was in `effectiveBlocks` + `activeParticipants` stays in the
frontend, but only for the graph — it is not used for the API call anymore.
The comment above `buildValidEdges` is updated to make this clear.

---

## Design Questions (resolved)

1. **Breaking API change?** No. New fields are optional; old clients that omit
   `relationships`, `block_groups`, and `disabled` fields continue to work.

2. **Where do new types live?** `internal/dto` — shared between CLI and server.
   No domain types are defined in `server/api.go`.

3. **`BlockGroupDTO.collapsed`?** Absent. `collapsed` is frontend-only UI state;
   the server and CLI have no use for it.

4. **Relationship deduplication on server?** Skipped. Duplicate blocks are safe
   for `lib.Solve` (a blocked edge stays blocked). The frontend already prevents
   duplicate relationships at entry time.

5. **Response shape change?** No. `SolveResponse` is unchanged.

---

## Acceptance Criteria

- `dto.BuildProblem` with relationships → same solve result as equivalent
  two-block input.
- `dto.BuildProblem` with a disabled participant → that participant and all their
  blocks/relationships are excluded.
- `dto.BuildProblem` with a disabled block group → all blocks with that group ID
  are excluded.
- CLI `solve` command accepts `relationships` and `block_groups` in its JSON input
  and processes them correctly.
- Old CLI inputs (no `relationships`, no disabled fields) continue to work.
- All existing handler tests and CLI tests pass.
- `go vet` and `staticcheck` clean.
