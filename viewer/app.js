// app.js — application glue: state, transforms, interaction

import { CircuitData, DOCK, buildView, computeWires } from './graph.js';
import { layout } from './layout.js';
import {
  createRenderer, indexWires, hitItem, hitWire,
} from './render.js';
import { Sim } from './sim.js';
import {
  renderInputs, refreshInputValues, renderInspector, renderCrumbs,
} from './ui.js';

const clamp = (v, a, b) => Math.min(b, Math.max(a, v));

window.addEventListener('error', (e) => {
  const el = document.getElementById('status');
  if (el) el.textContent = `ERROR: ${e.message} @ ${e.filename?.split('/').pop()}:${e.lineno}`;
});
window.addEventListener('unhandledrejection', (e) => {
  const el = document.getElementById('status');
  const reason = e.reason;
  const stack = reason && reason.stack ? ` :: ${String(reason.stack).slice(0, 400)}` : '';
  if (el) el.textContent = `REJECT: ${reason}${stack}`;
});

const state = {
  bundle: null,
  circuit: null,
  sim: null,
  drill: [0],
  expanded: new Set(),
  wires: null,
  layout: null,
  idx: null,
  view: { x: 0, y: 0, k: 1 },
  hovered: null,
  selected: null,
  dirty: true,
  drag: null, // {sx, sy, vx, vy, moved}
};

const $ = (id) => document.getElementById(id);
const canvas = $('canvas');
const renderer = createRenderer(canvas);
const tooltip = $('tooltip');

async function boot() {
  let circuits;
  if (window.EMBEDDED_CIRCUITS) {
    circuits = window.EMBEDDED_CIRCUITS.circuits;
  } else {
    const res = await fetch('/circuit.json');
    if (!res.ok) {
      setStatus(`failed to load circuit.json (${res.status})`);
      return;
    }
    circuits = (await res.json()).circuits;
  }
  state.bundle = { circuits };
  const sel = $('circuit-select');
  sel.innerHTML = '';
  state.bundle.circuits.forEach((c, i) => {
    const o = document.createElement('option');
    o.value = String(i);
    o.textContent = c.name;
    sel.append(o);
  });
  const last = localStorage.getItem('n2t.circuit');
  const idx = clamp(+last || 0, 0, state.bundle.circuits.length - 1);
  sel.value = String(idx);
  sel.addEventListener('change', () => setSelCircuit(+sel.value));
  setSelCircuit(idx);

  // URL params for shareable/testing views:
  // ?circuit=1&drill=38,39&expand=40&set=x=00ff,y=1234,zx=1
  const params = new URLSearchParams(location.search);
  if (params.get('circuit')) {
    const i = params.get('circuit');
    const byName = state.bundle.circuits.findIndex((c) => c.name === i);
    sel.value = String(byName >= 0 ? byName : clamp(+i || 0, 0, state.bundle.circuits.length - 1));
    setSelCircuit(+sel.value);
  }
  if (params.get('drill')) {
    const ids = params.get('drill').split(',').map(Number)
      .filter((id) => state.circuit.nodes.has(id) || state.circuit.arch.has(id));
    if (ids.length) {
      for (const id of ids) state.expanded.add(id);
      state.drill = [0, ...ids];
      rebuild(true);
    }
  }
  if (params.get('expand')) {
    for (const id of params.get('expand').split(',')) {
      const n = +id;
      if (state.circuit.nodes.has(n) || state.circuit.arch.has(n)) state.expanded.add(n);
    }
    rebuild(true);
  }
  if (params.get('select')) {
    const sid = +params.get('select');
    if (state.circuit.nodes.has(sid)) {
      setSelected({ id: String(sid), kind: 'chip', node: state.circuit.nodes.get(sid) });
    } else if (state.circuit.arch.has(sid)) {
      const a = state.circuit.arch.get(sid);
      setSelected({
        id: String(sid),
        kind: 'chip',
        node: {
          id: sid, name: a.name, arch: a, isNand: false,
          nandCount: a.nandCount, inputs: a.ins, outputs: a.outs,
        },
      });
    }
  }
  if (params.get('set')) {
    for (const pair of params.get('set').split(',')) {
      const eq = pair.indexOf('=');
      if (eq > 0) setBusValue(pair.slice(0, eq), pair.slice(eq + 1));
    }
    refreshInputs();
    renderInspectorPanel();
    state.dirty = true;
  }

  wireControls();
  paint(); // synchronous first paint (screenshots/OS snapshot before first RAF)
  requestAnimationFrame(tick);
}

