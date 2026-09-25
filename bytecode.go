package filo

import (
	"encoding/binary"
	"fmt"
	"math"
)

// Reading the bytecode the C runtime writes (docs/bytecode.md): a unit is a
// program compiled elsewhere, and loading it is a trust boundary — every
// section is bounds-checked here, and every index, slot, jump and stack
// access is checked by the machine when it is used.

const (
	bcHeader     = 20
	bcSection    = 12
	bcVersion    = 1
	bcKindUnit   = 1
	bcKindBundle = 2
	bcMember     = 12
	bcMembersMax = 256
	bcAlign      = 8
	bcConstsMax  = 4096
	bcFnsMax     = 1024
	bcExportsMax = 64
	bcStackMax   = 4096
	bcNameMax    = 256
	bcImportsMax = 128 // the C runtime's builtin table
	bcGlobalsMax = 512 // the C runtime's symbol table
)

const (
	bcSecImports = 1 + iota
	bcSecGlobals
	bcSecConstants
	bcSecFunctions
	bcSecCode
	bcSecExports
	bcSecDebug
	bcSecExterns
	bcSections = bcSecExterns
)

// Unit is a compiled program loaded for this engine: entry points that share
// one set of globals, run with Run.
type Unit struct {
	eng     *Engine
	code    []byte
	consts  []Value
	globals []int    // unit index to symbol id in the engine's table
	names   []string // the globals' names, by unit index
	externs []int    // unit indices of the globals the unit reads and never writes
	imports []bcImport
	fns     []bcFunc
	exports []bcExport
	debug   []byte
}

// bcImport is a function the unit calls: a builtin of the engine, or, when
// the engine has none of that name, a global holding a function, looked up
// when it is called.
type bcImport struct {
	name    string
	builtin builtinFunc
	sym     int
}

type bcFunc struct {
	unit     *Unit
	index    int // in the unit's function table, as a listing numbers them
	off      int
	length   int
	nparams  int
	nslots   int
	maxstack int
}

type bcExport struct {
	name string
	fn   int
}

type bcReader struct {
	b   []byte
	p   int
	bad bool
}

func (r *bcReader) byte() byte {
	if r.p >= len(r.b) {
		r.bad = true
		return 0
	}
	c := r.b[r.p]
	r.p++
	return c
}

// uleb reads a ULEB128. Past 2^31-1 no count, offset or index is valid, and
// an int of 32 bits could not hold it.
func (r *bcReader) uleb() int {
	var v uint64
	for shift := uint(0); shift < 35; shift += 7 {
		c := r.byte()
		if r.bad {
			return 0
		}
		v |= uint64(c&0x7f) << shift
		if c&0x80 == 0 {
			if v > math.MaxInt32 {
				r.bad = true
				return 0
			}
			return int(v)
		}
	}
	r.bad = true
	return 0
}

func (r *bcReader) bytes(n int) []byte {
	if n < 0 || n > len(r.b)-r.p {
		r.bad = true
		return nil
	}
	s := r.b[r.p : r.p+n]
	r.p += n
	return s
}

// name reads a length and its bytes. Names are what the C runtime keeps as C
// strings, so one with a zero byte in it is refused as C refuses it.
func (r *bcReader) name() string {
	n := r.uleb()
	if n > bcNameMax {
		r.bad = true
		return ""
	}
	s := r.bytes(n)
	for _, c := range s {
		if c == 0 {
			r.bad = true
		}
	}
	return string(s)
}

func fnv1a(h uint32, p []byte) uint32 {
	for _, c := range p {
		h ^= uint32(c)
		h *= 16777619
	}
	return h
}

// bcChecksum is FNV-1a 32 of the whole file with its own field zero.
func bcChecksum(data []byte) uint32 {
	h := fnv1a(2166136261, data[:8])
	h = fnv1a(h, []byte{0, 0, 0, 0})
	return fnv1a(h, data[12:])
}

// u32 is a field of the header or of a table, -1 past what an int of 32
// bits holds, which every check refuses.
func u32(b []byte) int {
	v := binary.LittleEndian.Uint32(b)
	if v > math.MaxInt32 {
		return -1
	}
	return int(v)
}

func bcBad(what string) error {
	return fmt.Errorf("bytecode: a damaged unit: %s", what)
}

