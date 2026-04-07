# Phase 14 — Solution Sharing & Graph UX (Frontend-only)

## Status

- [ ] **F1** — Deselect solution by clicking the active tab
- [ ] **F2** — Toggle valid/background edges via graph overlay button
- [ ] **F3** — Hash v3 encoder: solution cycles + presentation flag
- [ ] **F4** — Hash v3 decoder: reconstruct solution, detect presentation mode
- [ ] **F5** — Presentation mode on load: collapse sidebar, update banner

All changes are frontend-only (`server/web/`). No Go changes, no API changes,
no behavior changes except those explicitly described below.

---

## Overview

Two independent graph UX improvements (F1, F2) and a three-part extension
of the URL hash sharing mechanism (F3–F5).

**Graph UX** (F1, F2): deselect a selected solution by clicking its active
tab; toggle the grey "valid" background edges off so only the solution
overlay is visible.

**Sharing** (F3–F5): "Copy Link" is enriched — it now always encodes a
presentation mode flag that collapses the sidebar on load, and when a
solution is selected it also encodes the solution cycles directly in the hash
so the recipient sees the result immediately without re-solving.

F1 and F2 are independent and can be implemented in any order. F4 depends
on F3; F5 requires F3 and F4.

---

## F1 — Deselect Solution by Clicking the Active Tab

**File**: `app.js`

### State change

`state.selectedSolution` currently holds a `number` (0-indexed, always ≥ 0).
After this change it may also be `null`, meaning no solution is selected.

Audit of all call sites that read `state.selectedSolution`:

| Site | Behaviour with `null` | Action needed |
|---|---|---|
| `state.solutions[state.selectedSolution]` | `undefined` (same as empty array) | None — all consumers already guard |
| `buildSolutionEdges(undefined)` | Returns `[]` — already guarded | None |
| `buildNodeColors(undefined)` | Returns `{}` — already guarded | None |
| `onAddAsHistoryBlocks` | `if (!sol) return;` already present | None |
| `onDownload` | `_selected_solution: null` is valid JSON | None |
| `renderSolutionsPanel` | Crashes: `sol.score` on undefined | Fix — see below |
| `recolorGraph` | Calls `buildSolutionEdges(undefined)` → `[]` | None |
| `encodeStateToHash` (after F3) | `state.solutions[null]` → undefined → no `c` field | None |

### `renderSolutionsPanel` fix

The current function unconditionally destructures `sol.score` after the tab
loop. With `selectedSolution = null`, `sol` is `undefined` and this throws.

Restructure the function into three phases:

1. **No solutions** (`!state.solutions.length`): clear tabs, clear detail,
   hide both action buttons, return early. *(existing behaviour)*
2. **Solutions exist, none selected** (`selectedSolution === null`): render
   tabs (none marked `.active`), clear detail, hide both action buttons,
   return early. *(new)*
3. **Solution selected**: render tabs, render detail, show action buttons.
   *(existing behaviour, now the last branch)*

```
// phase 1
if (!state.solutions.length) { ... ; return; }

// tabs (shared by phases 2 and 3)
tabsEl.innerHTML = "";
state.solutions.forEach((_, i) => { ... });

// phase 2
const sol = state.solutions[state.selectedSolution];
if (!sol) {
  detailEl.innerHTML = "";
  histBtn.hidden = true;
  dlBtn.hidden = true;
  return;
}

// phase 3 — render detail (unchanged)
```

### Tab click handler

Toggle-off when clicking the already-active tab:

```js
btn.addEventListener("click", () => {
  state.selectedSolution = state.selectedSolution === i ? null : i;
  renderSolutionsPanel();
  recolorGraph();
});
```

### `mutated()` update

`mutated()` currently resets `state.selectedSolution = 0`. Change to `null`
since solutions are being cleared simultaneously and `0` is meaningless with
an empty solutions array:

```js
function mutated() {
  state.solutions = [];
  state.selectedSolution = null;
  ...
}
```

`onGenerate` (and `onImport`) explicitly set `state.selectedSolution = 0`
after a successful solve. No change needed there.

---

## F2 — Toggle Valid/Background Edges

**Files**: `app.js`, `index.html`, `style.css`

### State

Add a new field to the state object:

```js
showValidEdges: true,
```

This is session-only. It is **not** persisted to localStorage and **not**
encoded in the hash (it is a display preference, not problem state).

### Graph overlay button

A small toggle button is overlaid in the top-right corner of
`#graph-container`, spatially connected to what it controls.

**`index.html`** — inside `#graph-container`, after `#graph-empty`:

```html
<button id="btn-toggle-edges" class="graph-overlay-btn">Hide edges</button>
```

Label alternates: "Hide edges" when edges are visible, "Show edges" when hidden.

**`style.css`** — new rule in the graph section:

```css
.graph-overlay-btn {
  position: absolute;
  top: 8px;
  right: 8px;
  font-size: 11px;
  padding: 3px 8px;
  opacity: 0.7;
}
.graph-overlay-btn:hover { opacity: 1; }
```

