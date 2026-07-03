// Command rules shows the control-flow and list features aimed at the
// "users write validation and business rules" use case: cond for multi-way
// branching, the filter/range list builtins, and error for a clear failure a
// host can surface.
package main

import (
	"context"
	"fmt"
	"time"

	"github.com/crgimenes/filo"
)

func main() {
	eng := filo.NewEngine()
	ctx := context.Background()
	cfg := filo.EvalConfig{StepLimit: 10000, RecursionLimit: 64, Timeout: time.Second}

	// A tier rule (cond), the adults from a sample (filter over range as ages),
	// and a guard that would fail loudly on bad input (error) — shown by
	// classifying a valid age rather than tripping it.
	script := `
(def tier (fn (score)
  (cond
    ((< score 0)   (error "score must be non-negative"))
    ((< score 50)  "bronze")
    ((< score 80)  "silver")
    (else          "gold"))))

(def adults (filter (fn (age) (>= age 18)) (range 15 22)))

(list (tier 20) (tier 65) (tier 90) adults)`

	result, _, err := eng.RunScript(ctx, script, nil, cfg)
	if err != nil {
		panic(err)
	}

	items, err := result.AsList()
	if err != nil {
		panic(err)
	}
	b, _ := items[0].AsString()
	s, _ := items[1].AsString()
	g, _ := items[2].AsString()
	ages, _ := items[3].AsList()
	nums := make([]int, len(ages))
	for i, a := range ages {
		n, _ := a.AsNumber()
		nums[i] = int(n)
	}
	fmt.Printf("tiers=%s,%s,%s adults=%v\n", b, s, g, nums)
}

// Output: tiers=bronze,silver,gold adults=[18 19 20 21]
