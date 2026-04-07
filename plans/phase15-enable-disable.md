# Phase 15 — Enable/Disable (Frontend-only)

## Status

- [x] **E1** — State shape: add `disabled?` flag to participants, blocks, relationships, and block groups
- [x] **E2** — Filtering logic: `activeParticipants`, updated `effectiveBlocks`, updated `stateToRequest`
- [x] **E3** — Participant toggle: UI button + disabled styling + graph node dimming
- [x] **E4** — Block toggle: per-item toggle + disabled styling (individual blocks and within groups)
- [x] **E5** — Block group toggle: group-level disable, cascades to contained blocks
- [x] **E6** — Relationship toggle: per-relationship toggle + disabled styling
- [x] **E7** — Graph: disabled node visual treatment, exclude disabled participants from valid edges
- [x] **E8** — Hash v4: encode disabled state in URL (additive, backwards-compatible)

All changes are frontend-only (`server/web/`). No Go changes, no API changes.

---

## Overview

Allow any participant, block, block group, or relationship to be temporarily
excluded from the solve without being deleted. Disabled items remain visible
in the sidebar (with visual treatment indicating their inactive state) and their
graph nodes remain visible but dimmed. The filtering happens at the call site
that builds the API request, so the solver never sees disabled items.

E1 and E2 form the logic foundation and should be done first (in that order).
E3–E6 are the four UI pieces and can be done in any order after E2. E7 depends
on E2. E8 is independent and can be done last.

---

## Design Decisions

### The `disabled` flag

Disabled state is stored as an optional `disabled: true` field directly on the
object, following the same pattern as `group.collapsed` and `block.group`.
Absent or `false` means enabled (the default). This means:
- No migration needed for existing data (absent = enabled)
- Serializes automatically through JSON import/export and localStorage
- `buildValidEdges`, `effectiveBlocks`, and `stateToRequest` can read it inline

### Semantics

| Item disabled         | Effect on solve                                               |
| --------------------- | ------------------------------------------------------------- |
| Participant           | Excluded from participants; all their blocks and relationships also excluded |
| Block                 | Not included in effectiveBlocks                               |
| Block group           | All blocks in the group excluded (cascades); individual `disabled` flags on blocks within the group are additive but do not matter while the group is disabled |
| Relationship          | Not expanded into effectiveBlocks                             |

A block is *effectively disabled* if `block.disabled || group?.disabled`, where
`group` is the block's containing group (if any). This is resolved purely in
`effectiveBlocks`.

### Participant cap with disabled participants

The current 20-participant cap is enforced on **active** (enabled) participants
only. Disabled participants do not count toward the cap. This means users can
store more than 20 participants as long as ≤20 are enabled at once.

Concretely: the "Add" button and the nameInput become disabled when
`activeParticipants(state).length >= MAX_PARTICIPANTS` (not `state.participants.length`).

### Toggling clears solutions

Any enable/disable toggle calls `mutated()`, since the current solutions were
computed under a different set of active participants and blocks.

### Relationship toggles

The user explicitly named participants, individual blocks, and block groups.
Relationships are mentioned only as collateral filtering when a participant is
disabled. Per-relationship toggles are recommended for consistency with the
rest of the UI, but are included as E6 (last priority) so they can be deferred
if not wanted.

### Toggle affordance

Each sidebar item gets a small toggle button placed **before** the name span
(leftmost position in the flex row). This mirrors how the existing edit (✎)
and remove (×) buttons work but appears on the left so it reads as "item
status" rather than "item action." Styling:

- Enabled: `●` in `var(--text)` color
- Disabled: `○` in `var(--muted)` color

The entire `<li>` gets a `.disabled` class when the item is disabled:
- Name text: `opacity: 0.45` and `text-decoration: line-through`
- Background: unchanged (the muted name is sufficient)

Block group headers get the same toggle button and `.disabled` class, with the
label text dimmed. When a group is disabled, its contained blocks are rendered
with the same `.disabled` class regardless of their individual flags.

---

## E1 — State Shape

**File**: `app.js`

No changes to the `state` object itself. The `disabled` field is stored on
individual objects within `state.participants`, `state.blocks`,
`state.relationships`, and `state.blockGroups`.

```js
// Examples of what disabled objects look like:
{ id: "alice", name: "Alice", disabled: true }
{ from: "alice", to: "bob", group: "history-2025", disabled: true }
{ a: "alice", b: "bob", disabled: true }
{ id: "history-2025", label: "History 2025", collapsed: false, disabled: true }
```

