package main

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/crgimenes/filo"
	"github.com/crgimenes/filo/filostrings"
)

// This example shows how Filo can be used for data validation rules.
// Common use case: form validation, API input validation, business rules.

func main() {
	eng := filo.NewEngine()
	filostrings.RegisterBuiltins(eng)

	ctx := context.Background()
	cfg := filo.EvalConfig{StepLimit: 256, RecursionLimit: 16, Timeout: time.Second}

	// Validation rules written in Filo
	// These could be stored in a database or config file
	// #nosec G101 -- these are validation rule names and contain no credentials.
	rules := map[string]string{
		"email_format":    `(and (> (str-len email) 5) (str-find "@" email))`,
		"age_valid":       `(and (>= age 18) (<= age 120))`,
		"password_strong": `(>= (str-len password) 8)`,
		"username_valid":  `(and (>= (str-len username) 3) (<= (str-len username) 20))`,
	}

	// Deterministic order for output
	ruleNames := make([]string, 0, len(rules))
	for k := range rules {
		ruleNames = append(ruleNames, k)
	}
	sort.Strings(ruleNames)

	// Test data to validate
	testCases := []map[string]filo.Value{
		{"email": filo.VString("user@example.com"), "age": filo.VNum(25), "password": filo.VString("secret123"), "username": filo.VString("john")},
		{"email": filo.VString("bad"), "age": filo.VNum(15), "password": filo.VString("123"), "username": filo.VString("x")},
	}

	for i, data := range testCases {
		fmt.Printf("=== Test Case %d ===\n", i+1)
		for _, ruleName := range ruleNames {
			ruleScript := rules[ruleName]
			result, _, err := eng.RunScript(ctx, ruleScript, data, cfg)
			if err != nil {
				panic(fmt.Errorf("evaluate %s: %w", ruleName, err))
			}
			valid, err := result.AsBool()
			if err != nil {
				panic(fmt.Errorf("convert %s result: %w", ruleName, err))
			}
			status := "✓ PASS"
			if !valid {
				status = "✗ FAIL"
			}
			fmt.Printf("  %s: %s\n", ruleName, status)
		}
		fmt.Println()
	}
}

// Output:
// === Test Case 1 ===
//   age_valid: ✓ PASS
//   email_format: ✓ PASS
//   password_strong: ✓ PASS
//   username_valid: ✓ PASS
//
// === Test Case 2 ===
//   age_valid: ✗ FAIL
//   email_format: ✗ FAIL
//   password_strong: ✗ FAIL
//   username_valid: ✗ FAIL
