package filo

import "maps"

const smallEnvSize = 8

type entry struct {
	key string
	val Value
}

type Env struct {
	smallBind [smallEnvSize]entry
	smallLen  int
	largeBind map[string]Value
	parent    *Env
}

func NewEnv() *Env {
	return &Env{}
}

func (e *Env) WithChild() *Env {
	return &Env{parent: e}
}

func (e *Env) Get(name string) (Value, bool) {
	for cur := e; cur != nil; cur = cur.parent {
		// Check large map first if initialized
		if cur.largeBind != nil {
			v, ok := cur.largeBind[name]
			if ok {
				return v, true
			}
			// If strict usage is preferred, we could skip smallBind if largeBind exists,
			// but here largeBind is promoted, so smallBind entries are moved there.
			// However, to keep it robust:
			// "strictly use largeBind thereafter" logic implies smallBind is empty or ignored after promotion.
			// Let's assume if largeBind != nil, IT IS data source.
			continue
		}

		// Check small array
		for i := 0; i < cur.smallLen; i++ {
			if cur.smallBind[i].key == name {
				return cur.smallBind[i].val, true
			}
		}
	}
	return Value{}, false
}

func (e *Env) Define(name string, v Value) {
	// If large map exists, use it
	if e.largeBind != nil {
		e.largeBind[name] = v
		return
	}

	// Check if update existing in small env
	for i := 0; i < e.smallLen; i++ {
		if e.smallBind[i].key == name {
			e.smallBind[i].val = v
			return
		}
	}

	// If space likely available in small env
	if e.smallLen < smallEnvSize {
		e.smallBind[e.smallLen] = entry{key: name, val: v}
		e.smallLen++
		return
	}

	// Promote to large map
	e.largeBind = make(map[string]Value, smallEnvSize+4) // a bit of extra space
	for i := 0; i < e.smallLen; i++ {
		e.largeBind[e.smallBind[i].key] = e.smallBind[i].val
	}
	// Add the new one
	e.largeBind[name] = v
	// Clear smallBind just to be safe/clean? Not strictly necessary for GC if Env dies,
	// but good practice if Env lives long. But Env is struct value? Env is pointer *Env.
	// smallBind is array of values. Value might contain pointers.
	// To help GC, we could zero out smallBind, but let's delay micro-opt and trust promotion logic.
	// Actually, clearing smallLen = 0 is enough logic-wise.
	// But resetting values helps GC.
	var zeroVal Value
	for i := 0; i < e.smallLen; i++ {
		e.smallBind[i].val = zeroVal
	}
	e.smallLen = 0
}

func (e *Env) Assign(root *Env, name string, v Value) {
	for cur := e; cur != nil; cur = cur.parent {
		if cur.largeBind != nil {
			if _, ok := cur.largeBind[name]; ok {
				cur.largeBind[name] = v
				return
			}
			continue
		}

		for i := 0; i < cur.smallLen; i++ {
			if cur.smallBind[i].key == name {
				cur.smallBind[i].val = v
				return
			}
		}
	}
	root.Define(name, v)
}

// ToMap returns a copy of all variables in this environment scope (excluding parent).
// Useful for creating a map of new globals.
func (e *Env) ToMap() map[string]Value {
	if e.largeBind != nil {
		// Return copy of large map
		m := make(map[string]Value, len(e.largeBind))
		maps.Copy(m, e.largeBind)
		return m
	}

	// Convert small bind to map
	m := make(map[string]Value, e.smallLen)
	for i := 0; i < e.smallLen; i++ {
		m[e.smallBind[i].key] = e.smallBind[i].val
	}
	return m
}
