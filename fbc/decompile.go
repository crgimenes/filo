package fbc

import (
	"encoding/binary"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/crgimenes/filo"
)

// Decompiling a unit back to Filo. The compiler (filo's Engine.Build, the C
// runtime's filo_bc_build) writes each form in one shape, so the forms can be
// read back from the shapes: a stack machine run on expressions instead of
// values, and the jumps of if, cond, and and or. What the bytes do not keep
// cannot come back: the names of parameters and let bindings (made up here,
// x y z for parameters and a b c for bindings, a digit for how deeply the
// function is nested), comments and layout, cond against nested ifs, a
// constant against the expression folded into it. Forms that compile to the
// same bytes are one form here, so a unit decompiled and compiled again is
// the same unit, byte for byte but for its debug section: the test of this
// file.

// Source is an entry point decompiled: its name and its Filo, one top-level
// form a line.
type Source struct {
	Name string
	Text string
}

// Decompile is the unit's entry points as Filo, in the order the unit has
// them, which is the order to compile them in for the same unit.
func Decompile(u *Unit) ([]Source, error) {
	d := &decompiler{u: u, parent: make([]int, len(u.Fns)), names: map[[2]int]string{}, avoid: map[string]bool{}}
	for i, x := range u.Exports {
		if x.Fn != i {
			return nil, fmt.Errorf("entry %s is fn %d: this compiler puts entry i at fn i", x.Name, x.Fn)
		}
	}
	for _, n := range u.Imports {
		d.avoid[n] = true
	}
	for _, n := range u.Globals {
		d.avoid[n] = true
	}
	for n := range strings.FieldsSeq(specialForms) {
		d.avoid[n] = true
	}
	for i := range d.parent {
		d.parent[i] = -1
	}
	for fn, f := range u.Fns {
		for pc := f.Off; pc < f.Off+f.Len; {
			in := u.Insn(pc)
			if in.Len == 0 {
				return nil, fmt.Errorf("fn %d: an instruction cut short at %04d", fn, pc)
			}
			if in.Op == OpClosure {
				inner, _ := strconv.Atoi(in.Operands)
				if inner < len(d.parent) && inner > fn {
					d.parent[inner] = fn
				}
			}
			pc += in.Len
		}
	}
	var out []Source
	for i, x := range u.Exports {
		forms, err := d.function(i)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", x.Name, err)
		}
		var b strings.Builder
		for _, f := range forms {
			b.WriteString(f.text())
			b.WriteByte('\n')
		}
		out = append(out, Source{Name: x.Name, Text: b.String()})
	}
	return out, nil
}

const specialForms = "def fn let letv if cond else do set and or return exit values tuple list"

// node is a form: an atom, or a list of forms. The flags say what the
// shapes around it need to know.
type node struct {
	atom  string
	kids  []*node
	lit   bool // a constant the folder takes as a literal
	empty bool // the empty-list constant: (cond), and what an if without else gives
	do    bool // a (do ...): its forms fold into the body around it
	isIf  bool // (if test then [else]): kids are test, then, and else when there is one
	chain int  // 2 and, 3 or: the operands of the jumps to out
	out   int
	check bool // the last operand of an and or an or (JMP 4)
}

func atom(s string) *node { return &node{atom: s} }

func list(head string, kids ...*node) *node {
	return &node{kids: append([]*node{atom(head)}, kids...)}
}

func (n *node) text() string {
	var b strings.Builder
	n.write(&b)
	return b.String()
}

func (n *node) write(b *strings.Builder) {
	switch {
	case n.chain != 0 || n.check:
		kw := "and"
		if n.chain == 3 {
			kw = "or"
		}
		list(kw, n.kids...).write(b)
	case n.isIf:
		n.ifForm().write(b)
	case n.kids == nil:
		b.WriteString(n.atom)
	default:
		b.WriteByte('(')
		for i, k := range n.kids {
			if i > 0 {
				b.WriteByte(' ')
			}
			k.write(b)
		}
		b.WriteByte(')')
	}
}

