package filo

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
)

// Compiling to bytecode (docs/bytecode.md) from the IR: the C runtime's
// compiler (filo_bc_build), step for step, so the same program is the same
// bytes from either. The two are held to each other on every program the
// corpus compiles to (TestCUnits in conformance), as the Prolog spec holds
// the language to its semantics.

const (
	bcScopesMax  = 64
	bcScopePool  = 4096
	bcCodeMax    = 1 << 20
	bcHeaderSize = bcHeader + bcSections*bcSection
	bcGRead      = 1
	bcGWritten   = 2
)

// BuildEntry is a program this engine compiled and the name of the entry
// point it is in a unit.
type BuildEntry struct {
	Name    string
	Program *Program
}

type bcScope struct {
	fn   bool // a function's own frame, or a let inside it
	base int
}

type bcFnRec struct {
	root     *Instr // an entry point's program
	body     []*Instr
	nparams  int
	scopes   []bcScope // around it when it was made
	source   *sourceMap
	off      int
	len      int
	nslots   int
	maxstack int
}

type bcCompiler struct {
	eng      *Engine
	code     []byte
	consts   []Value
	globals  []string
	gused    []uint8 // bcGRead, bcGWritten
	imports  []string
	fns      []bcFnRec
	pooled   int // scopes kept for the functions queued, as the C pool counts them
	scopes   []bcScope
	nslots   int
	depth    int // values on the stack at this point
	maxdepth int
	err      error
	source   *sourceMap
	// the debug section as it is written: the position in effect, and the
	// last one recorded with the pc it starts at
	dbg     []byte
	line    int
	col     int
	dbgPC   int
	dbgLine int
	dbgCol  int
}

// Build compiles programs into one unit, each an entry point named by its
// entry, sharing the unit's globals: the bytes the C runtime's filo_bc_build
// writes for the same programs.
func (e *Engine) Build(entries []BuildEntry) ([]byte, error) {
	if len(entries) == 0 || len(entries) > bcExportsMax {
		return nil, errors.New("bytecode: a unit has 1 to 64 entry points")
	}
	c := &bcCompiler{eng: e}
	for _, x := range entries {
		if x.Program == nil || x.Program.eng != e {
			return nil, errors.New("bytecode: a program this engine did not compile")
		}
		c.source = x.Program.source
		c.queue(x.Program.ir, nil, 0)
	}
	for i := 0; i < len(c.fns) && c.err == nil; i++ {
		c.function(i) // compiling one may queue the closures it makes
	}
	if c.err != nil {
		return nil, c.err
	}
	return c.write(entries), nil
}

func (c *bcCompiler) fail(msg string) {
	if c.err == nil {
		c.err = errors.New(msg)
	}
}

func (c *bcCompiler) byte(b int) {
	if len(c.code) >= bcCodeMax {
		c.fail("bytecode: the code does not fit (a unit holds up to 1 MB, and the run arena bounds it)")
		return
	}
	c.code = append(c.code, byte(b)) // #nosec G115 -- the low byte is the byte meant
}

func (c *bcCompiler) dbgUleb(v int) {
	for {
		if len(c.dbg) >= bcCodeMax {
			c.fail("bytecode: the debug table does not fit the run arena")
			return
		}
		b := v & 0x7F
		v >>= 7
		if v != 0 {
			b |= 0x80
		}
		c.dbg = append(c.dbg, byte(b))
		if v == 0 {
			return
		}
	}
}

// mark starts an instruction: when the position in effect is not the last
// one recorded, the debug table gets an entry — the pc since the last one,
// the line as a signed difference (zigzag), the column as is.
func (c *bcCompiler) mark() {
	if c.line == 0 || (c.line == c.dbgLine && c.col == c.dbgCol) {
		return
	}
	pc := len(c.code)
	dl := c.line - c.dbgLine
	c.dbgUleb(pc - c.dbgPC)
	if dl < 0 {
		c.dbgUleb(-dl*2 - 1)
	} else {
		c.dbgUleb(dl * 2)
	}
	c.dbgUleb(c.col)
	c.dbgPC, c.dbgLine, c.dbgCol = pc, c.line, c.col
}

func (c *bcCompiler) uleb(v int) {
	for v >= 0x80 {
		c.byte(v&0x7F | 0x80)
		v >>= 7
	}
	c.byte(v)
}

// op writes the first byte, and the operand after it when three bits cannot
// hold it.
func (c *bcCompiler) op(op, x int) {
	c.mark()
	if x < 7 {
		c.byte(op<<3 | x)
		return
	}
	c.byte(op<<3 | 7)
	c.uleb(x)
}