// set a scalar or bus value by signal name (e.g. zx=1, x=00ff, x=0b1010...)
// MSB-first: name[0] is treated as the most significant bit (Hack convention).
function setBusValue(key, raw) {
  const { circuit, sim } = state;
  // scalar signal
  for (const [sig, name] of circuit.signalNames) {
    if (name === key) {
      sim.setInput(sig, +raw ? 1 : 0);
      return;
    }
  }
  // bus bits by name[idx]
  const bits = [];
  for (const [sig, name] of circuit.signalNames) {
    const m = new RegExp(`^${key.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')}\\[(\\d+)\\]$`).exec(name);
    if (m) bits[+m[1]] = sig;
  }
  if (!bits.length) return;
  let hex;
  if (/^0b/.test(raw)) {
    const str = raw.slice(2).padStart(bits.length, '0');
    for (let i = 0; i < bits.length; i++) {
      if (bits[i] !== undefined) sim.setInput(bits[i], str.charAt(i) === '1' ? 1 : 0);
    }
    return;
  }
  hex = parseInt(raw.replace(/^0x/, ''), 16);
  if (Number.isNaN(hex)) return;
  for (let i = 0; i < bits.length; i++) {
    if (bits[i] === undefined) continue;
    sim.setInput(bits[i], (hex >> (bits.length - 1 - i)) & 1 ? 1 : 0);
  }
}

function setStatus(msg) {
  $('status').textContent = msg;
}

function setSelCircuit(idx) {
  localStorage.setItem('n2t.circuit', String(idx));
  const def = state.bundle.circuits[idx];
  state.circuit = new CircuitData(def);
  state.sim = new Sim(state.circuit);
  state.drill = [0];
  state.expanded = new Set(
    JSON.parse(localStorage.getItem(`n2t.expanded.${def.name}`) || '[]')
      .filter((id) => state.circuit.nodes.has(id)), 
  );
  state.selected = null;
  state.hovered = null;
  rebuild(true);
}

function rebuild(fit = false) {
  localStorage.setItem(
    `n2t.expanded.${state.circuit.name}`,
    JSON.stringify([...state.expanded]),
  );
  state.hovered = null;
  const rootId = state.drill[state.drill.length - 1];
  const vm = buildView(state.circuit, rootId, state.expanded);
  state.vm = vm;
  state.wires = computeWires(state.circuit, vm);
  state.layout = layout(vm, state.wires, state.circuit);
  state.idx = indexWires(state.layout);
  if (fit) fitView();
  renderCrumbs($('crumbs'), state.circuit, state.drill, jumpTo);
  refreshInputs();
  renderInspectorPanel();
  setStatus(`${state.circuit.name} · ${state.layout.nodes.length} shown · ${state.wires.size} wires · sim#${state.sim.evalCount}`);
  state.dirty = true;
}

function refreshInputs() {
  const rootPins = currentRootPins();
  renderInputs($('inputs'), state.circuit, state.sim, rootPins, () => {
    refreshInputs();
    renderInspectorPanel();
    state.dirty = true;
  });
}

function currentRootPins() {
  return state.vm?.inPins || [];
}

function renderInspectorPanel() {
  renderInspector($('inspect-body'), state.selected, state.circuit, state.sim, {
    isExpanded: (id) => state.expanded.has(id),
    expand: (id) => { state.expanded.add(id); rebuild(); },
    collapse: (id) => { state.expanded.delete(id); rebuild(); },
    enter: (id) => { state.drill.push(id); state.selected = null; rebuild(true); },
    back: () => jumpTo(state.drill[state.drill.length - 2] ?? 0),
    select: (id) => setSelected({ id, kind: 'chip', node: state.circuit.nodes.get(id) }),
  });
}

