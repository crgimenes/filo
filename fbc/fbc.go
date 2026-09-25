// Package fbc reads and lists Filo bytecode: units (.fbc) and bundles (.fbb),
// as docs/bytecode.md describes them. It is written from that document alone
// and shares nothing with the engine's loader, so it is another reading of
// the format; its listing is the C runtime's fbc_dump, byte for byte, which
// clang_filo's make govm checks on every unit the corpus compiles to.
//
// The bytes are untrusted, as a loader's are. Unlike a loader, a unit whose
// checksum is wrong is read and reported, not refused: the listing of a
// damaged unit is exactly what someone looking at one wants.
package fbc

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"strconv"
)

// Kinds of file.
const (
	KindUnit   = 1
	KindBundle = 2
)

const (
	header     = 20
	section    = 12
	member     = 12
	namesMax   = 4096
	constsMax  = 4096
	fnsMax     = 1024
	exportsMax = 64
	membersMax = 256
)

// Opcodes.
const (
	OpPushK = iota
	OpPushG
	OpStoreG
	OpPushL
	OpStoreL
	OpPushUp
	OpStoreUp
	OpPop
	OpJmp
	OpCall
	OpCallB
	OpRet
	OpClosure
	OpTuple
	OpUnpack
	OpTrap
	OpPushB
	opCount
)

var mnemonic = [opCount]string{
	"PUSH_K", "PUSH_G", "STORE_G", "PUSH_L", "STORE_L", "PUSH_UP", "STORE_UP", "POP", "JMP",
	"CALL", "CALLB", "RET", "CLOSURE", "TUPLE", "UNPACK", "TRAP", "PUSH_B",
}

var condition = []string{"always", "if false", "and", "or", "check"}

// Span is where something lies in the file: Off from its start, Len bytes.
type Span struct {
	Off int
	Len int
}

// Fn is a function of a unit; Off is in the code section.
type Fn struct {
	Off    int
	Len    int
	Params int
	Slots  int
	Stack  int
}

// Section is an entry of the section table: its kind and where it lies.
type Section struct {
	Kind int
	Span
}

// SectionNames are the kinds the spec defines, by number; a kind past them
// is one this reader does not know.
var SectionNames = []string{"", "imports", "globals", "constants", "functions", "code",
	"exports", "debug", "externs"}

// Export is an entry point: a name and the function it runs.
type Export struct {
	Name string
	Fn   int
}

// Unit is a unit read: its header, and where each part of it is.
type Unit struct {
	Data        []byte
	Version     int
	Checksum    uint32
	ChecksumOK  bool
	WidestStack int
	WidestFrame int
	HeaderSize  int       // the fixed header and the section table
	Sections    []Section // as the table lists them
	Imports     []string
	Globals     []string
	Externs     map[int]bool // globals read and never written: the VM's to provide
	Consts      []int        // where each constant starts in Data
	Fns         []Fn
	Exports     []Export
	Code        Span
	Debug       Span // empty when the unit was stripped
}

// Member is a unit of a bundle.
type Member struct {
	Name string
	Unit Span
}

// Bundle is a bundle's header and table.
type Bundle struct {
	Data        []byte
	Version     int
	Checksum    uint32
	ChecksumOK  bool
	WidestStack int
	WidestFrame int
	Members     []Member
}

type reader struct {
	b   []byte
	at  int
	end int
	bad bool
}

func (r *reader) byte() uint32 {
	if r.bad || r.at >= r.end {
		r.bad = true
		return 0
	}
	c := r.b[r.at]
	r.at++
	return uint32(c)
}

// uleb reads a ULEB128; past 32 bits the reading is bad, but the bytes are
// still taken up to the last one, as the C reader takes them.
func (r *reader) uleb() uint32 {
	var v uint32
	for shift := uint32(0); shift < 35; shift += 7 {
		c := r.byte()
		if shift == 28 && c > 0x0F {
			r.bad = true
		}
		v |= (c & 0x7F) << shift
		if c&0x80 == 0 {
			return v
		}
	}
	r.bad = true
	return 0
}

func (r *reader) span() Span {
	n := r.uleb()
	s := Span{Off: r.at}
	if r.bad || int64(n) > int64(r.end-r.at) {
		r.bad = true
		return s
	}
	s.Len = int(n)
	r.at += s.Len
	return s
}

func (r *reader) count(most int) (int, bool) {
	n := r.uleb()
	return int(n), !r.bad && int64(n) <= int64(most)
}

func u16(p []byte) int {
	return int(binary.LittleEndian.Uint16(p))
}