func (c *bcCompiler) stack(delta int) {
	if delta < 0 && -delta > c.depth {
		c.fail("bytecode: stack underflow while compiling")
		return
	}
	c.depth += delta
	c.maxdepth = max(c.maxdepth, c.depth)
}

// jump is a jump forward to a place not known yet: where its offset goes.
func (c *bcCompiler) jump(cond int) int {
	c.mark()
	c.byte(bcJmp<<3 | cond)
	at := len(c.code)
	c.byte(0)
	c.byte(0)
	return at
}

func (c *bcCompiler) land(at int) {
	if c.err != nil {
		return
	}
	dist := len(c.code) - (at + 2)
	if dist > 32767 {
		c.fail("bytecode: a jump longer than 32767 bytes")
		return
	}
	c.code[at] = byte(dist)        // #nosec G115 -- the offset's low byte; dist is at most 32767
	c.code[at+1] = byte(dist >> 8) // #nosec G115 -- and its high byte
}

func sameConst(a, b Value) bool {
	if a.Kind != b.Kind {
		return false
	}
	switch a.Kind {
	case KNumber:
		return math.Float64bits(a.Num) == math.Float64bits(b.Num)
	case KBool:
		return a.Bool == b.Bool
	case KString:
		return a.Str == b.Str
	case KList: // only the empty list is ever a constant
		return len(a.List) == 0 && len(b.List) == 0
	}
	return false
}

func (c *bcCompiler) konst(v Value) int {
	if v.Kind == KNumber && math.IsNaN(v.Num) {
		v.Num = math.Float64frombits(0x7FF8000000000000) // one NaN, the same bytes as the C compiler's
	}
	ok := v.Kind == KNumber || v.Kind == KBool || v.Kind == KString || (v.Kind == KList && len(v.List) == 0)
	if !ok {
		c.fail("bytecode: a constant that is not a number, string, bool or ()")
		return 0
	}
	for i, k := range c.consts {
		if sameConst(k, v) {
			return i
		}
	}
	if len(c.consts) >= bcConstsMax {
		c.fail("bytecode: more constants than one unit holds")
		return 0
	}
	c.consts = append(c.consts, v)
	return len(c.consts) - 1
}

// globalUse is the unit's index of the global name, marked as read or
// written: one the unit reads and never writes is an extern, which the
// loading VM provides.
func (c *bcCompiler) globalUse(name string, use uint8) int {
	for i, g := range c.globals {
		if g == name {
			if c.err == nil {
				c.gused[i] |= use
			}
			return i
		}
	}
	if len(c.globals) >= bcGlobalsMax {
		c.fail("bytecode: more globals than one unit holds")
		return 0
	}
	c.globals = append(c.globals, name)
	c.gused = append(c.gused, 0)
	if c.err == nil {
		c.gused[len(c.gused)-1] |= use
	}
	return len(c.globals) - 1
}

func (c *bcCompiler) importOf(name string) int {
	_, ok := c.eng.builtins[name]
	if !ok {
		c.fail("bytecode: a builtin the context does not have")
		return 0
	}
	for i, imp := range c.imports {
		if imp == name {
			return i
		}
	}
	if len(c.imports) >= bcImportsMax {
		c.fail("bytecode: more imports than one unit holds")
		return 0
	}
	c.imports = append(c.imports, name)
	return len(c.imports) - 1
}

// trap is an error where the IR would raise it; in the structure around it
// the instruction stands for a value, though it never produces one.
func (c *bcCompiler) trap(msg string) {
	c.op(bcTrap, c.konst(VString(msg)))
	c.stack(1)
}

func (c *bcCompiler) queue(root *Instr, body []*Instr, nparams int) int {
	if len(c.fns) >= bcFnsMax || c.pooled+len(c.scopes) > bcScopePool {
		c.fail("bytecode: more functions than one unit holds")
		return 0
	}
	c.fns = append(c.fns, bcFnRec{
		root: root, body: body, nparams: nparams, source: c.source,
		scopes: append([]bcScope(nil), c.scopes...),
	})
	c.pooled += len(c.scopes)
	return len(c.fns) - 1
}

func (c *bcCompiler) pushScope(fn bool, base int) bool {
	if len(c.scopes) >= bcScopesMax {
		c.fail("bytecode: scopes nested deeper than one function holds")
		return false
	}
	c.scopes = append(c.scopes, bcScope{fn: fn, base: base})
	return true
}

