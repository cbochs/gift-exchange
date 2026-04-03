# Phase 6 — UI Polish

## Status

- [ ] Bug fix: solution display (names, per-assignment layout, no truncation)
- [ ] Bug fix: collapsible sidebar sections
- [ ] Feature: symmetric relationships
- [ ] Feature: "Add as history blocks" button
- [ ] Feature: mobile layout

## Goal

Five targeted improvements: two bug fixes to existing behavior, two new input features, and a mobile-friendly layout. All changes are frontend-only.

---

## 1. Bug Fix: Solution Display

### 1.1 Problem

The current rendering in `renderSolutionsPanel()` (app.js:440–444):

```js
const loop = esc([...cycle, cycle[0]].join(" → "));
return `<div class="cycle-line" ...>[${ci + 1}] ${loop}</div>`;
```

displays participant **IDs** (not names), and the CSS `.cycle-line { white-space: nowrap; overflow: hidden; text-overflow: ellipsis }` silently truncates long cycles.

### 1.2 New Display Format

Replace the single-line cycle string with grouped per-assignment rows:

```
● Cycle 1 · 15 people
  Alice Smith → Bob Jones
  Bob Jones → Carol Wu
  Carol Wu → Dave Kim
  ...

● Cycle 2 · 3 people
  Eve Lee → Frank Hall
  ...
```

The colored dot (`●`) matches the cycle color in the graph. Each assignment is its own line — no truncation possible.

**HTML structure:**

```html
<div class="cycle-group">
  <div class="cycle-header">
    <span class="cycle-dot" style="background:#4c72b0"></span>
    <span>Cycle 1 · 4 people</span>
  </div>
  <div class="cycle-assignments">
    <div class="assignment-line">Alice Smith → Bob Jones</div>
    <div class="assignment-line">Bob Jones → Carol Wu</div>
    ...
  </div>
</div>
```

**JS change.** Build a name lookup map before rendering:

```js
const nameOf = Object.fromEntries(
  state.participants.map((p) => [p.id, p.name]),
);
```

Use `nameOf[id] ?? id` when rendering each assignment (fallback to ID if a name is somehow missing).

**CSS changes.** Remove the old `.cycle-line` truncation rules. Add:

```css
.cycle-group {
  margin-bottom: 10px;
}
.cycle-header {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 11px;
  font-weight: 600;
  color: var(--muted);
  margin-bottom: 3px;
}
.cycle-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  flex-shrink: 0;
}
.assignment-line {
  font-family: ui-monospace, monospace;
  font-size: 12px;
  padding: 1px 0 1px 14px;
}
```

---

## 2. Bug Fix: Collapsible Sidebar Sections

### 2.1 Problem

When the Blocks list is long (e.g., 20+ entries from years of history), the sidebar requires significant scrolling to reach Options and Generate.

### 2.2 Design

Use native `<details>/<summary>` HTML. Each sidebar section becomes a `<details open>` element. The browser handles toggle behavior; no JavaScript state is needed. The `open` attribute persists across `renderParticipantList()` / `renderBlockList()` re-renders because those functions only update the inner list elements, not the wrapping `<details>`.

**HTML change.** Replace each `<section class="sidebar-section">` with:

```html
<details class="sidebar-section" open>
  <summary>Participants</summary>
  <!-- existing section content unchanged -->
</details>
```

**CSS.** Style `summary` to match the current `h2` headers; remove the browser default list marker; add a chevron indicator:

```css
details.sidebar-section > summary {
  font-size: 10px;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.06em;
  color: var(--muted);
  cursor: pointer;
  user-select: none;
  list-style: none;
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding-bottom: 10px;
}
details.sidebar-section > summary::marker,
details.sidebar-section > summary::-webkit-details-marker {
  display: none;
}
details.sidebar-section > summary::after {
  content: "▾";
  font-size: 10px;
}
details.sidebar-section:not([open]) > summary::after {
  content: "▸";
}
```

**Sections affected:** Participants, Relationships (new in Feature 3), Blocks, Options. The Generate row stays as-is (not a collapsible section).

---

## 3. Feature: Symmetric Relationships

### 3.1 Motivation

Partner and sibling constraints are inherently symmetric. Entering two directed blocks per pair is tedious. A Relationships section handles this as a single `Alice ↔ Bob` entry that auto-generates both directions.

### 3.2 State

```js
const state = {
  participants: [],
  relationships: [], // [{a, b}] — symmetric pairs
  blocks: [],        // directed history / asymmetric constraints
  ...
};
```

### 3.3 Effective Blocks

A helper computes the union of explicit blocks plus relationship expansions. Called everywhere `state.blocks` was previously passed to graph functions and `stateToRequest`:

```js
function effectiveBlocks(state) {
  return [
    ...state.blocks,
    ...state.relationships.flatMap((r) => [
      { from: r.a, to: r.b },
      { from: r.b, to: r.a },
    ]),
  ];
}
```

### 3.4 UI

**Sidebar order:** Participants → Relationships → Blocks → Options → Generate.

**Relationships section.** Two dropdowns (participant A, participant B) with no arrow label, Add button, list rendering `Alice ↔ Bob` with remove buttons. Structurally reuses the Blocks section layout.

