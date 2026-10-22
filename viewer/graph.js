// graph.js — circuit data model + visible-view construction

export const DOCK = 0; // virtual root node id (top-level canvas scope)

export class CircuitData {
  constructor(def) {
    this.name = def.name;
    this.order = []; // node ids in export order (evaluation order)
    this.nodes = new Map(); // id -> node
    for (const n of def.nodes) {
      const node = {
        id: n.id,
        name: n.name,
        parent: n.parent,
        inputs: n.inputs || [],
        outputs: n.outputs || [],
        nandCount: n.nandCount || 0,
        depth: n.depth || 0,
        isNand: !!n.isNand,
      };
      this.nodes.set(node.id, node);
      this.order.push(node.id);
    }
    this.signalNames = new Map(
      Object.entries(def.signalNames || {}).map(([k, v]) => [+k, v]),
    );
    this.topOutputs = def.outputs || [];

    // parent -> children (export order preserved)
    this.children = new Map();
    for (const id of this.order) {
      const n = this.nodes.get(id);
      pushMap(this.children, n.parent, id);
    }
    this.topNodes = this.children.get(0) || [];

    // signal -> producing (nand) node
    this.producer = new Map();
    for (const id of this.order) {
      const n = this.nodes.get(id);
      if (!n.isNand || n.outputs.length !== 1) continue;
      if (!this.producer.has(n.outputs[0])) this.producer.set(n.outputs[0], id);
    }

    // signal -> consuming nodes
    this.consumerOf = new Map();
    for (const id of this.order) {
      const n = this.nodes.get(id);
      for (const s of n.inputs) pushMap(this.consumerOf, s, id);
    }

    // primary inputs = named signals with no producer
    // (unnamed un-driven signals are constants — sim defaults them to 0)
    this.primary = new Set();
    for (const s of this.consumerOf.keys()) {
      if (!this.producer.has(s) && this.signalNames.has(s)) this.primary.add(s);
    }

    // buses: base name -> sorted bits (ascending index)
    this.buses = detectBuses(this.signalNames);
    const inBus = new Map(); // signal id -> base
    for (const [base, bits] of this.buses) {
      for (const b of bits) inBus.set(b, base);
    }
    this.busOf = inBus;

    // subtree membership + signal sets (for container pin analysis)
    this.subtreeNodes = new Map(); // id -> Set of descendant ids (incl self)
    this.subtreeSignals = new Map(); // id -> Set of signals produced in subtree
    const compute = (id) => {
      if (this.subtreeNodes.has(id)) return;
      if (!this.nodes.has(id)) {
        // virtual root: union of top-level chip subtrees
        const members = new Set([id]);
        const signals = new Set();
        for (const cid of this.topNodes) {
          compute(cid);
          for (const m of this.subtreeNodes.get(cid)) members.add(m);
          for (const s of this.subtreeSignals.get(cid)) signals.add(s);
        }
        this.subtreeNodes.set(id, members);
        this.subtreeSignals.set(id, signals);
        return;
      }
      const node = this.nodes.get(id);
      const members = new Set([id]);
      const signals = new Set();
      if (node.isNand) {
        for (const o of node.outputs) signals.add(o);
      }
      for (const cid of this.children.get(id) || []) {
        compute(cid);
        const cm = this.subtreeNodes.get(cid);
        for (const m of cm) members.add(m);
        for (const s of this.subtreeSignals.get(cid)) signals.add(s);
      }
      this.subtreeNodes.set(id, members);
      this.subtreeSignals.set(id, signals);
    };
    compute(0);
    for (const id of this.order) compute(id);
  }

  signalName(s) {
    return this.signalNames.get(s) ?? `s${s}`;
  }

  producerOf(signal) {
    return this.producer.get(signal) ?? null;
  }

  inSubtree(id, rootId) {
    return this.subtreeNodes.get(rootId)?.has(id) ?? false;
  }

  // nearest visible ancestor (visible = emitted set). returns node id or null.
  ownerOf(id, visible) {
    let cur = id;
    while (cur !== 0) {
      if (visible.has(cur)) return cur;
      cur = this.nodes.get(cur)?.parent ?? 0;
    }
    return visible.has(0) ? 0 : null;
  }
}

function pushMap(map, key, value) {
  const arr = map.get(key);
  if (arr) arr.push(value);
  else map.set(key, [value]);
}

export function detectBuses(signalNames) {
  const buses = new Map(); // base -> Map(idx -> sig)
  for (const [sig, name] of signalNames) {
    const m = /^(.*)\[(\d+)\]$/.exec(name);
    if (!m) continue;
    const base = m[1];
    const idx = +m[2];
    if (!buses.has(base)) buses.set(base, new Map());
    buses.get(base).set(idx, sig);
  }
  // to sorted bit arrays
  const out = new Map();
  for (const [base, m] of buses) {
    out.set(base, [...m.entries()].sort((a, b) => a[0] - b[0]).map((e) => e[1]));
  }
  // keep only multi-bit buses (>1 bit) — single [0] signals treated as scalars
  for (const [base, bits] of [...out]) {
    if (bits.length < 2) out.delete(base);
  }
  return out;
}

