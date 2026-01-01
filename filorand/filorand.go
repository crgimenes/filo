// Package filorand provides non-deterministic builtins for Filo.
// This includes random number generation and UUIDs.
// By design, these functions introduce side effects and external state dependencies.
package filorand

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/crgimenes/filo"
	"github.com/google/uuid"
)

var (
	rng   = rand.New(rand.NewSource(time.Now().UnixNano()))
	rngMu sync.Mutex
)

// RegisterRandomBuiltins adds random/UUID builtins to a Filo engine.
//
// Registered builtins:
//   - rand-float: Returns a random float in [0.0, 1.0)
//   - rand-int: Returns a random integer in [0, n)
//   - rand-seed: Seeds the random number generator
//   - uuid-v4: Returns a new random UUID v4 string
func RegisterRandomBuiltins(eng *filo.Engine) {
	eng.MustRegisterBuiltin("rand-float", builtinRandFloat)
	eng.MustRegisterBuiltin("rand-int", builtinRandInt)
	eng.MustRegisterBuiltin("rand-seed", builtinRandSeed)
	eng.MustRegisterBuiltin("uuid-v4", builtinUUIDv4)
}

func builtinRandFloat(_ context.Context, args []filo.Value) (filo.Value, error) {
	if len(args) != 0 {
		return filo.Value{}, fmt.Errorf("rand-float expects 0 arguments")
	}
	rngMu.Lock()
	v := rng.Float64()
	rngMu.Unlock()
	return filo.VNum(v), nil
}

func builtinRandInt(_ context.Context, args []filo.Value) (filo.Value, error) {
	if len(args) != 1 {
		return filo.Value{}, fmt.Errorf("rand-int expects 1 argument")
	}
	n, err := args[0].AsNumber()
	if err != nil {
		return filo.Value{}, err
	}
	if n <= 0 {
		return filo.Value{}, fmt.Errorf("rand-int expects positive number")
	}
	rngMu.Lock()
	v := rng.Intn(int(n))
	rngMu.Unlock()
	return filo.VNum(float64(v)), nil
}

func builtinRandSeed(_ context.Context, args []filo.Value) (filo.Value, error) {
	if len(args) != 1 {
		// If no args, seed with time
		rngMu.Lock()
		rng.Seed(time.Now().UnixNano())
		rngMu.Unlock()
		return filo.VList([]filo.Value{}), nil
	}

	// With 1 arg, use it as seed
	seed, err := args[0].AsNumber()
	if err != nil {
		return filo.Value{}, err
	}
	rngMu.Lock()
	rng.Seed(int64(seed))
	rngMu.Unlock()
	return filo.VList([]filo.Value{}), nil
}

func builtinUUIDv4(_ context.Context, args []filo.Value) (filo.Value, error) {
	if len(args) != 0 {
		return filo.Value{}, fmt.Errorf("uuid-v4 expects 0 arguments")
	}
	id := uuid.NewString()
	return filo.VString(id), nil
}
