import * as d3 from "https://cdn.jsdelivr.net/npm/d3@7/+esm";

// ─── Constants ────────────────────────────────────────────────────────────────

const API_URL = "/api/v1/solve";
const NODE_RADIUS = 20;
const CYCLE_COLORS = ["#4c72b0", "#dd8452", "#55a868", "#c44e52", "#8172b2", "#937860"];
const MAX_PARTICIPANTS = 20;

// ─── State ────────────────────────────────────────────────────────────────────

const state = {
  participants: [],    // [{id, name}]
  relationships: [],   // [{a, b}] symmetric pairs — expanded to two blocks at API call time
  blocks: [],          // [{from, to, group?}] directed blocks
  blockGroups: [],     // [{id, label, collapsed}] ordered group metadata
  options: {
    maxSolutions: 5,
    seed: null,        // null = random
  },
  solutions: [],       // SolutionDTO[]
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
          source: src.id, target: tgt.id,
          sourceId: src.id, targetId: tgt.id,
          kind: "valid",
          bidirectional: validSet.has(`${tgt.id}\u2192${src.id}`),
        });
      }
    }
  }
  return edges;
}

// Returns the union of explicit directed blocks and the two-direction expansion
// of symmetric relationships. This is what gets sent to the API.
export function effectiveBlocks(state) {
  return [
    ...state.blocks.map(({ from, to }) => ({ from, to })),
    ...state.relationships.flatMap(r => [
      { from: r.a, to: r.b },
      { from: r.b, to: r.a },
    ]),
  ];
}

export function stateToRequest(state) {
  const opts = { max_solutions: state.options.maxSolutions };
  if (state.options.seed != null) opts.seed = Number(state.options.seed);
  return { participants: state.participants, blocks: effectiveBlocks(state), options: opts };
}