**Deduplication.** Canonical key: `[min(a,b), max(a,b)].join("|")`. Silently skip duplicates.

**Self-pairing.** Skip if both dropdowns select the same participant.

**Cascade delete.** When a participant is removed:

```js
state.relationships = state.relationships.filter(
  (r) => r.a !== id && r.b !== id,
);
```

### 3.5 Import/Export

```json
{
  "participants": [...],
  "relationships": [{"a": "alice", "b": "bob"}],
  "blocks": [...],
  "options": {...},
  "_selected_solution": 0,
  "_solutions": [...]
}
```

`applyImport()` reads `doc.relationships ?? []`. Pre-Phase-6 imports default to empty — backward compatible.

---

## 4. Feature: "Add as History Blocks"

### 4.1 Motivation

After generating a solution, the organizer wants to block those assignments next year. One click adds all assignments from the selected solution directly to `state.blocks`.

### 4.2 Design

**Button.** In the solutions panel alongside Download JSON. Label: **Add as history blocks**. Visible when a solution is selected.

**Action.**

```js
function onAddAsHistoryBlocks() {
  const sol = state.solutions[state.selectedSolution];
  if (!sol) return;
  for (const { gifter_id, recipient_id } of sol.assignments) {
    if (
      !state.blocks.some((b) => b.from === gifter_id && b.to === recipient_id)
    ) {
      state.blocks.push({ from: gifter_id, to: recipient_id });
    }
  }
  // Solutions are now stale — clear them
  state.solutions = [];
  state.selectedSolution = 0;
  renderSidebar();
  renderSolutionsPanel();
  restartGraph();
}
```

**Side effects.** `state.solutions` is cleared (computed without these new blocks; would be invalid). User clicks Generate to get fresh solutions under the new constraints.

**Export.** Added blocks land in `state.blocks` → automatically included in JSON export. No special handling.

---

## 5. Feature: Mobile Layout

### 5.1 Design Decision

**Approach: stacked layout at ≤640px breakpoint.** No JavaScript tab-switching required. The collapsible sections (Feature 2 above) are the key enabler — users collapse input sections they've finished with, then scroll down to see the graph and solutions.

**Not chosen:** tab-based navigation (more complex to implement; not necessary given collapsible sections).

### 5.2 Desktop Layout (unchanged)

```
Header
├── Sidebar (272px fixed)   │   Graph (flex: 1)
│   Participants             │
│   Relationships            │   ──────────────
│   Blocks                   │   Solutions (210px)
│   Options                  │
│   Generate                 │
```

### 5.3 Mobile Layout (≤640px)

```
Header
Participants (collapsible)
Relationships (collapsible)
Blocks (collapsible)
Options (collapsible)
Generate
────────────────────────────
Graph (300px fixed height)
────────────────────────────
Solutions (height: auto)
```

Body scrolls vertically. The viewport-height constraint (`height: 100vh; overflow: hidden`) is lifted.

### 5.4 CSS Changes

```css
@media (max-width: 640px) {
  body {
    height: auto;
    min-height: 100vh;
    overflow-y: auto;
  }
  .layout {
    flex-direction: column;
    overflow: visible;
  }
  .sidebar {
    width: 100%;
    border-right: none;
    border-bottom: 1px solid var(--border);
    overflow-y: visible;
    flex-shrink: 0;
  }
  .main-panel {
    overflow: visible;
    min-height: 0;
  }
  #graph-container {
    height: 300px;
    flex-shrink: 0;
  }
  #solutions-panel {
    height: auto;
    flex-shrink: 0;
    border-top: 1px solid var(--border);
  }
  #solution-detail {
    max-height: none;
    overflow-y: visible;
  }
  .generate-row {
    margin-top: 0;
  }
}
```

### 5.5 Graph on Mobile

- `ResizeObserver` already recenters the simulation when the container resizes — no changes needed.
- `d3.drag()` supports touch events natively — node dragging works on touchscreens.
- Node labels may be small at 300px height with many participants, but the text solution list below provides the same information in readable form.

### 5.6 Header on Mobile

The header fits at 375px: "Gift Exchange" title + Import JSON button on one line. No changes required.

---

## 6. Implementation Plan

1. **Bug Fix: Solution display** — update `renderSolutionsPanel()` in `app.js`; update CSS in `style.css`. Remove `.cycle-line`; add `.cycle-group`, `.cycle-header`, `.cycle-dot`, `.assignment-line`.
2. **Bug Fix: Collapsible sections** — replace `<section>` with `<details open>` in `index.html`; update CSS.
3. **Feature: Relationships** — add `relationships: []` to state; add `effectiveBlocks()` helper; update `restartGraph()` and `stateToRequest()`; add `renderRelationshipList()` + `renderRelationshipDropdowns()`; update cascade delete; update `applyImport()` and `onDownload()`; add HTML section and CSS.
4. **Feature: Add as history blocks** — add button to solutions panel HTML; implement `onAddAsHistoryBlocks()` handler in `app.js`.
5. **Feature: Mobile layout** — add `@media (max-width: 640px)` block to `style.css`; verify on real device or browser devtools.

Each step is independently testable and committable.
