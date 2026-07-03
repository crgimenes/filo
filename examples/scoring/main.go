package main

import (
	"context"
	"fmt"
	"time"

	"github.com/crgimenes/filo"
	"github.com/crgimenes/filo/filomath"
	"github.com/crgimenes/filo/filostrings"
)

// This example shows how Filo can be used for scoring and ranking systems.
// Common use case: gamification, lead scoring, content ranking, risk assessment.

func main() {
	eng := filo.NewEngine()
	filostrings.RegisterBuiltins(eng)
	filomath.RegisterBuiltins(eng)

	ctx := context.Background()
	cfg := filo.EvalConfig{StepLimit: 512, RecursionLimit: 32, Timeout: time.Second}

	// A scoring formula for lead qualification
	// This could be edited by business users without code changes
	scoringScript := `
; Base score
(def base-score 0)

; Company size factor (0-30 points)
(def size-score
  (if (>= company-size 1000) 30
      (if (>= company-size 100) 20
          (if (>= company-size 10) 10 0))))

; Engagement factor (0-40 points)
(def engagement-score
  (+ (* page-views 0.5)
     (* email-opens 2)
     (* demo-requests 10)))

; Industry match (0-20 points)
(def industry-score
  (if (or (= industry "technology") (= industry "finance")) 20
      (if (or (= industry "healthcare") (= industry "retail")) 10 0)))

; Recency bonus (0-10 points)
(def recency-score
  (if (<= days-since-activity 7) 10
      (if (<= days-since-activity 30) 5 0)))

; Calculate total and grade
(def total (+ base-score size-score (math-min engagement-score 40) industry-score recency-score))
(def grade
  (if (>= total 80) "A"
      (if (>= total 60) "B"
          (if (>= total 40) "C" "D"))))

(values total grade)
`

	// Sample leads to score
	leads := []map[string]filo.Value{
		{
			"company-size": filo.VNum(500), "page-views": filo.VNum(50), "email-opens": filo.VNum(10),
			"demo-requests": filo.VNum(2), "industry": filo.VString("technology"), "days-since-activity": filo.VNum(3),
		},
		{
			"company-size": filo.VNum(25), "page-views": filo.VNum(10), "email-opens": filo.VNum(2),
			"demo-requests": filo.VNum(0), "industry": filo.VString("manufacturing"), "days-since-activity": filo.VNum(45),
		},
		{
			"company-size": filo.VNum(2000), "page-views": filo.VNum(100), "email-opens": filo.VNum(15),
			"demo-requests": filo.VNum(1), "industry": filo.VString("finance"), "days-since-activity": filo.VNum(14),
		},
	}

	leadNames := []string{"TechCorp", "SmallMfg", "BigBank"}

	fmt.Println("Lead Scoring Results")
	fmt.Println("====================")
	fmt.Println("Lead Name  | Score | Grade")
	fmt.Println("-----------|-------|------")

	for i, globals := range leads {
		result, _, err := eng.RunScript(ctx, scoringScript, globals, cfg)
		if err != nil {
			panic(fmt.Errorf("score %s: %w", leadNames[i], err))
		}

		tuple, err := result.AsTuple()
		if err != nil {
			panic(err)
		}
		score, err := tuple[0].AsNumber()
		if err != nil {
			panic(err)
		}
		grade, err := tuple[1].AsString()
		if err != nil {
			panic(err)
		}

		fmt.Printf("%-10s | %5.0f |   %s\n", leadNames[i], score, grade)
	}
}

// Output:
// Lead Scoring Results
// ====================
// Lead Name  | Score | Grade
// -----------|-------|------
// TechCorp   |    90 |   A
// SmallMfg   |    19 |   D
// BigBank    |    95 |   A
