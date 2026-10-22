// ui.js — DOM panels: input toggles, inspector, breadcrumbs, tooltip

export function renderInputs(el, circuit, sim, pins, onChange) {
  el.innerHTML = '';
  const frag = document.createDocumentFragment();

  for (const group of pins) {
    const g = document.createElement('div');
    g.className = 'bus-group';
    if (group.isBus) {
      const head = document.createElement('div');
      head.className = 'bus-title';
      const lbl = document.createElement('span');
      lbl.textContent = `${group.base}[${group.bits.length}]`;
      const val = document.createElement('span');
      val.className = 'bus-value';
      val.dataset.bus = group.base;
      head.append(lbl, val);
      g.append(head);
      const bits = document.createElement('div');
      bits.className = 'bits';
      for (const s of group.bits) bits.append(bitPill(s, circuit, sim, onChange, group.base));
      g.append(bits);
    } else {
      g.append(bitPill(group.bits[0], circuit, sim, onChange, null, true));
    }
    frag.append(g);
  }
  el.append(frag);
  refreshInputValues(el, circuit, sim);
}

function bitPill(signal, circuit, sim, onChange, busBase, wide) {
  const pill = document.createElement('div');
  pill.className = 'bit-pill';
  pill.dataset.signal = String(signal);
  if (wide) pill.style.gridTemplateColumns = '1fr';
  const dot = document.createElement('i');
  dot.className = 'v';
  const name = document.createElement('span');
  name.textContent = circuit.signalName(signal);
  pill.append(dot, name);
  pill.addEventListener('click', () => {
    sim.toggle(signal);
    onChange();
  });
  return pill;
}

export function refreshInputValues(el, circuit, sim) {
  for (const pill of el.querySelectorAll('.bit-pill')) {
    const s = +pill.dataset.signal;
    const v = sim.valueOf(s);
    pill.classList.toggle('on', v === 1);
  }
  for (const val of el.querySelectorAll('.bus-value')) {
    const base = val.dataset.bus;
    let v = 0;
    let known = true;
    for (const b of circuit.buses.get(base) || []) {
      if (!sim.inputs.has(b)) { known = false; break; }
      v = (v << 1) | sim.valueOf(b);
    }
    val.textContent = known
      ? `0x${v.toString(16).padStart(Math.ceil((circuit.buses.get(base).length) / 4), '0')}`
      : '';
  }
}

export function renderInspector(el, hit, circuit, sim, actions) {
  el.innerHTML = '';
  if (!hit) {
    el.innerHTML = '<p class="empty">click a chip or wire</p>';
    return;
  }

  if (hit.kind === 'wire') {
    el.append(h3('wire'));
    const w = hit.wire;
    el.append(kv([
      ['from', nodeName(circuit, w.src)],
      ['to', nodeName(circuit, w.dst)],
      ['bits', String(w.signals.size)],
    ]));
    const list = document.createElement('div');
    list.className = 'wire-chips';
    for (const g of w.groups) {
      const row = document.createElement('div');
      row.className = 'wire-chip';
      const left = document.createElement('span');
      left.textContent = g.label;
      const right = document.createElement('span');
      right.className = 'v';
      if (g.size === 1) {
        const v = sim.valueOf(g.bits[0]);
        left.textContent = g.label;
        right.textContent = v;
        right.classList.add(v === 1 ? 'v1' : 'v0');
      } else {
        const vals = g.bits.map((b) => sim.valueOf(b));
        const known = g.bits.every((b) => sim.values.has(b) || sim.inputs.has(b));
        right.textContent = known
          ? hexOf(g.bits, sim)
          : '?';
        right.classList.add(vals.every((v) => v === 1) ? 'v1' : vals.some((v) => v === 1) ? '' : 'v0');
        right.title = vals.join('');
      }
      row.append(left, right);
      list.append(row);
    }
    el.append(list);
    return;
  }

  if (hit.kind === 'pinIn' || hit.kind === 'pinOut') {
    el.append(h3(hit.kind === 'pinIn' ? 'input' : 'output'));
    const pin = hit.pin;
    el.append(kv([
      ['at', nodeName(circuit, +hit.container)],
    ]));
    const list = document.createElement('div');
    list.className = 'wire-chips';
    for (const s of pin.bits) {
      const row = document.createElement('div');
      row.className = 'wire-chip';
      const l = document.createElement('span');
      l.textContent = circuit.signalName(s);
      const r = document.createElement('span');
      const v = sim.valueOf(s);
      r.textContent = v;
      r.className = `v ${v === 1 ? 'v1' : 'v0'}`;
      row.append(l, r);
      list.append(row);
    }
    el.append(list);
    if (hit.kind === 'pinIn') {
      const note = document.createElement('p');
      note.className = 'hint';
      note.textContent = 'flip bits from the Inputs panel on the left';
      el.append(note);
    }
    return;
  }

  // node
  const node = hit.node;
  el.append(h3(node.arch ? 'chip group' : node.isNand ? 'gate' : 'chip'));
  const title = document.createElement('div');
  title.style.cssText = 'font-size:16px;font-weight:700;margin-bottom:6px';
  title.textContent = node.arch ? `${node.arch.name} ×${node.arch.count}` : node.name;
  el.append(title);

  const kids = node.arch ? node.arch.memberIds : (circuit.children.get(node.id) || []);
  el.append(kv([
    ['id', String(node.id)],
    ['NAND gates', node.isNand ? '1' : String(node.arch?.nandCount ?? node.nandCount)],
    [node.arch ? 'instances' : 'levels deep', node.arch ? String(node.arch.count) : String(node.depth)],
    ['inputs', String(node.inputs.length)],
    ['outputs', String(node.outputs.length)],
  ]));

  // live signal tables
  const inSigs = new Set(node.inputs);
  el.append(h3('inputs'));
  el.append(sigTable(inSigs, circuit, sim));
  const outSigs = new Set(node.outputs);
  if (outSigs.size) {
    el.append(h3('outputs'));
    el.append(sigTable(outSigs, circuit, sim));
  }

  const btnRow = document.createElement('div');
  btnRow.className = 'btn-row';
  if (!node.isNand && (kids.length || node.arch)) {
    const expanded = actions.isExpanded(node.id);
    const enter = mkBtn('Enter', () => actions.enter(node.id));
    btnRow.append(enter);
    const ex = mkBtn(expanded ? 'Collapse' : 'Expand', () => (expanded ? actions.collapse(node.id) : actions.expand(node.id)));
    btnRow.append(ex);
  }
  const navigate = mkBtn('Back', () => actions.back());
  btnRow.append(navigate);
  el.append(btnRow);

  if (kids.length && !node.isNand) {
    el.append(h3(`contents (${kids.length})`, true));
    const list = document.createElement('div');
    list.className = 'member-list';
    for (const cid of kids) {
      const c = circuit.nodes.get(cid);
      const b = mkBtn(node.arch ? `${c.name} #${cid}` : c.name, () => actions.select(cid));
      b.title = c.isNand ? 'NAND gate' : `${c.name} (${c.nandCount} NANDs)`;
      list.append(b);
    }
    el.append(list);
  }
}