// Populates state from an imported JSON document.
// Returns true if a new API call is needed, false if cached solutions can be used.
export function applyImport(doc, state) {
  state.participants = doc.participants ?? [];
  state.relationships = doc.relationships ?? [];
  state.blocks = doc.blocks ?? [];
  state.blockGroups = doc.blockGroups ?? [];
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

// ─── Persistence & link sharing ───────────────────────────────────────────────

const LS_KEY = "gift-exchange-v1";
let saveTimer = null;

function saveStateDebounced() {
  clearTimeout(saveTimer);
  saveTimer = setTimeout(saveState, 300);
}

function saveState() {
  try {
    localStorage.setItem(LS_KEY, JSON.stringify({
      participants: state.participants,
      relationships: state.relationships,
      blocks: state.blocks,
      blockGroups: state.blockGroups,
      options: {
        max_solutions: state.options.maxSolutions,
        ...(state.options.seed != null ? { seed: state.options.seed } : {}),
      },
    }));
  } catch { /* storage unavailable or quota exceeded */ }
}

function loadFromLocalStorage() {
  try {
    const raw = localStorage.getItem(LS_KEY);
    if (!raw) return false;
    applyImport(JSON.parse(raw), state);
    return true;
  } catch {
    return false;
  }
}

function hashEncode(obj) {
  const bytes = new TextEncoder().encode(JSON.stringify(obj));
  let binary = "";
  for (let i = 0; i < bytes.length; i++) binary += String.fromCharCode(bytes[i]);
  return btoa(binary).replace(/\+/g, "-").replace(/\//g, "_").replace(/=/g, "");
}

function hashDecode(b64) {
  const bytes = Uint8Array.from(atob(b64.replace(/-/g, "+").replace(/_/g, "/")), c => c.charCodeAt(0));
  return JSON.parse(new TextDecoder().decode(bytes));
}

export function encodeStateToHash(state) {
  const idxOf = Object.fromEntries(state.participants.map((p, i) => [p.id, i]));
  const groupIdxOf = Object.fromEntries(state.blockGroups.map((g, i) => [g.id, i]));

  const ungrouped = state.blocks.filter(b => !b.group);
  const grouped   = state.blocks.filter(b =>  b.group);

  const compact = {
    v: 2,
    p: state.participants.map(p => [p.id, p.name]),
    r: state.relationships.map(r => [idxOf[r.a], idxOf[r.b]]),
    b: ungrouped.map(b => [idxOf[b.from], idxOf[b.to]]),
    ...(grouped.length ? {
      g: state.blockGroups.map(g => g.label),
      bg: grouped.map(b => [idxOf[b.from], idxOf[b.to], groupIdxOf[b.group]]),
    } : {}),
  };
  if (state.options.maxSolutions !== 5 || state.options.seed != null) {
    compact.o = {};
    if (state.options.maxSolutions !== 5) compact.o.m = state.options.maxSolutions;
    if (state.options.seed != null) compact.o.s = state.options.seed;
  }
  return "#v2:" + hashEncode(compact);
}

export function decodeStateFromHash(hash) {
  try {
    if (hash.startsWith("#v2:")) return decodeV2(hashDecode(hash.slice(4)));
    if (hash.startsWith("#v1:")) return decodeV1(hashDecode(hash.slice(4)));
    return null;
  } catch {
    return null;
  }
}

function decodeV1(compact) {
  if (compact.v !== 1 || !Array.isArray(compact.p)) return null;
  const participants = compact.p.map(([id, name]) => ({ id, name }));
  const blocks = (compact.b ?? [])
    .map(([fi, ti]) => ({ from: participants[fi]?.id, to: participants[ti]?.id }))
    .filter(b => b.from != null && b.to != null);
  const relationships = (compact.r ?? [])
    .map(([ai, bi]) => ({ a: participants[ai]?.id, b: participants[bi]?.id }))
    .filter(r => r.a != null && r.b != null);
  return {
    participants, blocks, relationships, blockGroups: [],
    options: {
      max_solutions: compact.o?.m ?? 5,
      ...(compact.o?.s != null ? { seed: compact.o.s } : {}),
    },
  };
}

function decodeV2(compact) {
  if (compact.v !== 2 || !Array.isArray(compact.p)) return null;
  const participants = compact.p.map(([id, name]) => ({ id, name }));

  // Rebuild blockGroups from labels, assigning fresh IDs
  const groupLabels = compact.g ?? [];
  const existingGroupIds = new Set();
  const blockGroups = groupLabels.map(label => {
    const id = uniqueId(slugify(label), existingGroupIds);
    existingGroupIds.add(id);
    return { id, label, collapsed: false };
  });

  const blocks = [
    ...(compact.b ?? [])
      .map(([fi, ti]) => ({ from: participants[fi]?.id, to: participants[ti]?.id }))
      .filter(b => b.from != null && b.to != null),
    ...(compact.bg ?? [])
      .map(([fi, ti, gi]) => ({
        from: participants[fi]?.id,
        to: participants[ti]?.id,
        group: blockGroups[gi]?.id,
      }))
      .filter(b => b.from != null && b.to != null && b.group != null),
  ];
  const relationships = (compact.r ?? [])
    .map(([ai, bi]) => ({ a: participants[ai]?.id, b: participants[bi]?.id }))
    .filter(r => r.a != null && r.b != null);
  return {
    participants, blocks, relationships, blockGroups,
    options: {
      max_solutions: compact.o?.m ?? 5,
      ...(compact.o?.s != null ? { seed: compact.o.s } : {}),
    },
  };
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

let svgSel, zoomLayer, zoomBehavior, sim, validEdgeLayer, solutionEdgeLayer, nodeCircleLayer, nodeLabelLayer;
let graphWidth = 0, graphHeight = 0;
let prevNodeIdSet = "";

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

  // Single transform target for zoom/pan — all graph layers live inside this group.
  zoomLayer = svgSel.append("g").attr("class", "zoom-layer");

  // Layer order matters for z-index (last appended = topmost).
  validEdgeLayer = zoomLayer.append("g").attr("class", "valid-edges");
  solutionEdgeLayer = zoomLayer.append("g").attr("class", "solution-edges");
  nodeCircleLayer = zoomLayer.append("g").attr("class", "node-circles");
  nodeLabelLayer = zoomLayer.append("g").attr("class", "node-labels");

  // Zoom + pan with default d3 filter (handles wheel/pinch natively).
  // Node drag calls stopPropagation() so drag-start doesn't also trigger panning.
  zoomBehavior = d3.zoom()
    .scaleExtent([0.2, 4])
    .on("zoom", ev => zoomLayer.attr("transform", ev.transform));
  svgSel.call(zoomBehavior);

  sim = d3.forceSimulation()
    .force("link", d3.forceLink().id(d => d.id).distance(120))
    .force("charge", d3.forceManyBody().strength(-300))
    .force("center", d3.forceCenter(graphWidth / 2, graphHeight / 2))
    .force("x", d3.forceX(graphWidth / 2).strength(0.07))
    .force("y", d3.forceY(graphHeight / 2).strength(0.07))
    .force("collide", d3.forceCollide(NODE_RADIUS + 18))
    .on("tick", ticked);

  new ResizeObserver(() => {
    const c = document.getElementById("graph-container");
    graphWidth = c.clientWidth;
    graphHeight = c.clientHeight;
    sim.force("center", d3.forceCenter(graphWidth / 2, graphHeight / 2));
    sim.force("x", d3.forceX(graphWidth / 2).strength(0.07));
    sim.force("y", d3.forceY(graphHeight / 2).strength(0.07));
    if (state.participants.length > 0) sim.alpha(0.3).restart();
  }).observe(container);
}

// Full graph restart: called when participants, blocks, or solve results change.
function restartGraph() {
  const nodes = syncNodes(state.participants);

  // Reset zoom to identity when the participant set changes (add/remove).
  const nodeIdSet = state.participants.map(p => p.id).join(",");
  if (nodeIdSet !== prevNodeIdSet) {
    svgSel.call(zoomBehavior.transform, d3.zoomIdentity);
    prevNodeIdSet = nodeIdSet;
  }
  const validEdges = buildValidEdges(state.participants, effectiveBlocks(state));
  const solutionEdges = buildSolutionEdges(state.solutions[state.selectedSolution]);
  const nodeColors = buildNodeColors(state.solutions[state.selectedSolution]);

  const drag = d3.drag()
    .on("start", (ev, d) => { ev.sourceEvent.stopPropagation(); if (!ev.active) sim.alphaTarget(0.3).restart(); d.fx = d.x; d.fy = d.y; })
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
  const n = state.participants.length;
  const atCap = n >= MAX_PARTICIPANTS;
  document.getElementById("participant-count").textContent = n > 0 ? `${n} / ${MAX_PARTICIPANTS}` : "";
  document.getElementById("new-participant-name").disabled = atCap;
  document.getElementById("btn-add-participant").disabled = atCap;
  state.participants.forEach((p, i) => {
    const li = document.createElement("li");

    const nameSpan = document.createElement("span");
    nameSpan.textContent = p.name;
    nameSpan.style.flex = "1";
    li.appendChild(nameSpan);

    const editBtn = document.createElement("button");
    editBtn.className = "icon-btn edit-btn";
    editBtn.textContent = "✎";
    editBtn.title = "Rename";
    editBtn.addEventListener("click", () => {
      const input = document.createElement("input");
      input.type = "text";
      input.value = p.name;
      input.className = "rename-input";
      li.replaceChild(input, nameSpan);
      editBtn.disabled = true;
      input.focus();
      input.select();

      function commitRename() {
        const newName = input.value.trim();
        if (newName && newName !== p.name) {
          p.name = newName;
          saveStateDebounced();
          renderSidebar();
          restartGraph();
        } else {
          li.replaceChild(nameSpan, input);
          editBtn.disabled = false;
        }
      }
      input.addEventListener("keydown", e => {
        if (e.key === "Enter") { e.preventDefault(); commitRename(); }
        if (e.key === "Escape") { li.replaceChild(nameSpan, input); editBtn.disabled = false; }
      });
      input.addEventListener("blur", commitRename);
    });
    li.appendChild(editBtn);

    const removeBtn = document.createElement("button");
    removeBtn.className = "icon-btn remove-btn";
    removeBtn.textContent = "×";
    removeBtn.title = "Remove";
    removeBtn.addEventListener("click", () => {
      state.participants.splice(i, 1);
      state.relationships = state.relationships.filter(r => r.a !== p.id && r.b !== p.id);
      state.blocks = state.blocks.filter(b => b.from !== p.id && b.to !== p.id);
      mutated();
      updateEmptyState();
    });
    li.appendChild(removeBtn);

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

function makeBlockItem(b, i) {
  const fromName = state.participants.find(p => p.id === b.from)?.name ?? b.from;
  const toName = state.participants.find(p => p.id === b.to)?.name ?? b.to;
  const li = document.createElement("li");
  li.innerHTML = `<span>${esc(fromName)} → ${esc(toName)}</span>`;
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

function renderBlockList() {
  const ul = document.getElementById("block-list");
  ul.innerHTML = "";

  // Ungrouped blocks first
  state.blocks.forEach((b, i) => {
    if (!b.group) ul.appendChild(makeBlockItem(b, i));
  });

  // Then each group in order
  state.blockGroups.forEach(group => {
    const groupBlocks = state.blocks
      .map((b, i) => ({ b, i }))
      .filter(({ b }) => b.group === group.id);

    // Group header
    const header = document.createElement("div");
    header.className = "block-group-header";

    const toggle = document.createElement("span");
    toggle.className = "block-group-toggle";
    toggle.textContent = group.collapsed ? "▸" : "▾";
    header.appendChild(toggle);

    const labelSpan = document.createElement("span");
    labelSpan.className = "block-group-label";
    labelSpan.textContent = group.label;
    header.appendChild(labelSpan);

    // Edit button
    const editBtn = document.createElement("button");
    editBtn.className = "icon-btn edit-btn";
    editBtn.textContent = "✎";
    editBtn.title = "Rename group";
    editBtn.addEventListener("click", e => {
      e.stopPropagation();
      const input = document.createElement("input");
      input.type = "text";
      input.value = group.label;
      input.className = "rename-input";
      header.replaceChild(input, labelSpan);
      editBtn.disabled = true;
      input.focus();
      input.select();

      function confirmRename() {
        const newLabel = input.value.trim();
        if (newLabel && newLabel !== group.label) {
          group.label = newLabel;
          saveStateDebounced();
          renderSidebar();
        } else {
          header.replaceChild(labelSpan, input);
          editBtn.disabled = false;
        }
      }
      input.addEventListener("keydown", e2 => {
        if (e2.key === "Enter") { e2.preventDefault(); confirmRename(); }
        if (e2.key === "Escape") { header.replaceChild(labelSpan, input); editBtn.disabled = false; }
      });
      input.addEventListener("blur", confirmRename);
    });
    header.appendChild(editBtn);

    // Delete group button
    const delBtn = document.createElement("button");
    delBtn.className = "icon-btn remove-btn";
    delBtn.textContent = "×";
    delBtn.title = "Delete group and all its blocks";
    delBtn.addEventListener("click", e => {
      e.stopPropagation();
      state.blocks = state.blocks.filter(b => b.group !== group.id);
      state.blockGroups = state.blockGroups.filter(g => g.id !== group.id);
      mutated();
    });
    header.appendChild(delBtn);

    // Toggle collapse on header click (not on buttons)
    header.addEventListener("click", () => {
      group.collapsed = !group.collapsed;
      saveStateDebounced();
      renderBlockList();
    });

    ul.appendChild(header);

    // Inner list
    const inner = document.createElement("ul");
    inner.className = "block-group-inner";
    if (group.collapsed) inner.hidden = true;
    groupBlocks.forEach(({ b, i }) => inner.appendChild(makeBlockItem(b, i)));
    ul.appendChild(inner);
  });
}

function renderRelationshipDropdowns() {
  const aSel = document.getElementById("rel-a");
  const bSel = document.getElementById("rel-b");
  const prevA = aSel.value;
  const prevB = bSel.value;

  [aSel, bSel].forEach(sel => {
    sel.innerHTML = "";
    state.participants.forEach(p => {
      const opt = document.createElement("option");
      opt.value = p.id;
      opt.textContent = p.name;
      sel.appendChild(opt);
    });
  });

  if (prevA && state.participants.some(p => p.id === prevA)) aSel.value = prevA;
  if (prevB && state.participants.some(p => p.id === prevB)) bSel.value = prevB;

  const disabled = state.participants.length < 2;
  aSel.disabled = disabled;
  bSel.disabled = disabled;
  document.getElementById("btn-add-relationship").disabled = disabled;
}

function renderRelationshipList() {
  const ul = document.getElementById("relationship-list");
  ul.innerHTML = "";
  state.relationships.forEach((r, i) => {
    const aName = state.participants.find(p => p.id === r.a)?.name ?? r.a;
    const bName = state.participants.find(p => p.id === r.b)?.name ?? r.b;
    const li = document.createElement("li");
    li.innerHTML = `<span>${esc(aName)} ↔ ${esc(bName)}</span>`;
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
}

function renderOptions() {
  document.getElementById("opt-max-solutions").value = state.options.maxSolutions;
  document.getElementById("opt-seed").value = state.options.seed ?? "";
}

function renderSidebar() {
  renderParticipantList();
  renderRelationshipDropdowns();
  renderRelationshipList();
  renderBlockDropdowns();
  renderBlockList();

  const genBtn = document.getElementById("btn-generate");
  genBtn.disabled = state.loading;
  genBtn.textContent = state.loading ? "Generating…" : "Generate";

  const errEl = document.getElementById("generate-error");
  errEl.textContent = state.error ?? "";

  renderOptions();
}

// Called after any mutation that invalidates existing solutions.
function mutated() {
  state.solutions = [];
  state.selectedSolution = 0;
  saveStateDebounced();
  renderSidebar();
  renderSolutionsPanel();
  restartGraph();
}

// ─── Solutions panel rendering ────────────────────────────────────────────────

function renderSolutionsPanel() {
  const tabsEl = document.getElementById("solution-tabs");
  const detailEl = document.getElementById("solution-detail");
  const histBtn = document.getElementById("btn-add-history");
  const dlBtn = document.getElementById("btn-download");

  if (!state.solutions.length) {
    tabsEl.innerHTML = "";
    detailEl.innerHTML = "";
    histBtn.hidden = true;
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

  histBtn.hidden = false;
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
  } catch (err) {
    state.error = err.message;
    state.solutions = [];
    state.selectedSolution = 0;
  } finally {
    state.loading = false;
  }

  saveState();
  renderSidebar();
  renderSolutionsPanel();
  restartGraph();
}

// ─── JSON import/export ───────────────────────────────────────────────────────

function onAddAsHistoryBlocks() {
  const sol = state.solutions[state.selectedSolution];
  if (!sol) return;

  // Build a unique label for today, e.g. "History 2026-04-06" or "History 2026-04-06 (2)"
  const today = new Date().toISOString().slice(0, 10);
  const baseLabel = `History ${today}`;
  const existingLabels = new Set(state.blockGroups.map(g => g.label));
  let label = baseLabel;
  for (let n = 2; existingLabels.has(label); n++) label = `${baseLabel} (${n})`;

  const existingGroupIds = new Set(state.blockGroups.map(g => g.id));
  const groupId = uniqueId(slugify(label), existingGroupIds);
  state.blockGroups.push({ id: groupId, label, collapsed: false });

  for (const { gifter_id, recipient_id } of sol.assignments) {
    if (!state.blocks.some(b => b.from === gifter_id && b.to === recipient_id)) {
      state.blocks.push({ from: gifter_id, to: recipient_id, group: groupId });
    }
  }
  mutated();
}

function onDownload() {
  const doc = {
    participants: state.participants,
    ...(state.relationships.length ? { relationships: state.relationships } : {}),
    blocks: state.blocks,
    ...(state.blockGroups.length ? { blockGroups: state.blockGroups } : {}),
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

  saveState();
  renderSidebar();
  renderSolutionsPanel();
  restartGraph();
  updateEmptyState();
}

// ─── Reset / Copy Link / Hash banner ──────────────────────────────────────────

function onReset() {
  state.participants = [];
  state.relationships = [];
  state.blocks = [];
  state.blockGroups = [];
  state.options = { maxSolutions: 5, seed: null };
  state.solutions = [];
  state.selectedSolution = 0;
  state.loading = false;
  state.error = null;
  try { localStorage.removeItem(LS_KEY); } catch { /* ignore */ }
  history.replaceState(null, "", location.pathname);
  document.getElementById("hash-banner").hidden = true;
  renderSidebar();
  renderSolutionsPanel();
  restartGraph();
  updateEmptyState();
}

async function onCopyLink() {
  const btn = document.getElementById("btn-copy-link");
  const url = location.origin + location.pathname + encodeStateToHash(state);
  try {
    await navigator.clipboard.writeText(url);
    btn.textContent = "Copied!";
    setTimeout(() => { btn.textContent = "Copy Link"; }, 2000);
  } catch {
    prompt("Copy this link:", url);
  }
}

function showHashBanner() {
  const el = document.getElementById("hash-banner");
  el.hidden = false;
  const dismissTimer = setTimeout(() => { el.hidden = true; }, 8000);
  el.querySelector(".hash-banner-dismiss").addEventListener("click", () => {
    clearTimeout(dismissTimer);
    el.hidden = true;
  }, { once: true });
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
    if (state.participants.length >= MAX_PARTICIPANTS) return;
    const existingIds = new Set(state.participants.map(p => p.id));
    const id = uniqueId(name, existingIds);
    state.participants.push({ id, name });
    nameInput.value = "";
    mutated();
    updateEmptyState();
  }

  nameInput.addEventListener("keydown", e => { if (e.key === "Enter") addParticipant(); });
  document.getElementById("btn-add-participant").addEventListener("click", addParticipant);

  // Add relationship
  document.getElementById("btn-add-relationship").addEventListener("click", () => {
    const a = document.getElementById("rel-a").value;
    const b = document.getElementById("rel-b").value;
    if (!a || !b || a === b) return;
    const key = [a, b].sort().join("|");
    if (state.relationships.some(r => [r.a, r.b].sort().join("|") === key)) return;
    state.relationships.push({ a, b });
    mutated();
  });

  // Add block
  document.getElementById("btn-add-block").addEventListener("click", () => {
    const from = document.getElementById("block-from").value;
    const to = document.getElementById("block-to").value;
    if (!from || !to || from === to) return;
    if (state.blocks.some(b => b.from === from && b.to === to)) return;
    state.blocks.push({ from, to });
    mutated();
  });

  // Options (read at solve time; update state on change)
  document.getElementById("opt-max-solutions").addEventListener("input", e => {
    state.options.maxSolutions = Math.max(1, parseInt(e.target.value, 10) || 5);
    saveStateDebounced();
  });
  document.getElementById("opt-seed").addEventListener("input", e => {
    const v = e.target.value.trim();
    state.options.seed = v ? parseInt(v, 10) : null;
    saveStateDebounced();
  });

  // Generate
  document.getElementById("btn-generate").addEventListener("click", onGenerate);

  // Add as history blocks
  document.getElementById("btn-add-history").addEventListener("click", onAddAsHistoryBlocks);

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

  // Copy Link
  document.getElementById("btn-copy-link").addEventListener("click", onCopyLink);

  // Reset
  document.getElementById("btn-reset").addEventListener("click", onReset);
}

// ─── Init ─────────────────────────────────────────────────────────────────────

document.addEventListener("DOMContentLoaded", () => {
  initGraph();
  wireEvents();

  // Priority: URL hash > localStorage > empty state.
  // Only treat a hash as a shared link if it carries at least one participant.
  // Clear the hash immediately so reloads don't re-apply stale link state.
  const hashState = decodeStateFromHash(location.hash);
  if (hashState?.participants.length) {
    history.replaceState(null, "", location.pathname);
    applyImport(hashState, state);
    showHashBanner();
  } else {
    loadFromLocalStorage();
  }

  renderSidebar();
  renderSolutionsPanel();
  restartGraph();
  updateEmptyState();
});