// ifForm writes an if as if, or as cond: a chain of them reads better as
// one, and a test that is a literal must be a cond, which the folder leaves
// alone where it would fold the if. Both are the same bytes.
func (n *node) ifForm() *node {
	test := n.kids[0]
	nested := len(n.kids) == 3 && n.kids[2].isIf
	if !test.lit && !nested {
		return list("if", n.kids...)
	}
	c := list("cond")
	for at := n; ; {
		c.kids = append(c.kids, &node{kids: append([]*node{at.kids[0]}, body(at.kids[1])...)})
		if len(at.kids) < 3 {
			return c
		}
		next := at.kids[2]
		if !next.isIf {
			c.kids = append(c.kids, &node{kids: append([]*node{atom("else")}, body(next)...)})
			return c
		}
		at = next
	}
}

// body is n as the forms of a body: a (do ...) spread out.
func body(n *node) []*node {
	if n.do {
		return n.kids[1:]
	}
	return []*node{n}
}

// step is what a sequence ran before the expression it leads to: a form
// whose value was dropped, a let binding, or a letv's tuple.
type step struct {
	kind  int // stmt, bind, unpack
	v     *node
	slot  int   // a binding's
	slots []int // an unpack's, in order
}

const (
	stmt = iota
	bind
	unpack
)

// entry is a value on the stack: its form, and the steps that ran before it
// at that height, which go around it or around what it becomes part of.
type entry struct {
	n   *node
	pre []step
}

type decompiler struct {
	u      *Unit
	parent []int // the function each function is made in, -1 for an entry
	names  map[[2]int]string
	avoid  map[string]bool
}

// fnState is the function being read: its slots bound by a let so far.
type fnState struct {
	fn       int
	declared map[int]bool
}

// function is fn as forms: an entry's top level, or a closure's (fn ...).
func (d *decompiler) function(fn int) ([]*node, error) {
	f := d.u.Fns[fn]
	last := d.u.Insn(f.Off + f.Len - 1)
	if f.Len == 0 || last.Op != OpRet || last.Operands != "0" {
		return nil, fmt.Errorf("fn %d does not end in RET 0", fn)
	}
	st := &fnState{fn: fn, declared: map[int]bool{}}
	e, err := d.one(st, f.Off, f.Off+f.Len-1)
	if err != nil {
		return nil, err
	}
	forms := seqForms(e.pre, e.n)
	if d.parent[fn] < 0 {
		return forms, nil
	}
	params := &node{}
	for s := range f.Params {
		params.kids = append(params.kids, atom(d.name(fn, s)))
	}
	if params.kids == nil {
		params.kids = []*node{} // (), not an atom
	}
	return []*node{list("fn", append([]*node{params}, forms...)...)}, nil
}

// seqForms is the steps and then n as the forms of a body: statements as
// they are, bindings on consecutive slots as one let (as the compiler lays
// a let's slots out), a letv, each around what follows.
func seqForms(steps []step, n *node) []*node {
	if len(steps) == 0 {
		return body(n)
	}
	s := steps[0]
	switch s.kind {
	case stmt:
		return append([]*node{s.v}, seqForms(steps[1:], n)...)
	case unpack: // v holds the tuple and the names
		return []*node{list("letv", append([]*node{s.v.kids[1], s.v.kids[0]}, seqForms(steps[1:], n)...)...)}
	}
	binds := &node{kids: []*node{}}
	j := 0
	for j < len(steps) && steps[j].kind == bind && steps[j].slot == s.slot+j {
		binds.kids = append(binds.kids, steps[j].v)
		j++
	}
	return []*node{list("let", append([]*node{binds}, seqForms(steps[j:], n)...)...)}
}

