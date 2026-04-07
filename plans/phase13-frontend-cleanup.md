# Phase 13 — Frontend Code Cleanup (Frontend-only)

## Status

- [x] **B1** — Fix `onReset` not clearing `blockGroups`
- [x] **B2** — Remove `window._state` debug artifact
- [x] **S1** — Extract `mutated()` helper to DRY mutation/re-render sequences
- [x] **S2** — Fold `renderOptions()` into `renderSidebar()`; remove manual seed DOM update
- [x] **S3** — Rename `confirm` local variable to avoid shadowing global
- [x] **S4** — Simplify `buildValidEdges` to remove string round-trip
- [x] **H1** — Fix `<ul id="block-list">` invalid children: change to `<div>`
- [x] **C1** — Extract `--accent-ring` CSS custom property
- [x] **C2** — Unify `.edit-btn` / `.remove-btn` into a shared `.icon-btn` base class
- [x] **C3** — Remove redundant `.btn-primary:disabled` rule
- [x] **C4** — Add CSS tokens for hash-banner colors

All changes are frontend-only (`server/web/`). No Go changes, no API changes,
no behavior changes visible to users.

---

## Overview

This phase addresses two real bugs (B1, B2), four structural/DRY improvements
(S1–S4), one HTML validity issue (H1), and four CSS cleanup items (C1–C4).
None of these change observable behavior except B1 (bug fix).

Items are independent and can be implemented in any order. Suggested grouping:
bugs first (B1, B2), then JS (S1–S4), then HTML+CSS (H1, C1–C4).

---

## B1 — `onReset` Does Not Clear `blockGroups`

**File**: `app.js`

`onReset()` clears `participants`, `relationships`, `blocks`, `options`,
`solutions`, and `selectedSolution`, but leaves `state.blockGroups` populated.
After a reset the UI looks correct (no blocks shown), but the stale group
objects persist in memory and would be written to localStorage on the next
`saveState()` call. A subsequent "Add as history blocks" call could then find
pre-existing group IDs and produce unexpected collisions.

**Fix**: Add `state.blockGroups = [];` to `onReset()`.

---

## B2 — `window._state` Debug Artifact

**File**: `app.js`, line 26

```js
window._state = state; // ← delete this line
```

A console shortcut left in production. Leaks internal state object to the
global scope.

---

## S1 — Extract `mutated()` Helper

**File**: `app.js`

The sequence:

```js
state.solutions = [];
state.selectedSolution = 0;
saveStateDebounced();
renderSidebar();
renderSolutionsPanel();
restartGraph();
```

appears 8+ times: in the add-block handler, add-relationship handler, remove-
participant handler, remove-block handler (`makeBlockItem`), remove-relationship
handler, delete-group handler, and `onAddAsHistoryBlocks`. This is the dominant
maintainability problem in the file — every mutation that invalidates solutions
copies the same 6 lines.

**Fix**: Extract a helper and replace all call sites:

```js
// Called after any mutation that invalidates existing solutions.
function mutated() {
  state.solutions = [];
  state.selectedSolution = 0;
  saveStateDebounced();
  renderSidebar();
  renderSolutionsPanel();
  restartGraph();
}
```

Some call sites also call `updateEmptyState()` (participant add/remove). Keep
those explicit after `mutated()` since they are conditional on participant
count, not solution invalidation.

`onAddAsHistoryBlocks` calls `saveStateDebounced()` — after S1 it uses
`mutated()` which calls the same. Fine.

---

## S2 — Fold `renderOptions()` into `renderSidebar()`

**File**: `app.js`

