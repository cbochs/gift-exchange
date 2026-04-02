# Phase 5 — Web Frontend

## Status

- [ ] `web/index.html` — two-panel layout shell, D3 CDN script tag
- [ ] `web/style.css` — layout, form, solution tab styling
- [ ] `web/app.js` — state management, form rendering, API client, D3 graph, solution tabs, JSON import/export
- [ ] End-to-end smoke test (all manual checklist items)
- [ ] Verified: JSON export → re-import → identical result, no extra API call

## Goal

Build a minimal, dependency-light web frontend that serves as the presentation layer for the gift exchange API. It provides a form to define participants and constraints, visualizes the result as a network graph, and supports JSON import/export for reproducibility.

---

## 1. High-Level Design

### 1.1 Technology Choices

| Concern             | Choice                   | Rationale                                         |
| ------------------- | ------------------------ | ------------------------------------------------- |
| Framework           | Vanilla JS (ES modules)  | No build step, no npm, easy to audit and maintain |
| Styling             | Plain CSS                | No Tailwind/Bootstrap dependency                  |
| Graph visualization | D3.js v7 (CDN)           | Force-directed graph is a perfect fit; mature API |
| HTTP client         | `fetch` (browser native) | No axios/jQuery needed                            |

The entire frontend is three files: `index.html`, `app.js`, `style.css`. The backend serves them at `/` when run with `--static web/`.

### 1.2 Layout

```
┌──────────────────────────────────────────────────────────────────┐
│  Gift Exchange                               [Import JSON] [?]   │
├──────────────────┬───────────────────────────────────────────────┤
│                  │                                               │
│   PARTICIPANTS   │                                               │
│   ─────────────  │         GRAPH VISUALIZATION                  │
│   Alice Smith  ✕ │          (D3 force-directed)                 │
│   Bob Jones    ✕ │                                               │
│   Carol Wu     ✕ │    nodes = participants                       │
│   + Add person   │    grey edges = valid pairings               │
│                  │    colored edges = solution assignments        │
│   BLOCKS         │                                               │
│   ─────────────  │                                               │
│   Alice → Bob  ✕ │                                               │
│   + Add block    │                                               │
│                  ├───────────────────────────────────────────────┤
│   OPTIONS        │  SOLUTIONS                                    │
│   ─────────────  │  ──────────                                   │
│   Max results: 5 │  [Sol 1 ●] [Sol 2] [Sol 3]                   │
│   Seed: (auto)   │                                               │
│                  │  Score: min_cycle=4  cycles=1  max_cycle=4   │
│                  │  alice → carol → dave → bob → alice           │
│   [Generate]     │                                               │
│                  │  [Download JSON]                              │
└──────────────────┴───────────────────────────────────────────────┘
```

### 1.3 Application State

The frontend maintains a single state object, mutated only through explicit update functions. No framework reactivity — plain DOM manipulation driven by state diffs.

```js
const state = {
  participants: [], // [{id, name}]
  blocks: [], // [{from, to}]
  options: {
    // No minCycleLen: the backend determines this automatically via N/M progression.
    maxSolutions: 5,
    seed: null, // null = auto (backend picks random seed)
  },
  solutions: [], // SolutionDTO[] — ranked best-first by Score
  selectedSolution: 0, // index into solutions
  loading: false,
  error: null,
};
```

### 1.4 Participant and Block Management

- **Add participant**: text input for name; ID is auto-generated as a URL-safe slug of the name (with collision suffix if needed)
- **Remove participant**: ✕ button removes participant and any blocks referencing it
- **Add block**: two dropdowns (From, To) populated from current participant list; prevents self-blocking
- **Remove block**: ✕ button

### 1.5 Graph Visualization

The graph panel displays two overlapping layers using D3 force simulation:

**Participant graph (always visible):**

- Nodes: one per participant, labeled with name
- Edges: one per valid unblocked directed pairing (grey, thin, 50% opacity)
  - Computed client-side: all pairs minus self-pairs minus blocks