// ---------- grouping signals into drawable wire groups ----------
// each wire group: { label, bits: [signal ids], size, base|null, from, to }
// buses render as single thick wires; scalars render individually; anonymous
// multi-bit bundles render as one wire labeled "<n>w".

export function groupSignals(signals, circuit) {
  const groups = [];
  const byBus = new Map();
  const scalars = [];
  for (const s of signals) {
    const base = circuit.busOf.get(s);
    if (base) pushMap(byBus, base, s);
    else scalars.push(s);
  }
  for (const [base, bits] of byBus) {
    const withIdx = bits
      .map((s) => ({ s, idx: busIndex(circuit, base, s) }))
      .sort((a, b) => a.idx - b.idx);
    groups.push({
      label: `${base}[${withIdx[0].idx}:${withIdx[withIdx.length - 1].idx}]`,
      bits: withIdx.map((x) => x.s),
      size: withIdx.length,
      base,
    });
  }
  for (const s of scalars) {
    groups.push({ label: circuit.signalName(s), bits: [s], size: 1, base: null });
  }
  return groups;
}

function busIndex(circuit, base, signal) {
  const bits = circuit.buses.get(base);
  return bits.indexOf(signal);
}

// ---------- view model ----------

// viewNode: { node, children: viewNode[]|null, container: bool, inPins, outPins }
// Pins (computed only for containers): [{signals, bits, label, base, isBus}]
export function buildView(circuit, drillRootId, expanded) {
  const walk = (node) => {
    const hasKids = (circuit.children.get(node.id)?.length ?? 0) > 0;
    const open = hasKids && expanded.has(node.id);
    const v = {
      node,
      drillRoot: node.id === drillRootId,
      container: open || node.id === drillRootId,
      children: null,
      inPins: null,
      outPins: null,
    };
    if (v.container) {
      const kids = circuit.children.get(node.id) || [];
      v.children = kids.map((id) => walk(circuit.nodes.get(id)));
      [v.inPins, v.outPins] = computePins(circuit, node.id, open);
    }
    return v;
  };

  let root;
  if (drillRootId === DOCK) {
    const [inPins, outPins] = computePins(circuit, 0, true);
    root = {
      node: { id: 0, name: circuit.name, isNand: false, virtual: true },
      drillRoot: true,
      container: true,
      children: (circuit.topNodes).map((id) => walk(circuit.nodes.get(id))),
      inPins,
      outPins,
    };
  } else {
    root = walk(circuit.nodes.get(drillRootId));
  }
  return root;
}

function pinGroupsOf(signals, circuit) {
  // group pins by bus where possible, else scalars (sorted by signal id)
  const list = signals instanceof Set ? [...signals] : signals;
  const groups = [];
  const used = new Set();
  for (const [base, bits] of circuit.buses) {
    const present = bits.filter((b) => list.includes(b));
    // only bundle when at least half the bus arrives
    if (present.length >= 2 && present.length * 2 > bits.length) {
      const minIdx = busIndex(circuit, base, present[0]);
      groups.push({ base, bits: present, isBus: true, label: `${base}[${minIdx}..]` });
      for (const p of present) used.add(p);
    }
  }
  const rest = [...list].filter((s) => !used.has(s)).sort((a, b) => a - b);
  for (const s of rest) {
    groups.push({ base: null, bits: [s], isBus: false, label: circuit.signalName(s) });
  }
  // buses first, then scalars
  groups.sort((a, b) => (b.isBus ? 1 : 0) - (a.isBus ? 1 : 0));
  return groups;
}

function computePins(circuit, containerId, open) {
  const members = circuit.subtreeNodes.get(containerId);
  const produced = circuit.subtreeSignals.get(containerId);

  // input pins: consumed inside, produced outside
  const inSignals = new Set();
  for (const id of members) {
    const n = circuit.nodes.get(id);
    if (!n) continue; // virtual root has no own inputs
    if (!open && id !== containerId) continue; // collapsed container: only its own inputs
    for (const s of n.inputs) {
      if (!produced.has(s)) {
        // un-named un-driven signals are constants (e.g. carry-in 0), not inputs
        if (!circuit.producer.has(s) && !circuit.signalNames.has(s)) continue;
        inSignals.add(s);
      }
    }
  }
  // output pins: produced inside, consumed outside
  const outSignals = new Set();
  for (const s of produced) {
    const consumers = circuit.consumerOf.get(s) || [];
    if (consumers.some((c) => !members.has(c))) outSignals.add(s);
  }
  // top-level outputs (circuit result)
  for (const s of circuit.topOutputs) {
    if (produced.has(s)) outSignals.add(s);
  }

  return [pinGroupsOf(inSignals, circuit), pinGroupsOf(outSignals, circuit)];
}

