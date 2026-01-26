package filo

import (
	"sync"
)

// SymbolTable manages the mapping between global variable names and their integer IDs.
// It is thread-safe to allow concurrent compilation/execution sharing the same engine state.
type SymbolTable struct {
	mu      sync.RWMutex
	symbols map[string]int
	nextID  int
}

// NewSymbolTable creates a new empty SymbolTable.
func NewSymbolTable() *SymbolTable {
	return &SymbolTable{
		symbols: make(map[string]int),
		nextID:  0,
	}
}

// Resolve returns the ID for a given symbol name at a specified depth (global is usually 0 if used directly,
// but here "Resolve" just gets the ID in the table).
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

// Snapshot returns a copy of the current symbol map.
func (st *SymbolTable) Snapshot() map[string]int {
	st.mu.RLock()
	defer st.mu.RUnlock()
	snap := make(map[string]int, len(st.symbols))
	for k, v := range st.symbols {
		snap[k] = v
	}
	return snap
}

// Size returns the number of symbols currently registered.
func (st *SymbolTable) Size() int {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return st.nextID
}
