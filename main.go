package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
)

type Signal int

type Gate struct {
	NodeID     int
	Name       string
	Inputs     []Signal
	Output     Signal
	Outputs    []Signal // all output signals (chips with 1..n outputs)
	Parent     int      // 0 = top-level
	BusSources []int
}

type Circuit struct {
	Gates       []Gate
	nextID      int
	recording   bool
	parentStack []int // every scope (recorded or hidden abstraction)
	visStack    []int // innermost *recorded* chip scope; raw Nands attach here
	names       map[Signal]string
	clk         Signal
}

func NewCircuit() *Circuit {
	return &Circuit{
		recording: true,
		names:     make(map[Signal]string),
	}
}

func (c *Circuit) name(s Signal, n string) Signal {
	if n != "" {
		c.names[s] = n
	}
	return s
}

func (c *Circuit) alloc() Signal {
	s := Signal(c.nextID)
	c.nextID++
	return s
}

func (c *Circuit) alloc16(s Signal) [16]Signal {
	var bus [16]Signal
	for i := range 16 {
		bus[i] = s
	}
	return bus
}

func (c *Circuit) currentParent() int {
	if len(c.parentStack) == 0 {
		return 0
	}
	return c.parentStack[len(c.parentStack)-1]
}

// currentTopNode returns the node id of the innermost recorded scope (0 = none).
func (c *Circuit) currentTopNode() int {
	if len(c.visStack) == 0 {
		return 0
	}
	return c.visStack[len(c.visStack)-1]
}

func (c *Circuit) nand(a, b Signal) Signal {
	out := c.alloc()
	parent := 0
	if len(c.visStack) > 0 {
		parent = c.visStack[len(c.visStack)-1]
	}
	c.Gates = append(c.Gates, Gate{
		NodeID: int(out),
		Name:   "Nand",
		Inputs: []Signal{a, b},
		Output: out,
		Parent: parent,
	})
	return out
}

func (c *Circuit) quiet(fn func()) {
	old := c.recording
	c.recording = false
	fn()
	c.recording = old
}

// begin a hierarchical chip; returns the node ID to push.
// The node is recorded as a box only when recording is on, but the scope is
// always pushed so nested builder calls stay inside the chip.
func (c *Circuit) begin(name string, inputs []Signal) int {
	node := int(c.alloc())
	if c.recording {
		c.Gates = append(c.Gates, Gate{
			NodeID: node,
			Name:   name,
			Inputs: inputs,
			Parent: c.currentParent(),
		})
		c.visStack = append(c.visStack, node)
	}
	c.parentStack = append(c.parentStack, node)
	return node
}

// end closes the scope opened by begin(id) and attaches output signals.
func (c *Circuit) end(id int, outs ...Signal) {
	// pop parentStack back down to the begin id (should be the top entry)
	for len(c.parentStack) > 0 && c.parentStack[len(c.parentStack)-1] != id {
		c.parentStack = c.parentStack[:len(c.parentStack)-1]
	}
	if len(c.parentStack) > 0 && c.parentStack[len(c.parentStack)-1] == id {
		c.parentStack = c.parentStack[:len(c.parentStack)-1]
	}
	// pop the recorded-scope stack if it was pushed for this id
	if len(c.visStack) > 0 && c.visStack[len(c.visStack)-1] == id {
		c.visStack = c.visStack[:len(c.visStack)-1]
	}
	if len(outs) > 0 {
		for i := range c.Gates {
			if c.Gates[i].NodeID == id {
				c.Gates[i].Outputs = outs
				if len(outs) == 1 {
					c.Gates[i].Output = outs[0]
				}
				break
			}
		}
	}
}

func (c *Circuit) beginBus(name string, sources ...int) int {
	node := int(c.alloc())
	c.Gates = append(c.Gates, Gate{
		NodeID:     node,
		Name:       name,
		BusSources: sources,
		Parent:     c.currentParent(),
	})

	c.parentStack = append(c.parentStack, node)
	return node
}

func (c *Circuit) producerOf(s Signal) int {
	for i := len(c.Gates) - 1; i >= 0; i-- {
		g := c.Gates[i]
		if g.Output == s || (g.Name != "Nand" && contains(g, s)) {
			return g.NodeID
		}
	}
	return 0 // primary input
}

func contains(g Gate, s Signal) bool {
	for _, in := range g.Inputs {
		if in == s {
			return true
		}
	}
	return false
}

// ---------- Gates ----------

func (c *Circuit) Nand(a, b Signal) Signal { return c.nand(a, b) }

func (c *Circuit) Not(a Signal) Signal {
	id := c.begin("Not", []Signal{a})
	out := c.Nand(a, a) // primitive: expand Not to see its Nand
	c.end(id, out)
	return out
}

func (c *Circuit) Not16(a [16]Signal) (o [16]Signal) {
	id := c.begin("Not16", a[:])
	for i := range 16 {
		o[i] = c.Not(a[i])
	}
	c.end(id, o[:]...)
	return o
}
func (c *Circuit) And(a, b Signal) Signal {
	id := c.begin("And", []Signal{a, b})
	out := c.Not(c.Nand(a, b)) // guts: Nand + Not
	c.end(id, out)
	return out
}

func (c *Circuit) Or(a, b Signal) Signal {
	id := c.begin("Or", []Signal{a, b})
	out := c.Nand(c.Not(a), c.Not(b)) // guts: Not + Not + Nand
	c.end(id, out)
	return out
}

func (c *Circuit) Xor(a, b Signal) Signal {
	id := c.begin("Xor", []Signal{a, b})
	nb := c.Not(b)
	na := c.Not(a)
	out := c.Or(c.And(a, nb), c.And(b, na)) // guts: two Ands + two Nots + Or
	c.end(id, out)
	return out
}

