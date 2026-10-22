// sim.js — NAND netlist evaluation, mirrors Go's Circuit.Evaluate.

export class Sim {
  constructor(circuit) {
    this.circuit = circuit;
    this.inputs = new Map(); // signal id -> 0|1
    this.values = new Map(); // signal id -> 0|1
    this.evalCount = 0;
    this.reset();
  }

  reset() {
    this.inputs.clear();
    for (const s of this.circuit.primary) this.inputs.set(s, 0);
    this.evaluate();
  }

  setInput(signal, value) {
    this.inputs.set(signal, value ? 1 : 0);
    this.evaluate();
  }

  toggle(signal, to) {
    const cur = this.values.get(signal) ?? 0;
    this.setInput(signal, to !== undefined ? to : (cur ? 0 : 1));
  }

  valueOf(signal) {
    return this.values.get(signal) ?? 0;
  }

  evaluate() {
    const { circuit } = this;
    this.values.clear();
    for (const [s, v] of this.inputs) this.values.set(s, v);
    for (const id of circuit.order) {
      const n = circuit.nodes.get(id);
      if (!n.isNand) continue;
      const a = this.values.get(n.inputs[0]) ?? 0;
      const b = this.values.get(n.inputs[1]) ?? 0;
      this.values.set(n.outputs[0], a === 1 && b === 1 ? 0 : 1);
    }
    this.evalCount++;
    return this.values;
  }

  // hex representation of a bus
  busValue(base) {
    const bits = this.circuit.buses.get(base);
    if (!bits || !bits.length) return null;
    if (!bits.every((b) => this.values.has(b))) return null;
    let v = 0;
    for (const b of bits) v = (v << 1) | (this.values.get(b) ? 1 : 0);
    return `0x${v.toString(16).padStart(Math.ceil(bits.length / 4), '0')}`;
  }
}
