package filo

type Env struct {
	bind   map[string]Value
	parent *Env
}

func NewEnv() *Env {
	return &Env{bind: make(map[string]Value)}
}

func (e *Env) WithChild() *Env {
	return &Env{bind: make(map[string]Value), parent: e}
}

func (e *Env) Get(name string) (Value, bool) {
	for cur := e; cur != nil; cur = cur.parent {
		v, ok := cur.bind[name]
		if ok {
			return v, true
		}
	}
	return Value{}, false
}

func (e *Env) Define(name string, v Value) {
	e.bind[name] = v
}

func (e *Env) Assign(root *Env, name string, v Value) {
	for cur := e; cur != nil; cur = cur.parent {
		_, ok := cur.bind[name]
		if ok {
			cur.bind[name] = v
			return
		}
	}
	root.bind[name] = v
}
