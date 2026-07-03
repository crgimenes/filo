// Package filorand provides non-deterministic builtins for Filo.
// This includes random number generation and UUIDs.
// By design, these functions introduce side effects and external state dependencies.
package filorand

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math"
	"math/big"

	"github.com/crgimenes/filo"
)

// RegisterBuiltins adds random/UUID builtins to a Filo engine.
//
// Registered builtins:
//   - rand-float: Returns a random float in [0.0, 1.0)
//   - rand-int: Returns a random integer in [0, n)
//   - uuid-v4: Returns a new random UUID v4 string
func RegisterBuiltins(eng *filo.Engine) {
	eng.MustRegisterBuiltin("rand-float", builtinRandFloat)
	eng.MustRegisterBuiltin("rand-int", builtinRandInt)
	eng.MustRegisterBuiltin("uuid-v4", builtinUUIDv4)
}

func builtinRandFloat(_ context.Context, args []filo.Value) (filo.Value, error) {
	if len(args) != 0 {
		return filo.Value{}, fmt.Errorf("rand-float expects 0 arguments")
	}
	var b [8]byte
	_, err := rand.Read(b[:])
	if err != nil {
		return filo.Value{}, fmt.Errorf("rand-float entropy error: %w", err)
	}
	// Use upper 53 bits to mirror math/rand Float64 distribution
	n := binary.BigEndian.Uint64(b[:]) >> 11
	v := float64(n) / (1 << 53)
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
	if n > math.MaxInt64 {
		return filo.Value{}, fmt.Errorf("rand-int expects number <= %d", int64(math.MaxInt64))
	}

	max := big.NewInt(int64(n))
	v, err := rand.Int(rand.Reader, max)
	if err != nil {
		return filo.Value{}, fmt.Errorf("rand-int entropy error: %w", err)
	}
	return filo.VNum(float64(v.Int64())), nil
}

func builtinUUIDv4(_ context.Context, args []filo.Value) (filo.Value, error) {
	if len(args) != 0 {
		return filo.Value{}, fmt.Errorf("uuid-v4 expects 0 arguments")
	}
	id, err := newUUIDv4()
	if err != nil {
		return filo.Value{}, fmt.Errorf("uuid-v4 entropy error: %w", err)
	}
	return filo.VString(id), nil
}

// newUUIDv4 generates a random UUID v4 string.
// Format: xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx
// where x is any hex digit and y is one of 8, 9, a, or b.
func newUUIDv4() (string, error) {
	var b [16]byte
	_, err := rand.Read(b[:])
	if err != nil {
		return "", err
	}

	// Set version (4) in byte 6
	b[6] = (b[6] & 0x0f) | 0x40
	// Set variant (10) in byte 8
	b[8] = (b[8] & 0x3f) | 0x80

	// Format as standard UUID string
	var buf [36]byte
	hex.Encode(buf[0:8], b[0:4])
	buf[8] = '-'
	hex.Encode(buf[9:13], b[4:6])
	buf[13] = '-'
	hex.Encode(buf[14:18], b[6:8])
	buf[18] = '-'
	hex.Encode(buf[19:23], b[8:10])
	buf[23] = '-'
	hex.Encode(buf[24:36], b[10:16])

	return string(buf[:]), nil
}
