// layout.js — layered left→right layout with recursive container expansion.
// Pure data: no DOM/canvas access.

const M = {
  boxW: 128, boxH: 48,
  gateW: 66, gateH: 38,
  rankGap: 110,      // horizontal gap between columns
  rowGap: 14,        // vertical gap between rows
  pad: 26,           // container inner padding
  titleH: 30,        // container title strip
  pinH: 22,          // pin pill height
  pinW: 92,          // pin pill width (label drawn inside)
  pinGapX: 10,       // gap between pin band and first column
  pinGapY: 12,       // vertical gap between pin pills
  margin: 110,       // world margin around the root container
};

export function layout(vm, wires, circuit) {
  const content = solveContent(vm, wires, circuit);
  const minX = Math.min(0, ...content.nodes.map((n) => n.x - M.margin));
  return {
    width: content.w + 2 * M.margin,
    height: content.h + 2 * M.margin,
    offsetX: -minX,
    offsetY: M.margin,
    nodes: content.nodes,
    edges: content.edges,
  };
}

const GATE_NAMES = ['Nand', 'Not', 'And', 'Or', 'Nor', 'Xor', 'Mux', 'DMux'];

const isContainer = (vn) => !!(vn && vn.children);

// Solve the contents of container view-node vn in content-local coordinates.
// Returns { w, h, nodes, edges, inPins, outPins }.
function solveContent(vn, wires, circuit) {
  const containerId = vn.node.id;
  const kids = vn.children || [];

  // ---- solve each member (recursively for containers) ----
  const members = kids.map((k) => {
    if (isContainer(k)) {
      const inner = solveContent(k, wires, circuit);
      return {
        vn: k,
        node: k.node,
        kind: 'container',
        w: 2 * M.pad + inner.w,
        h: M.titleH + 2 * M.pad + inner.h,
        inner,
        x: 0, y: 0,
      };
    }
    // single-bit primitives render as shaped glyphs; everything else as boxes
    const gateNamed = !k.node.arch && GATE_NAMES.includes(k.node.name);
    const gk = k.node.isNand || gateNamed;
    return {
      vn: k,
      node: k.node,
      kind: gk ? 'gate' : 'chip',
      w: gk ? M.gateW : M.boxW,
      h: gk ? M.gateH : M.boxH,
      inner: null,
      x: 0, y: 0,
    };
  });

  // ---- edges belonging to this container ----
  const myEdges = [];
  if (wires) {
    for (const w of wires.values()) {
      if (w.vnId !== containerId) continue;
      myEdges.push(w);
    }
  }

  const memberIdx = new Map();
  members.forEach((m, i) => memberIdx.set(m.node.id, i));

  // ---- rank: longest path over member→member edges ----
  const preds = members.map(() => []);
  for (const e of myEdges) {
    if (typeof e.src !== 'number' || typeof e.dst !== 'number') continue;
    const si = memberIdx.get(e.src);
    const di = memberIdx.get(e.dst);
    if (si !== undefined && di !== undefined) preds[di].push(si);
  }
  const rank = new Array(members.length).fill(-1);
  const busy = new Array(members.length).fill(false);
  const rankOf = (i) => {
    if (rank[i] >= 0) return rank[i];
    if (busy[i]) return 0; // cycle guard
    busy[i] = true;
    let r = 0;
    for (const p of preds[i]) r = Math.max(r, rankOf(p) + 1);
    busy[i] = false;
    rank[i] = r;
    return r;
  };
  for (let i = 0; i < members.length; i++) rankOf(i);

  // ---- arrange into rank columns ----
  const byRank = new Map();
  members.forEach((m, i) => {
    if (!byRank.has(rank[i])) byRank.set(rank[i], []);
    byRank.get(rank[i]).push(i);
  });
  const ranks = [...byRank.keys()].sort((a, b) => a - b);
  for (const r of ranks) byRank.set(r, byRank.get(r).slice().sort((a, b) => a - b));

  // adjacency weights for ordering — include pin-fed wires so pins order members
  const cross = new Map(); // "a|b" -> weight
  for (const e of myEdges) {
    const si = typeof e.src === 'number' ? memberIdx.get(e.src) : -1;
    const di = typeof e.dst === 'number' ? memberIdx.get(e.dst) : -1;
    if (si < 0 && di < 0) continue;
    const k = `${si}|${di}`;
    cross.set(k, (cross.get(k) || 0) + 1);
  }
  // pseudo adjacency: pin-in sources sit at rank -1
  const pinAdj = new Map(); // memberIdx -> total pin weight
  for (const e of myEdges) {
    if (typeof e.src === 'string' && e.src.startsWith(`pin:${containerId}:i:`) && typeof e.dst === 'number') {
      const di = memberIdx.get(e.dst);
      if (di !== undefined) pinAdj.set(di, (pinAdj.get(di) || 0) + 1);
    }
  }

  const posOf = new Map();
  for (const [r, col] of byRank) col.forEach((i, p) => posOf.set(i, p));

  for (let sweep = 0; sweep < 4; sweep++) {
    const down = sweep % 2 === 0;
    const order = down ? ranks : [...ranks].reverse();
    for (const r of order) {
      const col = byRank.get(r);
      const key = (i) => {
        // barycenter over already-ordered neighbours (any adjacent offset rank)
        let sum = 0, cnt = 0;
        const collect = (other) => {
          const p = posOf.get(other);
          if (p === undefined) return;
          sum += p; cnt++;
        };
        for (const [k2, wgt] of cross) {
          const [a, b] = k2.split('|').map(Number);
          if (b === i && a >= 0) for (let w = 0; w < wgt; w++) collect(a);
          else if (a === i && b >= 0) for (let w = 0; w < wgt; w++) collect(b);
        }
        const pinBary = (() => {
          const pa = pinAdj.get(i) || 0;
          return pa ? 0 : NaN; // pin-fed members gravitate to top-ish
        })();
        if (cnt === 0) {
          for (let w = 0; w < (pinAdj.get(i) || 0); w++) sum += 0.5, cnt++;
          return Number.isNaN(pinBary) ? posOf.get(i) : sum / cnt;
        }
        return sum / cnt;
      };
      col.sort((x, y) => key(x) - key(y));
      col.forEach((i, p) => posOf.set(i, p));
    }
  }

  // ---- positions ----
  const pinBandI = (vn.inPins || []).length ? M.pinW + M.pinGapX : 0;
  const pinBandO = (vn.outPins || []).length ? M.pinW + M.pinGapX : 0;
  const colW = ranks.length ? ranks.map((r) => Math.max(...byRank.get(r).map((i) => members[i].w))) : [M.boxW];
  const colX = [];
  let xc = pinBandI;
  for (let ci = 0; ci < colW.length; ci++) {
    colX.push(xc);
    xc += colW[ci] + M.rankGap;
  }

  const colH = ranks.map((r, ci) => {
    const col = byRank.get(r);
    return col.reduce((a, i) => a + members[i].h, 0) + (col.length - 1) * M.rowGap;
  });
  const pinSpaceH = Math.max(
    (vn.inPins || []).length * (M.pinH + M.pinGapY),
    (vn.outPins || []).length * (M.pinH + M.pinGapY),
  );
  const contentH = Math.max(...colH, pinSpaceH, M.boxH);
  for (let ci = 0; ci < ranks.length; ci++) {
    const col = byRank.get(ranks[ci]);
    let y = Math.max(0, (contentH - colH[ci]) / 2);
    for (const i of col) {
      members[i].x = colX[ci];
      members[i].y = y;
      y += members[i].h + M.rowGap;
    }
  }
  const contentW = pinBandI
    + (colW.reduce((a, b) => a + b, 0) + Math.max(0, colW.length - 1) * M.rankGap)
    + pinBandO;

  // ---- pin pill placement along left/right boundary ----
  const inPins = [];
  const outPins = [];
  const pinYs = (pins, side) => {
    if (!pins.length) return [];
    const targets = [];
    for (let k = 0; k < pins.length; k++) {
      let sy = 0, n = 0;
      for (const e of myEdges) {
        let hit = false;
        if (side === 'i' && e.src === `pin:${containerId}:i:${k}` && typeof e.dst === 'number') hit = true;
        if (side === 'o' && e.dst === `pin:${containerId}:o:${k}` && typeof e.src === 'number') hit = true;
        if (hit) {
          const mi = memberIdx.get(side === 'i' ? e.dst : e.src);
          if (mi !== undefined) { sy += members[mi].y + members[mi].h / 2; n++; }
        }
      }
      targets.push(n ? sy / n : ((k + 0.5) / pins.length) * contentH);
    }
    const order = targets.map((t, i) => [t, i]).sort((a, b) => a[0] - b[0]);
    const ys = new Array(pins.length).fill(0);
    let cursor = M.pinH / 2 + 4;
    for (const [, idx] of order) {
      ys[idx] = Math.max(cursor, Math.min(targets[idx], contentH - M.pinH / 2 - 4));
      cursor = ys[idx] + M.pinH + M.pinGapY;
    }
    return ys;
  };
  const inYs = pinYs(vn.inPins || [], 'i');
  const outYs = pinYs(vn.outPins || [], 'o');
  const pinBaseX = pinBandI ? 0 : 0;
  void pinBaseX;
  (vn.inPins || []).forEach((p, k) => {
    inPins.push({ ...p, idx: k, side: 'i', container: containerId, x: 0, y: Math.max(0, inYs[k] - M.pinH / 2), w: M.pinW, h: M.pinH });
  });
  (vn.outPins || []).forEach((p, k) => {
    outPins.push({ ...p, idx: k, side: 'o', container: containerId, x: contentW - M.pinW, y: Math.max(0, outYs[k] - M.pinH / 2), w: M.pinW, h: M.pinH });
  });

  // ---- items & edge routing ----
  const items = [];
  const allEdges = [];
  const posData = new Map();
  for (const m of members) posData.set(m.node.id, { x: m.x, y: m.y, w: m.w, h: m.h });
  for (const m of members) {
    items.push({
      refId: m.node.id,
      kind: m.kind,
      node: m.node,
      x: m.x,
      y: m.y,
      w: m.w,
      h: m.h,
      expandable: m.node.arch
        ? true
        : !m.node.isNand && (circuit.children.get(m.node.id)?.length ?? 0) > 0,
    });
  }

  const pinAnchor = (v, end) => {
    const match = /^pin:(\d+):(i|o):(\d+)$/.exec(String(v));
    if (!match || +match[1] !== containerId) return null;
    const list = match[2] === 'i' ? inPins : outPins;
    const p = list[+match[3]];
    if (!p) return null;
    return { x: p.x + (match[2] === 'i' ? p.w : 0), y: p.y + p.h / 2 };
  };

  const anchor = (e, side) => {
    const v = side === 'from' ? e.src : e.dst;
    if (typeof v === 'number') {
      const m = posData.get(v);
      if (!m) return null;
      if (side === 'from') {
        const mv = memberIdx.get(v);
        if (mv !== undefined && members[mv].kind === 'container') {
          const pin = innerOutPin(members[mv], e);
          if (pin) return pin;
        }
        return { x: m.x + m.w, y: m.y + m.h / 2 };
      }
      return { x: m.x, y: m.y + m.h / 2 };
    }
    return pinAnchor(v, side);
  };

  for (const e of myEdges) {
    const from = anchor(e, 'from');
    const to = anchor(e, 'to');
    if (!from || !to) continue;
    e.pts = bezie(from, to);
    allEdges.push(e);
  }

  // ---- translate nested container layouts into our content coordinates ----
  for (const m of members) {
    if (m.kind !== 'container' || !m.inner) continue;
    const ox = m.x + M.pad;
    const oy = m.y + M.titleH + M.pad;
    for (const it of m.inner.nodes) {
      items.push({ ...it, x: it.x + ox, y: it.y + oy });
    }
    for (const ed of m.inner.edges) {
      allEdges.push({ ...ed, pts: ed.pts.map(([px, py]) => [px + ox, py + oy]) });
    }
  }

  // boundary pins as hittable items
  for (const p of inPins) {
    items.push({ refId: `pin:${containerId}:i:${p.idx}`, kind: 'pinIn', ...p });
  }
  for (const p of outPins) {
    items.push({ refId: `pin:${containerId}:o:${p.idx}`, kind: 'pinOut', ...p });
  }

  return { w: contentW, h: contentH, nodes: items, edges: allEdges, inPins, outPins };
}

function innerOutPin(memberItem, e) {
  const inner = memberItem.inner;
  if (!inner || !inner.outPins || !inner.outPins.length) return null;
  const s = [...e.signals][0];
  const pin = inner.outPins.find((p) => p.bits.includes(s)) || inner.outPins[0];
  return {
    x: memberItem.x + M.pad + pin.x + pin.w,
    y: memberItem.y + M.titleH + M.pad + pin.y + pin.h / 2,
  };
}

function bezie(from, to) {
  const dx = to.x - from.x;
  const lead = Math.max(36, Math.min(150, Math.abs(dx) * 0.45));
  const c1 = { x: from.x + lead, y: from.y };
  const c2 = { x: to.x - lead, y: to.y };
  const pts = [];
  const steps = 16;
  for (let t = 0; t <= steps; t++) {
    const u = t / steps;
    const v = 1 - u;
    pts.push([
      v * v * v * from.x + 3 * v * v * u * c1.x + 3 * v * u * u * c2.x + u * u * u * to.x,
      v * v * v * from.y + 3 * v * v * u * c1.y + 3 * v * u * u * c2.y + u * u * u * to.y,
    ]);
  }
  return pts;
}
