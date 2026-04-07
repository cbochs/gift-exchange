# Phase 11 — UI Polish (Frontend-only)

## Status

- [x] **P1** — Solutions panel: two-row header layout
- [x] **P2** — D3 graph: zoom and pan support
- [x] **P3** — D3 graph: disconnected-component gravity fix
- [x] **P4** — Participant cap: 20-person limit with count display

All changes are frontend-only (`server/web/`). No Go changes required.

---

## Overview

Four targeted improvements to the web UI, each scoped to a single concern.
None depend on each other; they can be implemented in any order.

---

## P1 — Solutions Panel: Two-Row Header Layout

### Problem

`#solutions-header` is a single flex row containing the "SOLUTIONS" label, tab
pills, and action buttons (`Add as history blocks`, `Download JSON`). With 5 tabs
the row wraps across ~3 lines, producing the cluttered layout shown in the
screenshot: tabs stack vertically in the center, pushing buttons to a separate
line.

Root cause: `flex-wrap: wrap` on `#solutions-header` with all three element
groups as siblings.

### Solution

Restructure `#solutions-header` into two rows:

```
Row 1:  [SOLUTIONS label]            [Add as history blocks] [Download JSON]
Row 2:  [Sol 1] [Sol 2] [Sol 3] [Sol 4] [Sol 5]
```

**HTML change** — wrap label+buttons in a `.solutions-top-row` div, keep
`#solution-tabs` as its own direct child of `#solutions-header`:

```html
<div id="solutions-header">
  <div class="solutions-top-row">
    <span class="solutions-label">Solutions</span>
    <button id="btn-add-history" class="btn-secondary" hidden>
      Add as history blocks
    </button>
    <button id="btn-download" class="btn-secondary" hidden>
      Download JSON
    </button>
  </div>
  <div id="solution-tabs"></div>
</div>
```

**CSS changes**:

- `#solutions-header`: `flex-direction: column; gap: 6px; padding: 10px 14px 6px`
  (drop `align-items: center`, drop `flex-wrap: wrap`)
- `.solutions-top-row`: `display: flex; align-items: center; gap: 8px`
  with `.solutions-label` having `flex: 1` so buttons align right
- `#solution-tabs`: `display: flex; gap: 5px; flex-wrap: wrap` (unchanged wrapping
  behavior — tabs may wrap but they now have a full-width row to do so cleanly)

**Mobile**: No additional changes needed. The two-row layout is already compact
on narrow screens.

**Impact on `--solutions-h`**: Increase from 210px to 230px to accommodate the
two-row header while keeping the detail scrollable area the same height.

---

## P2 — D3 Graph: Zoom and Pan Support

### Problem

The graph SVG has no pan/zoom behavior. On mobile, the container is 300px tall
and large graphs are unnavigable. On desktop, graphs with 15+ nodes can overflow
the visible area with no way to scroll or zoom.

### Solution

Add `d3.zoom()` to the SVG. All graph layers must be wrapped in a single
`<g id="zoom-layer">` so the transform applies uniformly.

**JS changes in `initGraph()`**:

1. Append a `<g id="zoom-layer">` immediately after creating `svgSel`; move all
   layer appends (`validEdgeLayer`, `solutionEdgeLayer`, `nodeCircleLayer`,
   `nodeLabelLayer`) to append to `zoomLayer` instead of `svgSel`.

2. Create and apply a zoom behavior:

   ```js
   const zoom = d3
     .zoom()
     .scaleExtent([0.3, 4])
     .filter((ev) => {
       // Allow wheel zoom always; allow drag-to-pan only on non-node targets
       if (ev.type === "wheel") return true;
       return (
         ev.target === svgSel.node() ||
         ev.target.closest("g.valid-edges") !== null ||
         ev.target.closest("g.solution-edges") !== null
       );
     })
     .on("zoom", (ev) => {
       zoomLayer.attr("transform", ev.transform);
     });
   svgSel.call(zoom);
   ```

   The `filter` ensures that pointer-down on a node circle falls through to node
   drag, not panning. Wheel events always zoom regardless of target.

3. Expose `zoom` and `svgSel` in module scope so `restartGraph` can call
   `svgSel.call(zoom.transform, d3.zoomIdentity)` to reset view on full restart
   (optional but nice — prevents old zoom state persisting after participant list
   changes).

