import * as d3 from "https://cdn.jsdelivr.net/npm/d3@7/+esm";

// ─── Constants ────────────────────────────────────────────────────────────────

const API_URL = "/api/v1/solve";
const NODE_RADIUS = 20;
const CYCLE_COLORS = ["#4c72b0", "#dd8452", "#55a868", "#c44e52", "#8172b2", "#937860"];

// ─── State ────────────────────────────────────────────────────────────────────

const state = {
  participants: [],   // [{id, name}]
  blocks: [],         // [{from, to}]
  options: {
    maxSolutions: 5,
    seed: null,       // null = random
  },
  solutions: [],      // SolutionDTO[]
  selectedSolution: 0,
  loading: false,
  error: null,
};

// ─── Utility functions ────────────────────────────────────────────────────────

export function slugify(name) {
  return name.trim().toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "")
    || "p";
}

export function uniqueId(name, existingIds) {
  const base = slugify(name);
  if (!existingIds.has(base)) return base;
  for (let i = 2; ; i++) {
    const id = `${base}-${i}`;
    if (!existingIds.has(id)) return id;
  }
}

export function buildValidEdges(participants, blocks) {
  const blockedSet = new Set(blocks.map(b => `${b.from}\u2192${b.to}`));
  const pairSet = new Set();

  for (const src of participants) {
    for (const tgt of participants) {
      if (src.id !== tgt.id && !blockedSet.has(`${src.id}\u2192${tgt.id}`)) {
        pairSet.add(`${src.id}\u2192${tgt.id}`);
      }
    }
  }

  const edges = [];
  for (const key of pairSet) {
    const arrow = key.indexOf("\u2192");
    const fromId = key.slice(0, arrow);
    const toId = key.slice(arrow + 1);
    edges.push({
      source: fromId, target: toId,
      sourceId: fromId, targetId: toId,
      kind: "valid",
      bidirectional: pairSet.has(`${toId}\u2192${fromId}`),
    });
  }
  return edges;
}

export function stateToRequest(state) {
  const opts = { max_solutions: state.options.maxSolutions };
  if (state.options.seed != null) opts.seed = Number(state.options.seed);
  return { participants: state.participants, blocks: state.blocks, options: opts };
}

// Populates state from an imported JSON document.
// Returns true if a new API call is needed, false if cached solutions can be used.
export function applyImport(doc, state) {
  state.participants = doc.participants ?? [];
  state.blocks = doc.blocks ?? [];
  state.options.maxSolutions = doc.options?.max_solutions ?? 5;
  state.options.seed = doc.options?.seed ?? null;
  if (doc._solutions?.length) {
    state.solutions = doc._solutions;
    state.selectedSolution = doc._selected_solution ?? 0;
    return false;
  }
  state.solutions = [];
  state.selectedSolution = 0;
  return true;
}

function esc(s) {
  return String(s)
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;");
}

// ─── API client ───────────────────────────────────────────────────────────────

async function solveExchange(state) {
  const resp = await fetch(API_URL, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(stateToRequest(state)),
  });
  const body = await resp.json();
  if (!resp.ok) throw new Error(body.error ?? `HTTP ${resp.status}`);
  return body; // SolveResponse
}

// ─── D3 graph ─────────────────────────────────────────────────────────────────

// nodeMap persists node objects (with x, y, vx, vy) across re-renders.
const nodeMap = new Map();

let svgSel, sim, validEdgeLayer, solutionEdgeLayer, nodeCircleLayer, nodeLabelLayer;
let graphWidth = 0, graphHeight = 0;

function syncNodes(participants) {
  const newIds = new Set(participants.map(p => p.id));
  for (const id of nodeMap.keys()) {
    if (!newIds.has(id)) nodeMap.delete(id);
  }
  return participants.map(p => {
    if (!nodeMap.has(p.id)) nodeMap.set(p.id, { id: p.id, name: p.name });
    else nodeMap.get(p.id).name = p.name;
    return nodeMap.get(p.id);
  });
}

function buildSolutionEdges(solution) {
  if (!solution) return [];
  const edgeSet = new Set();
  solution.cycles.forEach(cycle => {
    cycle.forEach((id, i) => edgeSet.add(`${id}\u2192${cycle[(i + 1) % cycle.length]}`));
  });
  return solution.cycles.flatMap((cycle, ci) =>
    cycle.map((id, i) => {
      const nextId = cycle[(i + 1) % cycle.length];
      return {
        source: nodeMap.get(id) ?? { id, x: graphWidth / 2, y: graphHeight / 2 },
        target: nodeMap.get(nextId) ?? { id: nextId, x: graphWidth / 2, y: graphHeight / 2 },
        sourceId: id,
        targetId: nextId,
        kind: "solution",
        cycleIdx: ci,
        bidirectional: edgeSet.has(`${nextId}\u2192${id}`),
      };
    })
  );
}

