package filo

import (
	"maps"
	"sync"
)

// SymbolTable manages the mapping between global variable names and their integer IDs.
// It is thread-safe to allow concurrent compilation/execution sharing the same engine state.
type SymbolTable struct {
	mu       sync.RWMutex
	symbols  map[string]int
	idToName []string
	nextID   int
}

// NewSymbolTable creates a new empty SymbolTable.
func NewSymbolTable() *SymbolTable {
	return &SymbolTable{
		symbols:  make(map[string]int),
		idToName: make([]string, 0),
		nextID:   0,
	}
}

// Resolve returns the ID for a given symbol name.
// If the symbol does not exist, it defines it and assigns a new ID.
func (st *SymbolTable) Resolve(name string) int {
	st.mu.RLock()
	id, ok := st.symbols[name]
	st.mu.RUnlock()
	if ok {
		return id
	}

	st.mu.Lock()
	defer st.mu.Unlock()

	// Double check
	if id, ok := st.symbols[name]; ok {
		return id
	}

	id = st.nextID
	st.symbols[name] = id
	st.idToName = append(st.idToName, name)
	st.nextID++
	return id
}

// GetID returns the ID for a name if it exists, and false otherwise.
func (st *SymbolTable) GetID(name string) (int, bool) {
	st.mu.RLock()
	defer st.mu.RUnlock()
	id, ok := st.symbols[name]
	return id, ok
}

// GetName returns the name for a given ID if it exists.
func (st *SymbolTable) GetName(id int) (string, bool) {
	st.mu.RLock()
	defer st.mu.RUnlock()
	if id < 0 || id >= len(st.idToName) {
		return "", false
	}
	return st.idToName[id], true
}

// Snapshot returns a copy of the current symbol map.
func (st *SymbolTable) Snapshot() map[string]int {
	st.mu.RLock()
	defer st.mu.RUnlock()
	snap := make(map[string]int, len(st.symbols))
	maps.Copy(snap, st.symbols)
	return snap
}

// Size returns the number of symbols currently registered.
func (st *SymbolTable) Size() int {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return st.nextID
}
