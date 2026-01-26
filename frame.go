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
func NewGlobalEnv(symbols *SymbolTable) *GlobalEnv {
	if symbols == nil {
		// Fallback or panic? For now, we assume Engine ALWAYS provides one.
		// Use empty one if nil to prevent crash
		symbols = NewSymbolTable()
	}
	size := symbols.Size()
	return &GlobalEnv{
		symbols: symbols,
		values:  make([]Value, size),
		defined: make([]bool, size),
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
	snapshot := g.symbols.Snapshot()
	m := make(map[string]Value, len(snapshot))
	for name, id := range snapshot {
		if id < len(g.values) && g.defined[id] {
			m[name] = g.values[id]
		}
	}
	return m
}