// wrap is e as one form: its steps around it.
func wrap(e entry) *node {
	forms := seqForms(e.pre, e.n)
	if len(forms) == 1 {
		return forms[0]
	}
	return &node{kids: append([]*node{atom("do")}, forms...), do: true}
}

// name is a local's: slot s of function fn, a parameter or a binding.
func (d *decompiler) name(fn, s int) string {
	key := [2]int{fn, s}
	if n, ok := d.names[key]; ok {
		return n
	}
	level := 0
	for p := d.parent[fn]; p >= 0; p = d.parent[p] {
		level++
	}
	const params, binds = "xyzuvw", "abcdefghijkmnopqrst"
	var n string
	if s < d.u.Fns[fn].Params {
		n = "p" + strconv.Itoa(s) + "_"
		if s < len(params) {
			n = params[s : s+1]
		}
		if level > 1 {
			n += strconv.Itoa(level)
		}
	} else {
		k := s - d.u.Fns[fn].Params
		n = "l" + strconv.Itoa(k) + "_"
		if k < len(binds) {
			n = binds[k : k+1]
		}
		if level > 0 {
			n += strconv.Itoa(level)
		}
	}
	for d.avoid[n] {
		n += "_"
	}
	d.names[key] = n
	return n
}

func (d *decompiler) ancestor(fn, depth int) int {
	for ; depth > 0 && fn >= 0; depth-- {
		fn = d.parent[fn]
	}
	return fn
}

// one reads [from, to) as one value: a branch, a body, an and's rest.
func (d *decompiler) one(st *fnState, from, to int) (entry, error) {
	stack, err := d.region(st, from, to)
	if err != nil {
		return entry{}, err
	}
	if len(stack) != 1 {
		return entry{}, fmt.Errorf("fn %d: %04d-%04d leaves %d values, not 1", st.fn, from, to, len(stack))
	}
	return stack[0], nil
}

// stackReader is a region being read: the stack of forms and, at each height,
// the steps waiting for the next value pushed there.
type stackReader struct {
	stack   []entry
	pending map[int][]step
}

func (r *stackReader) push(n *node) {
	h := len(r.stack)
	r.stack = append(r.stack, entry{n: n, pre: r.pending[h]})
	delete(r.pending, h)
}

// combine replaces the top k values with the form build makes of them. The
// steps before the first move out to the new form, unless that would leave
// the folder a form it folds (fold says so), where the original had them
// inside; the others stay around theirs.
func (r *stackReader) combine(k int, build func([]*node) *node, fold func([]*node) bool) error {
	if len(r.stack) < k {
		return fmt.Errorf("a stack of %d, %d wanted", len(r.stack), k)
	}
	es := r.stack[len(r.stack)-k:]
	r.stack = r.stack[:len(r.stack)-k]
	if k == 0 {
		r.push(build(nil))
		return nil
	}
	ns := make([]*node, k)
	for i, e := range es {
		ns[i] = e.n
		if i > 0 {
			ns[i] = wrap(e)
		}
	}
	pre := es[0].pre
	if len(pre) > 0 && fold != nil && fold(ns) {
		ns[0], pre = wrap(es[0]), nil
	}
	r.stack = append(r.stack, entry{n: build(ns), pre: pre})
	return nil
}

func (r *stackReader) pop() entry {
	e := r.stack[len(r.stack)-1]
	r.stack = r.stack[:len(r.stack)-1]
	return e
}

// drop is a value popped as a statement: it waits, with what ran before it,
// for the next value at its height.
func (r *stackReader) drop() {
	e := r.pop()
	h := len(r.stack)
	r.pending[h] = append(r.pending[h], e.pre...)
	r.pending[h] = append(r.pending[h], step{kind: stmt, v: e.n})
}

