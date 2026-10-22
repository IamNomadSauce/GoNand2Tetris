// render.js — canvas drawing + hit-testing for the circuit viewer

const C = {
  bg: '#0b0f1a',
  grid: 'rgba(70,90,140,0.07)',
  chipFill: '#1a2440',
  chipStroke: '#3b4a6b',
  containerFill: 'rgba(56,76,130,0.10)',
  containerStroke: '#38486e',
  titleText: '#aab6d4',
  gateFill: '#241d3d',
  gateStroke: '#8a5cf6',
  w0: '#2a3448',
  w1: '#34d399',
  wMixed: '#f59e0b',
  hover: '#6ea8ff',
  sel: '#fbbf24',
  text: '#dfe6f5',
  dim: '#7d8bab',
  badge: '#54618a',
  pinFill: '#182238',
  pinText: '#9fb0d0',
};

export function wireColor(values) {
  if (!values.length) return C.w0;
  if (values.every((v) => v === 1)) return C.w1;
  if (values.some((v) => v === 1)) return C.wMixed;
  return C.w0;
}

export function wireValueState(signals, sim) {
  const vals = [...signals].map((s) => sim.valueOf(s));
  return { vals, color: wireColor(vals) };
}

// index wires by their endpoints & bus group label (built once per layout)
export function indexWires(layout) {
  const byEndpoint = new Map(); // endpoint ref (number|string) -> Set(edge key)
  const byKey = new Map(); // edge key -> edge
  for (const e of layout.edges) {
    if (!e.pts) continue;
    const key = e.key2 ?? e.key;
    byKey.set(key, e);
    for (const v of [e.src, e.dst]) {
      const k = String(v);
      if (!byEndpoint.has(k)) byEndpoint.set(k, new Set());
      byEndpoint.get(k).add(key);
    }
  }
  return { byEndpoint, byKey };
}

