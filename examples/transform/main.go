package main

import (
	"context"
	"fmt"
	"time"

	"github.com/crgimenes/filo"
)

// This example shows how Filo can be used for data transformation pipelines.
// Common use case: ETL, data normalization, format conversion.

func main() {
	eng := filo.NewEngine()
	filo.RegisterStringBuiltins(eng)

	ctx := context.Background()
	cfg := filo.EvalConfig{StepLimit: 512, RecursionLimit: 32, Timeout: time.Second}

	// A transformation pipeline that processes a list of records
	transformScript := `
(let ()
  ; Helper to transform a single record
  (def transform-record (fn (record)
    (let ((name (nth record 0))
          (age (nth record 1))
          (salary (nth record 2)))
      (list
        (str-upper name)           ; Normalize name to uppercase
        age
        (* salary 1.1)             ; Apply 10% raise
        (if (>= age 65) "senior" 
            (if (>= age 30) "mid" "junior"))))))  ; Add category

  ; Apply transformation to all records
  (map transform-record records))
`

	// Input data: list of [name, age, salary]
	records := filo.VList([]filo.Value{
		filo.VList([]filo.Value{filo.VString("alice"), filo.VNum(28), filo.VNum(50000)}),
		filo.VList([]filo.Value{filo.VString("bob"), filo.VNum(45), filo.VNum(75000)}),
		filo.VList([]filo.Value{filo.VString("carol"), filo.VNum(67), filo.VNum(60000)}),
	})

	globals := map[string]filo.Value{"records": records}

	result, _, err := eng.RunScript(ctx, transformScript, globals, cfg)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Println("Transformed Records:")
	fmt.Println("Name       | Age | Salary   | Category")
	fmt.Println("-----------|-----|----------|--------")

	resultList, _ := result.AsList()
	for _, record := range resultList {
		rec, _ := record.AsList()
		name, _ := rec[0].AsString()
		age, _ := rec[1].AsNumber()
		salary, _ := rec[2].AsNumber()
		category, _ := rec[3].AsString()
		fmt.Printf("%-10s | %3.0f | %8.0f | %s\n", name, age, salary, category)
	}
}

// Output:
// Transformed Records:
// Name       | Age | Salary   | Category
// -----------|-----|----------|--------
// ALICE      |  28 |    55000 | junior
// BOB        |  45 |    82500 | mid
// CAROL      |  67 |    66000 | senior