No default value needs to be set for new objects — absent `disabled` means
enabled. No changes to `applyImport`, `saveState`, `loadFromLocalStorage`, or
`onReset` are needed (reset replaces state wholesale; import uses `?? []`).

---

## E2 — Filtering Logic

**File**: `app.js`

### `activeParticipants(state)`

New exported function. Used everywhere that the full participant list was used
for the solve (not for rendering — the sidebar still renders all participants).

```js
export function activeParticipants(state) {
  return state.participants.filter(p => !p.disabled);
}
```

### `effectiveBlocks(state)` — updated

Current signature and return type unchanged. Adds disabled filtering:

```js
export function effectiveBlocks(state) {
  const disabledParticipantIds = new Set(
    state.participants.filter(p => p.disabled).map(p => p.id)
  );
  const disabledGroupIds = new Set(
    state.blockGroups.filter(g => g.disabled).map(g => g.id)
  );

  return [
    ...state.blocks
      .filter(b =>
        !b.disabled &&
        !disabledGroupIds.has(b.group) &&
        !disabledParticipantIds.has(b.from) &&
        !disabledParticipantIds.has(b.to)
      )
      .map(({ from, to }) => ({ from, to })),
    ...state.relationships
      .filter(r =>
        !r.disabled &&
        !disabledParticipantIds.has(r.a) &&
        !disabledParticipantIds.has(r.b)
      )
      .flatMap(r => [
        { from: r.a, to: r.b },
        { from: r.b, to: r.a },
      ]),
  ];
}
```

### `stateToRequest(state)` — updated

```js
export function stateToRequest(state) {
  const opts = { max_solutions: state.options.maxSolutions };
  if (state.options.seed != null) opts.seed = Number(state.options.seed);
  return { participants: activeParticipants(state), blocks: effectiveBlocks(state), options: opts };
}
```

### `buildValidEdges(participants, blocks)` — call site update

`restartGraph()` currently calls `buildValidEdges(state.participants, ...)`.
Update to `buildValidEdges(activeParticipants(state), ...)` so disabled
participants have no valid edges in the graph (their node still exists but
floats unconnected).

The function signature and body are **unchanged**.

---

## E3 — Participant Toggle

**Files**: `app.js`, `style.css`

### `renderParticipantList()` changes

The participant `<li>` currently has this structure:
```
[nameSpan]  [editBtn ✎]  [removeBtn ×]
```

After this change:
```
[toggleBtn ●/○]  [nameSpan]  [editBtn ✎]  [removeBtn ×]
```

The toggle button:
```js
const toggleBtn = document.createElement("button");
toggleBtn.className = "icon-btn toggle-btn";
toggleBtn.textContent = p.disabled ? "○" : "●";
toggleBtn.title = p.disabled ? "Enable participant" : "Disable participant";
toggleBtn.addEventListener("click", () => {
  p.disabled = p.disabled ? undefined : true;
  mutated();
});
li.insertBefore(toggleBtn, nameSpan);
```

Setting `p.disabled = undefined` rather than `false` keeps the serialized JSON
clean (undefined fields are omitted by JSON.stringify).

The `<li>` gets a class when disabled:
```js
if (p.disabled) li.classList.add("disabled");
```

### Participant cap update

Change the condition that disables the Add input/button from:
```js
const atCap = n >= MAX_PARTICIPANTS;
```
to:
```js
const n = state.participants.length;
const activeN = state.participants.filter(p => !p.disabled).length;
const atCap = activeN >= MAX_PARTICIPANTS;
document.getElementById("participant-count").textContent =
  n > 0 ? `${activeN} / ${MAX_PARTICIPANTS}` : "";
```

The count badge shows active/max (not total/max) to communicate what's relevant
to the solver.

### CSS additions

```css
/* ─── Enable/disable ─────────────────────────────────────────────────────────── */
.toggle-btn {
  font-size: 11px;
  margin-right: 4px;
}
.toggle-btn:hover { color: var(--accent); }

li.disabled .item-name,
li.disabled > span:first-of-type {
  opacity: 0.45;
  text-decoration: line-through;
}
```

A shared `.item-name` class should be added to the name `<span>` in all three
list types (participants, blocks, relationships) to make the disabled CSS rule
target precisely.

---

## E4 — Block Toggle

**File**: `app.js`

### `makeBlockItem(b, i)` changes

Currently returns `<li><span>A → B</span><button ×></li>`.

After this change, the `<li>` has the same structure as participant items:
```
[toggleBtn ●/○]  [span A → B]  [removeBtn ×]
```