// LoadUnit loads a unit of bytecode (a .fbc) for this engine. The functions
// it imports resolve to the engine's builtins by name, or else to a global
// holding a function when it runs; Missing says what a run would lack.
func (e *Engine) LoadUnit(data []byte) (*Unit, error) {
	data = append([]byte(nil), data...) // the unit keeps it; the caller's stays theirs
	if len(data) < bcHeader || string(data[:4]) != "\x7fFBC" {
		return nil, fmt.Errorf("bytecode: not a Filo unit")
	}
	if data[4] != bcKindUnit || data[5] != bcVersion {
		return nil, fmt.Errorf("bytecode: a kind or version this runtime does not read")
	}
	hsize := int(binary.LittleEndian.Uint16(data[6:]))
	nsec := int(binary.LittleEndian.Uint16(data[16:]))
	if hsize < bcHeader || hsize > len(data) || nsec > 64 || bcHeader+nsec*bcSection > hsize {
		return nil, bcBad("header")
	}
	if bcChecksum(data) != binary.LittleEndian.Uint32(data[8:]) {
		return nil, fmt.Errorf("bytecode: the checksum does not match (a damaged unit)")
	}
	var sec [bcSections + 1][]byte
	var have [bcSections + 1]bool
	for i := range nsec {
		ent := data[bcHeader+i*bcSection:]
		kind := int(binary.LittleEndian.Uint16(ent))
		off, n := u32(ent[4:]), u32(ent[8:])
		if off < hsize || off > len(data) || n < 0 || n > len(data)-off {
			return nil, bcBad("section table")
		}
		if kind < 1 || kind > bcSections {
			continue // a kind from a later version: skipped
		}
		if have[kind] {
			return nil, bcBad("section table")
		}
		have[kind] = true
		sec[kind] = data[off : off+n]
	}
	if !have[bcSecCode] || !have[bcSecFunctions] || !have[bcSecExports] {
		return nil, bcBad("section table")
	}
	u := &Unit{eng: e, code: sec[bcSecCode], debug: sec[bcSecDebug]}
	steps := []struct {
		what string
		read func(*bcReader) bool
	}{
		{"imports", u.readImports},
		{"globals", u.readGlobals},
		{"externs", u.readExterns},
		{"constants", u.readConsts},
		{"functions", u.readFns},
		{"exports", u.readExports},
	}
	kinds := []int{bcSecImports, bcSecGlobals, bcSecExterns, bcSecConstants, bcSecFunctions, bcSecExports}
	for i, s := range steps {
		if !have[kinds[i]] {
			continue
		}
		r := &bcReader{b: sec[kinds[i]]}
		if !s.read(r) || r.bad {
			return nil, bcBad(s.what)
		}
	}
	return u, nil
}

func (u *Unit) readImports(r *bcReader) bool {
	n := r.uleb()
	if r.bad || n > bcImportsMax {
		return false
	}
	u.imports = make([]bcImport, n)
	for i := range u.imports {
		name := r.name()
		if r.bad {
			return false
		}
		imp := bcImport{name: name, builtin: u.eng.builtins[name]}
		if imp.builtin == nil {
			imp.sym = u.eng.symbols.Resolve(name)
		}
		u.imports[i] = imp
	}
	return true
}

func (u *Unit) readGlobals(r *bcReader) bool {
	n := r.uleb()
	if r.bad || n > bcGlobalsMax {
		return false
	}
	u.globals = make([]int, n)
	u.names = make([]string, n)
	for i := range u.globals {
		u.names[i] = r.name()
		if r.bad {
			return false
		}
		u.globals[i] = u.eng.symbols.Resolve(u.names[i])
	}
	return true
}

func (u *Unit) readExterns(r *bcReader) bool {
	n := r.uleb()
	if r.bad || n > len(u.globals) {
		return false
	}
	last := -1
	for range n {
		g := r.uleb()
		if r.bad || g >= len(u.globals) || g <= last {
			return false
		}
		last = g
		u.externs = append(u.externs, g)
	}
	return true
}

func (u *Unit) readConsts(r *bcReader) bool {
	n := r.uleb()
	if r.bad || n > bcConstsMax {
		return false
	}
	u.consts = make([]Value, n)
	for i := range u.consts {
		switch r.byte() {
		case 1:
			b := r.bytes(8)
			if b == nil {
				return false
			}
			u.consts[i] = VNum(math.Float64frombits(binary.LittleEndian.Uint64(b)))
		case 2:
			b := r.bytes(r.uleb())
			if b == nil {
				return false
			}
			u.consts[i] = VString(string(b))
		case 3:
			u.consts[i] = VBool(true)
		case 4:
			u.consts[i] = VBool(false)
		case 5:
			u.consts[i] = VList(nil)
		default:
			return false
		}
	}
	return true
}