// ---------- wires between visible nodes ----------

// returns Map key `${src}>${dst}` -> wire {src, dst, signals:Set, groups}
// src: node id | "dock:<signal>" | "pin:<containerId>:i:<index>"|"...:o:..."
// Emits wires for every visible consumer node + container-boundary edges.
export function computeWires(circuit, view) {
  const visible = new Set(); // emitted node ids
  const membersOf = new Map(); // container node id -> subtree member ids
  const collect = (vn) => {
    visible.add(vn.node.id);
    membersOf.set(vn.node.id, circuit.subtreeNodes.get(vn.node.id) || new Set([vn.node.id]));
    if (vn.children) for (const c of vn.children) collect(c);
  };
  collect(view);

  // map: signal -> the outPin pseudo source when produced inside a container
  // (handled via ownerOf: emitted container itself is the wire source)

  const wires = new Map();
  const addWire = (src, dst, s, vnId) => {
    if (src === dst) return;
    const key = `${src}>${dst}`;
    const w = wires.get(key);
    if (w) {
      w.signals.add(s);
    } else {
      wires.set(key, { src, dst, signals: new Set([s]), vnId });
    }
  };

  // helper: resolve the source "node" for a signal within a container scope
  const resolveForContainer = (vn, s) => {
    // returns: {type:'node', id} | {type:'pin', container, side, idx} | {type:'dock', signal}
    const prod = circuit.producerOf(s);
    const containerId = vn.node.id;
    const members = circuit.subtreeNodes.get(containerId);
    const produced = circuit.subtreeSignals.get(containerId);
    if (!produced.has(s)) {
      // produced outside (or primary) -> container's input pin (or dock at root)
      return { type: 'pinIn', container: containerId, s };
    }
    if (prod && members.has(prod)) {
      // produced inside; source = nearest visible ancestor of prod
      const own = circuit.ownerOf(prod, visibleMembers(circuit, vn));
      if (own == null) return { type: 'pinIn', container: containerId, s };
      return { type: 'node', id: own };
    }
    return { type: 'pinIn', container: containerId, s };
  };

  const assignOutPinIdx = new Map(); // `${containerId}:o:${signal}` -> pin index
  const assignInPinIdx = new Map(); // `${containerId}:i:${signal}` -> pin index
  const pinIndexOf = (vn, kind, s) => {
    const key = `${vn.node.id}:${kind}:${s}`;
    let idx = (kind === 'i' ? assignInPinIdx : assignOutPinIdx).get(key);
    if (idx === undefined) {
      const pins = kind === 'i' ? vn.inPins : vn.outPins;
      idx = pins.findIndex((p) => p.bits.includes(s));
      if (idx === -1) idx = pins.length; // shouldn't happen; clamp
      (kind === 'i' ? assignInPinIdx : assignOutPinIdx).set(key, idx);
    }
    return idx;
  };

  // walk all containers emitting wires for their visible members
  const walk = (vn) => {
    if (!vn.children) return;
    for (const child of vn.children) {
      for (const s of child.node.inputs) {
        // constants (unnamed un-driven signals) never get wires
        if (!circuit.producer.has(s) && !circuit.signalNames.has(s)) continue;
        const src = resolveForContainer(vn, s);
        if (src.type === 'node') {
          addWire(src.id, child.node.id, s, vn.node.id);
        } else if (src.type === 'pinIn') {
          addWire(`pin:${vn.node.id}:i:${pinIndexOf(vn, 'i', s)}`, child.node.id, s, vn.node.id);
        }
      }
      // inner production feeding the container's own output pins
      const members = circuit.subtreeNodes.get(vn.node.id);
      if (child.node.isNand) {
        for (const s of child.node.outputs) {
          const consumers = circuit.consumerOf.get(s) || [];
          if (consumers.some((c) => !members.has(c))) {
            addWire(child.node.id, `pin:${vn.node.id}:o:${pinIndexOf(vn, 'o', s)}`, s, vn.node.id);
          }
        }
      }
      walk(child);
    }
  };
  walk(view);

  // group each wire's signals
  for (const w of wires.values()) w.groups = groupSignals([...w.signals], circuit);
  return wires;
}

// visible members of a container *view node* — ids emitted inside it (not all subtree)
function visibleMembers(circuit, vn) {
  const set = new Set();
  const rec = (v) => {
    set.add(v.node.id);
    if (v.children) for (const c of v.children) rec(c);
  };
  rec(vn);
  return set;
}

// drill tree path helpers
export function isExpandable(circuit, node) {
  return (circuit.children.get(node.id)?.length ?? 0) > 0;
}