export function createRenderer(canvas) {
  const ctx = canvas.getContext('2d');

  function draw(model) {
    const { layout, view: t, state, idx } = model;
    const dpr = window.devicePixelRatio || 1;
    const w = canvas.clientWidth;
    const h = canvas.clientHeight;
    if (canvas.width !== Math.round(w * dpr) || canvas.height !== Math.round(h * dpr)) {
      canvas.width = Math.round(w * dpr);
      canvas.height = Math.round(h * dpr);
    }
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    ctx.fillStyle = C.bg;
    ctx.fillRect(0, 0, w, h);
    drawGrid(t, w, h);
    ctx.translate(t.x, t.y);
    ctx.scale(t.k, t.k);

    const focus = state.hovered || state.selected;
    const lit = focus ? lightSetFor(focus, idx) : null;

    // ---- edges ----
    for (const e of layout.edges) {
      if (!e.pts) continue;
      const key = e.key2 ?? e.key;
      const isLit = lit ? lit.edges.has(key) : false;
      ctx.globalAlpha = lit ? (isLit ? 1 : 0.12) : 1;
      const { color } = state.wireState(e);
      ctx.strokeStyle = isLit ? C.hover : color;
      const width = e.groups.some((g) => g.size > 1) ? 4 : 1.8;
      ctx.lineWidth = width * (isLit ? 1.8 : 1);
      ctx.beginPath();
      ctx.moveTo(e.pts[0][0], e.pts[0][1]);
      for (let i = 1; i < e.pts.length; i++) ctx.lineTo(e.pts[i][0], e.pts[i][1]);
      ctx.stroke();
    }
    ctx.globalAlpha = 1;

    // ---- items (back to front) ----
    for (const it of layout.nodes) {
      ctx.save();
      const isHover = state.hovered && state.hovered.id === String(it.refId);
      const isSel = state.selected && state.selected.id === String(it.refId);
      const dimmed = lit ? !lit.nodes.has(String(it.refId)) : false;
      if (dimmed) ctx.globalAlpha = 0.25;
      if (it.kind === 'container') drawContainer(it, isHover, isSel);
      else if (it.kind === 'gate') drawGate(it, isHover, isSel);
      else if (it.kind === 'chip') drawChip(it, isHover, isSel);
      else if (it.kind === 'pinIn' || it.kind === 'pinOut') drawPin(it, state);
      ctx.restore();
    }

    // wire labels when zoomed in enough
    if (t.k >= 0.9 && focus) {
      ctx.font = '10px ui-monospace, monospace';
      ctx.textAlign = 'center';
      ctx.textBaseline = 'middle';
      const seen = new Set();
      for (const e of layout.edges) {
        if (!e.pts) continue;
        const key = e.key2 ?? e.key;
        if (!lit || !lit.edges.has(key) || seen.has(key)) continue;
        seen.add(key);
        const mid = e.pts[Math.floor(e.pts.length / 2)];
        const top = e.pts.reduce((a, p) => (p[1] < a[1] ? p : a), e.pts[0]);
        ctx.fillStyle = 'rgba(10,14,24,0.85)';
        const labelText = e.groups.map((g) => g.label).join(', ');
        const tw = ctx.measureText(labelText).width + 10;
        ctx.fillRect(mid[0] - tw / 2, top[1] - 16, tw, 14);
        ctx.fillStyle = C.pinText;
        ctx.fillText(labelText, mid[0], top[1] - 9);
      }
    }
  }

  function drawGrid(t, w, h) {
    const step = 44 * t.k;
    if (step < 12) return;
    ctx.strokeStyle = C.grid;
    ctx.lineWidth = 1;
    ctx.beginPath();
    for (let x = ((t.x % step) + step) % step; x < w; x += step) { ctx.moveTo(x, 0); ctx.lineTo(x, h); }
    for (let y = ((t.y % step) + step) % step; y < h; y += step) { ctx.moveTo(0, y); ctx.lineTo(w, y); }
    ctx.stroke();
  }

  function roundRect(x, y, w, h, r) {
    ctx.beginPath();
    ctx.moveTo(x + r, y);
    ctx.arcTo(x + w, y, x + w, y + h, r);
    ctx.arcTo(x + w, y + h, x, y + h, r);
    ctx.arcTo(x, y + h, x, y, r);
    ctx.arcTo(x, y, x + w, y, r);
    ctx.closePath();
  }

  function drawChip(it, isHover, isSel) {
    const stroke = isSel ? C.sel : isHover ? C.hover : C.chipStroke;
    ctx.fillStyle = C.chipFill;
    ctx.strokeStyle = stroke;
    ctx.lineWidth = isHover || isSel ? 2 : 1.2;
    roundRect(it.x, it.y, it.w, it.h, 9);
    ctx.fill();
    ctx.stroke();
    ctx.fillStyle = C.text;
    ctx.textAlign = 'center';
    ctx.textBaseline = 'middle';
    const name = it.node.name;
    ctx.font = '12px ui-monospace, monospace';
    const tw = ctx.measureText(name).width;
    if (tw > it.w - 14) {
      const fs = Math.max(9.5, 12 * (it.w - 14) / tw);
      ctx.font = `${fs}px ui-monospace, monospace`;
    }
    const sub = it.node.nandCount > 0;
    ctx.fillText(name, it.x + it.w / 2, it.y + it.h / 2 - (sub ? 6 : 0), it.w - 12);
    if (sub) {
      ctx.font = '9px ui-monospace, monospace';
      ctx.fillStyle = C.dim;
      ctx.fillText(`${it.node.nandCount} NANDs`, it.x + it.w / 2, it.y + it.h / 2 + 12);
    }
    if (it.expandable) drawCornerBadge(it.x + it.w, it.y, '+', isHover || isSel);
  }

  function drawContainer(it, isHover, isSel) {
    const stroke = isSel ? C.sel : isHover ? C.hover : C.containerStroke;
    ctx.fillStyle = C.containerFill;
    roundRect(it.x, it.y, it.w, it.h, 14);
    ctx.fill();
    ctx.strokeStyle = stroke;
    ctx.lineWidth = isHover || isSel ? 2.2 : 1.5;
    roundRect(it.x, it.y, it.w, it.h, 14);
    ctx.stroke();
    ctx.fillStyle = 'rgba(30,42,74,0.6)';
    roundRect(it.x + 1, it.y + 1, it.w - 2, 22, 13);
    ctx.fill();
    ctx.font = '11px ui-monospace, monospace';
    ctx.fillStyle = C.titleText;
    ctx.textAlign = 'left';
    ctx.textBaseline = 'middle';
    const nandBadge = it.node?.nandCount ? ` · ${it.node.nandCount} NANDs` : '';
    ctx.fillText(`${it.node?.name ?? ''}${nandBadge}`, it.x + 12, it.y + 12);
    drawCornerBadge(it.x + it.w, it.y, '−', isHover || isSel);
  }

  function drawGate(it, isHover, isSel) {
    const stroke = isSel ? C.sel : isHover ? C.hover : C.gateStroke;
    ctx.fillStyle = C.gateFill;
    ctx.strokeStyle = stroke;
    ctx.lineWidth = isHover || isSel ? 2 : 1.2;
    const { x, y, w, h } = it;
    const bubble = 7;
    const bodyW = w - bubble * 2;
    ctx.beginPath();
    if (it.node.name === 'Nand') {
      ctx.moveTo(x, y);
      ctx.lineTo(x + bodyW * 0.55, y);
      ctx.arc(x + bodyW * 0.55, y + h / 2, h / 2, -Math.PI / 2, Math.PI / 2);
      ctx.lineTo(x, y + h);
      ctx.closePath();
      ctx.fill();
      ctx.stroke();
    } else if (it.node.name === 'Not') {
      ctx.moveTo(x, y);
      ctx.lineTo(x + bodyW, y + h / 2);
      ctx.lineTo(x, y + h);
      ctx.closePath();
      ctx.fill();
      ctx.stroke();
    } else {
      roundRect(x, y, w - bubble, h, 6);
      ctx.fill();
      ctx.stroke();
    }
    ctx.beginPath();
    ctx.arc(x + w - bubble, y + h / 2, bubble / 2, 0, Math.PI * 2);
    ctx.fill();
    ctx.stroke();
    ctx.font = '9px ui-monospace, monospace';
    ctx.fillStyle = C.dim;
    ctx.textAlign = 'center';
    ctx.textBaseline = 'middle';
    ctx.fillText(it.node.name, x + (w - bubble) / 2, y + h / 2, w - 10);
  }

  function drawPin(it, state) {
    const interactive = state.isRootInput(it);
    const vals = it.bits.map((s) => state.sim.valueOf(s));
    const all1 = vals.length > 0 && vals.every((v) => v === 1);
    const any1 = vals.some((v) => v === 1);
    const fill = all1 ? 'rgba(52,211,153,0.25)' : any1 ? 'rgba(245,158,11,0.2)' : C.pinFill;
    ctx.fillStyle = fill;
    ctx.strokeStyle = interactive && (state.hovered && state.hovered.id === String(it.refId)) ? C.hover : C.chipStroke;
    ctx.lineWidth = interactive ? 1.6 : 1;
    roundRect(it.x, it.y, it.w, it.h, it.h / 2);
    ctx.fill();
    ctx.stroke();
    ctx.font = '10px ui-monospace, monospace';
    ctx.fillStyle = interactive ? C.text : C.pinText;
    ctx.textAlign = 'center';
    ctx.textBaseline = 'middle';
    const label = it.base ? `${it.base}[${it.bits.length}]` : it.label;
    let val = '';
    if (it.bits.length === 1) {
      val = ` ${vals[0] ?? 0}`;
    }
    ctx.fillText(label + val, it.x + it.w / 2, it.y + it.h / 2, it.w - 10);
    // bus hex readout, drawn just outside the pill
    if (it.base && it.bits.length > 1) {
      const hex = state.sim.busValue(it.base);
      if (hex) {
        ctx.font = '11px ui-monospace, monospace';
        ctx.fillStyle = vals.some((v) => v === 1) ? C.w1 : C.dim;
        ctx.textAlign = it.side === 'o' ? 'left' : 'right';
        const hx = it.side === 'o' ? it.x + it.w + 8 : it.x - 8;
        ctx.fillText(hex, hx, it.y + it.h / 2);
      }
    }
  }

  function drawCornerBadge(x, y, ch, on) {
    ctx.fillStyle = on ? C.sel : C.badge;
    ctx.font = '10px ui-monospace, monospace';
    ctx.textAlign = 'right';
    ctx.textBaseline = 'top';
    ctx.fillText(`[${ch}]`, x, y);
  }

  return { draw, ctx };
}