### Wire event

In `wireEvents()`:

```js
document.getElementById("btn-toggle-edges").addEventListener("click", () => {
  state.showValidEdges = !state.showValidEdges;
  document.getElementById("btn-toggle-edges").textContent =
    state.showValidEdges ? "Hide edges" : "Show edges";
  validEdgeLayer.style("display", state.showValidEdges ? "" : "none");
});
```

No simulation restart needed — `validEdgeLayer` is already in module scope.

### `restartGraph()` update

After the existing `validEdgeLayer` `.data().join()` call, apply the
current visibility state so the toggle is respected across re-renders:

```js
validEdgeLayer.style("display", state.showValidEdges ? "" : "none");
```

---

## F3 — Hash v3 Encoder

**File**: `app.js`

### v3 compact format

Extends v2 with two new optional top-level fields:

| Field | Type | Included when | Meaning |
|---|---|---|---|
| `pres` | `true` | Always in v3 | Recipient loads in presentation mode |
| `c` | `number[][]` | Solution selected | Cycles as participant-index arrays |

The `c` field encodes `sol.cycles` using the same participant-index mapping
already computed in `encodeStateToHash` (`idxOf`). A 20-person Hamiltonian
cycle is 20 integers — roughly 60 extra base64 characters in the URL.

Assignments and score are **not** stored directly. The decoder reconstructs
them from the cycles (see F4).

### `encodeStateToHash` rewrite

```js
export function encodeStateToHash(state) {
  const idxOf = Object.fromEntries(state.participants.map((p, i) => [p.id, i]));
  const groupIdxOf = Object.fromEntries(state.blockGroups.map((g, i) => [g.id, i]));

  const ungrouped = state.blocks.filter(b => !b.group);
  const grouped   = state.blocks.filter(b =>  b.group);

  const compact = {
    v: 3,
    p: state.participants.map(p => [p.id, p.name]),
    r: state.relationships.map(r => [idxOf[r.a], idxOf[r.b]]),
    b: ungrouped.map(b => [idxOf[b.from], idxOf[b.to]]),
    ...(grouped.length ? {
      g: state.blockGroups.map(g => g.label),
      bg: grouped.map(b => [idxOf[b.from], idxOf[b.to], groupIdxOf[b.group]]),
    } : {}),
    pres: true,
  };

  if (state.options.maxSolutions !== 5 || state.options.seed != null) {
    compact.o = {};
    if (state.options.maxSolutions !== 5) compact.o.m = state.options.maxSolutions;
    if (state.options.seed != null) compact.o.s = state.options.seed;
  }

  const sol = state.solutions[state.selectedSolution];
  if (sol) {
    compact.c = sol.cycles.map(cycle => cycle.map(id => idxOf[id]));
  }

  return "#v3:" + hashEncode(compact);
}
```

---

## F4 — Hash v3 Decoder

**File**: `app.js`

### `decodeStateFromHash` update

Add a `#v3:` route before the existing v2/v1 routes:

```js
export function decodeStateFromHash(hash) {
  try {
    if (hash.startsWith("#v3:")) return decodeV3(hashDecode(hash.slice(4)));
    if (hash.startsWith("#v2:")) return decodeV2(hashDecode(hash.slice(4)));
    if (hash.startsWith("#v1:")) return decodeV1(hashDecode(hash.slice(4)));
    return null;
  } catch {
    return null;
  }
}
```

### `decodeV3`

v3 shares the same participant / block / relationship / group / options
parsing as v2. To avoid duplication, extract the shared parsing logic into
a private `parseV2Fields(compact)` helper that both `decodeV2` and
`decodeV3` call. `decodeV2` retains its version guard; `decodeV3` adds its
own. Neither changes externally visible behaviour.

`decodeV3` then reads the two new fields:

**Solution reconstruction from `compact.c`:**
- Map each cycle's index array back to participant IDs using `base.participants`
- Derive `assignments` from consecutive cycle pairs (the last element wraps
  to the first)
- Derive `score` from cycle lengths:
  - `num_cycles = cycles.length`
  - `min_cycle_len = Math.min(...cycles.map(c => c.length))`
  - `max_cycle_len = Math.max(...cycles.map(c => c.length))`

The reconstructed `SolutionDTO` is stored as `result._solutions = [sol]` and
`result._selected_solution = 0`, matching the shape `applyImport` already
reads (`doc._solutions`, `doc._selected_solution`). No changes to
`applyImport` are needed.

**Presentation flag:**

```js
result._presentation = compact.pres === true;
```

`_presentation` is ignored by `applyImport` — it is only read by the init
code (F5).

**Graceful degradation:** missing, empty, or malformed `c` is handled with
guards (`Array.isArray`, index bounds check, `.filter(Boolean)`). A bad `c`
results in no solution loaded (recipient sees "click Generate to solve"),
not a crash.

---

## F5 — Presentation Mode on Load

**Files**: `app.js`, `index.html`

### Sidebar collapse