func (u *Unit) readFns(r *bcReader) bool {
	n := r.uleb()
	if r.bad || n > bcFnsMax {
		return false
	}
	u.fns = make([]bcFunc, n)
	for i := range u.fns {
		f := &u.fns[i]
		f.unit, f.index = u, i
		f.off, f.length, f.nparams, f.nslots, f.maxstack = r.uleb(), r.uleb(), r.uleb(), r.uleb(), r.uleb()
		if r.bad || f.off > len(u.code) || f.length > len(u.code)-f.off || f.nparams > f.nslots ||
			f.nslots > 0xFFFF || f.maxstack > bcStackMax {
			return false
		}
	}
	return true
}

func (u *Unit) readExports(r *bcReader) bool {
	n := r.uleb()
	if r.bad || n == 0 || n > bcExportsMax {
		return false
	}
	u.exports = make([]bcExport, n)
	for i := range u.exports {
		u.exports[i] = bcExport{name: r.name(), fn: r.uleb()}
		if r.bad || u.exports[i].fn >= len(u.fns) {
			return false
		}
	}
	return true
}

// LoadBundle loads the member named member of a bundle (a .fbb): the bundle
// is checked whole, then the member is loaded as any unit is.
func (e *Engine) LoadBundle(data []byte, member string) (*Unit, error) {
	if len(data) < bcHeader || string(data[:4]) != "\x7fFBC" || data[4] != bcKindBundle {
		return nil, fmt.Errorf("bytecode: not a Filo bundle")
	}
	if data[5] != bcVersion {
		return nil, fmt.Errorf("bytecode: a kind or version this runtime does not read")
	}
	hsize := int(binary.LittleEndian.Uint16(data[6:]))
	n := int(binary.LittleEndian.Uint16(data[16:]))
	if n == 0 || n > bcMembersMax || hsize > len(data) || bcHeader+n*bcMember > hsize {
		return nil, bcBad("bundle header")
	}
	if bcChecksum(data) != binary.LittleEndian.Uint32(data[8:]) {
		return nil, fmt.Errorf("bytecode: the checksum does not match (a damaged bundle)")
	}
	var found []byte
	for i := range n {
		ent := data[bcHeader+i*bcMember:]
		off, mlen, nameOff := u32(ent), u32(ent[4:]), u32(ent[8:])
		if off < hsize || off > len(data) || mlen < 0 || mlen > len(data)-off || off%bcAlign != 0 ||
			nameOff < hsize || nameOff >= len(data) {
			return nil, bcBad("bundle table")
		}
		r := &bcReader{b: data, p: nameOff}
		name := r.name()
		if r.bad || name == "" {
			return nil, bcBad("bundle table")
		}
		if found == nil && name == member {
			found = data[off : off+mlen]
		}
	}
	if found == nil {
		return nil, fmt.Errorf("bytecode: no bundle member named %s", member)
	}
	return e.LoadUnit(found)
}

// Entries are the unit's entry points, in the order it names them.
func (u *Unit) Entries() []string {
	names := make([]string, len(u.exports))
	for i, x := range u.exports {
		names[i] = x.name
	}
	return names
}

// Missing is what a run with globals would lack, the functions first: an
// import that is neither a builtin nor a function among globals, an extern
// that is neither among globals nor a builtin. A host installing a program
// refuses it when this is not empty.
func (u *Unit) Missing(globals map[string]Value) []string {
	var lack []string
	for _, imp := range u.imports {
		v, ok := globals[imp.name]
		if imp.builtin == nil && (!ok || v.Kind != KFunc) {
			lack = append(lack, imp.name)
		}
	}
	for _, g := range u.externs {
		name := u.names[g]
		_, ok := globals[name]
		if _, builtin := u.eng.builtins[name]; !ok && !builtin {
			lack = append(lack, name)
		}
	}
	return lack
}

// position is where the instruction at pc came from in the source, from the
// debug section; false when the unit has none.
func (u *Unit) position(pc int) (int, int, bool) {
	r := &bcReader{b: u.debug}
	at, col := 0, 0
	var line int64
	found := false
	for r.p < len(r.b) {
		d, z, c := r.uleb(), r.uleb(), r.uleb()
		if r.bad || d > math.MaxInt32-at {
			break
		}
		at += d
		if at > pc {
			break
		}
		if z&1 != 0 {
			line -= int64(z+1) / 2
		} else {
			line += int64(z) / 2
		}
		if line < 1 || line > math.MaxInt32 {
			break
		}
		col = c
		found = true
	}
	return int(line), col, found
}
