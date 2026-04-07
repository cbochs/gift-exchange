# Phase 16 — Backend Enrichment (Full-stack)

## Status

> Not started. Planned after Phase 15.

---

## Motivation

The frontend currently owns three pieces of business logic that conceptually
belong in the server:

1. **Relationship expansion** — symmetric relationships are expanded into two
   directed blocks in `effectiveBlocks()` before the API call.
2. **Block group filtering** — block groups are a UI concept; their members are
   flattened before the API call.
3. **Enable/disable filtering** — disabled participants, blocks, relationships,
   and groups are filtered out in `effectiveBlocks()` / `stateToRequest()`.

Moving this logic to the server:
- Lets the frontend send its data model directly (no expansion/filtering code)
- Creates a richer API that reflects the domain model
- Positions the server to validate and reason about the input more fully

This is a full-stack change touching the Go server and the JavaScript frontend.
The solver library (`lib/`) is **not** changed — it still receives a flat list
of participants and blocks.

---

## Scope

### What moves to the backend

| Frontend today                     | After this phase                                |
| ---------------------------------- | ----------------------------------------------- |
| Expand relationships → 2 blocks    | Server expands in `dtoToProblem`                |
| Filter disabled participants       | Server filters before calling lib.Solve         |
| Filter disabled blocks/groups      | Server filters before calling lib.Solve         |
| Flatten block groups               | Server flattens (block groups are metadata)     |

### What stays in the frontend

- Rendering all items including disabled ones (visual state management)
- `mutated()` / solve invalidation logic
- Graph building and visualization

---

## API Changes

### `server/api.go`

New types added to the request schema:

```go
type RelationshipDTO struct {
    A       string `json:"a"`
    B       string `json:"b"`
    Disabled bool   `json:"disabled,omitempty"`
}

type BlockGroupDTO struct {
    ID       string `json:"id"`
    Label    string `json:"label"`
    Disabled bool   `json:"disabled,omitempty"`
}
```

`SolveRequest` gains new optional fields:

```go
type SolveRequest struct {
    Participants []dto.ParticipantDTO `json:"participants"`
    Blocks       []dto.BlockDTO       `json:"blocks"`
    // New fields:
    Relationships []RelationshipDTO   `json:"relationships,omitempty"`
    BlockGroups   []BlockGroupDTO     `json:"block_groups,omitempty"`
    Options       *OptionsDTO         `json:"options,omitempty"`
}
```

`dto.BlockDTO` gains a `Disabled` and optional `Group` field:

```go
type BlockDTO struct {
    From     string `json:"from"`
    To       string `json:"to"`
    Disabled bool   `json:"disabled,omitempty"`
    Group    string `json:"group,omitempty"`
}
```

`dto.ParticipantDTO` gains a `Disabled` field:

```go
type ParticipantDTO struct {
    ID       string `json:"id"`
    Name     string `json:"name"`
    Disabled bool   `json:"disabled,omitempty"`
}
```

### `server/handlers.go` — `dtoToProblem` update

The handler function that converts a `SolveRequest` to `lib.Problem` is
updated to:

1. Filter disabled participants from the participant list.
2. Build the effective block list:
   - Expand `Relationships` (filtering disabled ones and those involving
     disabled participants) into two directed blocks each.
   - Include `Blocks` that are not disabled, not in a disabled group,
     and don't involve a disabled participant.
3. Pass the filtered lists to `lib.Solve`.

This is the same logic currently in `effectiveBlocks()` and
`activeParticipants()` on the frontend — moved verbatim into Go.

### Frontend `stateToRequest(state)`

Simplified to send the full data model:

```js
export function stateToRequest(state) {
  const opts = { max_solutions: state.options.maxSolutions };
  if (state.options.seed != null) opts.seed = Number(state.options.seed);
  return {
    participants: state.participants,     // includes disabled: true where set
    relationships: state.relationships,   // includes disabled: true where set
    blocks: state.blocks,                 // includes disabled: true, group where set
    block_groups: state.blockGroups,      // includes disabled: true where set
    options: opts,
  };
}
```

`effectiveBlocks()` and `activeParticipants()` are removed from the frontend
(they become dead code once `stateToRequest` no longer calls them).

### Frontend `buildValidEdges()` — local use only

`buildValidEdges` is still used for the D3 graph (to show grey valid edges) and
still runs in the browser. It must compute the effective block set locally for
graph purposes. Post-Phase 16, it takes the same inputs as before and performs
its own filtering inline (or calls a trimmed local helper that is no longer
exported).

---

## Design Questions (resolve before implementation)

1. **Breaking API change?** Adding new optional fields to `SolveRequest` is
   backwards-compatible (old clients that send only `participants` and `blocks`
   continue to work). No version bump needed.

2. **Block groups as first-class API concept?** The `block_groups` field is
   purely metadata for the frontend (ordering, labels). The server only needs
   `group` on each block to know which group it belongs to, so it can filter
   disabled groups. The full `BlockGroupDTO` in the request gives the server
   access to `disabled` state. ✓

3. **Relationship deduplication on server?** The server should deduplicate
   symmetric pairs before building the block list (same as the frontend's
   `some(r => [r.a, r.b].sort().join("|") === key)` check). Or it can skip
   deduplication — duplicate blocks are safe for lib.Solve (a blocked edge
   stays blocked).

4. **Response shape change?** The `SolveResponse` does not change. The server
   still returns ranked `SolutionDTO[]`.

---

## Files to Change

| File                            | Changes                                                                   |
| ------------------------------- | ------------------------------------------------------------------------- |
| `server/api.go`                 | Add `RelationshipDTO`, `BlockGroupDTO`; extend `SolveRequest`, `BlockDTO`, `ParticipantDTO` |
| `server/handlers.go`            | Update `dtoToProblem` to expand relationships and filter disabled items   |
| `server/handlers_test.go`       | Add tests for relationship expansion, disabled filtering, group filtering |
| `internal/dto/types.go`         | Add `Disabled`, `Group` to `BlockDTO`; `Disabled` to `ParticipantDTO`    |
| `internal/dto/mapping.go`       | Update `BlocksToLib` to skip disabled blocks                              |
| `internal/dto/mapping_test.go`  | Roundtrip tests for new fields                                            |
| `server/web/app.js`             | Simplify `stateToRequest`; remove `effectiveBlocks`/`activeParticipants` |

---

## Acceptance Criteria (high level)

- Sending `relationships` in the request: the server produces the same solve
  result as the current frontend-expansion path.
- Sending `disabled: true` on a participant: that participant is excluded;
  all their blocks and relationships are also excluded.
- Sending `disabled: true` on a block group: all blocks with that group ID
  are excluded.
- Old requests (no `relationships`, no `disabled` fields) continue to work
  identically.
- All existing handler tests pass.