// ---- hit testing ----
export function hitItem(layout, world) {
  const { x, y } = world;
  const items = layout.nodes;
  let containerHit = null;
  for (let i = items.length - 1; i >= 0; i--) {
    const it = items[i];
    if (x < it.x || x > it.x + it.w || y < it.y || y > it.y + it.h) continue;
    if (it.kind === 'container') {
      const inTitle = y < it.y + 22;
      const nearEdge = x < it.x + 7 || x > it.x + it.w - 7 || y < it.y + 7 || y > it.y + it.h - 7;
      if (inTitle || nearEdge) {
        containerHit ??= hitOf(it);
      }
      continue;
    }
    return hitOf(it);
  }
  if (containerHit) return containerHit;
  return null;
}

export function hitWire(layout, world) {
  const { x, y } = world;
  let best = null;
  for (const e of layout.edges) {
    if (!e.pts) continue;
    for (const [px, py] of e.pts) {
      const d = Math.hypot(px - x, py - y);
      if (d < 6 && (!best || d < best.d)) {
        best = { d, e };
      }
    }
  }
  return best;
}

export function hitOf(it) {
  if (it.kind === 'pinIn' || it.kind === 'pinOut') {
    return {
      id: it.refId,
      kind: it.kind,
      pin: it,
      container: it.container,
      bits: it.bits,
      isBus: !!it.base,
      label: it.base ? `${it.base}[${it.bits.length}]` : it.label,
      node: null,
    };
  }
  return { id: String(it.refId), kind: it.kind, node: it.node, item: it, container: it.container ?? null };
}

// elements "lit" for a hovered/selected element
export function lightSetFor(hit, idx) {
  const edges = new Set();
  const nodes = new Set();
  if (hit.kind === 'wire' && hit.wire) {
    const e = hit.wire;
    edges.add(e.key2 ?? e.key);
    addEndpoint(e.src, nodes);
    addEndpoint(e.dst, nodes);
  } else {
    const id = String(hit.id);
    const touch = idx.byEndpoint.get(id)
      ?? idx.byEndpoint.get(idNum(id) ?? id);
    if (touch) for (const k of touch) {
      edges.add(k);
      const e = idx.byKey.get(k);
      if (e) { addEndpoint(e.src, nodes); addEndpoint(e.dst, nodes); }
    }
    nodes.add(id);
  }
  return { edges, nodes };
}

function addEndpoint(v, nodes) {
  if (typeof v === 'number') nodes.add(String(v));
  else nodes.add(String(v));
}

const idNumRe = /^pin:(\d+)/;
function idNum(id) {
  const m = idNumRe.exec(id);
  return m ? m[1] : null;
}