func (c *Circuit) Xor16(a, b [16]Signal) (out [16]Signal) {
	ins := make([]Signal, 0, 32)
	ins = append(ins, a[:]...)
	ins = append(ins, b[:]...)
	id := c.begin("Xor16", ins)
	for i := range 16 {
		out[i] = c.Xor(a[i], b[i])
	}
	c.end(id, out[:]...)
	return out
}

func (c *Circuit) HalfAdder(a, b Signal) (sum, carry Signal) {
	id := c.begin("HalfAdder", []Signal{a, b})
	carry = c.name(c.And(a, b), "carry")
	sum = c.name(c.Xor(a, b), "sum")
	c.end(id, sum, carry)
	return sum, carry
}

func (c *Circuit) FullAdder(a, b, cin Signal) (sum, cout Signal) {
	id := c.begin("FullAdder", []Signal{a, b, cin})
	s1, c1 := c.HalfAdder(a, b)
	s2, c2 := c.HalfAdder(s1, cin)
	cout = c.name(c.Or(c1, c2), "cout")
	sum = c.name(s2, "sum")
	c.end(id, sum, cout)
	return sum, cout
}

// ---------- Evaluation ----------

func (c *Circuit) Evaluate(inputs map[Signal]int) map[Signal]int {
	vals := make(map[Signal]int)
	for k, v := range inputs {
		vals[k] = v
	}
	for _, g := range c.Gates {
		if g.Name != "Nand" {
			continue
		}
		a, b := vals[g.Inputs[0]], vals[g.Inputs[1]]
		if a == 1 && b == 1 {
			vals[g.Output] = 0
		} else {
			vals[g.Output] = 1
		}
	}
	return vals
}

// ---------- JSON export for the interactive viewer ----------

type NodeJSON struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	Parent    int    `json:"parent"`
	Inputs    []int  `json:"inputs,omitempty"`
	Outputs   []int  `json:"outputs,omitempty"`
	NandCount int    `json:"nandCount,omitempty"`
	Depth     int    `json:"depth,omitempty"`
	IsNand    bool   `json:"isNand,omitempty"`
}

type CircuitJSON struct {
	Name        string         `json:"name"`
	Nodes       []NodeJSON     `json:"nodes"`
	SignalNames map[int]string `json:"signalNames"`
	Outputs     []int          `json:"outputs"`
}

type BundleJSON struct {
	Circuits []CircuitJSON `json:"circuits"`
}

// ToJSON serializes the whole gate tree (including raw Nand probes) for the
// canvas viewer. Node order is preserved so the JS side can evaluate the
// netlist with a single in-order pass, just like Evaluate does.
func (c *Circuit) ToJSON(name string) CircuitJSON {
	// childrenOf over all gates (Nands included) for subtree stats
	childrenOf := map[int][]int{}
	byID := map[int]*Gate{}
	for i := range c.Gates {
		g := &c.Gates[i]
		byID[g.NodeID] = g
		childrenOf[g.Parent] = append(childrenOf[g.Parent], g.NodeID)
	}

	nandCount := map[int]int{}
	depth := map[int]int{}
	var walk func(id int) (int, int)
	walk = func(id int) (int, int) {
		if v, ok := nandCount[id]; ok {
			return v, depth[id]
		}
		nand, d := 0, 0
		for _, kid := range childrenOf[id] {
			kg := byID[kid]
			if isNandGate(kg) {
				nand++
			} else {
				cn, cd := walk(kid)
				nand += cn
				if cd > d {
					d = cd
				}
			}
		}
		d++
		if isNandGate(byID[id]) {
			// primitives report themselves as leaves
			nand, d = 1, 0
		}
		nandCount[id] = nand
		depth[id] = d
		return nand, d
	}
	for i := range c.Gates {
		walk(c.Gates[i].NodeID)
	}

	out := CircuitJSON{
		Name:        name,
		Nodes:       make([]NodeJSON, 0, len(c.Gates)),
		SignalNames: make(map[int]string, len(c.names)),
	}
	for i := range c.Gates {
		g := &c.Gates[i]
		outs := g.Outputs
		if outs == nil && g.Output != 0 {
			outs = []Signal{g.Output}
		}
		if isNandGate(g) {
			outs = []Signal{g.Output}
		}
		nj := NodeJSON{
			ID:        g.NodeID,
			Name:      g.Name,
			Parent:    g.Parent,
			NandCount: nandCount[g.NodeID],
			Depth:     depth[g.NodeID],
			IsNand:    isNandGate(g),
		}
		if len(g.Inputs) > 0 {
			ins := make([]int, len(g.Inputs))
			for j, s := range g.Inputs {
				ins[j] = int(s)
			}
			nj.Inputs = ins
		}
		if !nj.IsNand && len(outs) > 0 {
			os := make([]int, len(outs))
			for j, s := range outs {
				os[j] = int(s)
			}
			nj.Outputs = os
		}
		if nj.IsNand && g.Output != 0 {
			nj.Outputs = []int{int(g.Output)}
		}
		out.Nodes = append(out.Nodes, nj)
	}
	for sig, nm := range c.names {
		out.SignalNames[int(sig)] = nm
	}
	// root outputs: outputs of every top-level node
	for _, g := range c.Gates {
		if g.Parent == 0 && g.Name != "Nand" {
			for _, s := range g.Outputs {
				out.Outputs = append(out.Outputs, int(s))
			}
		}
	}
	return out
}

func isNandGate(g *Gate) bool { return g.Name == "Nand" }

// ---------- Graphviz with clusters ----------