function sigTable(sigs, circuit, sim) {
  const tbl = document.createElement('table');
  tbl.className = 'sig-table';
  for (const s of [...sigs].sort(sortSignals(circuit))) {
    const tr = document.createElement('tr');
    const td1 = document.createElement('td');
    td1.textContent = circuit.signalName(s);
    const td2 = document.createElement('td');
    td2.className = `v v${sim.valueOf(s)}`;
    td2.textContent = sim.valueOf(s);
    tr.append(td1, td2);
    tbl.append(tr);
  }
  return tbl;
}

function sortSignals(circuit) {
  return (a, b) => {
    const na = circuit.signalName(a);
    const nb = circuit.signalName(b);
    const idx = (n) => (/^(.*)\[(\d+)\]$/.exec(n));
    const ma = idx(na);
    const mb = idx(nb);
    if (ma && mb && ma[1] === mb[1]) return +ma[2] - +mb[2];
    return na.localeCompare(nb, undefined, { numeric: true });
  };
}

function hexOf(bits, sim) {
  let v = 0;
  for (const b of bits) v = (v << 1) | (sim.valueOf(b) ? 1 : 0);
  return `0x${v.toString(16).padStart(Math.ceil(bits.length / 4), '0')}`;
}

export function nodeName(circuit, ref) {
  if (typeof ref === 'number') {
    if (circuit.arch && circuit.arch.has(ref)) {
      const a = circuit.arch.get(ref);
      return `${a.name} ×${a.count}`;
    }
    return circuit.nodes.get(ref)?.name ?? String(ref);
  }
  const m = /^pin:(-?\d+):(i|o):(\d+)$/.exec(String(ref));
  if (m) {
    const id = +m[1];
    const container = circuit.arch?.has(id)
      ? nodeName(circuit, id)
      : circuit.nodes.get(id)?.name ?? 'circuit';
    return `${container} ${m[2] === 'i' ? 'in' : 'out'}[${m[3]}]`;
  }
  return String(ref);
}

function h3(text) {
  const el = document.createElement('h3');
  el.className = 'sec';
  el.textContent = text;
  return el;
}

function kv(pairs) {
  const div = document.createElement('div');
  div.className = 'kv';
  for (const [k, v] of pairs) {
    const row = document.createElement('div');
    const key = document.createElement('span');
    key.textContent = `${k}: `;
    const val = document.createElement('b');
    val.textContent = v;
    row.append(key, val);
    div.append(row);
  }
  return div;
}

function mkBtn(text, fn) {
  const b = document.createElement('button');
  b.textContent = text;
  b.addEventListener('click', fn);
  return b;
}

export function renderCrumbs(el, circuit, drill, onJump) {
  el.innerHTML = '';
  const parts = [circuit.name, ...drill.slice(1).map((id) => nodeName(circuit, id))];
  parts.forEach((p, i) => {
    const span = document.createElement('span');
    span.textContent = p;
    if (i < parts.length - 1) {
      span.addEventListener('click', () => onJump(drill[i]));
    }
    el.append(span);
  });
}