**Solution overlay (visible after solve):**

- Colored directed edges representing the selected solution's assignments
- Edge color encodes cycle membership (cycle 1 = blue, cycle 2 = orange, etc.)
- Arrow markers on each edge indicating direction
- Node highlight color matches its cycle

The force simulation uses:

- `forceLink` for edges (distance scaled by n)
- `forceManyBody` for repulsion
- `forceCenter` to keep graph centered
- `forceCollide` to prevent node overlap

On solution selection change, only the edge/node colors update — the simulation does not restart.

### 1.6 JSON Import/Export

**Export (`[Download JSON]` button):**
Downloads a JSON file containing the complete request that produced the result plus the selected solution. This file can be re-imported exactly.

```json
{
  "participants": [...],
  "blocks": [...],
  "options": {
    "max_solutions": 5,
    "seed": 42
  },
  "_selected_solution": 0,
  "_solutions": [...]
}
```

The `_` prefix keys are frontend metadata — the backend ignores them. When re-importing, the frontend populates the form from the document and immediately displays the cached solutions without a new API call.

**Import (`[Import JSON]` button):**

- Opens a file picker (or drag-and-drop)
- Parses the JSON
- Populates `state` from the document
- If `_solutions` is present, renders them immediately
- If not, triggers a new solve

### 1.7 Error Handling

- Network errors: shown inline below the Generate button
- API 400: "Invalid input: ..." — highlight the offending field if identifiable
- API 422: "No valid assignment exists — try removing some blocks or reducing min cycle length"
- Participant name collision: prevented in the form before submission

### 1.8 Testing Strategy

Frontend testing focuses on behavior rather than DOM internals.

**Unit tests (plain JS, no framework):**

- `slugify(name)`: verify unique slug generation
- `buildValidEdges(participants, blocks)`: verify correct edge set
- `normalizeSolution(solution)`: verify cycle normalization
- `stateToRequest(state)`: verify correct API request shape
- `applyImport(doc, state)`: verify state correctly populated from import doc

**Manual smoke tests (checklist):**

- Add 4 participants, generate → graph animates, solution displayed
- Add a block, regenerate → blocked edge not in solution
- Click solution tabs → graph overlay changes
- Download JSON → re-import → identical result displayed
- Import a JSON with `_solutions` → no API call made
- Resize window → graph layout adjusts

---

## 2. Implementation Plan

1. **Create `web/index.html`**: static shell with layout skeleton, D3 CDN script tag
2. **Create `web/style.css`**: minimal two-column layout, form styling, solution tab styling
3. **Create `web/app.js`** in sections:
   a. State management + update functions
   b. Form rendering (participants, blocks, options)
   c. API client (`solveExchange(state)` → fetch → update state)
   d. Graph rendering (D3 force simulation setup, node/edge updates)
   e. Solution tab rendering + selection
   f. JSON import/export handlers
4. **Wire the backend** `--static web/` and test end-to-end in browser
5. **Smoke test** all manual test cases above

---

## 3. Implementation Snippets

### `web/app.js` — API client

```js
async function solveExchange(state) {
  const req = {
    participants: state.participants,
    blocks: state.blocks,
    options: {
      // No min_cycle_len: determined automatically by the backend.
      max_solutions: state.options.maxSolutions,
      ...(state.options.seed != null ? { seed: state.options.seed } : {}),
    },
  };

  const resp = await fetch("/api/v1/solve", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(req),
  });

  const body = await resp.json();
  if (!resp.ok) {
    throw new Error(body.error ?? `HTTP ${resp.status}`);
  }
  return body; // SolveResponse
}
```

### `web/app.js` — Valid edge computation (for graph)

```js
function buildValidEdges(participants, blocks) {
  const ids = participants.map((p) => p.id);
  const blockedSet = new Set(blocks.map((b) => `${b.from}→${b.to}`));

  const edges = [];
  for (const from of ids) {
    for (const to of ids) {
      if (from !== to && !blockedSet.has(`${from}→${to}`)) {
        edges.push({ source: from, target: to, type: "valid" });
      }
    }
  }
  return edges;
}
```

