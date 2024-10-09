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

type Chip struct {
	Name string
	Inputs []int
	Outputs	[]int
}

type Node struct {
	ID		int
	Label	string
	// Chip	Gate
	Children []Node
}

var node_tree = []Node{}
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

func main() {
	fmt.Println("Gates Running\n")
	// Nand
	// And
	// Not
	// Or
	// Xor
	//test_Mux()  // Passed
	////test_DMux() // Passed
	// Not16
	// And16
	// Or16
	// Mux16
	// Or8Way
	// Mux4Way16
	// Mux8Way16
	//test_DMX4W() // Pass
	//test_DMX8W() // Pass
	//test_halfadder() // Pass
	test_fulladder() //Pass
	test_Add16()
	// FullAdder
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

func Not16(x [16]int) []int {

	var output []int
	for i := range x {
		output = append(output, Nand(x[i], x[i]))		
	}

	return output
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

func test_Nand() {
}

func Xor(a, b int) (o int) {
	return Or(And(a, Not(b)), And(b, Not(a)))
}

func Xor16(a, b [16]int) []int {
	
	var xor16 []int
	for i := range a {
		xor16 = append(xor16, Xor(a[i], b[i]))
	}

	return xor16

}

// TODO Add n-bit adder/xor/or/nand, etc
func Adder3Way (a, b, c int) (o int) {
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
	for i := range a {
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

func And16(a, b [16]int) (o [16]int) {
	var outputs = [16]int{}
	for i := range a {
		outputs[i] = And(a[i], b[i])
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
	//fmt.Println("Half Adder", a, b)

	carry = And(a, b)
	sum = Xor(a, b)

	return sum, carry
}

func test_halfadder() {
	x := []int{0,0,1,1}
	y := []int{0,1,0,1}
	
	fmt.Println("x|y|c|s\n-------")
	for i := range x {
		s, c := HalfAdder(x[i], y[i])
		fmt.Printf("%d|%d|%d|%d\n", x[i], y[i], c, s)
	}
}

// Full Adder
func FullAdder(a, b, c int) (sum, carry int) {
	s1, c1 := HalfAdder(a, b)
	s2, c2 := HalfAdder(s1, c)
	carry = Xor(c1, c2)
	sum = s2

	return sum, carry
}

func test_fulladder() {
	fmt.Println("Test Full Adder")
	x := []int{0,0,0,0,1,1,1,1}
	y := []int{0,0,1,1,0,0,1,1}
	z := []int{0,1,0,1,0,1,0,1}
	carry := []int{0,0,0,0,0,0,0,0}
	sum := []int{0,0,0,0,0,0,0,0,0}

	fmt.Println("x|y|z||c|s")
	fmt.Println("--------------")

	for i := range x {
		s,c := FullAdder(x[i], y[i], z[i])
		sum[i] = s
		carry[i] = c
		fmt.Printf("%d|%d|%d||%d|%d\n", x[i], y[i], z[i], c, s)
	}

	fmt.Println("---------------------")
	fmt.Println("c= ", reverseSlice(carry), "\nx= ",reverseSlice(x), "\ny= ", reverseSlice(y), "\nz= ", reverseSlice(z), "\n--------------------------", "\ns= ", reverseSlice(sum))
}

func Add16(a, b [16]int) (out, carry [16]int) {
	// fmt.Println(len(a), "Bit Adder")
	sum := [16]int{}
	carry = [16]int{}

	s1, c1 := FullAdder(a[0], b[0], 0)
	sum[0] = s1

	carry[0] = c1
	for i := range a {
		if i > 0 {
			s1, c1 = FullAdder(a[i], b[i], c1)
			sum[i] = s1
			carry[i] = c1
		}
	}
	return sum, carry
}

func test_Add16() {
	fmt.Println("Test Add16")
	a := [16]int{1,1,0,1,0,0,0,0,0,0,0,0,0,0,0,0}
	b := [16]int{0,1,0,0,0,0,0,0,0,0,0,0,0,0,0,0}

	sum, carry := Add16(a, b)
	fmt.Println("c= ", carry, "\na= ", a, "\nb= ", b, "\n----------------------------\ns= ", sum)
}

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

// ALU performs various arithmetic and logic operations based on control flags.
func ALU(x, y [16]int, zx, nx, zy, ny, f, no int) (output [16]int) {

	//ZX operation
	var nxzx = [16]int{}
	
	for i := range x {
		o1 := And(x[i], Not(zx))
		xo1 := Xor(o1, nx)
		nxzx[i] = xo1
	}
	

	//Zy operation
	var nyzy = [16]int{}
	
	for i := range x {
		o1 := And(y[i], Not(zy))
		yo1 := Xor(o1, ny)
		nyzy[i] = yo1
	}
	
	// Adder function
	adder, _ := Add16(nxzx, nyzy)

	// Bitwise And
	bitwise_and := And16(nxzx, nyzy)

	f_out := Mux16(adder, bitwise_and, f)

	var no_out = [16]int{}
	for i := range f_out {
		no_out[i] = Xor(f_out[i], no)
	}

	return no_out
	
}

func DFF(d, c int) {
	
	nand1 := Nand(d, Not(c))
	nand2 := Nand(nand1, Not(c))

	nand3 := Nand(nand1, nand4)
	nand4 := Nand(nand3, nand2)

	nand5 := Nand(nand3, c)
	nand6 := Nand(nand5, c)

	nand7 := Nand(nand4, nand8)
	nand8 := Nand(nand7, nand6)

	fmt.Println(d, c, nand7, nand8)
}

// 1-bit register
type Bit struct {
	In	int
	Load int
	Out int
}

//func Register(in, load int) (out in) {
//}