function buildNodeColors(solution) {
  const map = {};
  if (!solution) return map;
  solution.cycles.forEach((cycle, ci) => {
    cycle.forEach(id => { map[id] = CYCLE_COLORS[ci % CYCLE_COLORS.length]; });
  });
  return map;
}

// Quadratic Bezier path from source to target.
// Bidirectional pairs curve to opposite sides; unidirectional are straight.
function arcPath(d) {
  const sx = d.source.x ?? 0, sy = d.source.y ?? 0;
  const tx = d.target.x ?? 0, ty = d.target.y ?? 0;
  const dx = tx - sx, dy = ty - sy;
  const len = Math.hypot(dx, dy) || 1;

  // End point at node edge so arrowhead lands at perimeter
  const ex = tx - (dx / len) * (NODE_RADIUS + 3);
  const ey = ty - (dy / len) * (NODE_RADIUS + 3);

  if (!d.bidirectional) {
    return `M${sx},${sy}L${ex},${ey}`;
  }
  // Offset control point perpendicular to midpoint; direction by ID order
  const sign = d.sourceId < d.targetId ? 1 : -1;
  const cpOffset = 36;
  const qx = (sx + tx) / 2 - (dy / len) * cpOffset * sign;
  const qy = (sy + ty) / 2 + (dx / len) * cpOffset * sign;
  return `M${sx},${sy}Q${qx},${qy} ${ex},${ey}`;
}

function ticked() {
  validEdgeLayer.selectAll("path").attr("d", arcPath);
  solutionEdgeLayer.selectAll("path").attr("d", arcPath);
  nodeCircleLayer.selectAll("circle").attr("cx", d => d.x).attr("cy", d => d.y);
  nodeLabelLayer.selectAll("text").attr("x", d => d.x).attr("y", d => d.y + NODE_RADIUS + 4);
}

function initGraph() {
  const container = document.getElementById("graph-container");
  graphWidth = container.clientWidth;
  graphHeight = container.clientHeight;

  svgSel = d3.select("#graph-svg");

  // Arrow markers — one per cycle color plus one for valid (grey) edges.
  // markerUnits="userSpaceOnUse" gives pixel-accurate sizing regardless of stroke-width.
  const markerData = [
    { id: "arrow-valid", color: "#b0b7c0" },
    ...CYCLE_COLORS.map((c, i) => ({ id: `arrow-cycle-${i}`, color: c })),
  ];
  svgSel.append("defs")
    .selectAll("marker")
    .data(markerData)
    .join("marker")
    .attr("id", d => d.id)
    .attr("viewBox", "0 -4 8 8")
    .attr("refX", 8)
    .attr("refY", 0)
    .attr("markerWidth", 8)
    .attr("markerHeight", 8)
    .attr("markerUnits", "userSpaceOnUse")
    .attr("orient", "auto")
    .append("path")
    .attr("d", "M0,-4L8,0L0,4")
    .attr("fill", d => d.color);

  // Layer order matters for z-index (last appended = topmost).
  validEdgeLayer = svgSel.append("g").attr("class", "valid-edges");
  solutionEdgeLayer = svgSel.append("g").attr("class", "solution-edges");
  nodeCircleLayer = svgSel.append("g").attr("class", "node-circles");
  nodeLabelLayer = svgSel.append("g").attr("class", "node-labels");

  sim = d3.forceSimulation()
    .force("link", d3.forceLink().id(d => d.id).distance(120))
    .force("charge", d3.forceManyBody().strength(-380))
    .force("center", d3.forceCenter(graphWidth / 2, graphHeight / 2))
    .force("collide", d3.forceCollide(NODE_RADIUS + 18))
    .on("tick", ticked);

  new ResizeObserver(() => {
    const c = document.getElementById("graph-container");
    graphWidth = c.clientWidth;
    graphHeight = c.clientHeight;
    sim.force("center", d3.forceCenter(graphWidth / 2, graphHeight / 2));
    if (state.participants.length > 0) sim.alpha(0.3).restart();
  }).observe(container);
}