// ---------- transforms ----------

function fitView() {
  const { layout, view } = state;
  const w = canvas.clientWidth;
  const h = canvas.clientHeight;
  const pad = 40;
  if (!layout.width || !layout.height) return;
  const k = clamp(Math.min((w - pad * 2) / layout.width, (h - pad * 2) / layout.height), 0.1, 2);
  view.k = k;
  view.x = (w - layout.width * k) / 2 + layout.offsetX * k;
  view.y = (h - layout.height * k) / 2 + layout.offsetY * k;
  state.dirty = true;
}

function toWorld(clientX, clientY) {
  const rect = canvas.getBoundingClientRect();
  const { view } = state;
  const sx = clientX - rect.left;
  const sy = clientY - rect.top;
  return { x: (sx - view.x) / view.k, y: (sy - view.y) / view.k };
}

function zoomAt(sx, sy, factor) {
  const { view } = state;
  const k2 = clamp(view.k * factor, 0.12, 3.5);
  const rect = canvas.getBoundingClientRect();
  const px = sx - rect.left;
  const py = sy - rect.top;
  view.x = px - (px - view.x) * (k2 / view.k);
  view.y = py - (py - view.y) * (k2 / view.k);
  view.k = k2;
  state.dirty = true;
  hideTooltip();
}

// ---------- interaction ----------

function wireControls() {
  canvas.addEventListener('wheel', (e) => {
    e.preventDefault();
    zoomAt(e.clientX, e.clientY, Math.exp(-e.deltaY * 0.0012));
  }, { passive: false });

  canvas.addEventListener('mousedown', (e) => {
    state.drag = { sx: e.clientX, sy: e.clientY, vx: state.view.x, vy: state.view.y, moved: false };
  });
  window.addEventListener('mousemove', onMove);
  window.addEventListener('mouseup', onUp);

  canvas.addEventListener('dblclick', (e) => {
    const hit = hoverAt(e);
    if (hit && (hit.kind === 'chip' || hit.kind === 'container')) {
      const id = +hit.id;
      const hasKids = (state.circuit.children.get(id)?.length ?? 0) > 0
        || state.circuit.arch.has(id);
      if (hasKids) {
        state.drill.push(id);
        state.selected = null;
        rebuild(true);
      }
    }
  });

  window.addEventListener('keydown', (e) => {
    if (e.target instanceof HTMLInputElement || e.target instanceof HTMLSelectElement) return;
    const id = state.selected && +state.selected.id;
    if (e.key === 'Escape') jumpTo(state.drill[state.drill.length - 2] ?? 0);
    else if (e.key === 'f') fitView();
    else if (e.key === '=' || e.key === '+') zoomAt(canvas.clientWidth / 2, canvas.clientHeight / 2, 1.2);
    else if (e.key === '-') zoomAt(canvas.clientWidth / 2, canvas.clientHeight / 2, 1 / 1.2);
    else if (id && e.key === 'e') { state.expanded.add(id); rebuild(); }
    else if (id && e.key === 'c') { state.expanded.delete(id); rebuild(); }
    else if (id && e.key === 'Enter') { state.drill.push(id); state.selected = null; rebuild(true); }
  });

  $('btn-fit').addEventListener('click', fitView);
  $('btn-export').addEventListener('click', exportPNG);
  $('btn-reset').addEventListener('click', () => {
    state.sim.reset();
    refreshInputs();
    renderInspectorPanel();
    setStatus(`${state.circuit.name} · inputs reset`);
    state.dirty = true;
  });
  $('btn-expand').addEventListener('click', () => {
    for (const n of expandableInDrill()) state.expanded.add(n);
    rebuild();
  });
  $('btn-collapse').addEventListener('click', () => {
    state.expanded = new Set();
    rebuild();
  });
  $('btn-collapse-one').addEventListener('click', () => {
    // collapse the deepest currently-expanded chip/archetype on the drill path
    for (let i = state.drill.length - 1; i >= 1; i--) {
      const p = state.drill[i];
      for (const cid of [...state.expanded]) {
        if (cid < 0) {
          // archetype: collapse if it sits inside this scope
          const parent = state.circuit.arch.get(cid)?.parentId;
          if (parent === p || (parent && state.circuit.inSubtree(parent, p))) {
            state.expanded.delete(cid);
            rebuild();
            return;
          }
          continue;
        }
        if (state.circuit.inSubtree(cid, p) && cid !== p) {
          state.expanded.delete(cid);
          rebuild();
          return;
        }
      }
    }
  });
  window.addEventListener('resize', () => { state.dirty = true; });
}