// folds says whether the compiler's folder turns the call of name on the
// literals ns into a constant: asked of the folder itself, which declines a
// call that fails, (!= 0 #t) among them.
func folds(name string, ns []*node) bool {
	for _, n := range ns {
		if !n.lit {
			return false
		}
	}
	tree, err := filo.Parse(list(name, ns...).text())
	if err != nil {
		return false
	}
	tree, _ = filo.FoldConstants(tree) // one form: the form itself
	switch tree.(type) {
	case *filo.NumberLit, *filo.BoolLit, *filo.StringLit:
		return true
	}
	return false
}

// region reads [from, to) and leaves what it pushed.
func (d *decompiler) region(st *fnState, from, to int) ([]entry, error) {
	r := &stackReader{pending: map[int][]step{}}
	for pc := from; pc < to; {
		in := d.u.Insn(pc)
		if in.Len == 0 || in.Op < 0 {
			return nil, fmt.Errorf("fn %d: no instruction at %04d", st.fn, pc)
		}
		next, err := d.insn(st, r, in, pc)
		if err != nil {
			return nil, fmt.Errorf("fn %d at %04d: %w", st.fn, pc, err)
		}
		pc = next
	}
	return r.stack, nil
}

// leaf is the form an instruction that pushes one thing pushes; an
// operand past its table is an error, a unit being untrusted bytes.
func (d *decompiler) leaf(st *fnState, in Insn, x int) (*node, bool, error) {
	switch in.Op {
	case OpPushK:
		if x >= len(d.u.Consts) {
			return nil, true, fmt.Errorf("a constant past the table")
		}
		return d.konst(x), true, nil
	case OpPushG:
		if x >= len(d.u.Globals) {
			return nil, true, fmt.Errorf("a global past the table")
		}
		return atom(d.u.Globals[x]), true, nil
	case OpPushL:
		n, err := d.local(st.fn, x)
		return atom(n), true, err
	case OpPushUp:
		n, err := d.local(d.ancestor(st.fn, x), operand2(in))
		return atom(n), true, err
	case OpPushB:
		if x >= len(d.u.Imports) {
			return nil, true, fmt.Errorf("an import past the table")
		}
		return atom(d.u.Imports[x]), true, nil
	}
	return nil, false, nil
}

// local is name, checked: slot s of function fn, within its frame.
func (d *decompiler) local(fn, s int) (string, error) {
	if fn < 0 || fn >= len(d.u.Fns) || s >= d.u.Fns[fn].Slots {
		return "", fmt.Errorf("a local out of reach")
	}
	return d.name(fn, s), nil
}

// insn reads the instruction at pc; where the next one starts.
func (d *decompiler) insn(st *fnState, r *stackReader, in Insn, pc int) (int, error) {
	x, next := operand(in), pc+in.Len
	if n, ok, err := d.leaf(st, in, x); ok {
		if err != nil {
			return next, err
		}
		r.push(n)
		return next, nil
	}
	switch in.Op {
	case OpClosure:
		if x <= st.fn || x >= len(d.u.Fns) {
			return next, fmt.Errorf("a closure of a function that is not a later one")
		}
		forms, err := d.function(x)
		if err != nil {
			return next, err
		}
		r.push(forms[0])
	case OpTrap:
		return next, d.trap(r, x)
	case OpStoreG:
		if x >= len(d.u.Globals) {
			return next, fmt.Errorf("a global past the table")
		}
		kw := "set"
		if d.parent[st.fn] < 0 {
			kw = "def"
		}
		return next, r.combine(1, func(ns []*node) *node { return list(kw, atom(d.u.Globals[x]), ns[0]) }, nil)
	case OpStoreL:
		return d.storeLocal(st, r, x, next)
	case OpStoreUp:
		name, err := d.local(d.ancestor(st.fn, x), operand2(in))
		if err != nil {
			return next, err
		}
		return next, r.combine(1, func(ns []*node) *node { return list("set", atom(name), ns[0]) }, nil)
	case OpPop:
		if len(r.stack) < x {
			return next, fmt.Errorf("POP %d of %d", x, len(r.stack))
		}
		for range x {
			r.drop()
		}
	case OpCallB:
		if operand2(in) >= len(d.u.Imports) {
			return next, fmt.Errorf("an import past the table")
		}
		name := d.u.Imports[operand2(in)]
		return next, r.combine(x, func(ns []*node) *node { return list(name, ns...) },
			func(ns []*node) bool { return folds(name, ns) })
	case OpCall:
		return next, r.combine(x+1, d.call, nil)
	case OpTuple:
		return next, r.combine(x, func(ns []*node) *node { return list("tuple", ns...) }, nil)
	case OpRet:
		kw := "return"
		if x == 1 {
			kw = "exit"
		}
		return next, r.combine(1, func(ns []*node) *node {
			if ns[0].empty {
				return list(kw)
			}
			return list(kw, ns[0])
		}, nil)
	case OpUnpack:
		return d.unpack(st, r, x, next)
	case OpJmp:
		return d.jump(st, r, pc, next)
	default:
		return next, fmt.Errorf("%s where no form starts", in.Name)
	}
	return next, nil
}

