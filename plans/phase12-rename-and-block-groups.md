# Phase 12 — Participant Rename & Block Groups (Frontend-only)

## Status

- [x] **F1** — Participant rename (stable IDs)
- [x] **F2** — Block group data model
- [x] **F3** — Block group UI (collapsible, rename, delete)
- [x] **F4** — "Add as history blocks" creates a named group
- [x] **F5** — URL hash v2 format (groups in shared links)

All changes are frontend-only (`server/web/`). No Go changes required.

---

## Overview

Two independent features:

1. **Participant rename** — change a participant's display name while keeping
   their internal ID (and all block/relationship references) stable.

2. **Block groups** — add an optional `group` field to blocks and a new
   `state.blockGroups` array so blocks can be organized into named, collapsible
   groups. "Add as history blocks" creates a dated group automatically.

Both features affect only `app.js`, `index.html`, and `style.css`.

---

## F1 — Participant Rename

### Background

Each participant is `{ id, name }`. The `id` is derived from the name at
creation time (via `slugify` + `uniqueId`) and is the stable key used
throughout: blocks, relationships, D3 `nodeMap`, solution cycles. The `name`
is display-only. `syncNodes` already propagates name changes to `nodeMap`
(line ~224), so D3 labels update automatically on the next `restartGraph`.

### UI design

Add a small edit button (✎) to each participant row, after the name and before
the × remove button.

On click:

- Replace the `<span>` containing the name with a pre-filled `<input>` (same
  width as the span's parent flex container, minus the two buttons)
- Focus the input and select all text
- Enter or blur → confirm; Escape → cancel (restore original name)

**Why an edit button rather than double-click?** Double-click is unreliable on
touch and creates gesture ambiguity. An explicit button is consistent with the
existing remove-button pattern and works on all platforms.

### Validation

- Empty name on confirm → reject and restore the previous value (no error
  message needed — the empty input is self-evident)
- Duplicate display name → silently allow. IDs remain distinct, so correctness
  is unaffected. A warning would be noisy for low consequence.

### After confirm

```js
p.name = newName;
saveStateDebounced();
renderSidebar(); // re-renders participant list (exits edit mode)
restartGraph(); // syncNodes picks up name change → D3 label updates
```

Node positions are preserved — `nodeMap` is keyed by ID, not name, so the
node stays in place with its new label.

### HTML change

Add an edit button to the participant list item template in
`renderParticipantList()`. No `index.html` change needed (list is rendered
via JS).

### CSS change

Style the edit button similarly to the remove button (`.remove-btn`). May
share the same class or a new `.edit-btn` class with a different icon color
on hover (e.g., `var(--accent)` instead of `var(--danger)`).

---

## F2 — Block Group Data Model

### State changes

Add `blockGroups` to the state object:

```js
const state = {
  // ...existing fields...
  blockGroups: [], // [{ id: string, label: string, collapsed: boolean }]
};
```

Extend blocks with an optional `group` field (the `id` of a `blockGroup`):

```js
// Before: { from: string, to: string }
// After:  { from: string, to: string, group?: string }
```

Blocks without a `group` (or `group: null / undefined`) are "ungrouped" and
render before all groups.

### Group ID generation

Use `uniqueId(slugify(label), existingGroupIds)` — same mechanism as
participant IDs. For a label "History 2026-04-06" this yields `"history-2026-04-06"`,
or `"history-2026-04-06-2"` if that already exists.

### API impact

`effectiveBlocks()` currently spreads `state.blocks` directly into the API
payload. Update it to strip `group` before sending:

```js
export function effectiveBlocks(state) {
  return [
    ...state.blocks.map(({ from, to }) => ({ from, to })),
    ...state.relationships.flatMap((r) => [
      { from: r.a, to: r.b },
      { from: r.b, to: r.a },
    ]),
  ];
}
```

Go's `json.Unmarshal` ignores unknown fields, so this is also safe without the
strip — but explicit is better.

### Persistence

**localStorage** — `saveState()` adds `blockGroups`:

```js
blockGroups: state.blockGroups,
```

**`applyImport()`** — restore from doc:

```js
state.blockGroups = doc.blockGroups ?? [];
```

**`collapsed` state** — included in localStorage (preserved across sessions),
NOT included in the URL hash (it is a local UI preference, not part of the
solve setup).

**Downloaded JSON** — include `blockGroups` in the export for round-trip
fidelity. When importing a JSON without `blockGroups`, all blocks are treated
as ungrouped.

---

## F3 — Block Group UI

### Rendering

`renderBlockList()` becomes group-aware. Structure:

```
[ungrouped block] A → B             [×]
[ungrouped block] C → D             [×]

▾ Siblings                       [✎] [×]
  E → F                          [×]
  F → E                          [×]

▸ History 2025-12-25              [✎] [×]   ← collapsed
```

Implementation approach:

1. Separate `state.blocks` into ungrouped (no `group` field) and per-group
   buckets. Render ungrouped blocks first as today, then each group in
   `state.blockGroups` order.
2. Each group renders as a group header `<div>` followed by an inner list
   (hidden when `collapsed: true`).

**Group header** contains:

- Toggle arrow (▾ / ▸) that sets `collapsed` and re-renders
- Group label text
- Edit button (✎) for inline rename
- Delete button (×) that removes the group and all its blocks

**Inline group rename:** Same Enter/blur/Escape pattern as participant rename.
The group `label` updates in place; the `id` stays stable.

**Empty groups:** Kept visible (header shown, inner list empty). Can be
deleted explicitly. Auto-removing on last block removal would be surprising.

### CSS additions

```css
.block-group-header {
  display: flex;
  align-items: center;
  gap: 5px;
  padding: 5px 0 3px;
  font-size: 11px;
  font-weight: 600;
  color: var(--muted);
  cursor: pointer;
  user-select: none;
}

.block-group-toggle {
  flex-shrink: 0;
  font-size: 9px;
}
.block-group-label {
  flex: 1;
}

.block-group-inner {
  padding-left: 10px;
}
.block-group-inner[hidden] {
  display: none;
}
```

Edit/delete buttons on group headers reuse `.remove-btn` / `.edit-btn` styles
from the participant list.

### HTML change

No `index.html` change required — the group structure is rendered entirely
by `renderBlockList()`.

---

## F4 — "Add as history blocks" Creates a Named Group

### Current behavior

`onAddAsHistoryBlocks()` iterates `sol.assignments`, deduplicates against
existing blocks, and pushes `{ from: gifter_id, to: recipient_id }` objects
directly into `state.blocks`.

### New behavior

```js
function onAddAsHistoryBlocks() {
  const sol = state.solutions[state.selectedSolution];
  if (!sol) return;

  // Build label, ensuring uniqueness if clicked multiple times same day
  const today = new Date().toISOString().slice(0, 10); // "YYYY-MM-DD"
  const baseLabel = `History ${today}`;
  const existingLabels = new Set(state.blockGroups.map((g) => g.label));
  let label = baseLabel;
  for (let n = 2; existingLabels.has(label); n++) {
    label = `${baseLabel} (${n})`;
  }

  // Create the group
  const existingGroupIds = new Set(state.blockGroups.map((g) => g.id));
  const groupId = uniqueId(slugify(label), existingGroupIds);
  state.blockGroups.push({ id: groupId, label, collapsed: false });

  // Add non-duplicate blocks tagged with the new group
  for (const { gifter_id, recipient_id } of sol.assignments) {
    if (
      !state.blocks.some((b) => b.from === gifter_id && b.to === recipient_id)
    ) {
      state.blocks.push({ from: gifter_id, to: recipient_id, group: groupId });
    }
  }

  state.solutions = [];
  state.selectedSolution = 0;
  saveStateDebounced();
  renderSidebar();
  renderSolutionsPanel();
  restartGraph();
}
```

**Duplicate handling:** Blocks already present (with or without a group) are
skipped. This preserves existing behavior and avoids duplicates across groups.

---

## F5 — URL Hash v2 Format

### Motivation

Groups can represent meaningful structure ("Siblings", "History 2024") that
should survive link sharing. The hash is the right place for this — it already
encodes the full problem setup.

### v2 format

Add `g` (group labels array) to the compact object, and a third element to
each block entry for the group index:

```js
{
  v: 2,
  p: [[id, name], ...],               // unchanged
  r: [[ai, bi], ...],                 // unchanged
  b: [[fromIdx, toIdx], ...],         // ungrouped blocks (no group element)
  bg: [[fromIdx, toIdx, gi], ...],    // grouped blocks (gi = index into g)
  g: ["Siblings", "History 2026-04-06", ...],  // group labels in order
  o: { m, s },                        // unchanged
}
```

Splitting blocks into `b` (ungrouped) and `bg` (grouped) keeps the v2 format
cleanly backward-compatible at the field level.

**`collapsed` state is excluded** — it is a local UI preference.

**Group IDs are not encoded** — only labels. On decode, new IDs are generated
from the labels using `uniqueId(slugify(label), ...)`. This is fine because IDs
are internal references; the URL was generated from the current state and
decoded fresh.

### Backward compatibility

`decodeStateFromHash` already checks `if (!hash.startsWith("#v1:")) return null`.
Extend to handle both:

```js
export function decodeStateFromHash(hash) {
  if (hash.startsWith("#v2:")) return decodeV2(hash);
  if (hash.startsWith("#v1:")) return decodeV1(hash);
  return null;
}
```

`encodeStateToHash` always writes `#v2:` going forward. Old `#v1:` links
continue to decode correctly (all blocks become ungrouped, `blockGroups: []`).

---

## Files to Change

| File                    | Changes                                                                                                                                                                                                                                                                 |
| ----------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `server/web/app.js`     | F1: edit button + rename logic in `renderParticipantList`; F2: `state.blockGroups`, `effectiveBlocks` strip, `saveState`/`applyImport` updates; F3: group-aware `renderBlockList`; F4: `onAddAsHistoryBlocks` rewrite; F5: `encodeStateToHash`/`decodeStateFromHash` v2 |
| `server/web/style.css`  | F1: `.edit-btn` style; F3: `.block-group-header`, `.block-group-inner`, `.block-group-toggle`, `.block-group-label` styles                                                                                                                                              |
| `server/web/index.html` | No changes expected                                                                                                                                                                                                                                                     |

No Go files, no API changes, no test changes.

---

## Acceptance Criteria

- **F1**: Double-clicking the ✎ button on a participant row shows an input
  pre-filled with the name. Enter/blur confirms; Escape restores. The D3 node
  label updates immediately. All blocks and relationships referencing that
  participant by ID continue to work. Empty name is rejected.

- **F2**: `state.blockGroups` is saved to and loaded from localStorage.
  `effectiveBlocks()` strips the `group` field — the API payload is
  `[{from, to}, ...]` with no `group` field.

- **F3**: Ungrouped blocks render as today. Each group renders as a collapsible
  section with a header showing the label, ✎ rename, and × delete. Collapsing
  a group persists across page reloads (via localStorage). Deleting a group
  removes all its blocks and clears solutions. Renaming a group updates the
  label in place (ID unchanged).

- **F4**: Clicking "Add as history blocks" creates a new group named
  `"History YYYY-MM-DD"` (or with a counter suffix if that label already
  exists) and adds the solution's assignments to it as blocks (deduplicating
  against all existing blocks regardless of group).

- **F5**: `encodeStateToHash` produces a `#v2:` URL that includes group labels
  and group assignments for each block. `decodeStateFromHash` handles both
  `#v1:` and `#v2:` prefixes. Old v1 links load correctly with all blocks
  ungrouped.