When `hashState._presentation` is true, remove the `open` attribute from
all `<details class="sidebar-section">` elements after the initial render.
This is done after `renderSidebar()` so the render itself doesn't need to
know about presentation mode.

```js
if (hashState?._presentation) {
  document.querySelectorAll("details.sidebar-section")
    .forEach(el => el.removeAttribute("open"));
}
```

### Banner message

Two banner messages are now needed:

- **Solution in hash** (`_presentation && _solutions.length`):
  `"Viewing a shared solution — click Generate to re-solve, or edit freely."`
- **Problem only** (default, existing):
  `"Loaded from a shared link — click Generate to solve, or edit freely."`

To allow the text to be set dynamically, wrap the banner text in a `<span>`
in `index.html`:

```html
<div id="hash-banner" class="hash-banner" hidden>
  <span id="hash-banner-msg">Loaded from a shared link — click <strong>Generate</strong> to solve, or edit freely.</span>
  <button class="hash-banner-dismiss" aria-label="Dismiss">×</button>
</div>
```

Update `showHashBanner` to accept two booleans:

```js
function showHashBanner(isPresentation, hasSolution) {
  const el = document.getElementById("hash-banner");
  document.getElementById("hash-banner-msg").innerHTML =
    (isPresentation && hasSolution)
      ? "Viewing a shared solution — click <strong>Generate</strong> to re-solve, or edit freely."
      : "Loaded from a shared link — click <strong>Generate</strong> to solve, or edit freely.";
  el.hidden = false;
  const dismissTimer = setTimeout(() => { el.hidden = true; }, 8000);
  el.querySelector(".hash-banner-dismiss").addEventListener("click", () => {
    clearTimeout(dismissTimer);
    el.hidden = true;
  }, { once: true });
}
```

### `DOMContentLoaded` init update

```js
const hashState = decodeStateFromHash(location.hash);
if (hashState?.participants.length) {
  history.replaceState(null, "", location.pathname);
  applyImport(hashState, state);
  showHashBanner(hashState._presentation, hashState._solutions?.length > 0);
} else {
  loadFromLocalStorage();
}

renderSidebar();
renderSolutionsPanel();
restartGraph();
updateEmptyState();

if (hashState?._presentation) {
  document.querySelectorAll("details.sidebar-section")
    .forEach(el => el.removeAttribute("open"));
}
```

---

## Files to Change

| File | Changes |
|---|---|
| `server/web/app.js` | F1: tab toggle + null selectedSolution + renderSolutionsPanel fix + mutated; F2: state + wire; F3: encodeStateToHash; F4: decodeStateFromHash + decodeV3 + parseV2Fields; F5: DOMContentLoaded + showHashBanner |
| `server/web/index.html` | F2: overlay button; F5: banner message span |
| `server/web/style.css` | F2: `.graph-overlay-btn` rule |

---

## Acceptance Criteria

### F1
- Clicking the active tab sets `state.selectedSolution = null`; no tab has
  `.active`; the detail area is empty; both action buttons are hidden.
- Clicking any tab when nothing is selected selects it normally.
- `mutated()` sets `state.selectedSolution = null`.
- `recolorGraph()` with `selectedSolution = null` clears all coloured edges
  and resets nodes to grey.
- `buildSolutionEdges` / `buildNodeColors` receive `undefined` and return
  `[]` / `{}` without throwing.

### F2
- Clicking the overlay button hides the grey valid-edge paths; button reads
  "Show edges".
- Clicking again shows them; button reads "Hide edges".
- Coloured solution edges are unaffected by the toggle.
- After `restartGraph()` (e.g., participant added), valid edge visibility
  reflects the current toggle state.

### F3
- `encodeStateToHash` emits a `#v3:` hash with `pres: true` in all cases.
- When a solution is selected, the hash contains `c` with cycle arrays
  encoded as participant indices; indices are correct (verify manually for a
  small case).
- When no solution is selected, `c` is absent.
- Old `#v1:` and `#v2:` links still decode correctly via existing paths.

### F4
- Loading a `#v3:` hash with a valid `c` field: `state.solutions` contains
  one SolutionDTO with correct `cycles`, `assignments`, and `score`;
  `state.selectedSolution` is `0`; solutions panel renders immediately.
- Assignments: every consecutive pair in every cycle appears as a
  `gifter_id → recipient_id` entry, including the last→first wrap.
- Score: `num_cycles`, `min_cycle_len`, `max_cycle_len` match the cycles.
- Loading a `#v3:` hash with no `c` field: solutions panel is empty; banner
  says "click Generate to solve".
- Malformed `c` (out-of-range indices, empty arrays): no crash; falls back
  gracefully to no-solution state.

### F5
- Loading a `#v3:` link: all `<details class="sidebar-section">` start
  without `open`; sidebar appears collapsed.
- Loading from localStorage (no hash): sidebar opens normally.
- Banner text is "Viewing a shared solution…" when a solution was in the
  hash; "Loaded from a shared link…" when no solution was in the hash.
- Recipient can open sidebar sections and click Generate freely after load.