func u32(p []byte) uint32 {
	return binary.LittleEndian.Uint32(p)
}

func fnv1a(h uint32, p []byte) uint32 {
	for _, c := range p {
		h ^= uint32(c)
		h *= 16777619
	}
	return h
}

// checksum is FNV-1a 32 of the whole file with its own field as zero.
func checksum(data []byte) uint32 {
	h := fnv1a(2166136261, data[:8])
	h = fnv1a(h, []byte{0, 0, 0, 0})
	return fnv1a(h, data[12:])
}

// Kind is KindUnit or KindBundle, or 0 for anything else.
func Kind(data []byte) int {
	if len(data) < header || string(data[:4]) != "\x7fFBC" {
		return 0
	}
	if data[4] == KindUnit || data[4] == KindBundle {
		return int(data[4])
	}
	return 0
}

func (u *Unit) text(s Span) string {
	return string(u.Data[s.Off : s.Off+s.Len])
}

func (u *Unit) names(r *reader) ([]string, bool) {
	n, ok := r.count(namesMax)
	if !ok {
		return nil, false
	}
	names := make([]string, 0, n)
	for range n {
		names = append(names, u.text(r.span()))
	}
	return names, !r.bad
}

func (u *Unit) readConsts(r *reader) bool {
	n, ok := r.count(constsMax)
	if !ok {
		return false
	}
	for i := 0; i < n && !r.bad; i++ {
		u.Consts = append(u.Consts, r.at)
		switch tag := r.byte(); tag {
		case 1:
			for range 8 {
				r.byte()
			}
		case 2:
			r.span()
		case 3, 4, 5:
		default:
			return false
		}
	}
	return !r.bad
}

func (u *Unit) readFns(r *reader) bool {
	n, ok := r.count(fnsMax)
	if !ok {
		return false
	}
	for range n {
		f := Fn{Off: int(r.uleb()), Len: int(r.uleb()), Params: int(r.uleb()), Slots: int(r.uleb()), Stack: int(r.uleb())}
		u.Fns = append(u.Fns, f)
	}
	return !r.bad
}

func (u *Unit) readExports(r *reader) bool {
	n, ok := r.count(exportsMax)
	if !ok {
		return false
	}
	for range n {
		name := u.text(r.span())
		u.Exports = append(u.Exports, Export{Name: name, Fn: int(r.uleb())})
	}
	return !r.bad
}

// readExterns reads indices into the globals, whatever the order of the
// section table puts them in.
func (u *Unit) readExterns(r *reader) bool {
	n, ok := r.count(namesMax)
	if !ok {
		return false
	}
	for range n {
		g := r.uleb()
		if r.bad || g >= namesMax {
			return false
		}
		u.Externs[int(g)] = true
	}
	return true
}

func (u *Unit) readSection(kind int, s Span) bool {
	r := &reader{b: u.Data, at: s.Off, end: s.Off + s.Len}
	var ok bool
	switch kind {
	case 1:
		u.Imports, ok = u.names(r)
	case 2:
		u.Globals, ok = u.names(r)
	case 3:
		ok = u.readConsts(r)
	case 4:
		ok = u.readFns(r)
	case 5:
		u.Code, ok = s, true
	case 6:
		ok = u.readExports(r)
	case 7:
		u.Debug, ok = s, true
	case 8:
		ok = u.readExterns(r)
	default:
		ok = true // a kind this reader does not know is skipped, as the spec says
	}
	return ok
}