func (c *Circuit) ToDOT(maxDepth, maxChildren int) string {
	var b strings.Builder
	b.WriteString("digraph Circuit {\n")
	b.WriteString("  rankdir=LR;\n")
	b.WriteString("  compound=true;\n")
	b.WriteString("  node [shape=box, style=filled, fillcolor=lightblue];\n")
	b.WriteString("  edge [arrowsize=0.7];\n")

	// parent → children (skip raw Nands)
	childrenOf := map[int][]Gate{}
	for _, g := range c.Gates {
		if g.Name == "Nand" {
			continue
		}
		childrenOf[g.Parent] = append(childrenOf[g.Parent], g)
	}

	// which nodes become clusters
	open := map[int]bool{}
	var consider func(id, depth int)
	consider = func(id, depth int) {
		if maxDepth >= 0 && depth > maxDepth {
			return
		}
		kids := childrenOf[id]
		if len(kids) == 0 || (maxChildren > 0 && len(kids) > maxChildren) {
			return
		}
		open[id] = true
		for _, k := range kids {
			consider(k.NodeID, depth+1)
		}
	}
	consider(0, 0)

	// emit clusters and leaf boxes
	var emit func(parent int, indent string)
	emit = func(parent int, indent string) {
		for _, g := range childrenOf[parent] {
			if open[g.NodeID] {
				b.WriteString(indent + "subgraph cluster_" + fmt.Sprint(g.NodeID) + " {\n")
				b.WriteString(indent + "  label=\"" + g.Name + "\";\n")
				b.WriteString(indent + "  style=filled;\n")
				b.WriteString(indent + "  fillcolor=lightyellow;\n")
				b.WriteString(indent + "  color=gray;\n")
				emit(g.NodeID, indent+"  ")
				b.WriteString(indent + "}\n")
			} else {
				b.WriteString(indent + "g" + fmt.Sprint(g.NodeID) + " [label=\"" + g.Name + "\"];\n")
			}
		}
	}
	emit(0, "  ")

	// nodes that are actually drawn as boxes
	emitted := map[int]bool{}
	var collect func(parent int)
	collect = func(parent int) {
		for _, g := range childrenOf[parent] {
			if open[g.NodeID] {
				collect(g.NodeID)
			} else {
				emitted[g.NodeID] = true
			}
		}
	}
	collect(0)

	// primary inputs
	produced := map[Signal]bool{}
	for _, g := range c.Gates {
		if g.Output != 0 {
			produced[g.Output] = true
		}
	}
	primary := map[Signal]bool{}
	for _, g := range c.Gates {
		if g.Name == "Nand" {
			continue
		}
		for _, in := range g.Inputs {
			if !produced[in] {
				primary[in] = true
			}
		}
	}
	for in := range primary {
		label := fmt.Sprintf("in_%d", in)
		if n, ok := c.names[in]; ok {
			label = n
		}
		b.WriteString(fmt.Sprintf("  in_%d [label=\"%s\", shape=ellipse, fillcolor=lightgreen];\n", in, label))
	}
	if len(primary) > 0 {
		b.WriteString("  { rank=source;")
		for in := range primary {
			b.WriteString(fmt.Sprintf(" in_%d;", in))
		}
		b.WriteString(" }\n")
	}

	// signal → gate that produces it
	producer := map[Signal]int{}
	for _, g := range c.Gates {
		if g.Output != 0 && g.Name != "Nand" {
			producer[g.Output] = g.NodeID
		}
	}

	// nearest emitted (drawn) ancestor of a node
	emittedOwner := func(id int) int {
		for id != 0 {
			if emitted[id] {
				return id
			}
			found := false
			for _, g := range c.Gates {
				if g.NodeID == id {
					id = g.Parent
					found = true
					break
				}
			}
			if !found {
				break
			}
		}
		return 0
	}

	// edges only between emitted nodes (deduplicated)
	type ekey struct{ src, dst string }
	seen := map[ekey]bool{}

	for _, g := range c.Gates {
		if !emitted[g.NodeID] || len(g.Inputs) == 0 {
			continue
		}
		dst := "g" + fmt.Sprint(g.NodeID)
		for _, in := range g.Inputs {
			var src string
			if pid, ok := producer[in]; ok {
				owner := emittedOwner(pid)
				if owner == 0 || owner == g.NodeID {
					continue
				}
				src = "g" + fmt.Sprint(owner)
			} else if primary[in] {
				src = "in_" + fmt.Sprint(in)
			} else {
				continue
			}
			k := ekey{src, dst}
			if seen[k] {
				continue
			}
			seen[k] = true
			b.WriteString(fmt.Sprintf("  %s -> %s;\n", src, dst))
		}
	}

	b.WriteString("}\n")
	return b.String()
}

// func main() {
// 	c := NewCircuit()
// 	a, b, cin := c.name(c.alloc(), "a"), c.name(c.alloc(), "b"), c.name(c.alloc(), "cin")
// 	sum, cout := c.FullAdder(a, b, cin)
// 	c.DebugParents()
//
// 	fmt.Println("sum", sum, "cout", cout)
//
// 	if err := writeDOTPNG("halfadder-d3", c.ToDOT(3, 200)); err != nil {
// 		fmt.Println("Error:", err)
// 	} else {
// 		fmt.Println("Wrote halfadder-d3.dot and halfadder-d3.png")
// 	}
//
// 	if err := writeDOTPNG("halfadder-d1", c.ToDOT(1, 200)); err != nil {
// 		fmt.Println("Error:", err)
// 	} else {
// 		fmt.Println("Wrote halfadder-d1.dot and halfadder-d1.png")
// 	}
//
// 	if err := writeDOTPNG("halfadder-d2", c.ToDOT(2, 200)); err != nil {
// 		fmt.Println("Error:", err)
// 	} else {
// 		fmt.Println("Wrote halfadder-d2.dot and halfadder-d2.png")
// 	}
//
// }

func writeDOTPNG(baseName, dot string) error {
	dotFile := baseName + ".dot"
	pngFile := baseName + ".png"

	if err := os.WriteFile(dotFile, []byte(dot), 0644); err != nil {
		return fmt.Errorf("write %s: %w", dotFile, err)
	}

	cmd := exec.Command("dot", "-Tpng", dotFile, "-o", pngFile)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("dot failed: %w\n%s", err, out)
	}
	return nil
}

// ---------- Sample circuits ----------