export function jumpTo(drillId) {
  const idx = state.drill.indexOf(drillId);
  if (idx === -1) return;
  state.drill = state.drill.slice(0, Math.max(1, idx + 1));
  state.selected = null;
  rebuild(true);
}

// ---------- picking ----------

function hoverAt(e) {
  const w = toWorld(e.clientX, e.clientY);
  const node = hitItem(state.layout, w);
  if (node) return node;
  const wire = hitWire(state.layout, w);
  if (wire) return { id: `wire:${wire.e.key2 ?? wire.e.key}`, kind: 'wire', wire: wire.e };
  return null;
}

function onMove(e) {
  if (state.drag) {
    const dx = e.clientX - state.drag.sx;
    const dy = e.clientY - state.drag.sy;
    if (Math.abs(dx) + Math.abs(dy) > 3) state.drag.moved = true;
    if (state.drag.moved) {
      state.view.x = state.drag.vx + dx;
      state.view.y = state.drag.vy + dy;
      state.dirty = true;
      hideTooltip();
    }
    return;
  }
  const hover = hoverAt(e);
  state.hovered = hover;
  if (hover) {
    showTooltip(e, tooltipFor(hover));
    canvas.style.cursor = hover.kind === 'wire' ? 'pointer' : 'grab';
  } else {
    hideTooltip();
    canvas.style.cursor = 'grab';
  }
  state.dirty = true;
}

function onUp(e) {
  const drag = state.drag;
  state.drag = null;
  if (!drag || drag.moved) return;
  const hit = hoverAt(e);
  if (!hit) {
    setSelected(null);
    return;
  }
  // interactive input pins on the root scope
  if (hit.kind === 'pinIn' && isRootScopePin(hit)) {
    if (hit.isBus || !state.circuit.primary.has(hit.bits[0])) {
      // multi-bit groups and internal (non-primary) signals just select
      setSelected(hit);
    } else {
      state.sim.toggle(hit.bits[0]);
      refreshInputs();
      renderInspectorPanel();
      state.dirty = true;
    }
    return;
  }
  if (hit.kind === 'container' && isBadgeZone(hit, e)) {
    state.expanded.delete(+hit.id);
    rebuild();
    return;
  }
  if (hit.kind === 'chip' && isBadgeZone(hit, e)) {
    state.expanded.add(+hit.id);
    rebuild();
    return;
  }
  setSelected(hit);
  // archetype boxes ladder on click: reveal the canon decomposition inline
  if (hit.node?.arch && !state.expanded.has(+hit.id)) {
    state.expanded.add(+hit.id);
    rebuild();
  }
}

function isRootScopePin(hit) {
  const rootId = state.drill[state.drill.length - 1];
  return +hit.container === rootId;
}

function isBadgeZone(hit, e) {
  const item = hit.item || hit;
  const w = toWorld(e.clientX, e.clientY);
  return w.x > (item.x ?? 0) + (item.w ?? 0) - 16 && w.y < (item.y ?? 0) + 14;
}

function setSelected(hit) {
  state.selected = hit;
  renderInspectorPanel();
  state.dirty = true;
}

// ---------- tooltip ----------