// local is a local of the IR — depth counts every let and function scope —
// as a slot of the frame of the function that owns it.
func (c *bcCompiler) local(depth, slot int, store bool) {
	if depth >= len(c.scopes) {
		c.fail("bytecode: a local outside every scope")
		return
	}
	t := len(c.scopes) - 1 - depth
	crossed := 0
	for i := len(c.scopes) - 1; i > t; i-- {
		if c.scopes[i].fn {
			crossed++
		}
	}
	flat := c.scopes[t].base + slot
	if crossed == 0 {
		op := bcPushL
		if store {
			op = bcStoreL
		}
		c.op(op, flat)
		return
	}
	op := bcPushUp
	if store {
		op = bcStoreUp
	}
	c.op(op, crossed)
	c.uleb(flat)
}

func (c *bcCompiler) seq(body []*Instr) {
	if len(body) == 0 {
		c.trap("empty body")
		return
	}
	for i, in := range body {
		c.expr(in)
		if i+1 < len(body) {
			c.op(bcPop, 1)
			c.stack(-1)
		}
	}
}

func (c *bcCompiler) empty() {
	c.op(bcPushK, c.konst(VList(nil)))
	c.stack(1)
}

func (c *bcCompiler) ifx(in *Instr) {
	if len(in.Args) < 2 || len(in.Args) > 3 {
		c.trap("if expects 2 or 3 arguments (condition then [else])")
		return
	}
	c.expr(in.Args[0])
	toElse := c.jump(bcJumpFalse)
	c.stack(-1)
	base := c.depth
	c.expr(in.Args[1])
	toEnd := c.jump(bcJumpAlways)
	c.depth = base
	c.land(toElse)
	if len(in.Args) == 3 {
		c.expr(in.Args[2])
	} else {
		c.empty()
	}
	c.land(toEnd)
}

func (c *bcCompiler) cond(in *Instr) {
	var ends []int
	base := c.depth
	closed := false // an else or a bad clause: nothing after it runs
	for i := 0; i < len(in.Clauses) && !closed; i++ {
		cl := in.Clauses[i]
		if cl.Invalid {
			c.trap("cond clause must be a list of a test and a body")
			closed = true
			continue
		}
		if cl.Else {
			c.seq(cl.Body)
			closed = true
			continue
		}
		c.expr(cl.Test)
		next := c.jump(bcJumpFalse)
		c.stack(-1)
		c.seq(cl.Body)
		ends = append(ends, c.jump(bcJumpAlways))
		c.depth = base
		c.land(next)
	}
	if !closed {
		c.empty()
	}
	for _, at := range ends {
		c.land(at)
	}
}

func (c *bcCompiler) logic(in *Instr, isAnd bool) {
	if len(in.Args) == 0 {
		c.op(bcPushK, c.konst(VBool(isAnd)))
		c.stack(1)
		return
	}
	cond := bcJumpOr
	if isAnd {
		cond = bcJumpAnd
	}
	outs := make([]int, 0, len(in.Args))
	for i, arg := range in.Args {
		c.expr(arg)
		if i+1 < len(in.Args) {
			outs = append(outs, c.jump(cond))
			c.stack(-1) // on through, the value was consumed
		}
	}
	// the last operand must be a bool too, and it is the result
	c.land(c.jump(bcJumpCheck))
	for _, at := range outs {
		c.land(at)
	}
}

func (c *bcCompiler) let(in *Instr) {
	n := in.A
	if len(in.Args) <= n {
		c.trap("let expects bindings and body")
		return
	}
	base := c.nslots
	c.nslots += n // never reused: a closure keeps its let's slot
	if !c.pushScope(false, base) {
		return
	}
	for i := range n {
		c.expr(in.Args[i])
		c.op(bcStoreL, base+i)
		c.op(bcPop, 1)
		c.stack(-1)
	}
	c.seq(in.Args[n:])
	c.scopes = c.scopes[:len(c.scopes)-1]
}

func (c *bcCompiler) letv(in *Instr) {
	n := len(in.Names)
	c.expr(in.Args[0]) // in the scope around the letv
	c.op(bcUnpack, n)
	c.stack(n - 1)
	base := c.nslots
	c.nslots += n
	if !c.pushScope(false, base) {
		return
	}
	for i := n; i > 0; i-- {
		c.op(bcStoreL, base+i-1)
		c.op(bcPop, 1)
		c.stack(-1)
	}
	c.seq(in.Args[1:])
	c.scopes = c.scopes[:len(c.scopes)-1]
}