func buildHalfAdder() *Circuit {
	c := NewCircuit()
	a := c.name(c.alloc(), "a")
	b := c.name(c.alloc(), "b")
	_, _ = c.HalfAdder(a, b)
	return c
}

func buildFullAdder() *Circuit {
	c := NewCircuit()
	a := c.name(c.alloc(), "a")
	b := c.name(c.alloc(), "b")
	cin := c.name(c.alloc(), "cin")
	_, _ = c.FullAdder(a, b, cin)
	return c
}

func buildAdd16() *Circuit {
	c := NewCircuit()
	var a, b [16]Signal
	for i := range 16 {
		a[i] = c.name(c.alloc(), fmt.Sprintf("a[%d]", i))
		b[i] = c.name(c.alloc(), fmt.Sprintf("b[%d]", i))
	}
	sum, _ := c.Add16(a, b)
	for i := range sum {
		c.name(sum[i], fmt.Sprintf("sum[%d]", i))
	}
	return c
}

func buildMux16() *Circuit {
	c := NewCircuit()
	var a, b [16]Signal
	for i := range 16 {
		a[i] = c.name(c.alloc(), fmt.Sprintf("a[%d]", i))
		b[i] = c.name(c.alloc(), fmt.Sprintf("b[%d]", i))
	}
	sel := c.name(c.alloc(), "sel")
	out := c.Mux16(a, b, sel)
	for i := range out {
		c.name(out[i], fmt.Sprintf("out[%d]", i))
	}
	return c
}

func buildALU() *Circuit {
	c := NewCircuit()
	var x, y [16]Signal
	for i := 0; i < 16; i++ {
		x[i] = c.name(c.alloc(), fmt.Sprintf("x[%d]", i))
		y[i] = c.name(c.alloc(), fmt.Sprintf("y[%d]", i))
	}
	zx := c.name(c.alloc(), "zx")
	nx := c.name(c.alloc(), "nx")
	zy := c.name(c.alloc(), "zy")
	ny := c.name(c.alloc(), "ny")
	f := c.name(c.alloc(), "f")
	no := c.name(c.alloc(), "no")

	out := c.ALU(x, y, zx, nx, zy, ny, f, no)
	for i := range out {
		c.name(out[i], fmt.Sprintf("out[%d]", i))
	}
	return c
}

func buildDFF() *Circuit {
	c := NewCircuit()
	in := c.name(c.alloc(), "in")
	out := c.DFF(in)
	c.name(out, "out")
	return c
}

func buildRegister() *Circuit {
	c := NewCircuit()
	var in [16]Signal
	for i := range 16 {
		in[i] = c.name(c.alloc(), fmt.Sprintf("in[%d]", i))
	}
	load := c.name(c.alloc(), "load")
	out := c.Register16(in, load)
	for i := range out {
		c.name(out[i], fmt.Sprintf("out[%d]", i))
	}
	return c
}

func buildRAM8() *Circuit {
	c := NewCircuit()
	var in [16]Signal
	for i := range 16 {
		in[i] = c.name(c.alloc(), fmt.Sprintf("in[%d]", i))
	}
	load := c.name(c.alloc(), "load")
	var address [3]Signal
	for i := range 3 {
		address[i] = c.name(c.alloc(), fmt.Sprintf("address[%d]", i))
	}
	out := c.RAM8(in, load, address)
	for i := range out {
		c.name(out[i], fmt.Sprintf("out[%d]", i))
	}
	return c
}

func buildBundle() BundleJSON {
	type named struct {
		name string
		c    *Circuit
	}
	defs := []named{
		{"ALU", buildALU()},
		{"Add16", buildAdd16()},
		{"Mux16", buildMux16()},
		{"FullAdder", buildFullAdder()},
		{"HalfAdder", buildHalfAdder()},
		{"DFF", buildDFF()},
		{"Register", buildRegister()},
		{"RAM8", buildRAM8()},
	}
	bundle := BundleJSON{}
	for _, d := range defs {
		bundle.Circuits = append(bundle.Circuits, d.c.ToJSON(d.name))
	}
	return bundle
}

// ---------- Serve ----------

func serve(port int) {
	payload, err := json.MarshalIndent(buildBundle(), "", "  ")
	if err != nil {
		fmt.Println("Error marshaling circuits:", err)
		return
	}

	indexBytes, err := os.ReadFile("./viewer/index.html")
	if err != nil {
		fmt.Println("Error reading viewer/index.html:", err)
		return
	}
	injected := strings.Replace(string(indexBytes),
		"</head>",
		"<script>window.EMBEDDED_CIRCUITS = "+string(payload)+";</script>\n</head>",
		1)
	if injected == string(indexBytes) {
		fmt.Println("warning: could not inject circuit data into index.html")
	}

	http.HandleFunc("/circuit.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(payload)
	})
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			path := "./viewer" + r.URL.Path
			if _, err := os.Stat(path); err != nil {
				http.NotFound(w, r)
				return
			}
			http.ServeFile(w, r, path)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(injected))
	})

	addr := fmt.Sprintf("localhost:%d", port)
	fmt.Printf("circuit viewer: http://%s\n", addr)
	fmt.Printf("circuit data:   http://%s/circuit.json\n", addr)
	if err := http.ListenAndServe(addr, nil); err != nil {
		fmt.Println("Error:", err)
	}
}

func main() {
	dotMode := flag.Bool("dot", false, "legacy: write .dot/.png files instead of serving the viewer")
	port := flag.Int("port", 8081, "port for the viewer server")
	flag.Parse()

	if *dotMode {
		c := buildALU()
		if err := writeDOTPNG("alu-d0", c.ToDOT(0, 200)); err != nil {
			fmt.Println("Error:", err)
		} else {
			fmt.Println("Wrote alu-d0.dot and alu-d0.png")
		}
		if err := writeDOTPNG("alu-d1", c.ToDOT(1, 200)); err != nil {
			fmt.Println("Error:", err)
		} else {
			fmt.Println("Wrote alu-d1.dot and alu-d1.png")
		}
		if err := writeDOTPNG("alu-d2", c.ToDOT(2, 200)); err != nil {
			fmt.Println("Error:", err)
		} else {
			fmt.Println("Wrote alu-d2.dot and alu-d2.png")
		}
		return
	}

	serve(*port)
}