// call is a CALL's form: the head and the arguments. A builtin as the head
// goes in a do, since a builtin named there would be a CALLB.
func (d *decompiler) call(ns []*node) *node {
	head := ns[0]
	if head.kids == nil && !head.lit && d.isImport(head.atom) {
		head = list("do", head)
	}
	return &node{kids: append([]*node{head}, ns[1:]...)}
}

// storeLocal is a STORE_L: a let binding when it is the slot's first store
// and a POP follows, else a set.
func (d *decompiler) storeLocal(st *fnState, r *stackReader, x, next int) (int, error) {
	if _, err := d.local(st.fn, x); err != nil {
		return next, err
	}
	after := d.u.Insn(next)
	if st.declared[x] || x < d.u.Fns[st.fn].Params || after.Op != OpPop || after.Operands != "1" || len(r.stack) == 0 {
		return next, r.combine(1, func(ns []*node) *node { return list("set", atom(d.name(st.fn, x)), ns[0]) }, nil)
	}
	st.declared[x] = true
	e := r.pop()
	// a let takes its slots before its values run, so a let inside the
	// value has higher ones: it stays inside
	keep := len(e.pre)
	for i, p := range e.pre {
		if p.kind == bind && p.slot > x || p.kind == unpack && len(p.slots) > 0 && p.slots[0] > x {
			keep = i
			break
		}
	}
	value := wrap(entry{n: e.n, pre: e.pre[keep:]})
	h := len(r.stack)
	r.pending[h] = append(r.pending[h], e.pre[:keep]...)
	r.pending[h] = append(r.pending[h], step{kind: bind, slot: x, v: list(d.name(st.fn, x), value)})
	return next + after.Len, nil
}

func (d *decompiler) isImport(name string) bool {
	return slices.Contains(d.u.Imports, name)
}

// unpack is a letv: the tuple, UNPACK n, and each name stored from the last,
// a binding step for the value after it.
func (d *decompiler) unpack(st *fnState, r *stackReader, n, at int) (int, error) {
	if len(r.stack) == 0 {
		return at, fmt.Errorf("UNPACK of nothing")
	}
	e := r.pop()
	slots := make([]int, n)
	for i := n - 1; i >= 0; i-- {
		store := d.u.Insn(at)
		pop := d.u.Insn(at + store.Len)
		if store.Op != OpStoreL || pop.Op != OpPop || pop.Operands != "1" {
			return at, fmt.Errorf("UNPACK %d not followed by its stores", n)
		}
		slots[i] = operand(store)
		if _, err := d.local(st.fn, slots[i]); err != nil {
			return at, err
		}
		st.declared[slots[i]] = true
		at += store.Len + pop.Len
	}
	names := &node{kids: []*node{}}
	for _, s := range slots {
		names.kids = append(names.kids, atom(d.name(st.fn, s)))
	}
	h := len(r.stack)
	r.pending[h] = append(r.pending[h], e.pre...)
	r.pending[h] = append(r.pending[h], step{kind: unpack, slots: slots, v: &node{kids: []*node{e.n, names}}})
	return at, nil
}