func (c *bcCompiler) set(in *Instr) {
	if len(in.Args) != 2 {
		c.trap("set expects name and expression")
		return
	}
	c.expr(in.Args[1])
	target := in.Args[0]
	switch target.Op {
	case OpLocal:
		c.local(target.A, target.B, true)
	case OpGlobal:
		c.op(bcStoreG, c.globalUse(target.Name, bcGWritten))
	default:
		c.op(bcPop, 1)
		c.stack(-1)
		c.trap("set name must be symbol")
	}
}

func (c *bcCompiler) def(in *Instr) {
	if in.Msg != "" {
		c.trap(in.Msg)
		return
	}
	c.expr(in.Args[0])
	c.op(bcStoreG, c.globalUse(in.Name, bcGWritten))
}

func (c *bcCompiler) signal(in *Instr, name string, exit int) {
	if len(in.Args) > 1 {
		c.trap(name + " expects 0 or 1 argument")
		return
	}
	if len(in.Args) == 1 {
		c.expr(in.Args[0])
	} else {
		c.empty()
	}
	c.op(bcRet, exit)
}

// expr compiles in with its position in effect, so the instructions made for
// it are marked with it, and puts the enclosing one back.
func (c *bcCompiler) expr(in *Instr) {
	line, col := c.line, c.col
	if in.pos > 0 && c.source != nil {
		c.line, c.col = c.source.lineCol(in.pos)
	}
	c.node(in)
	c.line, c.col = line, col
}

func (c *bcCompiler) args(in *Instr) {
	for _, a := range in.Args {
		c.expr(a)
	}
}

func (c *bcCompiler) node(in *Instr) {
	if c.err != nil {
		return
	}
	switch in.Op {
	case OpConst:
		c.op(bcPushK, c.konst(in.Val))
		c.stack(1)
	case OpLocal:
		c.local(in.A, in.B, false)
		c.stack(1)
	case OpGlobal:
		c.op(bcPushG, c.globalUse(in.Name, bcGRead))
		c.stack(1)
	case OpDynamic:
		c.trap("undefined symbol: " + in.Name)
	case OpBuiltin:
		c.op(bcPushB, c.importOf(in.Name))
		c.stack(1)
	case OpEmpty:
		c.trap("empty list expression")
	case OpInvalid:
		c.trap("in " + in.Name + ": " + in.Msg)
	case OpIf:
		c.ifx(in)
	case OpCond:
		c.cond(in)
	case OpDo:
		if len(in.Args) == 0 {
			c.trap("do expects at least 1 expression")
			return
		}
		c.seq(in.Args)
	case OpAnd, OpOr:
		c.logic(in, in.Op == OpAnd)
	case OpLet:
		c.let(in)
	case OpLetv:
		c.letv(in)
	case OpSet:
		c.set(in)
	case OpFn:
		if len(in.Args) == 0 {
			c.trap("fn expects parameters and body")
			return
		}
		c.op(bcClosure, c.queue(nil, in.Args, len(in.Names)))
		c.stack(1)
	case OpDef:
		c.def(in)
	case OpTuple:
		c.args(in)
		c.op(bcTuple, len(in.Args))
		c.stack(1 - len(in.Args))
	case OpExit:
		c.signal(in, "exit", 1)
	case OpReturn:
		c.signal(in, "return", 0)
	case OpCallB:
		c.args(in)
		c.op(bcCallB, len(in.Args))
		c.uleb(c.importOf(in.Name))
		c.stack(1 - len(in.Args))
	case OpCall:
		c.args(in)
		argc := max(len(in.Args)-1, 0)
		c.op(bcCall, argc)
		c.stack(-argc)
	default:
		c.trap("unknown instruction")
	}
}

func (c *bcCompiler) function(idx int) {
	f := c.fns[idx]
	c.scopes = append(c.scopes[:0], f.scopes...)
	c.source = f.source
	if !c.pushScope(true, 0) {
		return
	}
	c.nslots = f.nparams
	c.depth, c.maxdepth = 0, 0
	off := len(c.code)
	if f.root != nil {
		c.expr(f.root)
	} else {
		c.seq(f.body)
	}
	c.op(bcRet, 0)
	g := &c.fns[idx] // the queue may have grown under compiling it
	g.off, g.len, g.nslots, g.maxstack = off, len(c.code)-off, c.nslots, c.maxdepth
}