func (c *Circuit) DebugParents() {
	fmt.Println("NodeID  Name        Parent")
	for _, g := range c.Gates {
		if g.Name == "Nand" {
			continue
		}
		fmt.Printf("%-6d  %-10s  %d\n", g.NodeID, g.Name, g.Parent)
	}
}

// 1-bit register
type Bit struct {
	In   int
	Load int
	Out  int
}

// func (c *Circuit) HalfAdder(a, b Signal) (sum, carry Signal) {
// 	//fmt.Println("Half Adder", a, b)
// 	carry = c.And(a, b)
// 	sum = c.Xor(a, b)
// 	return sum, carry
// }
//
// func (c *Circuit) Xor(a, b Signal) Signal {
// 	var out Signal
// 	c.quiet(func() {
// 		out = c.Or(
// 			c.And(a, c.Not(b)),
// 			c.And(b, c.Not(a)),
// 		)
// 	})
// 	return c.chip("Xor", []Signal{a, b}, out)
// }
//
// // // Full Adder
// func (c *Circuit) FullAdder(a, b, cin Signal) (sum, cout Signal) {
// 	var s, co Signal
// 	c.quiet(func() {
// 		// two half-adders + or for carry
// 		s1, c1 := c.HalfAdder(a, b)
// 		s2, c2 := c.HalfAdder(s1, cin)
// 		co = c.Or(c1, c2)
// 		s = s2
// 	})
// 	sum = c.chip("Sum", []Signal{a, b, cin}, s)
// 	cout = c.chip("Cout", []Signal{a, b, cin}, co)
// 	return sum, cout
// }

func (c *Circuit) Add16(a, b [16]Signal) (out, carry [16]Signal) {
	ins := make([]Signal, 0, 32)
	ins = append(ins, a[:]...)
	ins = append(ins, b[:]...)
	id := c.begin("Add16", ins)
	out = [16]Signal{}
	carry = [16]Signal{}

	// Hack buses are MSB-first (x[0] = most significant bit), so the carry
	// chain runs from the least significant bit (index 15) toward index 0.
	cin0 := c.alloc() // un-driven signal = constant 0
	s1, c1 := c.FullAdder(a[15], b[15], cin0)
	out[15] = s1
	carry[15] = c1
	for i := 14; i >= 0; i-- {
		s1, c1 = c.FullAdder(a[i], b[i], c1)
		out[i] = s1
		carry[i] = c1
	}
	c.end(id, out[:]...)
	return out, carry
}

func (c *Circuit) DMux(in, sel Signal) (a, b Signal) {
	id := c.begin("DMux", []Signal{in, sel})
	a = c.And(in, c.Not(sel))
	b = c.And(in, sel)
	c.end(id, a, b)
	return a, b
}

func (c *Circuit) DMux4Way(in Signal, sel [2]Signal) (a, b, c_, d Signal) {
	// s1,s0||a,b,c,d
	// 0,0 || a,0,0,0
	// 0,1 || 0,b,0,0
	// 1,0 || 0,0,c,0
	// 1,1 || 0,0,0,d
	// ----------
	// 2xDmux
	// out=a||b
	// Not(sel[1]) so that sel1=1
	ab := c.And(c.Not(sel[1]), in)
	a, b = c.DMux(ab, sel[0])
	// out=c||d
	cd := c.And(sel[1], in)
	c_, d = c.DMux(cd, sel[0])

	return a, b, c_, d
}

func (c *Circuit) DMux8Way(in Signal, sel [3]Signal) (a, b, c_, d, e, f, g, h Signal) {

	// ns := Not(sel[2])
	sel2 := [2]Signal{sel[0], sel[1]}
	a, b, c_, d = c.DMux4Way(in, sel2)
	e, f, g, h = c.DMux4Way(in, sel2)

	a = c.And(a, c.Not(sel[2]))
	b = c.And(b, c.Not(sel[2]))
	c_ = c.And(c_, c.Not(sel[2]))
	d = c.And(d, c.Not(sel[2]))
	e = c.And(e, sel[2])
	f = c.And(f, sel[2])
	g = c.And(g, sel[2])
	h = c.And(h, sel[2])

	return a, b, c_, d, e, f, g, h
}

func (c *Circuit) And16(a, b [16]Signal) (o [16]Signal) {
	// flatten the two buses into the Inputs slice for the diagram
	ins := make([]Signal, 0, 32)
	ins = append(ins, a[:]...)
	ins = append(ins, b[:]...)
	id := c.begin("And16", ins)

	for i := range 16 {
		o[i] = c.And(a[i], b[i])
	}
	c.end(id, o[:]...)
	return o
}

func (c *Circuit) Mux(a, b, sel Signal) Signal {
	id := c.begin("Mux", []Signal{a, b, sel})
	nsel := c.Not(sel) // guts: Not + two Ands + Or
	selA := c.And(a, nsel)
	selB := c.And(b, sel)
	out := c.Or(selA, selB)
	c.end(id, out)
	return out
}

func (c *Circuit) Mux16(a, b [16]Signal, sel Signal) (o [16]Signal) {
	ins := make([]Signal, 0, 33)
	ins = append(ins, a[:]...)
	ins = append(ins, b[:]...)
	ins = append(ins, sel)
	id := c.begin("Mux16", ins)
	for i := range 16 {
		o[i] = c.Mux(a[i], b[i], sel)
	}
	c.end(id, o[:]...)
	return o
}