// Full graph restart: called when participants, blocks, or solve results change.
function restartGraph() {
  const nodes = syncNodes(state.participants);
  const validEdges = buildValidEdges(state.participants, state.blocks);
  const solutionEdges = buildSolutionEdges(state.solutions[state.selectedSolution]);
  const nodeColors = buildNodeColors(state.solutions[state.selectedSolution]);

  const drag = d3.drag()
    .on("start", (ev, d) => { if (!ev.active) sim.alphaTarget(0.3).restart(); d.fx = d.x; d.fy = d.y; })
    .on("drag", (ev, d) => { d.fx = ev.x; d.fy = ev.y; })
    .on("end", (ev, d) => { if (!ev.active) sim.alphaTarget(0); d.fx = null; d.fy = null; });

  // Valid edges
  validEdgeLayer.selectAll("path")
    .data(validEdges, d => `${d.sourceId}\u2192${d.targetId}`)
    .join("path")
    .attr("class", "edge-valid")
    .attr("marker-end", "url(#arrow-valid)");

  // Solution edges (visual overlay only — not fed into forceLink)
  solutionEdgeLayer.selectAll("path")
    .data(solutionEdges, d => `${d.sourceId}\u2192${d.targetId}`)
    .join("path")
    .attr("class", "edge-solution")
    .attr("stroke", d => CYCLE_COLORS[d.cycleIdx % CYCLE_COLORS.length])
    .attr("marker-end", d => `url(#arrow-cycle-${d.cycleIdx % CYCLE_COLORS.length})`);

  // Node circles
  nodeCircleLayer.selectAll("circle")
    .data(nodes, d => d.id)
    .join(
      enter => enter.append("circle")
        .attr("class", "node-circle")
        .attr("r", NODE_RADIUS)
        .call(drag),
    )
    .attr("fill", d => nodeColors[d.id] ?? "#d1d5db");

  // Labels (separate layer so they render above circles)
  nodeLabelLayer.selectAll("text")
    .data(nodes, d => d.id)
    .join("text")
    .attr("class", "node-label")
    .text(d => d.name);

  // Only valid edges drive the physics layout
  sim.nodes(nodes);
  sim.force("link").links(validEdges);
  sim.alpha(0.4).restart();
}

// Color-only update: called when switching solution tabs.
// Does NOT restart the simulation — just repaints.
function recolorGraph() {
  const sol = state.solutions[state.selectedSolution];
  const solutionEdges = buildSolutionEdges(sol);
  const nodeColors = buildNodeColors(sol);

  solutionEdgeLayer.selectAll("path")
    .data(solutionEdges, d => `${d.sourceId}\u2192${d.targetId}`)
    .join("path")
    .attr("class", "edge-solution")
    .attr("stroke", d => CYCLE_COLORS[d.cycleIdx % CYCLE_COLORS.length])
    .attr("marker-end", d => `url(#arrow-cycle-${d.cycleIdx % CYCLE_COLORS.length})`)
    .attr("d", arcPath);

  nodeCircleLayer.selectAll("circle")
    .attr("fill", d => nodeColors[d.id] ?? "#d1d5db");
}

// ─── Sidebar rendering ────────────────────────────────────────────────────────

function renderParticipantList() {
  const ul = document.getElementById("participant-list");
  ul.innerHTML = "";
  state.participants.forEach((p, i) => {
    const li = document.createElement("li");
    li.innerHTML = `<span>${esc(p.name)}</span>`;
    const btn = document.createElement("button");
    btn.className = "remove-btn";
    btn.textContent = "×";
    btn.title = "Remove";
    btn.addEventListener("click", () => {
      state.participants.splice(i, 1);
      state.blocks = state.blocks.filter(b => b.from !== p.id && b.to !== p.id);
      state.solutions = [];
      state.selectedSolution = 0;
      renderSidebar();
      renderSolutionsPanel();
      restartGraph();
      updateEmptyState();
    });
    li.appendChild(btn);
    ul.appendChild(li);
  });
}

function renderBlockDropdowns() {
  const fromSel = document.getElementById("block-from");
  const toSel = document.getElementById("block-to");
  const prevFrom = fromSel.value;
  const prevTo = toSel.value;

  [fromSel, toSel].forEach(sel => {
    sel.innerHTML = "";
    state.participants.forEach(p => {
      const opt = document.createElement("option");
      opt.value = p.id;
      opt.textContent = p.name;
      sel.appendChild(opt);
    });
  });

  if (prevFrom && state.participants.some(p => p.id === prevFrom)) fromSel.value = prevFrom;
  if (prevTo && state.participants.some(p => p.id === prevTo)) toSel.value = prevTo;

  const disabled = state.participants.length < 2;
  fromSel.disabled = disabled;
  toSel.disabled = disabled;
  document.getElementById("btn-add-block").disabled = disabled;
}