func appendUleb(out []byte, v int) []byte {
	for v >= 0x80 {
		out = append(out, byte(v&0x7F|0x80)) // #nosec G115 -- seven bits and the continuation bit
		v >>= 7
	}
	return append(out, byte(v)) // #nosec G115 -- the last seven bits
}

func appendName(out []byte, s string) []byte {
	return append(appendUleb(out, len(s)), s...)
}

func appendConst(out []byte, v Value) []byte {
	switch v.Kind {
	case KNumber:
		out = append(out, 1)
		return binary.LittleEndian.AppendUint64(out, math.Float64bits(v.Num))
	case KString:
		return append(appendUleb(append(out, 2), len(v.Str)), v.Str...)
	case KBool:
		if v.Bool {
			return append(out, 3)
		}
		return append(out, 4)
	}
	return append(out, 5)
}

// write lays the unit out: the header and the table of the eight sections,
// then imports, globals, constants, functions, code, exports, debug and
// externs, in that order, and the checksum over it all.
func (c *bcCompiler) write(entries []BuildEntry) []byte {
	out := make([]byte, bcHeaderSize)
	var off [bcSections]int
	off[0] = len(out)
	out = appendUleb(out, len(c.imports))
	for _, name := range c.imports {
		out = appendName(out, name)
	}
	off[1] = len(out)
	out = appendUleb(out, len(c.globals))
	for _, name := range c.globals {
		out = appendName(out, name)
	}
	off[2] = len(out)
	out = appendUleb(out, len(c.consts))
	for _, k := range c.consts {
		out = appendConst(out, k)
	}
	off[3] = len(out)
	out = appendUleb(out, len(c.fns))
	widestStack, widestFrame := 0, 0
	for _, f := range c.fns {
		for _, v := range []int{f.off, f.len, f.nparams, f.nslots, f.maxstack} {
			out = appendUleb(out, v)
		}
		widestStack = max(widestStack, f.maxstack)
		widestFrame = max(widestFrame, f.nslots)
	}
	off[4] = len(out)
	out = append(out, c.code...)
	off[5] = len(out)
	out = appendUleb(out, len(entries))
	for i, x := range entries {
		out = appendUleb(appendName(out, x.Name), i)
	}
	off[6] = len(out)
	out = append(out, c.dbg...)
	off[7] = len(out)
	var externs []int
	for i, use := range c.gused {
		if use == bcGRead {
			externs = append(externs, i)
		}
	}
	out = appendUleb(out, len(externs))
	for _, g := range externs {
		out = appendUleb(out, g)
	}
	copy(out, "\x7fFBC")
	out[4], out[5] = bcKindUnit, bcVersion
	binary.LittleEndian.PutUint16(out[6:], uint16(bcHeaderSize))
	binary.LittleEndian.PutUint16(out[12:], uint16(min(widestStack, 0xFFFF))) // #nosec G115 -- capped just here
	binary.LittleEndian.PutUint16(out[14:], uint16(min(widestFrame, 0xFFFF))) // #nosec G115 -- capped just here
	binary.LittleEndian.PutUint16(out[16:], bcSections)
	for i := range bcSections {
		end := len(out)
		if i+1 < bcSections {
			end = off[i+1]
		}
		e := out[bcHeader+i*bcSection:]
		binary.LittleEndian.PutUint16(e, uint16(i+1))            // #nosec G115 -- 1 to 8
		binary.LittleEndian.PutUint32(e[4:], uint32(off[i]))     // #nosec G115 -- within 1 MB of code and its tables
		binary.LittleEndian.PutUint32(e[8:], uint32(end-off[i])) // #nosec G115 -- the same
	}
	binary.LittleEndian.PutUint32(out[8:], bcChecksum(out))
	return out
}