**CSS change**: Add `cursor: default` on `#graph-svg` (the SVG background will
change cursor to indicate panning via d3.zoom's built-in cursor management).
No other CSS changes needed.

**Mobile**: `d3.zoom()` automatically handles touch events (pinch-to-zoom,
one-finger drag for pan). The 300px mobile height stays but is now explorable.

**Interaction model**:

- Desktop: scroll to zoom, drag background to pan, drag node to reposition
- Mobile: pinch to zoom, one-finger drag on background to pan, drag node to reposition

---

## P3 — D3 Graph: Disconnected-Component Gravity Fix

### Problem

When participants have no valid edges between some subsets (e.g., relationship
blocks isolate a group), the force simulation repels disconnected components
apart. `forceCenter` provides only a one-time centering impulse, not a persistent
centering force. With `forceManyBody` at `-380`, disconnected groups drift to
opposite edges of the container.

### Solution

Add weak `forceX` and `forceY` centering forces alongside the existing forces.
These apply a gentle drift toward the center for _all_ nodes regardless of graph
connectivity, preventing components from escaping:

**JS change in `initGraph()`**:

```js
sim = d3
  .forceSimulation()
  .force(
    "link",
    d3
      .forceLink()
      .id((d) => d.id)
      .distance(120),
  )
  .force("charge", d3.forceManyBody().strength(-300)) // reduced from -380
  .force("center", d3.forceCenter(graphWidth / 2, graphHeight / 2))
  .force("x", d3.forceX(graphWidth / 2).strength(0.07))
  .force("y", d3.forceY(graphHeight / 2).strength(0.07))
  .force("collide", d3.forceCollide(NODE_RADIUS + 18))
  .on("tick", ticked);
```

Also update the `ResizeObserver` callback to update the x/y forces alongside
`forceCenter` when the container resizes:

```js
sim.force("x", d3.forceX(graphWidth / 2).strength(0.07));
sim.force("y", d3.forceY(graphHeight / 2).strength(0.07));
```

**Charge reduction (-380 → -300)**: The forceX/Y gravity counteracts some
spreading, so the net visual spacing is similar with slightly lower repulsion.
This avoids overcorrection (all nodes collapsing to center).

**Impact on connected graphs**: Negligible. The link force at 120px distance
dominates layout for connected nodes; the X/Y gravity is too weak to distort
normal layouts.

---

## P4 — Participant Cap: 20-Person Limit with Count Display

### Problem

There is no UI limit on participant count. Beyond ~20 participants the graph
becomes visually crowded and the exchange stops being a practical holiday tool.
Users have no indication of the intended scale.

### Solution

Enforce a maximum of 20 participants in the UI with a live count indicator.

**Constant** (add to `app.js`):

```js
const MAX_PARTICIPANTS = 20;
```

**HTML change** — update the Participants summary to include a count span:

```html
<summary>
  Participants <span id="participant-count" class="section-count"></span>
</summary>
```

**JS changes**:

1. `renderParticipantList()`: after rendering the list, update the count span and
   toggle the disabled state of the Add button and name input:

   ```js
   const n = state.participants.length;
   document.getElementById("participant-count").textContent =
     n > 0 ? `${n} / ${MAX_PARTICIPANTS}` : "";
   const atCap = n >= MAX_PARTICIPANTS;
   document.getElementById("new-participant-name").disabled = atCap;
   document.getElementById("btn-add-participant").disabled = atCap;
   ```

2. `addParticipant()`: add a guard at the top as a defensive backstop:

   ```js
   if (state.participants.length >= MAX_PARTICIPANTS) return;
   ```

**CSS** — add the count style:

```css
.section-count {
  font-size: 10px;
  font-weight: 400;
  color: var(--muted);
  letter-spacing: 0;
  text-transform: none;
}
```

**Behavior**:

- Count is hidden when 0 participants (empty string)
- Count shows "N / 20" for N > 0
- At N = 20: Add button and name input are disabled; count reads "20 / 20"
- Removing a participant re-enables the Add control immediately (next
  `renderParticipantList()` call clears `disabled`)

---

## Files to Change

| File                    | Changes                                                                                                                        |
| ----------------------- | ------------------------------------------------------------------------------------------------------------------------------ |
| `server/web/index.html` | P1: restructure solutions header; P4: add count span to Participants summary                                                   |
| `server/web/app.js`     | P2: zoom behavior + zoom layer; P3: forceX/Y + charge reduction; P4: count update + disabled state + MAX_PARTICIPANTS constant |
| `server/web/style.css`  | P1: two-row header styles + solutions height; P2: SVG cursor; P4: `.section-count` style                                       |

No Go files, no API changes, no test changes.

---

## Acceptance Criteria

- **P1**: With 5 solutions, the Solutions header shows a single clean top row
  (label left, buttons right) and a second row of tabs. No visual wrapping of
  the label or action buttons at any viewport width above 400px.
- **P2**: On desktop, scrolling over the graph zooms in/out. Dragging the
  background pans. Dragging a node repositions it. On mobile/touch, pinch zooms
  and single-finger drag pans.
- **P3**: With a disconnected participant graph (e.g., two groups with blocks
  isolating them), both groups remain visible near the center of the graph
  container rather than drifting to opposite edges.
- **P4**: The Participants section shows "N / 20" for any N > 0. At 20
  participants, the Add button and name input are disabled. Removing a
  participant immediately re-enables them.
