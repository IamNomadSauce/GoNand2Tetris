package main

import (
	"fmt"
	"strconv"
)

type Inputs struct {
	X int
	Y int
}
type ThreeBitIn struct {
	A int
	B int
	C int
}

var tree = []string{}

var count = 0

// Need to reverse the order sometems due to Go reading from the left
func reverseSlice[T any](s []T) []T {
	n := len(s)
	reversed := make([]T, n)
	for i, element := range s {
		reversed[n-1-i] = element
	}
	return reversed
}

var a = []int{1}
var b = []int{1}

func run_alu() {

	var out = []int{0}
	var x = []int{1}
	var y = []int{1}
	// var x = []int{1}
	// var y = []int{1}
	// var x = []int{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	// // var y = []int{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1}        // 16 bit
	var zx = []int{1, 1, 1, 0, 1, 0, 1, 0, 1, 0, 1, 0, 1, 0, 0, 0, 0, 0} // 18
	var nx = []int{0, 1, 1, 0, 1, 0, 1, 0, 1, 1, 1, 0, 1, 0, 1, 0, 0, 1}
	var zy = []int{1, 1, 1, 1, 0, 1, 0, 1, 0, 1, 0, 1, 0, 0, 0, 0, 0, 0}
	var ny = []int{0, 1, 0, 1, 0, 1, 0, 1, 0, 1, 1, 1, 0, 0, 0, 1, 0, 1}
	var f = []int{1, 1, 1, 0, 0, 0, 0, 1, 1, 1, 1, 1, 1, 1, 1, 1, 0, 0}
	var no = []int{0, 1, 0, 0, 0, 1, 1, 1, 1, 1, 1, 0, 0, 0, 1, 1, 0, 1}

	for i := range len(zx) {
		// fmt.Println(x, y, "|", zx[i], nx[i], zy[i], ny[i], f[i], no[i], "|")

		x[0] = 1
		fmt.Println(x, y, "|", a, b, "|", zx[i], nx[i], zy[i], ny[i], f[i], no[i], "|", out)
		out = ALU(x, y, zx[i], nx[i], zy[i], ny[i], f[i], no[i])

		fmt.Println("\n")
		fmt.Println(x, y, "|", a, b, "|", zx[i], nx[i], zy[i], ny[i], f[i], no[i], "|", out)
		x[0] = 1
	}

}

func main() {
	fmt.Println("Gates Running\n")
	// Nand
	// And
	// Not
	// Or
	// Xor
	test_Mux()  // Passed
	test_DMux() // Passed
	// Not16
	// And16
	// Or16
	// Mux16
	// Or8Way
	// Mux4Way16
	// Mux8Way16
	test_DMX4W() // Passed
	test_DMX8W() // Passed
	// HalfAdder
	// FullAdder
	// Add16
	// Inc16
	// ALU
}

// type int uint8

func And(x, y int) (o int) {
	count++
	tree = append(tree, "And["+strconv.Itoa(count))
	a := Nand(x, y)
	b := Not(a)

	tree = append(tree, "]")
	return b
	// return Not(Nand(x, y))
}

func Or(x, y int) (o int) {
	count++
	tree = append(tree, "Or["+strconv.Itoa(count))
	// return Not(And(Not(x), Not(y)))
	a := Not(x)
	b := Not(y)
	c := Nand(a, b)
	tree = append(tree, "]")
	return c
	// return Nand(Not(x), Not(y))
}

func Not(x int) (o int) {
	count++
	tree = append(tree, "Not["+strconv.Itoa(count))
	c := Nand(x, x)
	tree = append(tree, "]")
	return c
	// return Nand(x, x)

}

func Nand(in_1, in_2 int) (out int) {
	count++
	tree = append(tree, "Nand["+strconv.Itoa(count))
	if in_1 == 1 && in_2 == 1 {
		tree = append(tree, "]")
		return 0
	}
	tree = append(tree, "]")
	return 1
	// return !(a && b)
}

func test_Nand()