// StripDebug is the unit without its debug section, for a machine that has
// no use for positions: every other section byte for byte, the table one
// entry shorter and the checksum recomputed, as the C runtime's
// filo_bc_strip. A unit without one is copied.
func StripDebug(unit []byte) ([]byte, error) {
	if len(unit) < bcHeader || string(unit[:4]) != "\x7fFBC" || unit[4] != bcKindUnit {
		return nil, errors.New("bytecode: not a Filo unit")
	}
	hsize := int(binary.LittleEndian.Uint16(unit[6:]))
	nsec := int(binary.LittleEndian.Uint16(unit[16:]))
	if hsize < bcHeader || hsize > len(unit) || nsec > 64 || bcHeader+nsec*bcSection > hsize {
		return nil, errors.New("bytecode: malformed header")
	}
	keep := 0
	for i := range nsec {
		e := unit[bcHeader+i*bcSection:]
		off, n := int64(binary.LittleEndian.Uint32(e[4:])), int64(binary.LittleEndian.Uint32(e[8:]))
		if off < int64(hsize) || off > int64(len(unit)) || n > int64(len(unit))-off {
			return nil, errors.New("bytecode: malformed section table")
		}
		if binary.LittleEndian.Uint16(e) != bcSecDebug {
			keep++
		}
	}
	nh := bcHeader + keep*bcSection
	out := make([]byte, nh)
	copy(out, unit[:bcHeader])
	k := 0
	for i := range nsec {
		e := unit[bcHeader+i*bcSection:]
		if binary.LittleEndian.Uint16(e) == bcSecDebug {
			continue
		}
		off, n := binary.LittleEndian.Uint32(e[4:]), binary.LittleEndian.Uint32(e[8:])
		at := len(out)
		out = append(out, unit[off:off+n]...)
		d := out[bcHeader+k*bcSection:]
		copy(d, e[:4])
		binary.LittleEndian.PutUint32(d[4:], uint32(at)) // #nosec G115 -- within the unit it came from
		binary.LittleEndian.PutUint32(d[8:], n)
		k++
	}
	binary.LittleEndian.PutUint16(out[6:], uint16(nh))    // #nosec G115 -- 20 + 12 a section, at most 64
	binary.LittleEndian.PutUint16(out[16:], uint16(keep)) // #nosec G115 -- at most 64
	binary.LittleEndian.PutUint32(out[8:], bcChecksum(out))
	return out, nil
}

// BundleMember is a unit and the name it has in a bundle.
type BundleMember struct {
	Name string
	Unit []byte
}

// BuildBundle puts units, each whole and named, in one file (docs/bytecode.md,
// "Bundles"), as the C runtime's filo_bundle_build: a member is copied in
// unchanged, so it loads in place from the bundle's bytes.
func BuildBundle(members []BundleMember) ([]byte, error) {
	if len(members) == 0 || len(members) > bcMembersMax {
		return nil, errors.New("bytecode: a bundle holds 1 to 256 units")
	}
	hsize := bcHeader + len(members)*bcMember
	out := make([]byte, hsize)
	nameAt := make([]int, len(members))
	unitAt := make([]int, len(members))
	widestStack, widestFrame := 0, 0
	for i, m := range members {
		if m.Name == "" || len(m.Name) > bcNameMax {
			return nil, errors.New("bytecode: a bundle member needs a name of 1 to 256 bytes")
		}
		for _, other := range members[:i] {
			if other.Name == m.Name {
				return nil, fmt.Errorf("bytecode: two bundle members named %s", m.Name)
			}
		}
		u := m.Unit
		if len(u) < bcHeader || string(u[:4]) != "\x7fFBC" || u[4] != bcKindUnit {
			return nil, fmt.Errorf("bytecode: a bundle member is not a unit: %s", m.Name)
		}
		widestStack = max(widestStack, int(binary.LittleEndian.Uint16(u[12:])))
		widestFrame = max(widestFrame, int(binary.LittleEndian.Uint16(u[14:])))
		nameAt[i] = len(out)
		out = appendName(out, m.Name)
	}
	for i, m := range members {
		for len(out)%bcAlign != 0 {
			out = append(out, 0)
		}
		unitAt[i] = len(out)
		out = append(out, m.Unit...)
	}
	copy(out, "\x7fFBC")
	out[4], out[5] = bcKindBundle, bcVersion
	binary.LittleEndian.PutUint16(out[6:], uint16(hsize))         // #nosec G115 -- 20 + 12 a member, at most 256
	binary.LittleEndian.PutUint16(out[12:], uint16(widestStack))  // #nosec G115 -- read from 16 bits
	binary.LittleEndian.PutUint16(out[14:], uint16(widestFrame))  // #nosec G115 -- read from 16 bits
	binary.LittleEndian.PutUint16(out[16:], uint16(len(members))) // #nosec G115 -- at most 256
	for i, m := range members {
		e := out[bcHeader+i*bcMember:]
		binary.LittleEndian.PutUint32(e, uint32(unitAt[i]))       // #nosec G115 -- a file under 4 GB
		binary.LittleEndian.PutUint32(e[4:], uint32(len(m.Unit))) // #nosec G115 -- the same
		binary.LittleEndian.PutUint32(e[8:], uint32(nameAt[i]))   // #nosec G115 -- the same
	}
	binary.LittleEndian.PutUint32(out[8:], bcChecksum(out))
	return out, nil
}