func (c *Circuit) Mux8Way16(a, b, c_, d, e, f, g, h [16]Signal, sel [3]Signal) (out [16]Signal) {
	ins := make([]Signal, 0, 128+3)
	ins = append(ins, a[:]...)
	ins = append(ins, b[:]...)
	ins = append(ins, c_[:]...)
	ins = append(ins, d[:]...)
	ins = append(ins, e[:]...)
	ins = append(ins, f[:]...)
	ins = append(ins, g[:]...)
	ins = append(ins, h[:]...)
	ins = append(ins, sel[:]...)
	id := c.begin("Mux8Way16", ins)

	ab := c.Mux16(a, b, sel[0])
	cd := c.Mux16(c_, d, sel[0])
	ef := c.Mux16(e, f, sel[0])
	gh := c.Mux16(g, h, sel[0])
	abcd := c.Mux16(ab, cd, sel[1])
	efgh := c.Mux16(ef, gh, sel[1])

	out = c.Mux16(abcd, efgh, sel[2])
	c.end(id, out[:]...)
	return out
}

// ALU performs various arithmetic and logic operations based on control flags.
func (c *Circuit) ALU(x, y [16]Signal, zx, nx, zy, ny, f, no Signal) (out [16]Signal) {
	ins := make([]Signal, 0, 38)
	ins = append(ins, x[:]...)
	ins = append(ins, y[:]...)
	ins = append(ins, zx, nx, zy, ny, f, no)

	c.begin("ALU", ins)
	aluID := c.currentTopNode()

	// Zero x/y
	zx16 := c.alloc16(zx)
	zy16 := c.alloc16(zy)
	notZx := c.Not16(zx16)
	xZero := c.And16(x, notZx)
	yZero := c.And16(y, c.Not16(zy16))

	// Negate x/y
	nx16 := c.alloc16(nx)
	ny16 := c.alloc16(ny)
	xOut := c.Xor16(xZero, nx16)
	yOut := c.Xor16(yZero, ny16)

	//  add or and
	sum, _ := c.Add16(xOut, yOut)
	and := c.And16(xOut, yOut)
	fOut := c.Mux16(and, sum, f)

	// negate the output
	no16 := c.alloc16(no)
	out = c.Xor16(fOut, no16)

	c.end(aluID, out[:]...)
	return out
}

//

func (c *Circuit) Clock() Signal {
	if c.clk == 0 {
		c.clk = c.name(c.alloc(), "clk")
	}
	return c.clk
}

func (c *Circuit) DLatch(d, clk Signal) (q, qn Signal) {
	id := c.begin("DLatch", []Signal{d, clk})

	notD := c.Not(d)
	s := c.Nand(d, clk)    // active-low set
	r := c.Nand(notD, clk) // active-low reset

	// Cross-coupled NAND pair (feedback).
	// Pre-allocate the two output signals so we can close the loop,
	// then emit the two Nand gates directly (same parent as a normal Nand).
	q = c.alloc()
	qn = c.alloc()
	parent := c.currentTopNode()

	c.Gates = append(c.Gates, Gate{
		NodeID: int(q),
		Name:   "Nand",
		Inputs: []Signal{s, qn},
		Output: q,
		Parent: parent,
	})
	c.Gates = append(c.Gates, Gate{
		NodeID: int(qn),
		Name:   "Nand",
		Inputs: []Signal{r, q},
		Output: qn,
		Parent: parent,
	})

	c.end(id, q, qn)
	return q, qn
}

func (c *Circuit) DFF(in Signal) Signal {
	id := c.begin("DFF", []Signal{in})

	clk := c.Clock()
	notClk := c.Not(clk)

	qm, _ := c.DLatch(in, notClk)
	q, _ := c.DLatch(qm, clk)

	c.end(id, q)

	return q
}

func (c *Circuit) Bit(in, load Signal) Signal {
	id := c.begin("Bit", []Signal{in, load})

	out := c.alloc()
	muxOut := c.Mux(out, in, load)
	dffOut := c.DFF(muxOut)

	c.end(id, dffOut)

	return dffOut
}

func (c *Circuit) Register16(in [16]Signal, load Signal) (out [16]Signal) {
	ins := make([]Signal, 0, 17)
	ins = append(ins, in[:]...)
	ins = append(ins, load)
	id := c.begin("Register", ins)

	for i := range 16 {
		out[i] = c.Bit(in[i], load)
	}

	c.end(id, out[:]...)
	return out
}

func (c *Circuit) RAM8(in [16]Signal, load Signal, address [3]Signal) (out [16]Signal) {
	// address_width = log2(n)
	// RAMn in[16], address[k], load
	// out[16]
	// out(t) = RAM[address(t-1)](t)
	ins := make([]Signal, 0, 16+1+3)
	ins = append(ins, in[:]...)
	ins = append(ins, load)
	ins = append(ins, address[:]...)
	id := c.begin("RAM8", ins)

	var la, lb, lc, ld, le, lf, lg, lh = c.DMux8Way(load, address)

	var ra, rb, rc, rd, re, rf, rg, rh [16]Signal

	ra = c.Register16(in, la)
	rb = c.Register16(in, lb)
	rc = c.Register16(in, lc)
	rd = c.Register16(in, ld)
	re = c.Register16(in, le)
	rf = c.Register16(in, lf)
	rg = c.Register16(in, lg)
	rh = c.Register16(in, lh)

	out = c.Mux8Way16(ra, rb, rc, rd, re, rf, rg, rh, address)

	c.end(id, out[:]...)

	return out
}