// jump reads the form a jump starts: an if (or a cond clause) at JMP 1, an
// and or an or at JMP 2 or 3, the check of the last operand at JMP 4.
func (d *decompiler) jump(st *fnState, r *stackReader, pc, next int) (int, error) {
	cond, target := d.jmp(pc)
	f := d.u.Fns[st.fn]
	if target < next || target > f.Off+f.Len {
		return next, fmt.Errorf("a jump out of its function or backward")
	}
	switch cond {
	case 1:
		if target < 3 {
			return next, fmt.Errorf("an if with no jump over its else")
		}
		back := d.u.Insn(target - 3)
		if back.Op != OpJmp {
			return next, fmt.Errorf("an if with no jump over its else")
		}
		always, end := d.jmp(target - 3)
		if always != 0 {
			return next, fmt.Errorf("an if whose then ends in JMP %d", always)
		}
		if end < target || end > f.Off+f.Len {
			return next, fmt.Errorf("an if whose end is out of its function")
		}
		then, err := d.one(st, next, target-3)
		if err != nil {
			return next, err
		}
		els, err := d.one(st, target, end)
		if err != nil {
			return next, err
		}
		t, e := wrap(then), wrap(els)
		err = r.combine(1, func(ns []*node) *node {
			n := &node{isIf: true, kids: []*node{ns[0], t}}
			if !e.empty || len(els.pre) != 0 {
				n.kids = append(n.kids, e)
			}
			return n
		}, nil) // a literal test makes it a cond, which does not fold
		return end, err
	case 2, 3:
		rest, err := d.one(st, next, target)
		if err != nil {
			return next, err
		}
		var more []*node
		switch {
		case rest.n.chain == cond && rest.n.out == target:
			more = rest.n.kids
		case rest.n.check:
			more = rest.n.kids
		default:
			return next, fmt.Errorf("an and/or whose rest is not one")
		}
		if len(rest.pre) > 0 { // what ran before the rest's first operand is its own
			more = append([]*node{wrap(entry{n: more[0], pre: rest.pre})}, more[1:]...)
		}
		err = r.combine(1, func(ns []*node) *node {
			return &node{chain: cond, out: target, kids: append([]*node{ns[0]}, more...)}
		}, nil)
		return target, err
	case 4:
		return next, r.combine(1, func(ns []*node) *node { return &node{check: true, kids: ns} }, nil)
	}
	return next, fmt.Errorf("JMP %d where no form starts", cond)
}

// jmp is a jump's condition and where it goes.
func (d *decompiler) jmp(pc int) (cond, target int) {
	b := d.u.Data[d.u.Code.Off+pc:]
	off := int(int16(binary.LittleEndian.Uint16(b[1:3]))) // #nosec G115 -- the signed 16-bit offset of docs/bytecode.md
	return int(b[0] & 7), pc + 3 + off
}

// traps are the forms that compile to nothing but a TRAP with their
// message: the compiler leaves a malformed form to fail where it runs.
var traps = map[string]string{
	"empty list expression":                               "()",
	"do expects at least 1 expression":                    "(do)",
	"if expects 2 or 3 arguments (condition then [else])": "(if)",
	"in let: let expects bindings and body":               "(let)",
	"in let: let expects binding list":                    "(let x 1)",
	"in letv: letv expects bindings and body":             "(letv)",
	"in letv: letv expects name list":                     "(letv x 1)",
	"in fn: fn expects parameters and body":               "(fn)",
	"in fn: fn expects parameter list":                    "(fn x 1)",
	"in def: def expects name and expression":             "(def)",
	"def name must be symbol":                             "(def 1 2)",
	"set expects name and expression":                     "(set)",
	"exit expects 0 or 1 argument":                        "(exit 1 2)",
	"return expects 0 or 1 argument":                      "(return 1 2)",
	"cond clause must be a list of a test and a body":     "(cond 1)",
}

