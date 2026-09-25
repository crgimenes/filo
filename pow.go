package filo

import "math"

// powInt is a raised to an integral power, the same double on every machine
// and in the C runtime, which computes it step for step the same way
// (pow_int in filo.c). The C library's pow differs between machines and
// Go's is not correctly rounded; a program is the same bytes everywhere only
// if its folded constants are, and the same result only if pow is.
//
// The mantissa is raised in double-double (a pair of doubles, about 106
// bits) by squaring, with the exponent kept apart in an integer, so no step
// leaves the range of the doubles; the power of two is put back once, at the
// end, with one rounding. Every product is rounded where it is written
// (float64 of it): Go may fuse a multiply and an add, which rounds once
// where these steps round twice, and the C runtime does not fuse.
func powInt(a, b float64) float64 {
	if b == 0 {
		return 1
	}
	if math.IsNaN(a) {
		return a
	}
	neg := b < 0
	n := math.Abs(b)
	odd := n < 1<<53 && math.Mod(n, 2) == 1 // past 2^53 every double is even
	if n > 1<<62 {
		n = 1 << 62 // as large and even: the result is 0, 1 or +Inf all the same
	}
	if a == 0 || math.IsInf(a, 0) {
		return powEdge(a, neg, odd)
	}
	m, k := frexp(math.Abs(a)) // |a| = m·2^k, m in [0.5, 1)
	rh, rl, rE := 1.0, 0.0, 0
	bh, bl, bE := m, 0.0, k
	for e := uint64(n); ; {
		if e&1 != 0 {
			rh, rl = ddMul(rh, rl, bh, bl)
			rh, rl, rE = ddNorm(rh, rl, rE+bE)
		}
		e >>= 1
		if e == 0 {
			break
		}
		if bE > 4096 || bE < -4096 {
			// every factor is a power of |a|, all above 1 or all below: one
			// this far out decides the result, past any double
			rE = bE
			break
		}
		bh, bl = ddMul(bh, bl, bh, bl)
		bh, bl, bE = ddNorm(bh, bl, 2*bE)
	}
	if neg {
		rh, rl = ddRecip(rh, rl)
		rh, rl, rE = ddNorm(rh, rl, -rE)
	}
	r := scale2(rh+rl, rE)
	if a < 0 && odd {
		return -r
	}
	return r
}

// powEdge is ±0 or ±Inf raised to an integral power, as IEEE 754's pow.
func powEdge(a float64, neg, odd bool) float64 {
	zero := a == 0
	if neg {
		zero = !zero
	}
	r := math.Inf(1)
	if zero {
		r = 0
	}
	if odd && math.Signbit(a) {
		return -r
	}
	return r
}

// ddMul multiplies two double-doubles.
func ddMul(xh, xl, yh, yl float64) (float64, float64) {
	p, e := twoProd(xh, yh)
	e += float64(xh*yl) + float64(xl*yh)
	return twoSum(p, e)
}

// ddRecip is 1/(h+l): the quotient of the high part, corrected by its
// remainder.
func ddRecip(h, l float64) (float64, float64) {
	q := 1 / h
	p, pe := twoProd(q, h)
	rem := ((1 - p) - pe) - float64(q*l)
	return twoSum(q, rem/h)
}

// ddNorm brings h into [0.5, 1), l with it, and E by as much.
func ddNorm(h, l float64, e int) (float64, float64, int) {
	f, k := frexp(h)
	return f, float64(l * pow2(-k)), e + k
}

// twoProd is a*b rounded, and what the rounding lost (Dekker's, without an
// fma). The operands here are mantissas, near 1: nothing overflows.
func twoProd(a, b float64) (float64, float64) {
	p := float64(a * b)
	ah, al := split(a)
	bh, bl := split(b)
	e := ((float64(ah*bh) - p) + float64(ah*bl) + float64(al*bh)) + float64(al*bl)
	return p, e
}

// split is a as two halves of 26 bits, whose products are exact.
func split(a float64) (float64, float64) {
	c := float64(134217729 * a) // 2^27 + 1
	h := c - (c - a)
	return h, a - h
}

// twoSum is a+b rounded, and what the rounding lost.
func twoSum(a, b float64) (float64, float64) {
	s := a + b
	bb := s - a
	return s, (a - (s - bb)) + (b - bb)
}

// frexp is x = f·2^e with f in [0.5, 1), for a finite x other than 0; from
// the bits, as the C runtime does it without a C library.
func frexp(x float64) (float64, int) {
	bits := math.Float64bits(x)
	exp := int(bits>>52) & 0x7FF
	adjust := 0
	if exp == 0 { // subnormal: made normal first, exactly
		x *= pow2(54)
		bits = math.Float64bits(x)
		exp = int(bits>>52) & 0x7FF
		adjust = -54
	}
	f := math.Float64frombits(bits&^(0x7FF<<52) | 1022<<52)
	return f, exp - 1022 + adjust
}

// pow2 is 2^k, for k from -1022 to 1023.
func pow2(k int) float64 {
	return math.Float64frombits(uint64(k+1023) << 52) // #nosec G115 -- k+1023 is from 1 to 2046
}

// scale2 is s·2^e for s in [0.5, 1], rounded once: a power of two within
// the normal doubles scales exactly, and only the last step can round.
func scale2(s float64, e int) float64 {
	switch {
	case e > 2046:
		return math.Inf(1)
	case e > 1023:
		return float64(s*pow2(1023)) * pow2(e-1023)
	case e >= -1022:
		return float64(s * pow2(e))
	case e >= -2022:
		return float64(s*pow2(-1000)) * pow2(e+1000)
	}
	return 0
}

// integral says whether b is a whole number (every double past 2^52 is).
func integral(b float64) bool {
	if math.IsInf(b, 0) || math.IsNaN(b) {
		return false
	}
	return math.Abs(b) >= 1<<52 || b == math.Trunc(b)
}