//
/*
func run_alu() {

	var out = []int{0}
	var x = []int{1}
	var y = []int{1}
	// var x = []int{1}
	// var y = []int{1}
	// var x = []int{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	// // var y = []int{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1}        // 16 bit
	zx := []int{1, 1, 1, 0, 1, 0, 1, 0, 1, 0, 1, 0, 1, 0, 0, 0, 0, 0} // 18
	var nx = []int{0, 1, 1, 0, 1, 0, 1, 0, 1, 1, 1, 0, 1, 0, 1, 0, 0, 1}
	var zy = []int{1, 1, 1, 1, 0, 1, 0, 1, 0, 1, 0, 1, 0, 0, 0, 0, 0, 0}
	var ny = []int{0, 1, 0, 1, 0, 1, 0, 1, 0, 1, 1, 1, 0, 0, 0, 1, 0, 1}
	var f = []int{1, 1, 1, 0, 0, 0, 0, 1, 1, 1, 1, 1, 1, 1, 1, 1, 0, 0}
	var no = []int{0, 1, 0, 0, 0, 1, 1, 1, 1, 1, 1, 0, 0, 0, 1, 1, 0, 1}

	for i := range zx {
		// fmt.Println(x, y, "|", zx[i], nx[i], zy[i], ny[i], f[i], no[i], "|")

		x[0] = 1
		fmt.Println(x, y, "|", a, b, "|", zx[i], nx[i], zy[i], ny[i], f[i], no[i], "|", out)
		out = ALU(x, y, zx[i], nx[i], zy[i], ny[i], f[i], no[i])

		fmt.Println("\n")
		fmt.Println(x, y, "|", a, b, "|", zx[i], nx[i], zy[i], ny[i], f[i], no[i], "|", out)
		x[0] = 1
	}
}
*/

// type int uint8

// func And(x, y int) (o int) {
// 	count++
//
// 	tree = append(tree, "And["+strconv.Itoa(count))
// 	a := Nand(x, y)
// 	b := Not(a)
//
// 	tree = append(tree, "]")
// 	return b
// 	// return Not(Nand(x, y))
// }
//
// func Or(x, y int) (o int) {
// 	count++
// 	tree = append(tree, "Or["+strconv.Itoa(count))
// 	// return Not(And(Not(x), Not(y)))
// 	a := Not(x)
// 	b := Not(y)
// 	c := Nand(a, b)
// 	tree = append(tree, "]")
// 	return c
// 	// return Nand(Not(x), Not(y))
// }
//
// func Not(x int) (o int) {
// 	count++
// 	tree = append(tree, "Not["+strconv.Itoa(count))
// 	c := Nand(x, x)
// 	tree = append(tree, "]")
// 	return c
// 	// return Nand(x, x)
// }
//
// func Not16(x [16]int) []int {
//
// 	var output []int
// 	for i := range x {
// 		output = append(output, Nand(x[i], x[i]))
// 	}
//
// 	return output
// }
//
// func Nand(in_1, in_2 int) (out int) {
// 	count++
// 	tree = append(tree, "Nand["+strconv.Itoa(count))
// 	if in_1 == 1 && in_2 == 1 {
// 		tree = append(tree, "]")
// 		return 0
// 	}
// 	tree = append(tree, "]")
// 	return 1
// 	// return !(a && b)
// }
//
// func test_Nand() {
// }
//
// func Xor(a, b int) (o int) {
// 	return Or(And(a, Not(b)), And(b, Not(a)))
// }
//
// func Xor16(a, b [16]int) []int {
//
// 	var xor16 []int
// 	for i := range a {
// 		xor16 = append(xor16, Xor(a[i], b[i]))
// 	}
//
// 	return xor16
//
// }

// TODO Add n-bit adder/xor/or/nand, etc
// func Adder3Way(a, b, c int) (o int) {
// 	ab := And(a, b)
// 	d := And(ab, c)
// 	return d
// }
// func Xor3way(a, b, c int) (o int) {
// 	ab := Xor(a, b)
// 	d := Xor(ab, c)
// 	return d
// }

// func test_Mux() {
// 	fmt.Println("\nMux")
// 	a := []int{0, 0, 1, 1, 0, 0, 1, 1}
// 	b := []int{0, 1, 0, 1, 0, 1, 0, 1}
// 	sel := []int{0, 0, 0, 0, 1, 1, 1, 1}
//
// 	for i := range sel {
// 		fmt.Println(a[i], b[i], sel[i], "|", Mux(a[i], b[i], sel[i]))
// 	}
// }

// func AndMuxOr(a, b, sel int) (o int) {
// 	if sel == 0 {
// 		return And(a, b)
// 	}
// 	return Or(a, b)
// }
//
//
// func Mux4Way16(a, b, c, d [16]int, sel [2]int) [16]int {
// 	// in = a,b,c,d, sel_1, sel_2
// 	// sel_1 | sel_2 || out
// 	// 0|0||a
// 	// 0|1||b
// 	// 1|0||c
// 	// 1|1||d
//
// 	r1 := Mux16(a, b, sel[0])
// 	r2 := Mux16(c, d, sel[0])
// 	return Mux16(r1, r2, sel[1])
// }