function renderBlockList() {
  const ul = document.getElementById("block-list");
  ul.innerHTML = "";
  state.blocks.forEach((b, i) => {
    const fromName = state.participants.find(p => p.id === b.from)?.name ?? b.from;
    const toName = state.participants.find(p => p.id === b.to)?.name ?? b.to;
    const li = document.createElement("li");
    li.innerHTML = `<span>${esc(fromName)} → ${esc(toName)}</span>`;
    const btn = document.createElement("button");
    btn.className = "remove-btn";
    btn.textContent = "×";
    btn.title = "Remove";
    btn.addEventListener("click", () => {
      state.blocks.splice(i, 1);
      state.solutions = [];
      state.selectedSolution = 0;
      renderSidebar();
      renderSolutionsPanel();
      restartGraph();
    });
    li.appendChild(btn);
    ul.appendChild(li);
  });
}

function renderOptions() {
  document.getElementById("opt-max-solutions").value = state.options.maxSolutions;
  document.getElementById("opt-seed").value = state.options.seed ?? "";
}

function renderSidebar() {
  renderParticipantList();
  renderBlockDropdowns();
  renderBlockList();

  const genBtn = document.getElementById("btn-generate");
  genBtn.disabled = state.loading;
  genBtn.textContent = state.loading ? "Generating…" : "Generate";

  const errEl = document.getElementById("generate-error");
  errEl.textContent = state.error ?? "";
}

// ─── Solutions panel rendering ────────────────────────────────────────────────

function renderSolutionsPanel() {
  const tabsEl = document.getElementById("solution-tabs");
  const detailEl = document.getElementById("solution-detail");
  const dlBtn = document.getElementById("btn-download");

  if (!state.solutions.length) {
    tabsEl.innerHTML = "";
    detailEl.innerHTML = "";
    dlBtn.hidden = true;
    return;
  }

  // Tabs
  tabsEl.innerHTML = "";
  state.solutions.forEach((_, i) => {
    const btn = document.createElement("button");
    btn.className = "tab-btn" + (i === state.selectedSolution ? " active" : "");
    btn.textContent = `Sol ${i + 1}`;
    btn.addEventListener("click", () => {
      state.selectedSolution = i;
      renderSolutionsPanel();
      recolorGraph();
    });
    tabsEl.appendChild(btn);
  });

  // Detail
  const sol = state.solutions[state.selectedSolution];
  const { min_cycle_len, num_cycles, max_cycle_len } = sol.score;
  const nameOf = Object.fromEntries(state.participants.map(p => [p.id, p.name]));
  const cycleGroups = sol.cycles.map((cycle, ci) => {
    const color = CYCLE_COLORS[ci % CYCLE_COLORS.length];
    const lines = cycle.map((id, i) => {
      const nextId = cycle[(i + 1) % cycle.length];
      return `<div class="assignment-line">${esc(nameOf[id] ?? id)} → ${esc(nameOf[nextId] ?? nextId)}</div>`;
    }).join("");
    return `<div class="cycle-group">
      <div class="cycle-header">
        <span class="cycle-dot" style="background:${color}"></span>
        <span>Cycle ${ci + 1} · ${cycle.length} ${cycle.length === 1 ? "person" : "people"}</span>
      </div>
      <div class="cycle-assignments">${lines}</div>
    </div>`;
  }).join("");

  detailEl.innerHTML = `
    <div class="solution-score">min_cycle=${min_cycle_len} &nbsp; cycles=${num_cycles} &nbsp; max_cycle=${max_cycle_len}</div>
    <div class="solution-cycles">${cycleGroups}</div>`;

  dlBtn.hidden = false;
}

// ─── Empty state ──────────────────────────────────────────────────────────────

function updateEmptyState() {
  const el = document.getElementById("graph-empty");
  el.style.display = state.participants.length === 0 ? "" : "none";
}

// ─── Generate ─────────────────────────────────────────────────────────────────

