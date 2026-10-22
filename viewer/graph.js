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

    // ---- archetype registry -------------------------------------------------
    // A homogeneous group of same-named sibling chips (the 16 Mux bit-slices
    // inside Mux16, the 16 FullAdders rippling through Add16, ...) is presented
    // in the view as ONE representative box with a count badge. Expanding an
    // archetype shows its guts grouped by ROLE across all instances (virtual
    // sub-archetypes), recursively down to "Nand ×N"; Nand-level archetypes
    // expand to their real instances. Archetype ids are negative so they never
    // collide with real node ids (which are >= 1) or DOCK (0).
    this.arch = new Map(); // archId -> {id, parentId, name, count, memberIds, gutIds, inPins, outPins, ins, outs}
    this.archOfChild = new Map(); // instance id -> archId
    this.archOfParent = new Map(); // parent node id -> archId
    {
      const nextId = { n: -1 };
      const makeArch = (parentId, name, memberIds, isVirtual) => {
        const aid = nextId.n--;
        const a = { id: aid, parentId, name, count: memberIds.length, memberIds, virtual: !!isVirtual, gutIds: [] };
        this.arch.set(aid, a);
        for (const k of memberIds) this.archOfChild.set(k, aid);
        return a;
      };

      // real archetypes: homogeneous named siblings of a real container
      for (const id of this.order) { // parents are created before children
        const kids = this.children.get(id) || [];
        if (kids.length < 2) continue;
        const name = this.nodes.get(kids[0]).name;
        if (!kids.every((k) => this.nodes.get(k).name === name)) continue;
        const a = makeArch(id, name, [...kids], false);
        this.archOfParent.set(id, a.id);
      }

      // virtual archetypes: the guts of an archetype, grouped by structural
      // role (name + ordinal among same-named gut chips) across instances
      const buildGuts = (a) => {
        const groups = new Map(); // `${name}#${slot}` -> memberIds
        for (const m of a.memberIds) {
          const kids = this.children.get(m) || [];
          const slotOf = new Map();
          for (const k of kids) {
            const nm = this.nodes.get(k).name;
            const slot = slotOf.get(nm) ?? 0;
            slotOf.set(nm, slot + 1);
            const key = `${nm}#${slot}`;
            if (!groups.has(key)) groups.set(key, []);
            groups.get(key).push(k);
          }
        }
        for (const [key, members] of groups) {
          const [name] = key.split('#');
          const g = makeArch(a.id, name, members, true);
          a.gutIds.push(g.id);
          buildGuts(g); // recurse (Nand members have no guts -> terminates)
        }
      };
      for (const a of Array.from(this.arch.values())) { // snapshot: recursion appends
        if (!a.virtual) buildGuts(a);
      }

      for (const a of this.arch.values()) {
        const { inGroups, outGroups, ins, outs, nandCount } = archAnalysis(this, a);
        a.inPins = inGroups;
        a.outPins = outGroups;
        a.ins = ins;
        a.outs = outs;
        a.nandCount = nandCount;
      }
    }
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
  // view node for an archetype (real or virtual): expansion reveals its role-
  // grouped guts when they exist, otherwise the real instances (Nand level)
  const archVnOf = (a) => {
    const openA = expanded.has(a.id);
    const kids = openA
      ? (a.gutIds.length
        ? a.gutIds.map((gid) => archVnOf(circuit.arch.get(gid)))
        : a.memberIds.map((mid) => walk(circuit.nodes.get(mid))))
      : null;
    return {
      node: {
        id: a.id,
        name: a.name,
        parent: a.parentId,
        isNand: false,
        arch: a,
        nandCount: a.nandCount,
        inputs: a.ins,
        outputs: a.outs,
      },
      arch: a,
      drillRoot: a.id === drillRootId,
      container: openA,
      children: kids,
      inPins: a.inPins,
      outPins: a.outPins,
    };
  };

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
      const archId = circuit.archOfParent.get(node.id);
      if (archId !== undefined) {
        const a = circuit.arch.get(archId);
        v.children = [archVnOf(a)];
        // the container externally behaves exactly like its archetype: reuse
        // the role-bundled ports instead of re-deriving per-bit pills
        v.inPins = a.inPins;
        v.outPins = a.outPins;
      } else {
        const kids = circuit.children.get(node.id) || [];
        v.children = kids.map((id) => walk(circuit.nodes.get(id)));
        [v.inPins, v.outPins] = computePins(circuit, node.id, open);
      }
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
  } else if (circuit.arch.has(drillRootId)) {
    // full-screen view of an archetype: bit-space, the real instances
    const a = circuit.arch.get(drillRootId);
    root = archVnOf(a);
    root.drillRoot = true;
    root.container = true; // drill always shows the member instances
    root.children = a.memberIds.map((mid) => walk(circuit.nodes.get(mid)));
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

// one archetype port role -> one pin group; bus even when the member bits
// carry unrelated (anonymous) signal names, labeled "<n>w" then
function portGroup(b, circuit) {
  const bits = [...b];
  if (bits.length < 2) {
    return { base: null, bits, isBus: false, label: circuit.signalName(bits[0]) };
  }
  let sharedBase = null;
  let shared = true;
  for (const s of bits) {
    const m = /^(.*)\[\d+\]$/.exec(circuit.signalName(s));
    const base = m ? m[1] : null;
    if (base == null) { shared = false; break; }
    if (sharedBase == null) sharedBase = base;
    else if (sharedBase !== base) { shared = false; break; }
  }
  if (shared && sharedBase) {
    return { base: sharedBase, bits, isBus: true, label: `${sharedBase}[${bits.length}]` };
  }
  return { base: null, bits, isBus: true, label: `${bits.length}w` };
}

// archetype boundary analysis: unions over member subtrees.
// Ports are bundled by ROLE: the j-th declared input of each member instance
// belongs to the same conceptual port (slot bundling), so bit-slices of
// anonymous internal wires collapse into one bus pill even without shared
// signal names. Leftover signals fall back to name-based grouping.
function archAnalysis(circuit, a) {
  const members = new Set(); // union of instance subtrees (incl guts)
  for (const m of a.memberIds) {
    for (const x of circuit.subtreeNodes.get(m)) members.add(x);
  }
  const produced = new Set(); // signals produced anywhere inside
  for (const m of a.memberIds) {
    for (const s of circuit.subtreeSignals.get(m)) produced.add(s);
  }
  const isSignal = (s) => circuit.producer.has(s) || circuit.signalNames.has(s);

  const inSignals = new Set();
  for (const id of members) {
    const n = circuit.nodes.get(id);
    for (const s of n.inputs) {
      if (produced.has(s)) continue;
      if (!isSignal(s)) continue; // constants (e.g. carry-in 0)
      inSignals.add(s);
    }
  }
  const outSignals = new Set();
  for (const s of produced) {
    const consumers = circuit.consumerOf.get(s) || [];
    if (consumers.some((c) => !members.has(c))) outSignals.add(s);
  }
  for (const s of circuit.topOutputs) {
    if (produced.has(s)) outSignals.add(s);
  }

  // role bundles: slot j of each member's declared ports
  const inBundles = []; // index j -> Set of signals
  for (const m of a.memberIds) {
    const n = circuit.nodes.get(m);
    n.inputs.forEach((s, j) => {
      if (!inSignals.has(s)) return; // produced inside / constant
      (inBundles[j] ||= new Set()).add(s);
    });
  }
  const outBundles = [];
  for (const m of a.memberIds) {
    const n = circuit.nodes.get(m);
    n.outputs.forEach((s, j) => {
      if (!outSignals.has(s)) return;
      (outBundles[j] ||= new Set()).add(s);
    });
  }

  const inGroups = [];
  for (const b of inBundles) if (b?.size) inGroups.push(portGroup(b, circuit));
  const covered = new Set(inBundles.flatMap((b) => (b ? [...b] : [])));
  const extras = new Set([...inSignals].filter((s) => !covered.has(s)));
  if (extras.size) inGroups.push(...pinGroupsOf(extras, circuit));

  const outGroups = [];
  for (const b of outBundles) if (b?.size) outGroups.push(portGroup(b, circuit));
  const outCovered = new Set(outBundles.flatMap((b) => (b ? [...b] : [])));
  const outExtras = new Set([...outSignals].filter((s) => !outCovered.has(s)));
  if (outExtras.size) outGroups.push(...pinGroupsOf(outExtras, circuit));

  let nandCount = 0;
  for (const m of a.memberIds) nandCount += circuit.nodes.get(m).nandCount || 0;

  return {
    inGroups,
    outGroups,
    ins: [...inSignals].sort((x, y) => x - y),
    outs: [...outSignals].sort((x, y) => x - y),
    nandCount,
  };
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

  // deepest visible archetype containing each real node pre-registers itself
  // via its direct members; resolution climbs from the producer through real
  // parents until a member (or a visible node) is hit
  const virtualByMember = new Map(); // real member id -> arch id
  const registerVirtual = (vn) => {
    if (vn.node.arch) {
      for (const m of vn.node.arch.memberIds) virtualByMember.set(m, vn.node.id);
    }
    if (vn.children) for (const c of vn.children) registerVirtual(c);
  };
  registerVirtual(view); // pre-order: deeper archetypes overwrite shallower

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
  const scopeSets = (vn) => {
    const id = vn.node.id;
    if (vn.arch) {
      // archetype scope: cached unions over the instance subtrees
      const members = new Set();
      const produced = new Set();
      for (const m of vn.arch.memberIds) {
        for (const x of circuit.subtreeNodes.get(m)) members.add(x);
        for (const s of circuit.subtreeSignals.get(m)) produced.add(s);
      }
      return { members, produced };
    }
    return {
      members: circuit.subtreeNodes.get(id),
      produced: circuit.subtreeSignals.get(id),
    };
  };

  const resolveForContainer = (vn, s) => {
    // returns: {type:'node', id} | {type:'pin', container, side, idx} | {type:'dock', signal}
    const prod = circuit.producerOf(s);
    const containerId = vn.node.id;
    const { members, produced } = scopeSets(vn);
    if (!produced.has(s)) {
      // produced outside (or primary) -> container's input pin (or dock at root)
      return { type: 'pinIn', container: containerId, s };
    }
    if (prod && members.has(prod)) {
      // produced inside; source = deepest visible view unit containing prod
      // (a visible real node, or a visible archetype box owning it)
      let cur = prod;
      while (cur) {
        if (visible.has(cur)) return { type: 'node', id: cur };
        const g = virtualByMember.get(cur);
        if (g !== undefined && visible.has(g)) return { type: 'node', id: g };
        cur = circuit.nodes.get(cur)?.parent ?? 0;
      }
      return { type: 'pinIn', container: containerId, s };
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
      const members = scopeSets(vn).members;
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

// drill tree path helpers
export function isExpandable(circuit, node) {
  return (circuit.children.get(node.id)?.length ?? 0) > 0;
}