func Xor(a, b int) (o int) {
	return Or(And(a, Not(b)), And(b, Not(a)))
}

// TODO Add n-bit adder/xor/or/nand, etc
func Adder3Way(a, b, c int) (o int) {
	ab := And(a, b)
	d := And(ab, c)
	return d
}
func Xor3way(a, b, c int) (o int) {
	ab := Xor(a, b)
	d := Xor(ab, c)
	return d
}

func Mux(a, b, sel int) (o int) {
	sel_a := And(a, Not(sel)) // out=a
	// 0|0|0||0
	// 0|1|0||0
	// 1|0|0||1
	// 1|1|0||1
	sel_b := And(b, sel) // out=b
	// 0|0|1||0
	// 0|1|1||1
	// 1|0|1||0
	// 1|1|1||1
	return Or(sel_a, sel_b)
}
func test_Mux() {
	fmt.Println("\nMux")
	a := []int{0, 0, 1, 1, 0, 0, 1, 1}
	b := []int{0, 1, 0, 1, 0, 1, 0, 1}
	sel := []int{0, 0, 0, 0, 1, 1, 1, 1}

	for i := range sel {
		fmt.Println(a[i], b[i], sel[i], "|", Mux(a[i], b[i], sel[i]))
	}
}

func Mux16(a, b [16]int, sel int) (out [16]int) {
	for i := range 16 {
		out[i] = Mux(a[i], b[i], sel)
	}
	return out
}

func AndMuxOr(a, b, sel int) (o int) {
	if sel == 0 {
		return And(a, b)
	}
	return Or(a, b)
}

func And16(a, b []int) (o []int) {
	var outputs = []int{}
	for i := range a {
		outputs = append(outputs, And(a[i], b[i]))
	}
	return outputs

}

func Mux4Way16(a, b, c, d [16]int, sel [2]int) [16]int {
	// in = a,b,c,d, sel_1, sel_2
	// sel_1 | sel_2 || out
	// 0|0||a
	// 0|1||b
	// 1|0||c
	// 1|1||d

	r1 := Mux16(a, b, sel[0])
	r2 := Mux16(c, d, sel[0])
	return Mux16(r1, r2, sel[1])
}

// DMUX
func DMux(in, sel int) (a, b int) {
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

	a = And(in, Not(sel))
	b = And(in, sel)
	return a, b
}

func test_DMux() {
	fmt.Println("\nDMux")
	sel := []int{0, 1}
	for i := range sel {
		a, b := DMux(1, sel[i])
		fmt.Println(sel[i], "|", a, b)
	}

}

func DMux4Way(in int, sel [2]int) (a, b, c, d int) {
	// s1,s0||a,b,c,d
	// 0,0 || a,0,0,0
	// 0,1 || 0,b,0,0
	// 1,0 || 0,0,c,0
	// 1,1 || 0,0,0,d
	// ----------
	// 2xDmux
	// out=a||b
	// Not(sel[1]) so that sel1=1
	ab := And(Not(sel[1]), in)
	a, b = DMux(ab, sel[0])
	// out=c||d
	cd := And(sel[1], in)
	c, d = DMux(cd, sel[0])

	return a, b, c, d
}

func test_DMX4W() {
	fmt.Println("\nDMux4Way")
	sel1 := []int{
		0, 0, 1, 1,
	}
	sel0 := []int{
		0, 1, 0, 1,
	}
	// a := 1
	for i := range sel1 {
		sel := [2]int{sel0[i], sel1[i]}
		a, b, c, d := DMux4Way(1, sel)
		fmt.Println(sel, a, b, c, d)
	}
}

func DMux8Way(in int, sel [3]int) (a, b, c, d, e, f, g, h int) {

	// ns := Not(sel[2])
	sel2 := [2]int{sel[0], sel[1]}
	a, b, c, d = DMux4Way(in, sel2)
	e, f, g, h = DMux4Way(in, sel2)

	a = And(a, Not(sel[2]))
	b = And(b, Not(sel[2]))
	c = And(c, Not(sel[2]))
	d = And(d, Not(sel[2]))
	e = And(e, sel[2])
	f = And(f, sel[2])
	g = And(g, sel[2])
	h = And(h, sel[2])

	return a, b, c, d, e, f, g, h
}