async function onGenerate() {
  if (state.participants.length < 2) {
    state.error = "Add at least 2 participants.";
    renderSidebar();
    return;
  }
  state.loading = true;
  state.error = null;
  renderSidebar();

  try {
    const resp = await solveExchange(state);
    state.solutions = resp.solutions;
    state.selectedSolution = 0;
    // Lock in the seed actually used so Download JSON reproduces the result.
    state.options.seed = resp.seed_used;
    document.getElementById("opt-seed").value = resp.seed_used;
  } catch (err) {
    state.error = err.message;
    state.solutions = [];
    state.selectedSolution = 0;
  } finally {
    state.loading = false;
  }

  renderSidebar();
  renderSolutionsPanel();
  restartGraph();
}

// ─── JSON import/export ───────────────────────────────────────────────────────

function onDownload() {
  const doc = {
    participants: state.participants,
    blocks: state.blocks,
    options: {
      max_solutions: state.options.maxSolutions,
      ...(state.options.seed != null ? { seed: state.options.seed } : {}),
    },
    _selected_solution: state.selectedSolution,
    _solutions: state.solutions,
  };
  const blob = new Blob([JSON.stringify(doc, null, 2)], { type: "application/json" });
  const url = URL.createObjectURL(blob);
  const a = Object.assign(document.createElement("a"), {
    href: url,
    download: `gift-exchange-${Date.now()}.json`,
  });
  document.body.appendChild(a);
  a.click();
  document.body.removeChild(a);
  URL.revokeObjectURL(url);
}

async function onImport(file) {
  try {
    const text = await file.text();
    const doc = JSON.parse(text);
    const needsSolve = applyImport(doc, state);

    if (needsSolve) {
      state.loading = true;
      state.error = null;
      renderSidebar();
      try {
        const resp = await solveExchange(state);
        state.solutions = resp.solutions;
        state.selectedSolution = 0;
        state.options.seed = resp.seed_used;
      } catch (err) {
        state.error = err.message;
        state.solutions = [];
        state.selectedSolution = 0;
      } finally {
        state.loading = false;
      }
    }
  } catch (err) {
    state.error = "Import failed: " + err.message;
  }

  renderSidebar();
  renderOptions();
  renderSolutionsPanel();
  restartGraph();
  updateEmptyState();
}

// ─── Event wiring ─────────────────────────────────────────────────────────────

function wireEvents() {
  // Add participant
  const nameInput = document.getElementById("new-participant-name");
  const partErrEl = document.getElementById("participant-error");

  function addParticipant() {
    const name = nameInput.value.trim();
    partErrEl.textContent = "";
    if (!name) return;
    const existingIds = new Set(state.participants.map(p => p.id));
    const id = uniqueId(name, existingIds);
    state.participants.push({ id, name });
    state.solutions = [];
    state.selectedSolution = 0;
    nameInput.value = "";
    renderSidebar();
    renderSolutionsPanel();
    restartGraph();
    updateEmptyState();
  }

  nameInput.addEventListener("keydown", e => { if (e.key === "Enter") addParticipant(); });
  document.getElementById("btn-add-participant").addEventListener("click", addParticipant);

  // Add block
  document.getElementById("btn-add-block").addEventListener("click", () => {
    const from = document.getElementById("block-from").value;
    const to = document.getElementById("block-to").value;
    if (!from || !to || from === to) return;
    if (state.blocks.some(b => b.from === from && b.to === to)) return;
    state.blocks.push({ from, to });
    state.solutions = [];
    state.selectedSolution = 0;
    renderSidebar();
    renderSolutionsPanel();
    restartGraph();
  });

  // Options (read at solve time; update state on change)
  document.getElementById("opt-max-solutions").addEventListener("input", e => {
    state.options.maxSolutions = Math.max(1, parseInt(e.target.value, 10) || 5);
  });
  document.getElementById("opt-seed").addEventListener("input", e => {
    const v = e.target.value.trim();
    state.options.seed = v ? parseInt(v, 10) : null;
  });

  // Generate
  document.getElementById("btn-generate").addEventListener("click", onGenerate);

  // Download
  document.getElementById("btn-download").addEventListener("click", onDownload);

  // Import
  const fileInput = document.getElementById("import-file-input");
  document.getElementById("btn-import").addEventListener("click", () => fileInput.click());
  fileInput.addEventListener("change", e => {
    const file = e.target.files[0];
    if (file) onImport(file);
    fileInput.value = "";
  });
}

// ─── Init ─────────────────────────────────────────────────────────────────────

document.addEventListener("DOMContentLoaded", () => {
  initGraph();
  wireEvents();
  renderSidebar();
  renderOptions();
  renderSolutionsPanel();
  updateEmptyState();
});
