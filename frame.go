package filo

// Frame represents a static lexical scope (activation record).
// It replaces the map-based Env for local variables.
type Frame struct {
	slots  []Value
	parent *Frame
}

// newFrame returns a frame of n slots whose parent is parent. Up to four
// slots share the frame's allocation: one allocation per call and per let
// instead of two, and no bigger than the two were.
func newFrame(n int, parent *Frame) *Frame {
	switch n {
	case 0:
		return &Frame{parent: parent}
	case 1:
		f := &struct {
			Frame
			s [1]Value
		}{}
		f.slots, f.parent = f.s[:], parent
		return &f.Frame
	case 2:
		f := &struct {
			Frame
			s [2]Value
		}{}
		f.slots, f.parent = f.s[:], parent
		return &f.Frame
	case 3:
		f := &struct {
			Frame
			s [3]Value
		}{}
		f.slots, f.parent = f.s[:], parent
		return &f.Frame
	case 4:
		f := &struct {
			Frame
			s [4]Value
		}{}
		f.slots, f.parent = f.s[:], parent
		return &f.Frame
	}
	return &Frame{slots: make([]Value, n), parent: parent}
}

// framesKept bounds the spare frames an evaluator keeps of each size: enough
// for the depth a loop of calls reaches, never a whole recursion.
const framesKept = 32

// frameSpares holds an evaluator's spare frames by number of slots, up to
// four. A run that never calls a function never makes one.
type frameSpares [5][]*Frame

// takeFrame is newFrame, from the evaluator's spares when it has one of
// that size: a frame no closure captured is reused instead of left to the
// collector.
func (ev *evaluator) takeFrame(n int, parent *Frame) *Frame {
	if ev.spare != nil && n < len(ev.spare) {
		kept := ev.spare[n]
		if k := len(kept); k > 0 {
			f := kept[k-1]
			ev.spare[n] = kept[:k-1]
			f.parent = parent
			return f
		}
	}
	return newFrame(n, parent)
}

// dropFrame gives f back when no closure was made since escapes was read:
// only a closure can hold a frame, so nothing else can reach it. Its slots
// are cleared so a spare keeps nothing alive.
func (ev *evaluator) dropFrame(f *Frame, escapes uint64) {
	n := len(f.slots)
	if ev.escapes != escapes || n >= len(frameSpares{}) {
		return
	}
	if ev.spare == nil {
		ev.spare = &frameSpares{}
	}
	if len(ev.spare[n]) >= framesKept {
		return
	}
	clear(f.slots)
	f.parent = nil
	ev.spare[n] = append(ev.spare[n], f)
}

// GlobalEnv represents the global environment (map-based).
// Unresolved symbols (globals) are looked up here.
// GlobalEnv represents the global environment using a SymbolTable and array storage.
type GlobalEnv struct {
	symbols  *SymbolTable
	values   []Value
	defined  []bool
	dirtyIDs []int
}

// NewGlobalEnv creates a new global environment linked to a symbol table.
// It allocates backing storage based on symbol table size.
func NewGlobalEnv(symbols *SymbolTable) *GlobalEnv {
	if symbols == nil {
		symbols = NewSymbolTable()
	}
	size := symbols.Size()
	return &GlobalEnv{
		symbols:  symbols,
		values:   make([]Value, size),
		defined:  make([]bool, size),
		dirtyIDs: make([]int, 0, 16),
	}
}

// Reset clears the global environment for reuse.
// It keeps the backing array but resets values and defined status.
func (g *GlobalEnv) Reset(symbols *SymbolTable) {
	requiredSize := symbols.Size()
	g.symbols = symbols
	oldLen := len(g.values)

	// Resize if necessary
	if cap(g.values) < requiredSize {
		// New allocation is zeroed by default
		g.values = make([]Value, requiredSize)
		g.defined = make([]bool, requiredSize)
		// dirtyIDs are irrelevant (new memory is clean)
	} else {
		// Capacity is sufficient
		g.values = g.values[:requiredSize]
		g.defined = g.defined[:requiredSize]

		// Zero out newly exposed slots if we extended into existing capacity
		if requiredSize > oldLen {
			for i := oldLen; i < requiredSize; i++ {
				g.values[i] = Value{}
				g.defined[i] = false
			}
		}

		// Zero out modified slots from previous run
		for _, id := range g.dirtyIDs {
			// Bounds check in case we shrank (unlikely) or id is weird
			if id < len(g.values) {
				g.values[id] = Value{}
				g.defined[id] = false
			}
		}
	}
	g.dirtyIDs = g.dirtyIDs[:0]
}

func (g *GlobalEnv) Get(name string) (Value, bool) {
	id, ok := g.symbols.GetID(name)
	if !ok || id >= len(g.values) {
		return Value{}, false
	}
	if !g.defined[id] {
		return Value{}, false
	}
	return g.values[id], true
}

func (g *GlobalEnv) GetByID(id int) (Value, bool) {
	if id < 0 || id >= len(g.values) {
		return Value{}, false
	}
	if !g.defined[id] {
		return Value{}, false
	}
	return g.values[id], true
}

func (g *GlobalEnv) Define(name string, v Value) {
	id := g.symbols.Resolve(name)
	g.ensureSize(id + 1)
	g.values[id] = v
	if !g.defined[id] {
		g.defined[id] = true
	}
	g.dirtyIDs = append(g.dirtyIDs, id)
}

func (g *GlobalEnv) DefineID(id int, v Value) {
	g.ensureSize(id + 1)
	g.values[id] = v
	if !g.defined[id] {
		g.defined[id] = true
	}
	g.dirtyIDs = append(g.dirtyIDs, id)
}

func (g *GlobalEnv) ensureSize(size int) {
	if size <= len(g.values) {
		return
	}

	// Calculate new capacity if expansion is needed
	if size > cap(g.values) {
		newCap := max(cap(g.values)*2, size)

		// Grow values
		newValues := make([]Value, size, newCap)
		copy(newValues, g.values)
		g.values = newValues

		// Grow defined
		newDefined := make([]bool, size, newCap)
		copy(newDefined, g.defined)
		g.defined = newDefined
	} else {
		// Capacity is sufficient, just extend slices
		oldLen := len(g.values)
		g.values = g.values[:size]
		g.defined = g.defined[:size]

		// Zero out newly exposed slots (crucial for reused envs)
		for i := oldLen; i < size; i++ {
			g.values[i] = Value{}
			g.defined[i] = false
		}
	}
}

// ToMap returns a copy of globals (compatibility helper)
func (g *GlobalEnv) ToMap() map[string]Value {
	// Optimization: Only iterate defined slots
	// We can iterate dirtyIDs? No, duplicates and incomplete.
	// Iterate defined array is still O(N) but fast bit/bool check.
	m := make(map[string]Value)
	for id, defined := range g.defined {
		if defined && id < len(g.values) {
			name, ok := g.symbols.GetName(id)
			if ok {
				m[name] = g.values[id]
			}
		}
	}
	return m
}