`renderSidebar()` renders participants, relationships, blocks, and the generate
button, but not options. Every call site that needs a complete sidebar re-render
must remember to call `renderOptions()` separately. This is inconsistently
applied: `onReset` and `onImport` call both; add/remove operations call neither
(relying on the fact that options don't change during those flows).

Additionally, `onGenerate` manually mutates the seed DOM element:

```js
document.getElementById("opt-seed").value = resp.seed_used;
```

This is the only place state is written to the DOM outside of a render
function. Since `state.options.seed = resp.seed_used` is set just before, a
`renderOptions()` call would handle it correctly.

**Fix**:

1. Add `renderOptions()` call at the end of `renderSidebar()`.
2. Replace the manual DOM update in `onGenerate` with nothing — `renderSidebar()`
   (already called after generate) will handle it.
3. Remove the now-redundant standalone `renderOptions()` calls from `onReset`
   and `onImport`.

---

## S3 — Rename `confirm` Local Variable

**File**: `app.js`, `renderParticipantList`

```js
function confirm() {   // ← shadows window.confirm
  ...
}
```

Rename to `commitRename` (or `applyRename`). Same change applies to the
analogous `confirmRename` function inside `renderBlockList`'s group edit handler
— that one is already named differently (`confirmRename`), so only the
participant case needs updating.

---

## S4 — Simplify `buildValidEdges`

**File**: `app.js`

Current flow:

1. Build `blockedSet` — set of `"from→to"` strings. ✓
2. Build `pairSet` — set of all valid `"from→to"` strings (O(n²)).
3. Iterate `pairSet`, call `indexOf("→")` on each key to recover `fromId`/`toId`,
   then build the edge object (including a `.has()` check back into `pairSet`
   for bidirectionality).

Step 3 is the awkward part: strings are built in step 2 only to be parsed back
in step 3. The bidirectionality check is the only reason for the pre-pass.

**Fix**: Build edge objects directly in the nested loop, and determine
bidirectionality with a second `.has()` check against the same `pairSet` (which
must be pre-computed to support this check). The `pairSet` stays, but the
string-parse loop is replaced by a direct nested iteration that avoids
rebuilding the key:

```js
export function buildValidEdges(participants, blocks) {
  const blockedSet = new Set(blocks.map((b) => `${b.from}\u2192${b.to}`));
  const validSet = new Set();
  for (const src of participants)
    for (const tgt of participants)
      if (src.id !== tgt.id && !blockedSet.has(`${src.id}\u2192${tgt.id}`))
        validSet.add(`${src.id}\u2192${tgt.id}`);

  const edges = [];
  for (const src of participants) {
    for (const tgt of participants) {
      const key = `${src.id}\u2192${tgt.id}`;
      if (validSet.has(key)) {
        edges.push({
          source: src.id,
          target: src.id, // d3 resolves by id
          sourceId: src.id,
          targetId: tgt.id,
          kind: "valid",
          bidirectional: validSet.has(`${tgt.id}\u2192${src.id}`),
        });
      }
    }
  }
  return edges;
}
```

This eliminates the `indexOf` / `slice` string parsing and keeps all logic in
the two nested loops. The algorithm is the same; only the implementation is
cleaner.

Note: `source` and `target` are set to `src.id` and `tgt.id` (strings) — d3's
`forceLink` with `.id(d => d.id)` resolves these to node objects. The existing
code already does this correctly.

---

## H1 — `<ul id="block-list">` Has Invalid Direct Children

**Files**: `index.html`, `app.js`, `style.css`

After the block groups feature, `renderBlockList()` appends `<div>` (group
headers) and nested `<ul>` (group inner lists) as direct children of
`<ul id="block-list">`. Per the HTML spec, `<ul>` may only contain `<li>`
elements as direct children. Browsers handle this gracefully but it is invalid
markup and can confuse accessibility tools.

**Fix**: Change `<ul id="block-list">` to `<div id="block-list">` in
`index.html`. No JS changes needed — `renderBlockList()` already appends the
correct element types. The CSS rules targeting `#block-list li` remain valid
(they match descendant `<li>` elements). The general list container styles
(`list-style: none; display: flex; flex-direction: column; gap: 3px`) should
move from the combined `#participant-list, #relationship-list, #block-list`
rule to only `#participant-list, #relationship-list` for the `<ul>` elements,
and a separate `#block-list` rule for the `<div>`.

---

## C1 — Extract `--accent-ring` CSS Custom Property

**File**: `style.css`

`rgba(37, 99, 235, 0.15)` — the blue focus ring color — appears in three
rules:

- `.add-row input:focus, .add-row select:focus`
- `.option-fields input:focus`
- `.rename-input`

Add to `:root`:

```css
--accent-ring: rgba(37, 99, 235, 0.15);
```

Replace all three occurrences with `var(--accent-ring)`.

---

## C2 — Unify `.edit-btn` / `.remove-btn` into `.icon-btn`

**File**: `style.css`

The participant list `.remove-btn` and `.edit-btn` styles share:
`background: none; border: none; cursor: pointer; color: var(--muted);
line-height: 1; padding: 0 2px; flex-shrink: 0;`

The only differences are `font-size` (15px vs 13px) and `margin-left`
(6px vs 4px), and the hover color (`--danger` vs `--accent`).

Extract a base class:

```css
.icon-btn {
  background: none;
  border: none;
  cursor: pointer;
  color: var(--muted);
  line-height: 1;
  padding: 0 2px;
  flex-shrink: 0;
}
.icon-btn:disabled {
  opacity: 0.3;
  cursor: default;
}
```

Then keep only the differing properties on `.remove-btn` and `.edit-btn`:

```css
.remove-btn {
  font-size: 15px;
  margin-left: 6px;
}
.remove-btn:hover {
  color: var(--danger);
}

.edit-btn {
  font-size: 13px;
  margin-left: 4px;
}
.edit-btn:hover {
  color: var(--accent);
}
```

Update HTML/JS to use class `icon-btn remove-btn` and `icon-btn edit-btn`.
The block-group-header context overrides (`margin-left: 0`, `opacity: 0`)
remain on `.block-group-header .edit-btn` / `.block-group-header .remove-btn`.

---

## C3 — Remove Redundant `.btn-primary:disabled`

**File**: `style.css`

```css
.btn-primary:disabled {
  opacity: 0.55;
  cursor: not-allowed;
} /* ← remove */
```

`button:disabled { opacity: 0.55; cursor: not-allowed; }` already covers this.
The primary-button rule is redundant.

---

## C4 — CSS Tokens for Hash-Banner Colors

**File**: `style.css`

The hash-banner uses four hardcoded colors that don't appear elsewhere:
`#eff6ff` (bg), `#bfdbfe` (border), `#1e40af` (text), `#93c5fd` (dismiss).

Add to `:root`:

```css
--banner-bg: #eff6ff;
--banner-border: #bfdbfe;
--banner-text: #1e40af;
--banner-dismiss: #93c5fd;
```

Replace in `.hash-banner` and `.hash-banner-dismiss` rules. Lower priority
than C1–C3 but keeps all color decisions in one place.

---

## Files to Change

| File                    | Changes                              |
| ----------------------- | ------------------------------------ |
| `server/web/app.js`     | B1, B2, S1, S2, S3, S4               |
| `server/web/index.html` | H1 (ul → div for block-list)         |
| `server/web/style.css`  | H1 (selector update), C1, C2, C3, C4 |

---

## Acceptance Criteria

- **B1**: After clicking Reset with block groups present, `state.blockGroups`
  is empty. localStorage contains no block groups after the next save.
- **B2**: `window._state` is undefined after page load.
- **S1**: All 8+ mutation call sites use `mutated()`. No inline copies of the
  solutions-clear + re-render sequence remain.
- **S2**: `renderSidebar()` calls `renderOptions()`. `onGenerate` does not
  manually mutate any DOM elements. `onReset` and `onImport` do not call
  `renderOptions()` standalone.
- **S3**: No local variable named `confirm` exists in `app.js`.
- **S4**: `buildValidEdges` contains no `indexOf` / `slice` string-parsing.
  Behavior is identical (same edges returned for same input).
- **H1**: `<ul id="block-list">` is replaced with `<div id="block-list">`.
  Block list renders correctly with groups and individual items. HTML validates
  without errors on the block list structure.
- **C1**: `--accent-ring` defined in `:root`; `rgba(37, 99, 235, 0.15)` does
  not appear elsewhere in the file.
- **C2**: `.icon-btn` class exists. `.remove-btn` and `.edit-btn` elements in
  JS use both classes. No shared properties remain duplicated between the two.
- **C3**: `.btn-primary:disabled` rule removed.
- **C4**: Hash-banner color literals replaced with CSS custom properties.