// trap is a TRAP: the malformed form it stands for. (set 1 v) evaluates v
// and drops it first, so v is the statement waiting there.
func (d *decompiler) trap(r *stackReader, k int) error {
	msg, ok := d.constString(k)
	if !ok {
		return fmt.Errorf("TRAP of a constant that is not a string")
	}
	if msg == "set name must be symbol" {
		h := len(r.stack)
		p := r.pending[h]
		if len(p) == 0 || p[len(p)-1].kind != stmt {
			return fmt.Errorf("(set 1 v) without its v")
		}
		v := p[len(p)-1].v
		r.pending[h] = p[:len(p)-1]
		r.push(list("set", atom("1"), v))
		return nil
	}
	src, ok := traps[msg]
	if !ok {
		return fmt.Errorf("TRAP %q: no form known to make it", msg)
	}
	r.push(atom(src))
	return nil
}

func (d *decompiler) constString(k int) (string, bool) {
	if k >= len(d.u.Consts) {
		return "", false
	}
	at := d.u.Consts[k]
	if d.u.Data[at] != 2 {
		return "", false
	}
	rd := &reader{b: d.u.Data, at: at + 1, end: len(d.u.Data)}
	sp := rd.span()
	if rd.bad {
		return "", false
	}
	return string(d.u.Data[sp.Off : sp.Off+sp.Len]), true
}

// konst is constant k as Filo writes it; the infinities and NaN have no
// literal, so they are the expressions the folder makes them from.
func (d *decompiler) konst(k int) *node {
	at := d.u.Consts[k]
	switch d.u.Data[at] {
	case 1:
		v := math.Float64frombits(binary.LittleEndian.Uint64(d.u.Data[at+1:]))
		switch {
		case math.IsNaN(v):
			return &node{atom: "(- 1e400 1e400)", lit: true}
		case math.IsInf(v, 1):
			return &node{atom: "1e400", lit: true}
		case math.IsInf(v, -1):
			return &node{atom: "-1e400", lit: true}
		}
		return &node{atom: strconv.FormatFloat(v, 'g', -1, 64), lit: true}
	case 2:
		s, _ := d.constString(k)
		return &node{atom: literal(s), lit: true}
	case 3:
		return &node{atom: "#t", lit: true}
	case 4:
		return &node{atom: "#f", lit: true}
	}
	return &node{atom: "(cond)", empty: true} // no clause, no value but ()
}

// literal is s as Filo reads it back: the escapes the reader knows, every
// other byte as it is.
func literal(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for i := 0; i < len(s); i++ {
		switch c := s[i]; c {
		case '"', '\\':
			b.WriteByte('\\')
			b.WriteByte(c)
		case '\n':
			b.WriteString(`\n`)
		case '\t':
			b.WriteString(`\t`)
		case '\r':
			b.WriteString(`\r`)
		case 0:
			b.WriteString(`\0`)
		case '\a':
			b.WriteString(`\a`)
		case '\b':
			b.WriteString(`\b`)
		case '\f':
			b.WriteString(`\f`)
		case '\v':
			b.WriteString(`\v`)
		default:
			b.WriteByte(c)
		}
	}
	b.WriteByte('"')
	return b.String()
}

// operand is an instruction's first operand; operand2 its second.
func operand(in Insn) int {
	f := strings.Fields(in.Operands)
	if len(f) == 0 {
		return 0
	}
	n, _ := strconv.Atoi(f[0])
	return n
}

func operand2(in Insn) int {
	f := strings.Fields(in.Operands)
	if len(f) < 2 {
		return 0
	}
	n, _ := strconv.Atoi(f[1])
	return n
}