```js
function makeBlockItem(b, i) {
  const fromName = state.participants.find(p => p.id === b.from)?.name ?? b.from;
  const toName   = state.participants.find(p => p.id === b.to)?.name ?? b.to;
  const li = document.createElement("li");

  // Effective disabled = individually disabled OR group disabled
  const groupDisabled = b.group && state.blockGroups.find(g => g.id === b.group)?.disabled;
  const effectivelyDisabled = !!(b.disabled || groupDisabled);

  if (effectivelyDisabled) li.classList.add("disabled");

  const toggleBtn = document.createElement("button");
  toggleBtn.className = "icon-btn toggle-btn";
  toggleBtn.textContent = b.disabled ? "○" : "●";
  toggleBtn.title = b.disabled ? "Enable block" : "Disable block";
  toggleBtn.disabled = !!groupDisabled; // can't individually toggle when group is disabled
  toggleBtn.addEventListener("click", () => {
    b.disabled = b.disabled ? undefined : true;
    mutated();
  });
  li.appendChild(toggleBtn);

  const span = document.createElement("span");
  span.className = "item-name";
  span.textContent = `${fromName} → ${toName}`;
  li.appendChild(span);

  const btn = document.createElement("button");
  btn.className = "icon-btn remove-btn";
  btn.textContent = "×";
  btn.title = "Remove block";
  btn.addEventListener("click", () => {
    state.blocks.splice(i, 1);
    mutated();
  });
  li.appendChild(btn);
  return li;
}
```

When a group is disabled, the individual block toggle buttons within it are
disabled (grayed out). The block's own `.disabled` flag is preserved — it will
take effect again if the group is later re-enabled.

---

## E5 — Block Group Toggle

**File**: `app.js`

### `renderBlockList()` changes to group header

The group header currently has:
```
[toggle ▾/▸]  [labelSpan]  [editBtn ✎]  [delBtn ×]
```

After this change:
```
[toggle ▾/▸]  [labelSpan]  [enableBtn ●/○]  [editBtn ✎]  [delBtn ×]
```

The enable/disable button is inserted after the label span, before the edit
button:

```js
const enableBtn = document.createElement("button");
enableBtn.className = "icon-btn toggle-btn";
enableBtn.textContent = group.disabled ? "○" : "●";
enableBtn.title = group.disabled ? "Enable group" : "Disable group";
enableBtn.addEventListener("click", e => {
  e.stopPropagation();
  group.disabled = group.disabled ? undefined : true;
  mutated();
});
header.insertBefore(enableBtn, editBtn);
```

When `group.disabled`, add `.disabled` class to the header element:
```js
if (group.disabled) header.classList.add("disabled");
```

The `.block-group-header.disabled .block-group-label` text is dimmed via the
existing `.disabled` CSS rule (`.disabled > span` selector, or explicit class).

---

## E6 — Relationship Toggle

**File**: `app.js`

### `renderRelationshipList()` changes

Same pattern as E3/E4: add toggle button leftmost in each `<li>`:

```js
state.relationships.forEach((r, i) => {
  const aName = state.participants.find(p => p.id === r.a)?.name ?? r.a;
  const bName = state.participants.find(p => p.id === r.b)?.name ?? r.b;
  const li = document.createElement("li");
  if (r.disabled) li.classList.add("disabled");

  const toggleBtn = document.createElement("button");
  toggleBtn.className = "icon-btn toggle-btn";
  toggleBtn.textContent = r.disabled ? "○" : "●";
  toggleBtn.title = r.disabled ? "Enable relationship" : "Disable relationship";
  toggleBtn.addEventListener("click", () => {
    r.disabled = r.disabled ? undefined : true;
    mutated();
  });
  li.appendChild(toggleBtn);

  const span = document.createElement("span");
  span.className = "item-name";
  span.textContent = `${esc(aName)} ↔ ${esc(bName)}`;
  li.appendChild(span);

  const btn = document.createElement("button");
  btn.className = "icon-btn remove-btn";
  btn.textContent = "×";
  btn.title = "Remove";
  btn.addEventListener("click", () => {
    state.relationships.splice(i, 1);
    mutated();
  });
  li.appendChild(btn);
  ul.appendChild(li);
});
```

---

## E7 — Graph: Disabled Node Styling

**File**: `app.js`

Disabled participants still appear in the graph (so the user can see who they
disabled and re-enable easily), but with distinct visual treatment:
- Circle fill: `#e5e7eb` (same as `var(--border)`) rather than the default grey `#d1d5db`
- Circle stroke: dashed, `var(--muted)` color
- Label: muted color

### `restartGraph()` changes

After the circle `.join()` call, add/update a `.classed` and style call:

```js
nodeCircleLayer.selectAll("circle")
  .data(nodes, d => d.id)
  .join(
    enter => enter.append("circle")
      .attr("class", "node-circle")
      .attr("r", NODE_RADIUS)
      .call(drag),
    update => update,
    exit => exit.remove()
  )
  .attr("fill", d => {
    if (state.participants.find(p => p.id === d.id)?.disabled) return "#e5e7eb";
    return nodeColors[d.id] ?? "#d1d5db";
  })
  .classed("node-disabled", d =>
    !!state.participants.find(p => p.id === d.id)?.disabled
  );
```

Add a CSS rule for the dashed stroke:

```css
.node-circle.node-disabled {
  stroke: var(--muted);
  stroke-width: 1.5px;
  stroke-dasharray: 4 3;
}
```

`recolorGraph()` must also respect disabled state: disabled participants return
`#e5e7eb` regardless of solution color, and keep the `node-disabled` class.
Update the `nodeCircleLayer.selectAll("circle").attr("fill", ...)` call in
`recolorGraph()` with the same conditional.

Note: `buildValidEdges` receives `activeParticipants(state)` (from E2), so
disabled participants automatically have no valid edges in the simulation.
They will float freely in the graph, pulled only by the center/gravity forces.

---

## E8 — Hash v4

**File**: `app.js`

### When to bump the version

v4 is only needed when at least one item is disabled. If nothing is disabled,
`encodeStateToHash` continues to emit `#v3:` (fully backwards compatible —
all v3 links remain valid).

When any item is disabled, emit `#v4:` with the additional disabled-index
fields. v3 and older decoders receiving a `#v4:` link fall through to
`return null` and show an empty state with a banner (the same graceful
degradation as an unknown hash version today).

### v4 compact format

Extends v3 with four optional arrays of indices into their respective
base arrays. Arrays are omitted entirely when empty (keeps hash size minimal
for common case).

| Field | Type       | Indexes into | Meaning                                |
| ----- | ---------- | ------------ | -------------------------------------- |
| `dp`  | `number[]` | `p`          | Disabled participant indices           |
| `dr`  | `number[]` | `r`          | Disabled relationship indices          |
| `db`  | `number[]` | `b`          | Disabled ungrouped-block indices       |
| `dg`  | `number[]` | `g`          | Disabled block-group indices           |
| `dbg` | `number[]` | `bg`         | Disabled grouped-block indices         |

### `encodeStateToHash` update