// Read reads a unit's header and sections; the Unit points into data. The
// error says why the bytes are not a unit this reader can list.
func Read(data []byte) (*Unit, error) {
	u := &Unit{Data: data, Externs: map[int]bool{}}
	if len(data) < header || string(data[:4]) != "\x7fFBC" {
		return nil, errors.New(`not a Filo unit (no \x7fFBC at the start)`)
	}
	if data[4] != KindUnit {
		return nil, errors.New("not a unit (kind is not 1)")
	}
	u.Version = int(data[5])
	hsize := u16(data[6:])
	u.Checksum = u32(data[8:])
	u.WidestStack = u16(data[12:])
	u.WidestFrame = u16(data[14:])
	nsec := u16(data[16:])
	if hsize < header || hsize > len(data) || nsec > (hsize-header)/section {
		return nil, errors.New("the header does not fit the file")
	}
	u.HeaderSize = hsize
	u.ChecksumOK = checksum(data) == u.Checksum
	var seen [8]bool
	for i := range nsec {
		e := data[header+i*section:]
		kind := u16(e)
		off, n := int64(u32(e[4:])), int64(u32(e[8:]))
		if off > int64(len(data)) || n > int64(len(data))-off {
			return nil, errors.New("a section lies outside the file")
		}
		if kind < len(seen) {
			if seen[kind] {
				return nil, errors.New("a section appears twice")
			}
			seen[kind] = true
		}
		s := Span{Off: int(off), Len: int(n)} // #nosec G115 -- both within len(data), checked above
		if !u.readSection(kind, s) {
			return nil, errors.New("a section does not read as the spec says")
		}
		u.Sections = append(u.Sections, Section{Kind: kind, Span: s})
	}
	// negative only where an int has 32 bits: a value the C reader takes
	// as past 2^31, and so past the code, as here
	for _, f := range u.Fns {
		if f.Off < 0 || f.Len < 0 || f.Off > u.Code.Len || f.Len > u.Code.Len-f.Off {
			return nil, errors.New("a function lies outside the code section")
		}
	}
	for _, x := range u.Exports {
		if x.Fn < 0 || x.Fn >= len(u.Fns) {
			return nil, errors.New("an export names no function")
		}
	}
	return u, nil
}

// ReadBundle reads a bundle's header and table, as Read reads a unit.
func ReadBundle(data []byte) (*Bundle, error) {
	if Kind(data) != KindBundle {
		return nil, errors.New("not a Filo bundle")
	}
	b := &Bundle{
		Data:        data,
		Version:     int(data[5]),
		Checksum:    u32(data[8:]),
		ChecksumOK:  checksum(data) == u32(data[8:]),
		WidestStack: u16(data[12:]),
		WidestFrame: u16(data[14:]),
	}
	hsize := u16(data[6:])
	n := u16(data[16:])
	if n == 0 || n > membersMax || hsize > len(data) || header+n*member > hsize {
		return nil, errors.New("the bundle's header does not fit the file")
	}
	for i := range n {
		e := data[header+i*member:]
		off, size, nameAt := int64(u32(e)), int64(u32(e[4:])), int64(u32(e[8:]))
		end := int64(len(data))
		if off < int64(hsize) || off > end || size > end-off || off%8 != 0 || nameAt < int64(hsize) || nameAt >= end {
			return nil, errors.New("a member lies outside the bundle")
		}
		r := &reader{b: data, at: int(nameAt), end: len(data)} // #nosec G115 -- within len(data), checked above
		s := r.span()
		if r.bad || s.Len == 0 {
			return nil, errors.New("a member's name does not read")
		}
		b.Members = append(b.Members, Member{
			Name: string(data[s.Off : s.Off+s.Len]),
			Unit: Span{Off: int(off), Len: int(size)}, // #nosec G115 -- within len(data), checked above
		})
	}
	return b, nil
}

// Number is how Filo writes a number: Go's strconv with 'g' and the
// shortest precision.
func Number(x float64) string {
	return strconv.FormatFloat(x, 'g', -1, 64)
}

// ConstText is constant i as the listing writes it, in at most most-1
// bytes: a string is quoted, its control bytes escaped and a long one cut.
func (u *Unit) ConstText(i, most int) string {
	at := u.Consts[i]
	switch u.Data[at] {
	case 1:
		return Number(math.Float64frombits(binary.LittleEndian.Uint64(u.Data[at+1:])))
	case 2:
		r := &reader{b: u.Data, at: at + 1, end: len(u.Data)}
		s := r.span()
		out := []byte{'"'}
		for k := 0; k < s.Len && len(out)+8 < most; k++ {
			out = appendEscaped(out, u.Data[s.Off+k])
			if k == 40 && s.Len > 44 {
				out = append(out, "..."...)
				break
			}
		}
		return string(append(out, '"'))
	case 3:
		return "#t"
	case 4:
		return "#f"
	}
	return "(list)"
}

func appendEscaped(out []byte, c byte) []byte {
	switch {
	case c == '"' || c == '\\':
		return append(out, '\\', c)
	case c == '\n':
		return append(out, `\n`...)
	case c < 0x20 || c == 0x7F:
		return fmt.Appendf(out, `\x%02x`, c)
	}
	return append(out, c)
}

// Insn is an instruction read: where it is in the code section, how many
// bytes it takes (0 when it cannot be read whole), and its text.
type Insn struct {
	PC       int
	Len      int
	Op       int    // -1 for a byte that is no opcode
	Name     string // the mnemonic
	Operands string
	Note     string // what the operand stands for: a name, a constant, a target
}