func test_DMX8W() {
	fmt.Println("\nDMux8Way")
	// 3,2,1 ||a,b,c,d,e,f,g,h
	// ----------------------- -> s2 = [sel0, sel1]
	// 0,0,0 ||i,0,0,0,0,0,0,0 | x,0,0
	// 0,0,1 ||0,i,0,0,0,0,0,0 | x,0,1
	// 0,1,0 ||0,0,i,0,0,0,0,0 | x,1,0
	// 0,1,1 ||0,0,0,i,0,0,0,0 | x.1,1
	// -----------------------
	// 1,0,0 ||0,0,0,0,i,0,0,0
	// 1,0,1 ||0,0,0,0,0,i,0,0
	// 1,1,0 ||0,0,0,0,0,0,i,0
	// 1,1,1 ||0,0,0,0,0,0,0,i
	// -----------------------

	in := 1
	sel2 := []int{0, 0, 0, 0, 1, 1, 1, 1}
	sel1 := []int{0, 0, 1, 1, 0, 0, 1, 1}
	sel0 := []int{0, 1, 0, 1, 0, 1, 0, 1}
	// a,b,c,d := DMux4Way()

	for i := range sel0 {
		a, b, c, d, e, f, g, h := DMux8Way(in, [3]int{sel0[i], sel1[i], sel2[i]})
		fmt.Println(sel0[i], sel1[i], sel2[i], "||", a, b, c, d, e, f, g, h)
	}
}

// -----------------------------------------
// ADDERs
// -----------------------------------------

func HalfAdder(a, b int) (sum, carry int) {

	carry = And(a, b)
	sum = Xor(a, b)

	return sum, carry
}

// Full Adder
func FullAdder(a, b, c int) (sum, carry int) {
	s1, c1 := HalfAdder(a, b)
	s2, c2 := HalfAdder(s1, c)
	carry = Or(c1, c2)
	sum = s2

	return sum, carry

}

func N_BitAdder(a, b []int) (out []int) {
	// fmt.Println(len(a), "Bit Adder")
	sum := []int{}
	// carry := []int{}
	s1, c1 := FullAdder(a[0], b[0], 0)
	sum = append(sum, s1)
	// carry = append(carry, c1)
	for i := range len(a) {
		if i > 0 {
			s1, c1 = FullAdder(a[i], b[i], c1)
			sum = append(sum, s1)
			// carry = append(carry, c1)
		}
	}
	return sum
}

func Incrementer(a []int) (out []int) {
	b := make([]int, len(a))
	b[0] = 1
	c := N_BitAdder(a, b)

	fmt.Println()
	fmt.Println(reverseSlice(a))
	fmt.Println(reverseSlice(b))
	fmt.Println("----------------------")
	fmt.Println(reverseSlice(c))
	return c

}

// ALU performs various arithmetic and logic operations based on control flags.
func ALU(x, y []int, zx, nx, zy, ny, f, no int) []int {
	// Apply zx operation
	if zx == 1 {
		for i := range x {
			x[i] = 0
		}
	}
	// Apply nx operation
	if nx == 1 {
		for i := range x {
			x[i] = 1 - x[i] // Flip the bit
		}
	}
	// Apply zy operation
	if zy == 1 {
		for i := range y {
			y[i] = 0
		}
	}
	// Apply ny operation
	if ny == 1 {
		for i := range y {
			y[i] = 1 - y[i] // Flip the bit
		}
	}

	// Declare the output slice
	var out []int

	// Apply f operation
	if f == 1 {
		out = N_BitAdder(x, y)
	} else {
		out = And16(x, y)
	}

	// Apply no operation
	if no == 1 {
		for i := range out {
			out[i] = 1 - out[i] // Flip the bit
		}
	}

	return out
}