```js
export function encodeStateToHash(state) {
  const idxOf = Object.fromEntries(state.participants.map((p, i) => [p.id, i]));
  const groupIdxOf = Object.fromEntries(state.blockGroups.map((g, i) => [g.id, i]));

  const ungrouped = state.blocks.filter(b => !b.group);
  const grouped   = state.blocks.filter(b =>  b.group);

  const hasDisabled =
    state.participants.some(p => p.disabled) ||
    state.relationships.some(r => r.disabled) ||
    state.blocks.some(b => b.disabled) ||
    state.blockGroups.some(g => g.disabled);

  const compact = {
    v: hasDisabled ? 4 : 3,
    p: state.participants.map(p => [p.id, p.name]),
    r: state.relationships.map(r => [idxOf[r.a], idxOf[r.b]]),
    b: ungrouped.map(b => [idxOf[b.from], idxOf[b.to]]),
    ...(grouped.length ? {
      g: state.blockGroups.map(g => g.label),
      bg: grouped.map(b => [idxOf[b.from], idxOf[b.to], groupIdxOf[b.group]]),
    } : {}),
    pres: true,
  };

  if (hasDisabled) {
    const dp  = state.participants.map((p, i) => p.disabled ? i : -1).filter(i => i >= 0);
    const dr  = state.relationships.map((r, i) => r.disabled ? i : -1).filter(i => i >= 0);
    const db  = ungrouped.map((b, i) => b.disabled ? i : -1).filter(i => i >= 0);
    const dg  = state.blockGroups.map((g, i) => g.disabled ? i : -1).filter(i => i >= 0);
    const dbg = grouped.map((b, i) => b.disabled ? i : -1).filter(i => i >= 0);
    if (dp.length)  compact.dp  = dp;
    if (dr.length)  compact.dr  = dr;
    if (db.length)  compact.db  = db;
    if (dg.length)  compact.dg  = dg;
    if (dbg.length) compact.dbg = dbg;
  }

  if (state.options.maxSolutions !== 5 || state.options.seed != null) {
    compact.o = {};
    if (state.options.maxSolutions !== 5) compact.o.m = state.options.maxSolutions;
    if (state.options.seed != null) compact.o.s = state.options.seed;
  }

  const sol = state.solutions[state.selectedSolution];
  if (sol) {
    compact.c = sol.cycles.map(cycle => cycle.map(id => idxOf[id]));
  }

  return `#v${compact.v}:` + hashEncode(compact);
}
```

### `decodeStateFromHash` update

```js
export function decodeStateFromHash(hash) {
  try {
    if (hash.startsWith("#v4:")) return decodeV4(hashDecode(hash.slice(4)));
    if (hash.startsWith("#v3:")) return decodeV3(hashDecode(hash.slice(4)));
    if (hash.startsWith("#v2:")) return decodeV2(hashDecode(hash.slice(4)));
    if (hash.startsWith("#v1:")) return decodeV1(hashDecode(hash.slice(4)));
    return null;
  } catch {
    return null;
  }
}
```

### `decodeV4`

v4 shares all v3 parsing. Factor `parseV3Fields(compact)` (or extend the
existing `parseV2Fields`) to also handle the `c` and `pres` fields — currently
inline in `decodeV3`. Then `decodeV4` calls the shared parser and applies
disabled indices on top:

```js
function decodeV4(compact) {
  if (compact.v !== 4 || !Array.isArray(compact.p)) return null;
  const base = parseV3Fields(compact);  // handles p, r, b, g, bg, o, pres, c
  if (!base) return null;

  // Apply disabled indices
  (compact.dp  ?? []).forEach(i => { if (base.participants[i])  base.participants[i].disabled = true; });
  (compact.dr  ?? []).forEach(i => { if (base.relationships[i]) base.relationships[i].disabled = true; });
  // ungrouped blocks are first in base.blocks
  const ungroupedBlocks = base.blocks.filter(b => !b.group);
  const groupedBlocks   = base.blocks.filter(b =>  b.group);
  (compact.db  ?? []).forEach(i => { if (ungroupedBlocks[i]) ungroupedBlocks[i].disabled = true; });
  (compact.dg  ?? []).forEach(i => { if (base.blockGroups[i]) base.blockGroups[i].disabled = true; });
  (compact.dbg ?? []).forEach(i => { if (groupedBlocks[i])   groupedBlocks[i].disabled = true; });

  return base;
}
```

---

## Files to Change

| File                   | Changes                                                                                            |
| ---------------------- | -------------------------------------------------------------------------------------------------- |
| `server/web/app.js`    | E1–E8: filtering functions, all four render functions, restartGraph/recolorGraph, hash encode/decode |
| `server/web/style.css` | E3: `.toggle-btn`, `li.disabled`, `.node-disabled`                                                 |

No HTML changes needed — all new UI elements are created via JS.

---

## Acceptance Criteria

### E1 / E2

- Disabling a participant filters them from `activeParticipants(state)` and
  from `stateToRequest` (they are not sent to the API).
- Blocks and relationships referencing a disabled participant are also absent
  from `effectiveBlocks(state)`.
- Disabling a block removes it from `effectiveBlocks`.
- Disabling a block group removes all its blocks from `effectiveBlocks`,
  regardless of individual block flags.
- Disabling a relationship removes both of its directed block expansions from
  `effectiveBlocks`.

### E3

- Each participant row has a `●` (enabled) or `○` (disabled) button at left.
- Clicking it toggles `p.disabled`, calls `mutated()`, and re-renders.
- Disabled participants have strikethrough, muted name text.
- The participant count badge shows active/max, not total/max.
- The Add input and button disable when active participants reaches the cap;
  stored disabled participants do not count toward the cap.

### E4 / E5

- Each block item has a toggle button.
- Blocks within a disabled group show disabled styling regardless of their own
  flag; their individual toggle buttons are grayed out and non-functional.
- Disabling a group disables all its blocks effectively but preserves each
  block's individual `disabled` flag (re-enabling the group restores prior
  individual disabled states).

### E6

- Each relationship row has a toggle button; same behavior as blocks.

### E7

- Disabled participant nodes have a dashed border and lighter fill.
- Disabled participants have no valid edges (they float in the graph).
- Solution coloring skips disabled participants (they retain the disabled style
  even after a solve — they weren't in the solution).
- Re-enabling a participant causes `mutated()` which clears the solution and
  restarts the graph with them included.

### E8

- `encodeStateToHash` emits `#v3:` when nothing is disabled; `#v4:` otherwise.
- Loading a `#v4:` link correctly restores disabled state for all item types.
- Loading a `#v3:` link still works without changes.
- Empty `dp`/`dr`/`db`/`dg`/`dbg` arrays are omitted from the hash.
