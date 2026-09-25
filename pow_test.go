package filo

import (
	"bufio"
	"math"
	"os"
	"strconv"
	"strings"
	"testing"
)

// testdata/pow.txt pins pow with an integral exponent to the double nearest
// the exact power, bit for bit, on every machine the tests run on; the C
// runtime reads the same file.
func TestPowIsTheSameEverywhere(t *testing.T) {
	f, err := os.Open("testdata/pow.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	n := 0
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		ab, _ := strconv.ParseUint(fields[0], 16, 64)
		e, _ := strconv.ParseInt(fields[1], 10, 64)
		want, _ := strconv.ParseUint(fields[2], 16, 64)
		got := math.Float64bits(powInt(math.Float64frombits(ab), float64(e)))
		if got != want && !(math.IsNaN(math.Float64frombits(got)) && math.IsNaN(math.Float64frombits(want))) {
			t.Fatalf("pow(%v, %d) = %v, want %v", math.Float64frombits(ab), e, math.Float64frombits(got), math.Float64frombits(want))
		}
		n++
	}
	if n < 1000 {
		t.Fatalf("only %d cases", n)
	}
}

// The edges, as IEEE 754's pow has them.
func TestPowEdges(t *testing.T) {
	inf, nan := math.Inf(1), math.NaN()
	for _, c := range []struct{ a, b, want float64 }{
		{2, 0, 1}, {nan, 0, 1}, {0, 3, 0}, {math.Copysign(0, -1), 3, math.Copysign(0, -1)},
		{math.Copysign(0, -1), 2, 0}, {0, -3, inf}, {math.Copysign(0, -1), -3, -inf},
		{inf, 3, inf}, {-inf, 3, -inf}, {-inf, 2, inf}, {inf, -1, 0}, {-inf, -1, math.Copysign(0, -1)},
		{-1, 1 << 60, 1}, {-1, 1<<53 + 1, 1}, {2, 1e300, inf}, {0.5, 1e300, 0}, {2, -1e300, 0},
		{-2, -1075, math.Copysign(0, -1)}, {99, 82, 4.3861750180991106e+163},
	} {
		got := powInt(c.a, c.b)
		if math.Float64bits(got) != math.Float64bits(c.want) {
			t.Errorf("pow(%v, %v) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
	if !math.IsNaN(powInt(nan, 2)) {
		t.Error("pow(NaN, 2) is not NaN")
	}
}