// func test_DMux() {
// 	fmt.Println("\nDMux")
// 	sel := []int{0, 1}
// 	for i := range sel {
// 		a, b := DMux(1, sel[i])
// 		fmt.Println(sel[i], "|", a, b)
// 	}
//
// }
//
// func DMux4Way(in int, sel [2]int) (a, b, c, d int) {
// 	// s1,s0||a,b,c,d
// 	// 0,0 || a,0,0,0
// 	// 0,1 || 0,b,0,0
// 	// 1,0 || 0,0,c,0
// 	// 1,1 || 0,0,0,d
// 	// ----------
// 	// 2xDmux
// 	// out=a||b
// 	// Not(sel[1]) so that sel1=1
// 	ab := And(Not(sel[1]), in)
// 	a, b = DMux(ab, sel[0])
// 	// out=c||d
// 	cd := And(sel[1], in)
// 	c, d = DMux(cd, sel[0])
//
// 	return a, b, c, d
// }
//
// func test_DMX4W() {
// 	fmt.Println("\nDMux4Way")
// 	sel1 := []int{
// 		0, 0, 1, 1,
// 	}
// 	sel0 := []int{
// 		0, 1, 0, 1,
// 	}
// 	// a := 1
// 	for i := range sel1 {
// 		sel := [2]int{sel0[i], sel1[i]}
// 		a, b, c, d := DMux4Way(1, sel)
// 		fmt.Println(sel, a, b, c, d)
// 	}
// }
//
// func DMux8Way(in int, sel [3]int) (a, b, c, d, e, f, g, h int) {
//
// 	// ns := Not(sel[2])
// 	sel2 := [2]int{sel[0], sel[1]}
// 	a, b, c, d = DMux4Way(in, sel2)
// 	e, f, g, h = DMux4Way(in, sel2)
//
// 	a = And(a, Not(sel[2]))
// 	b = And(b, Not(sel[2]))
// 	c = And(c, Not(sel[2]))
// 	d = And(d, Not(sel[2]))
// 	e = And(e, sel[2])
// 	f = And(f, sel[2])
// 	g = And(g, sel[2])
// 	h = And(h, sel[2])
//
// 	return a, b, c, d, e, f, g, h
// }
//
// func test_DMX8W() {
// 	fmt.Println("\nDMux8Way")
// 	// 3,2,1 ||a,b,c,d,e,f,g,h
// 	// ----------------------- -> s2 = [sel0, sel1]
// 	// 0,0,0 ||i,0,0,0,0,0,0,0 | x,0,0
// 	// 0,0,1 ||0,i,0,0,0,0,0,0 | x,0,1
// 	// 0,1,0 ||0,0,i,0,0,0,0,0 | x,1,0
// 	// 0,1,1 ||0,0,0,i,0,0,0,0 | x.1,1
// 	// -----------------------
// 	// 1,0,0 ||0,0,0,0,i,0,0,0
// 	// 1,0,1 ||0,0,0,0,0,i,0,0
// 	// 1,1,0 ||0,0,0,0,0,0,i,0
// 	// 1,1,1 ||0,0,0,0,0,0,0,i
// 	// -----------------------
//
// 	in := 1
// 	sel2 := []int{0, 0, 0, 0, 1, 1, 1, 1}
// 	sel1 := []int{0, 0, 1, 1, 0, 0, 1, 1}
// 	sel0 := []int{0, 1, 0, 1, 0, 1, 0, 1}
// 	// a,b,c,d := DMux4Way()
//
// 	for i := range sel0 {
// 		a, b, c, d, e, f, g, h := DMux8Way(in, [3]int{sel0[i], sel1[i], sel2[i]})
// 		fmt.Println(sel0[i], sel1[i], sel2[i], "||", a, b, c, d, e, f, g, h)
// 	}
// }
//
// -----------------------------------------
// ADDERs
// -----------------------------------------

// func HalfAdder(a, b int) (sum, carry int) {
// 	//fmt.Println("Half Adder", a, b)
//
// 	carry = And(a, b)
// 	sum = Xor(a, b)
//
// 	return sum, carry
// }

// func test_halfadder() {
// 	x := []int{0, 0, 1, 1}
// 	y := []int{0, 1, 0, 1}
//
// 	fmt.Println("x|y|c|s\n-------")
// 	for i := range x {
// 		s, c := HalfAdder(x[i], y[i])
// 		fmt.Printf("%d|%d|%d|%d\n", x[i], y[i], c, s)
// 	}
// }

// Full Adder
// func FullAdder(a, b, c int) (sum, carry int) {
// 	s1, c1 := HalfAdder(a, b)
// 	s2, c2 := HalfAdder(s1, c)
// 	carry = Xor(c1, c2)
// 	sum = s2
//
// 	return sum, carry
// }
//
// func test_fulladder() {
// 	fmt.Println("Test Full Adder")
// 	x := []int{0, 0, 0, 0, 1, 1, 1, 1}
// 	y := []int{0, 0, 1, 1, 0, 0, 1, 1}
// 	z := []int{0, 1, 0, 1, 0, 1, 0, 1}
// 	carry := []int{0, 0, 0, 0, 0, 0, 0, 0}
// 	sum := []int{0, 0, 0, 0, 0, 0, 0, 0, 0}
//
// 	fmt.Println("x|y|z||c|s")
// 	fmt.Println("--------------")
//
// 	for i := range x {
// 		s, c := FullAdder(x[i], y[i], z[i])
// 		sum[i] = s
// 		carry[i] = c
// 		fmt.Printf("%d|%d|%d||%d|%d\n", x[i], y[i], z[i], c, s)
// 	}
//
// 	fmt.Println("---------------------")
// 	fmt.Println("c= ", reverseSlice(carry), "\nx= ", reverseSlice(x), "\ny= ", reverseSlice(y), "\nz= ", reverseSlice(z), "\n--------------------------", "\ns= ", reverseSlice(sum))
// }

//
// func test_Add16() {
// 	fmt.Println("Test Add16")
// 	a := [16]int{1, 1, 0, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
// 	b := [16]int{0, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
//
// 	sum, carry := Add16(a, b)
// 	fmt.Println("c= ", carry, "\na= ", a, "\nb= ", b, "\n----------------------------\ns= ", sum)
// }

// func DFF(d, c int) {
//
// 	nand1 := Nand(d, Not(c))
// 	nand2 := Nand(nand1, Not(c))
//
// 	nand3 := Nand(nand1, nand4)
// 	nand4 := Nand(nand3, nand2)
//
// 	nand5 := Nand(nand3, c)
// 	nand6 := Nand(nand5, c)
//
// 	nand7 := Nand(nand4, nand8)
// 	nand8 := Nand(nand7, nand6)
//
// 	fmt.Println(d, c, nand7, nand8)
// }

//func Register(in, load int) (out in) {
//}
/*
func Incrementer(a [16]int) (out [16]int) {
	b := make([16]int, len(a))
	b[0] = 1
	c := Adder16(a, b)

	fmt.Println()
	fmt.Println(reverseSlice(a))
	fmt.Println(reverseSlice(b))
	fmt.Println("----------------------")
	fmt.Println(reverseSlice(c))
	return c

}
*/
