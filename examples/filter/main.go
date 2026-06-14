package main

import (
	"context"
	"fmt"
	"time"

	"github.com/crgimenes/filo"
	"github.com/crgimenes/filo/filostrings"
)

// This example shows how Filo can be used for filtering data.
// Common use case: search filters, query builders, access control.

func main() {
	eng := filo.NewEngine()
	filostrings.RegisterBuiltins(eng)

	ctx := context.Background()
	cfg := filo.EvalConfig{StepLimit: 512, RecursionLimit: 32, Timeout: time.Second}

	// User-defined filter that can be stored in database
	// This simulates a user creating their own filter criteria
	filterScript := `
(let ()
  ; Define the filter predicate based on user criteria
  (def matches-filter (fn (item)
    (let ((name (nth item 0))
          (price (nth item 1))
          (category (nth item 2))
          (in-stock (nth item 3)))
      (and
        ; Price range filter
        (>= price min-price)
        (<= price max-price)
        ; Only in-stock items if required
        (or (not require-stock) in-stock)
        ; Category filter (empty = all categories)
        (or (is-empty filter-category) (= category filter-category))))))

  ; Filter the items
  (fold (fn (acc item)
          (if (matches-filter item)
              (list-append acc item)
              acc))
        (list)
        items))
`

	// Sample inventory data: [name, price, category, in-stock]
	items := filo.VList([]filo.Value{
		filo.VList([]filo.Value{filo.VString("Laptop"), filo.VNum(999), filo.VString("electronics"), filo.VBool(true)}),
		filo.VList([]filo.Value{filo.VString("Mouse"), filo.VNum(29), filo.VString("electronics"), filo.VBool(true)}),
		filo.VList([]filo.Value{filo.VString("Desk"), filo.VNum(299), filo.VString("furniture"), filo.VBool(false)}),
		filo.VList([]filo.Value{filo.VString("Chair"), filo.VNum(199), filo.VString("furniture"), filo.VBool(true)}),
		filo.VList([]filo.Value{filo.VString("Monitor"), filo.VNum(399), filo.VString("electronics"), filo.VBool(false)}),
		filo.VList([]filo.Value{filo.VString("Keyboard"), filo.VNum(79), filo.VString("electronics"), filo.VBool(true)}),
	})

	// Different filter scenarios
	filters := []map[string]filo.Value{
		{
			"items": items, "min-price": filo.VNum(0), "max-price": filo.VNum(100),
			"require-stock": filo.VBool(true), "filter-category": filo.VString(""),
		},
		{
			"items": items, "min-price": filo.VNum(100), "max-price": filo.VNum(500),
			"require-stock": filo.VBool(false), "filter-category": filo.VString("electronics"),
		},
		{
			"items": items, "min-price": filo.VNum(0), "max-price": filo.VNum(9999),
			"require-stock": filo.VBool(true), "filter-category": filo.VString("furniture"),
		},
	}

	filterNames := []string{
		"Under $100, in stock only",
		"Electronics $100-$500",
		"Furniture in stock",
	}

	for i, globals := range filters {
		fmt.Printf("=== Filter: %s ===\n", filterNames[i])

		result, _, err := eng.RunScript(ctx, filterScript, globals, cfg)
		if err != nil {
			panic(err)
		}

		resultList, err := result.AsList()
		if err != nil {
			panic(err)
		}
		if len(resultList) == 0 {
			fmt.Println("  No items match")
		}
		for _, item := range resultList {
			rec, err := item.AsList()
			if err != nil {
				panic(err)
			}
			name, err := rec[0].AsString()
			if err != nil {
				panic(err)
			}
			price, err := rec[1].AsNumber()
			if err != nil {
				panic(err)
			}
			fmt.Printf("  - %s ($%.0f)\n", name, price)
		}
		fmt.Println()
	}
}
