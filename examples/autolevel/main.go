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
	cfg := filo.EvalConfig{StepLimit: 256, RecursionLimit: 32, Timeout: time.Second}

	script := `(let ()
  (def thresholds (list 0 300 900 2700 6500 15000))

  (def auto-level (fn (xp thresholds)
    (fold (fn (lvl threshold)
            (if (>= xp threshold) (+ lvl 1) lvl))
         0
         thresholds)))

  (def auto-level-progress (fn (xp thresholds)
    (let ((lvl (auto-level xp thresholds))
          (total (length thresholds)))
      (if (>= lvl total)
          (values lvl 0)
          (let ((next (nth thresholds lvl)))
            (values lvl (- next xp)))))))

  (auto-level-progress 1200 thresholds))`

	result, _, err := eng.RunScript(ctx, script, nil, cfg)
	if err != nil {
		panic(err)
	}

	tuple, err := result.AsTuple()
	if err != nil {
		panic(err)
	}

	level, err := tuple[0].AsNumber()
	if err != nil {
		panic(err)
	}

	remaining, err := tuple[1].AsNumber()
	if err != nil {
		panic(err)
	}

	fmt.Printf("Level: %.0f, XP to next: %.0f\n", level, remaining)
}

// Output: Level: 3, XP to next: 1500
