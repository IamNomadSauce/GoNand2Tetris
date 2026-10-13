package main

import (
	"fmt"
	"os"
	"strings"
)

type Signal int

type Gate struct {
	NodeID     int
	Name       string
	Inputs     []Signal
	Output     Signal
	Parent     int // 0 = top-level
	BusSources []int
}

type Circuit struct {
	Gates       []Gate
	nextID      int
	recording   bool
	parentStack []int
}

// type Detail int
//
// const (
// 	DetailTop Detail = iota
// 	DetailMid
// 	DetailFull
// )

func NewCircuit() *Circuit {
	return &Circuit{recording: true}
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

func (c *Circuit) nand(a, b Signal) Signal {
	out := c.alloc()
	c.Gates = append(c.Gates, Gate{
		NodeID: int(out),
		Name:   "Nand",
		Inputs: []Signal{a, b},
		Output: out,
		Parent: c.currentParent(),
	})
	return out
}

func (c *Circuit) chip(name string, inputs []Signal, out Signal) Signal {
	if !c.recording {
		return out
	}
	node := int(c.alloc())
	c.Gates = append(c.Gates, Gate{
		NodeID: node,
		Name:   name,
		Inputs: inputs,
		Output: out,
		Parent: c.currentParent(),
	})
	return out
}

func (c *Circuit) quiet(fn func()) {
	old := c.recording
	c.recording = false
	fn()
	c.recording = old
}

// begin a hierarchical chip; returns the node ID to push
func (c *Circuit) begin(name string, inputs []Signal) int {
	node := int(c.alloc())
	c.Gates = append(c.Gates, Gate{
		NodeID: node,
		Name:   name,
		Inputs: inputs,
		Parent: c.currentParent(),
	})
	c.parentStack = append(c.parentStack, node)
	return node
}

func (c *Circuit) end() {
	if len(c.parentStack) > 0 {
		c.parentStack = c.parentStack[:len(c.parentStack)-1]
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
	var out Signal
	c.quiet(func() { out = c.Nand(a, a) })
	return c.chip("Not", []Signal{a}, out)
}

func (c *Circuit) Not16(a [16]Signal) (o [16]Signal) {
	c.begin("Not16", nil)
	for i := range 16 {
		o[i] = c.Not(a[i])
	}
	c.end()
	return o
}
func (c *Circuit) And(a, b Signal) Signal {
	c.begin("And", nil)
	var out Signal
	c.quiet(func() { out = c.Not(c.Nand(a, b)) })
	c.end()
	return c.chip("And", []Signal{a, b}, out)
}

func (c *Circuit) Or(a, b Signal) Signal {
	c.begin("Or", nil)
	var out Signal
	c.quiet(func() { out = c.Nand(c.Not(a), c.Not(b)) })
	c.end()
	return c.chip("Or", []Signal{a, b}, out)
}

func (c *Circuit) Xor(a, b Signal) Signal {
	var out Signal
	c.quiet(func() {
		out = c.Or(c.And(a, c.Not(b)), c.And(b, c.Not(a)))
	})
	return c.chip("Xor", []Signal{a, b}, out)
}

func (c *Circuit) Xor16(a, b [16]Signal) (out [16]Signal) {
	// ins := make([]Signal, 0, 32)
	// ins = append(ins, a[:]...)
	// ins = append(ins, b[:]...)
	c.begin("Xor16", nil)
	for i := range 16 {
		out[i] = c.Xor(a[i], b[i])
	}
	c.end()
	return out
}

func (c *Circuit) HalfAdder(a, b Signal) (sum, carry Signal) {
	c.begin("HalfAdder", []Signal{a, b})
	carry = c.And(a, b)
	sum = c.Xor(a, b)
	c.end()
	return sum, carry
}

func (c *Circuit) FullAdder(a, b, cin Signal) (sum, cout Signal) {
	c.begin("FullAdder", []Signal{a, b, cin})
	s1, c1 := c.HalfAdder(a, b)
	s2, c2 := c.HalfAdder(s1, cin)
	cout = c.Or(c1, c2)
	sum = s2
	c.end()
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

// ---------- Graphviz with clusters ----------

func (c *Circuit) ToDOT(maxDepth, maxChildren int) string {
	var b strings.Builder
	b.WriteString("digraph Circuit {\n")
	b.WriteString("  rankdir=LR;\n")
	b.WriteString("  compound=true;\n")
	b.WriteString("  node [shape=box, style=filled, fillcolor=lightblue];\n")

	childrenOf := map[int][]Gate{}
	for _, g := range c.Gates {
		if g.Name == "Nand" {
			continue
		}
		childrenOf[g.Parent] = append(childrenOf[g.Parent], g)
	}

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

	produced := map[Signal]bool{}
	for _, g := range c.Gates {
		if g.Output != 0 {
			produced[g.Output] = true
		}
	}

	primary := map[Signal]bool{}
	for _, g := range c.Gates {
		if !emitted[g.NodeID] {
			continue
		}
		for _, in := range g.Inputs {
			if !produced[in] {
				primary[in] = true
			}
		}
	}
	for in := range primary {
		b.WriteString("  in_" + fmt.Sprint(in) + " [label=\"in_" + fmt.Sprint(in) + "\", shape=ellipse, fillcolor=lightgreen];\n")
	}

	// Force primary inputs to the left
	if len(primary) > 0 {
		b.WriteString("  { rank=source;")
		for in := range primary {
			b.WriteString(" in_" + fmt.Sprint(in) + ";")
		}
		b.WriteString(" }\n")
	}

	// Edges
	producer := map[Signal]int{}
	for _, g := range c.Gates {
		if g.Name == "Nand" || g.Output == 0 {
			continue
		}
		// Prefer an emitted leaf; otherwise walk up to the nearest open container
		if emitted[g.NodeID] {
			producer[g.Output] = g.NodeID
			continue
		}
		// find nearest visible ancestor
		anc := g.Parent
		for anc != 0 {
			if open[anc] || emitted[anc] {
				producer[g.Output] = anc
				break
			}
			// move up
			found := false
			for _, pg := range c.Gates {
				if pg.NodeID == anc {
					anc = pg.Parent
					found = true
					break
				}
			}
			if !found {
				break
			}
		}
	}

	// For each collapsed node, collect the signals that truly come from outside it
	externalIn := map[int]map[Signal]bool{}
	for _, g := range c.Gates {
		if g.Name == "Nand" || !emitted[g.NodeID] {
			continue
		}
		// walk every gate that belongs to this component (Parent chain reaches g.NodeID)
		// and look at its inputs
		var walk func(id int)
		walk = func(id int) {
			for _, child := range c.Gates {
				if child.Parent != id {
					continue
				}
				for _, in := range child.Inputs {
					prod, ok := producer[in]
					// signal is external if its visible producer is not this node
					// (and not a descendant — but producer already collapsed descendants)
					if !ok || prod != g.NodeID {
						if externalIn[g.NodeID] == nil {
							externalIn[g.NodeID] = map[Signal]bool{}
						}
						externalIn[g.NodeID][in] = true
					}
				}
				walk(child.NodeID)
			}
		}
		walk(g.NodeID)
	}

	type edgeKey struct{ src, dst string }
	seen := map[edgeKey]struct{}{}

	emitEdge := func(src, dst string) {
		k := edgeKey{src, dst}
		if _, ok := seen[k]; ok {
			return
		}
		seen[k] = struct{}{}
		b.WriteString(fmt.Sprintf("  %s -> %s;\n", src, dst))
	}

	// Edges into collapsed chips (from the externalIn map you built)
	for nodeID, sigs := range externalIn {
		dst := "g" + fmt.Sprint(nodeID)
		for in := range sigs {
			var src string
			if nid, ok := producer[in]; ok && nid != nodeID {
				src = "g" + fmt.Sprint(nid)
			} else if primary[in] {
				src = "in_" + fmt.Sprint(in)
			} else {
				continue
			}
			emitEdge(src, dst)
		}
	}

	b.WriteString("}\n")
	return b.String()
}

func main() {
	c := NewCircuit()

	var x, y [16]Signal
	for i := 0; i < 16; i++ {
		x[i] = c.alloc()
		y[i] = c.alloc()
	}
	zx := c.alloc()
	nx := c.alloc()
	zy := c.alloc()
	ny := c.alloc()
	f := c.alloc()
	no := c.alloc()

	_ = c.ALU(x, y, zx, nx, zy, ny, f, no)
	aluID := 0
	for _, g := range c.Gates {
		if g.Name == "ALU" {
			aluID = g.NodeID
			break
		}
	}
	count := 0
	for _, g := range c.Gates {
		if g.Parent == aluID && g.Name != "Nand" {
			count++
		}
	}
	fmt.Println("ALU direct children:", count)

	// low-detail overview
	// os.WriteFile("alu-d0.dot", []byte(c.ToDOT(0, 30)), 0644)
	os.WriteFile("alu-d1.dot", []byte(c.ToDOT(1, 200)), 0644)
	os.WriteFile("alu-d2.dot", []byte(c.ToDOT(2, 200)), 0644)

	fmt.Println("Wrote alu-d1.dot")
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
	// ins := make([]Signal, 0, 32)
	// ins = append(ins, a[:]...)
	// ins = append(ins, b[:]...)
	c.begin("Add16", nil)
	sum := [16]Signal{}
	carry = [16]Signal{}

	s1, c1 := c.FullAdder(a[0], b[0], 0)
	sum[0] = s1

	carry[0] = c1
	for i := range a {
		if i > 0 {
			s1, c1 = c.FullAdder(a[i], b[i], c1)
			sum[i] = s1
			carry[i] = c1
		}
	}
	c.end()
	return sum, carry
}

func (c *Circuit) DMux(in, sel Signal) (a, b Signal) {
	// s||a|b || (i = (in))
	// --------
	// 0||i|0
	// 1||0|i
	// --------
	// 0||0|0
	// 0||1|0
	// 1||0|0
	// 1||0|1

	// If sel == 0 -> out = (in, 0)
	// out = in , 0
	// if in = 0 -> 0||0|0
	// if in = 1 -> 0||1,0 -> And(in, Not(sel))

	// if sel == 1 -> out = (0, in)
	// out = 0 , in
	// if in = 0 -> 1||0|0
	// if in = 1 -> 1||0|1 -> And(in, sel)

	a = c.And(in, c.Not(sel))
	b = c.And(in, sel)
	return a, b
}

func (c *Circuit) And16(a, b [16]Signal) (o [16]Signal) {
	// flatten the two buses into the Inputs slice for the diagram
	// ins := make([]Signal, 0, 32)
	// ins = append(ins, a[:]...)
	// ins = append(ins, b[:]...)
	c.begin("And16", nil)

	for i := range 16 {
		o[i] = c.And(a[i], b[i])
	}
	c.end()
	return o
}

func (c *Circuit) Mux(a, b, sel Signal) Signal {
	c.begin("Mux", nil)
	var out Signal
	c.quiet(func() {
		selA := c.And(a, c.Not(sel))
		selB := c.And(b, sel)
		out = c.Or(selA, selB)
	})
	c.end()
	return c.chip("Mux", []Signal{a, b, sel}, out)
}

func (c *Circuit) Mux16(a, b [16]Signal, sel Signal) (o [16]Signal) {
	// ins := make([]Signal, 0, 33)
	// ins = append(ins, a[:]...)
	// ins = append(ins, b[:]...)
	// ins = append(ins, sel)
	c.begin("Mux16", nil)
	for i := range 16 {
		o[i] = c.Mux(a[i], b[i], sel)
	}
	c.end()
	return o
}

// ALU performs various arithmetic and logic operations based on control flags.
func (c *Circuit) ALU(x, y [16]Signal, zx, nx, zy, ny, f, no Signal) (out [16]Signal) {

	c.begin("ALU", nil)

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

	c.end()
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
