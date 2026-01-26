package filo

// Frame represents a static lexical scope (activation record).
// It replaces the map-based Env for local variables.
type Frame struct {
	slots  []Value
	parent *Frame
}

// GlobalEnv represents the global environment (map-based).
// Unresolved symbols (globals) are looked up here.
// GlobalEnv represents the global environment using a SymbolTable and array storage.
type GlobalEnv struct {
	symbols *SymbolTable
	values  []Value
	defined []bool
}

// NewGlobalEnv creates a new global environment linked to a symbol table.
// It allocates backing storage based on symbol table size.
func NewGlobalEnv(symbols *SymbolTable) *GlobalEnv {
	if symbols == nil {
		symbols = NewSymbolTable()
	}
	size := symbols.Size()
	return &GlobalEnv{
		symbols: symbols,
		values:  make([]Value, size),
		defined: make([]bool, size),
	}
}

// Reset clears the global environment for reuse.
// It keeps the backing array but resets values and defined status.
func (g *GlobalEnv) Reset(symbols *SymbolTable) {
	requiredSize := symbols.Size()
	g.symbols = symbols

	// Resize if necessary
	if cap(g.values) < requiredSize {
		g.values = make([]Value, requiredSize)
		g.defined = make([]bool, requiredSize)
	} else {
		// Slice strictly to required size
		g.values = g.values[:requiredSize]
		g.defined = g.defined[:requiredSize]
		// Zero out memory
		for i := range g.values {
			g.values[i] = Value{}
			g.defined[i] = false
		}
	}
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
	g.defined[id] = true
}

func (g *GlobalEnv) DefineID(id int, v Value) {
	g.ensureSize(id + 1)
	g.values[id] = v
	g.defined[id] = true
}

func (g *GlobalEnv) ensureSize(size int) {
	if size > len(g.values) {
		newValues := make([]Value, size)
		copy(newValues, g.values)
		g.values = newValues

		newDefined := make([]bool, size)
		copy(newDefined, g.defined)
		g.defined = newDefined
	}
}

// ToMap returns a copy of globals (compatibility helper)
func (g *GlobalEnv) ToMap() map[string]Value {
	// Optimization: Only iterate defined slots
	m := make(map[string]Value)
	for id, defined := range g.defined {
		if defined && id < len(g.values) {
			if name, ok := g.symbols.GetName(id); ok {
				m[name] = g.values[id]
			}
		}
	}
	return m
}