func (in Insn) String() string {
	if in.Op < 0 {
		return in.Note
	}
	if in.Note != "" {
		return pad(in.Name, 8) + " " + pad(in.Operands, 9) + " " + in.Note
	}
	return pad(in.Name, 8) + " " + in.Operands
}

// The widths the listing gives a note and a name, as the C one does.
const (
	noteMost = 96
	nameMost = 128
	lineMost = 256
)

// Insn reads the instruction at pc of the code section.
func (u *Unit) Insn(pc int) Insn {
	in := Insn{PC: pc}
	if pc < 0 || pc >= u.Code.Len {
		return in
	}
	r := &reader{b: u.Data, at: u.Code.Off + pc, end: u.Code.Off + u.Code.Len}
	b := r.byte()
	op, x := b>>3, b&7
	if op >= opCount {
		in.Op, in.Len, in.Note = -1, 1, fmt.Sprintf("(unknown 0x%02x)", b)
		return in
	}
	if x == 7 && op != OpJmp {
		x = r.uleb()
	}
	in.Op, in.Name, in.Operands = int(op), mnemonic[op], strconv.FormatUint(uint64(x), 10)
	u.operands(&in, r, x)
	if r.bad {
		return Insn{PC: pc, Op: in.Op, Name: in.Name}
	}
	in.Len = r.at - u.Code.Off - pc
	return in
}

func (u *Unit) operands(in *Insn, r *reader, x uint32) {
	switch in.Op {
	case OpPushK, OpTrap:
		if int(x) < len(u.Consts) {
			in.Note = u.ConstText(int(x), noteMost)
		}
	case OpPushG, OpStoreG:
		if int(x) < len(u.Globals) {
			in.Note = cut(u.Globals[x], noteMost)
		}
	case OpPushL, OpStoreL:
		in.Note = fmt.Sprintf("slot %d", x)
	case OpPushUp, OpStoreUp:
		slot := r.uleb()
		s := "s"
		if x == 1 {
			s = ""
		}
		in.Operands = fmt.Sprintf("%d %d", x, slot)
		in.Note = fmt.Sprintf("slot %d, %d function%s out", slot, x, s)
	case OpJmp:
		lo, hi := r.byte(), r.byte()
		off := int(int16(lo | hi<<8)) // #nosec G115 -- the offset is a signed 16-bit field (docs/bytecode.md)
		to := r.at - u.Code.Off + off
		in.Operands = "?"
		if int(x) < len(condition) {
			in.Operands = condition[x]
		}
		if x != 4 {
			in.Note = fmt.Sprintf("-> %04d", to)
		}
	case OpCallB:
		imp := r.uleb()
		in.Operands = fmt.Sprintf("%d %d", x, imp)
		if int(imp) < len(u.Imports) {
			in.Note = cut(u.Imports[imp], noteMost)
		}
	case OpPushB:
		if int(x) < len(u.Imports) {
			in.Note = cut(u.Imports[x], noteMost)
		}
	case OpRet:
		in.Note = "exit"
		if x == 0 {
			in.Note = "return"
		}
	case OpClosure:
		in.Note = fmt.Sprintf("fn %d", x)
	}
}

// Position is where the instruction at pc came from in the source, by the
// debug section; false when it does not say.
func (u *Unit) Position(pc int) (line, col int, ok bool) {
	r := &reader{b: u.Data, at: u.Debug.Off, end: u.Debug.Off + u.Debug.Len}
	var at, c uint32
	var l int64
	for r.at < r.end {
		d, z, cc := r.uleb(), r.uleb(), r.uleb()
		if r.bad || d > math.MaxUint32-at {
			break
		}
		at += d
		if int64(at) > int64(pc) {
			break
		}
		if z&1 != 0 {
			l -= int64((z + 1) / 2) // in 32 bits, as the C reader: 2^32-1 wraps to 0
		} else {
			l += int64(z / 2)
		}
		c = cc
		ok = l >= 1
	}
	if !ok {
		return 0, 0, false
	}
	return int(uint32(l)), int(c), true // #nosec G115 -- as the C reader, a line past 2^32 wraps
}

// cut is s as a C string of most bytes holds it: up to its first zero byte,
// and at most most-1 bytes.
func cut(s string, most int) string {
	for i := 0; i < len(s); i++ {
		if s[i] == 0 {
			s = s[:i]
			break
		}
	}
	if len(s) > most-1 {
		s = s[:most-1]
	}
	return s
}

// pad brings s to width bytes with spaces on the right, as printf's %-Ns.
func pad(s string, width int) string {
	for len(s) < width {
		s += " "
	}
	return s
}