function tooltipFor(hit) {
  const { circuit, sim } = state;
  if (hit.kind === 'wire') {
    const { wire } = hit;
    const lines = wire.groups.map((g) => {
      if (g.size === 1) return `${g.label} = ${sim.valueOf(g.bits[0])}`;
      const vals = g.bits.map((b) => sim.valueOf(b)).reverse();
      const hex = hexOf(g.bits, sim);
      return `${g.label} = ${hex}  (b0..${g.bits.length - 1}: ${vals.join('')})`;
    });
    return lines.join('\n') || '(signals)';
  }
  if (hit.kind === 'pinIn' || hit.kind === 'pinOut') {
    const rows = hit.bits.map((s) => `${circuit.signalName(s)} = ${sim.valueOf(s)}`);
    return `${hit.label}\n${rows.join('\n')}`;
  }
  const n = hit.node;
  if (!n) return '';
  const kids = circuit.children.get(n.id)?.length ?? 0;
  return `${n.name}\n${n.isNand ? 'NAND gate' : `chip${kids ? ` · ${kids} parts` : ''} · ${n.nandCount} NANDs`}\nsig ${n.inputs.join(',')}`;
}

function showTooltip(e, text) {
  if (!text) { hideTooltip(); return; }
  tooltip.textContent = text;
  tooltip.style.display = 'block';
  const stage = $('stage').getBoundingClientRect();
  let x = e.clientX - stage.left + 14;
  let y = e.clientY - stage.top + 14;
  x = Math.min(x, stage.width - 350);
  y = Math.min(y, stage.height - 90);
  tooltip.style.left = `${x}px`;
  tooltip.style.top = `${y}px`;
}

function hideTooltip() {
  tooltip.style.display = 'none';
}

function hexOf(bits, sim) {
  let v = 0;
  for (const b of bits) {
    if (!sim.values.has(b) && !sim.inputs.has(b)) return '—';
    v = (v << 1) | (sim.valueOf(b) ? 1 : 0);
  }
  return `0x${v.toString(16).padStart(Math.ceil(bits.length / 4), '0')}`;
}

// ---------- misc controls ----------

function expandableInDrill() {
  const out = [];
  const seen = new Set();
  const rootId = state.drill[state.drill.length - 1];
  const stack = [rootId];
  const push = (id) => {
    if (seen.has(id)) return;
    seen.add(id);
    out.push(id);
    stack.push(id);
  };
  while (stack.length) {
    const id = stack.pop();
    const a = state.circuit.arch.get(id);
    if (a) {
      // archetype: its guts if they exist, otherwise the raw bit instances
      for (const k of (a.gutIds.length ? a.gutIds : a.memberIds)) push(k);
      continue;
    }
    // real node: descend into its children, behind archetype markers too
    const archId = state.circuit.archOfChild.get(id);
    if (archId !== undefined) push(archId);
    for (const c of state.circuit.children.get(id) || []) push(c);
  }
  return out.filter((n) => !state.circuit.nodes.get(n)?.isNand);
}

function exportPNG() {
  const url = canvas.toDataURL('image/png');
  const a = document.createElement('a');
  a.href = url;
  a.download = `${state.circuit.name}-view.png`;
  a.click();
}

// ---------- render loop ----------

function paint() {
  if (!state.layout) return;
  renderer.draw({
    layout: state.layout,
    view: state.view,
    state: {
      hovered: state.hovered,
      selected: state.selected,
      sim: state.sim,
      wireState: (e) => {
        const vals = [];
        for (const g of e.groups) for (const b of g.bits) vals.push(state.sim.valueOf(b));
        let c = '#2a3448';
        if (vals.length && vals.every((v) => v === 1)) c = '#34d399';
        else if (vals.some((v) => v === 1)) c = '#f59e0b';
        return { color: c };
      },
      isRootInput: (it) => +it.container === state.drill[state.drill.length - 1] && it.kind === 'pinIn',
      rootPins: new Set(),
    },
    idx: state.idx,
  });
}

function tick(t) {
  requestAnimationFrame(tick);
  if (!state.dirty) return;
  state.dirty = false;
  paint();
}

boot();