### `web/app.js` — D3 graph initialization

```js
function initGraph(svg, width, height) {
  const sim = d3
    .forceSimulation()
    .force(
      "link",
      d3
        .forceLink()
        .id((d) => d.id)
        .distance(120),
    )
    .force("charge", d3.forceManyBody().strength(-400))
    .force("center", d3.forceCenter(width / 2, height / 2))
    .force("collide", d3.forceCollide(40));

  // Arrow markers for directed edges
  svg
    .append("defs")
    .selectAll("marker")
    .data(["valid", "cycle-0", "cycle-1", "cycle-2"])
    .join("marker")
    .attr("id", (d) => `arrow-${d}`)
    .attr("viewBox", "0 -5 10 10")
    .attr("refX", 24)
    .attr("refY", 0)
    .attr("markerWidth", 6)
    .attr("markerHeight", 6)
    .attr("orient", "auto")
    .append("path")
    .attr("d", "M0,-5L10,0L0,5");

  return sim;
}
```

### `web/app.js` — Graph update (called on state change)

```js
function updateGraph(svg, sim, state) {
  const participants = state.participants;
  const validEdges = buildValidEdges(participants, state.blocks);
  const solution = state.solutions[state.selectedSolution];

  // Build solution edges with cycle coloring
  const solutionEdges = solution
    ? solution.cycles.flatMap((cycle, cycleIdx) =>
        cycle.map((id, i) => ({
          source: id,
          target: cycle[(i + 1) % cycle.length],
          type: `cycle-${cycleIdx % 3}`,
        })),
      )
    : [];

  const nodeColors = {};
  if (solution) {
    solution.cycles.forEach((cycle, idx) => {
      cycle.forEach((id) => {
        nodeColors[id] = CYCLE_COLORS[idx % CYCLE_COLORS.length];
      });
    });
  }

  // Update nodes
  svg
    .selectAll(".node")
    .data(participants, (d) => d.id)
    .join("circle")
    .attr("class", "node")
    .attr("r", 20)
    .attr("fill", (d) => nodeColors[d.id] ?? "#ccc");

  // ... update valid-edges and solution-edges link layers ...

  sim.nodes(participants).on("tick", ticked);
  sim.force("link").links([...validEdges, ...solutionEdges]);
  sim.alpha(0.3).restart();
}
```

### `web/app.js` — JSON export

```js
function downloadJSON(state) {
  const doc = {
    participants: state.participants,
    blocks: state.blocks,
    options: {
      // No min_cycle_len in exported doc; backend determines it automatically.
      max_solutions: state.options.maxSolutions,
      seed: state.options.seed,
    },
    _selected_solution: state.selectedSolution,
    _solutions: state.solutions,
  };
  const blob = new Blob([JSON.stringify(doc, null, 2)], {
    type: "application/json",
  });
  const url = URL.createObjectURL(blob);
  const a = Object.assign(document.createElement("a"), {
    href: url,
    download: `gift-exchange-${Date.now()}.json`,
  });
  a.click();
  URL.revokeObjectURL(url);
}
```

### `web/app.js` — JSON import

```js
async function importJSON(file, state, render) {
  const text = await file.text();
  const doc = JSON.parse(text);

  state.participants = doc.participants ?? [];
  state.blocks = doc.blocks ?? [];
  state.options = {
    maxSolutions: doc.options?.max_solutions ?? 5,
    seed: doc.options?.seed ?? null,
  };

  if (doc._solutions?.length) {
    // Use cached solutions — no API call
    state.solutions = doc._solutions;
    state.selectedSolution = doc._selected_solution ?? 0;
  } else {
    // Re-solve
    state.loading = true;
    render();
    const resp = await solveExchange(state);
    state.solutions = resp.solutions;
    state.selectedSolution = 0;
    state.loading = false;
  }
  render();
}
```
