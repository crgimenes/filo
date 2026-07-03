// Example marshal demonstrates converting Go structs to Filo values and back.
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/crgimenes/filo"
)

// Config represents application configuration
type Config struct {
	Name    string `filo:"name"`
	Port    int    `filo:"port"`
	Debug   bool   `filo:"debug"`
	Retries int    `filo:"retries"`
}

func complexExample() {
	type Nested struct {
		Items []int             `filo:"items"`
		Data  map[string]string `filo:"data"`
	}
	type Complex struct {
		Name   string  `filo:"name"`
		Value  float64 `filo:"value"`
		Active bool    `filo:"active"`
		Nested Nested  `filo:"nested"`
	}

	c := Complex{
		Name:   "test",
		Value:  123.456,
		Active: true,
		Nested: Nested{
			Items: []int{1, 2, 3, 4, 5},
			Data:  map[string]string{"key1": "val1", "key2": "val2"},
		},
	}

	val, err := filo.Marshal(c)
	if err != nil {
		log.Fatalf("Marshal error: %v", err)
	}

	fmt.Printf("Filo value: %v\n", val)

	// MarshalIndent for pretty printing
	prettyVal, err := filo.MarshalIndent(c, "", "  ")
	if err != nil {
		log.Fatalf("MarshalIndent error: %v", err)
	}

	fmt.Printf("Pretty Filo value:\n%v\n", prettyVal)

	var c2 Complex
	err = filo.Unmarshal(val, &c2)
	if err != nil {
		log.Fatalf("Unmarshal error: %v", err)
	}

	var c3 Complex
	err = filo.Unmarshal(prettyVal, &c3)
	if err != nil {
		log.Fatalf("Unmarshal error: %v", err)
	}

	fmt.Printf("Go struct: %+v\n", c2)
	fmt.Printf("Go struct from pretty: %+v\n", c3)
}

func optionsExample() {
	// MarshalWithOptions exposes formatting control (prefix, indent).

	type Config struct {
		Name string `filo:"name"`
		Val  int    `filo:"value"`
	}
	cfg := Config{Name: "opt-test", Val: 42}

	opts := filo.MarshalOptions{
		Indent: "  ",
		Prefix: "> ", // Add a prefix to each line
	}

	s, err := filo.MarshalWithOptions(cfg, opts)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(s)
}

func mapExample() {
	// Example with map[string]interface{}
	data := map[string]any{
		"username": "admin",
		"password": "secret",
		"active":   true,
		"roles":    []string{"admin", "user"},
	}

	val, err := filo.Marshal(data)
	if err != nil {
		log.Fatalf("Marshal error: %v", err)
	}
	fmt.Printf("Filo value: %v\n", val)

	// MarshalIndent for pretty printing
	prettyVal, err := filo.MarshalIndent(data, "> ", "  ")
	if err != nil {
		log.Fatalf("MarshalIndent error: %v", err)
	}

	fmt.Printf("Pretty Filo value:\n%v\n", prettyVal)

	// Unmarshal back to map
	var data2 map[string]any
	err = filo.Unmarshal(val, &data2)
	if err != nil {
		log.Fatalf("Unmarshal error: %v", err)
	}
	fmt.Printf("Go map: %+v\n", data2)
}

func main() {
	// Example 1: Marshal Go struct to Filo Value
	fmt.Println("=== Marshal: Go struct -> Filo Value ===")
	cfg := Config{
		Name:    "my-app",
		Port:    8080,
		Debug:   true,
		Retries: 3,
	}

	val, err := filo.Marshal(cfg)
	if err != nil {
		log.Fatalf("Marshal error: %v", err)
	}
	fmt.Printf("Filo value: %v\n\n", val)

	// Example 2: Unmarshal Filo Value back to Go struct
	fmt.Println("=== Unmarshal: Filo Value -> Go struct ===")
	var cfg2 Config
	err = filo.Unmarshal(val, &cfg2)
	if err != nil {
		log.Fatalf("Unmarshal error: %v", err)
	}
	fmt.Printf("Go struct: %+v\n\n", cfg2)

	// Example 3: Create config in Filo and unmarshal to Go
	fmt.Println("=== Create config in Filo, with expressions ===")
	eng := filo.NewEngine()

	// Note: expressions are evaluated by Filo, not by Unmarshal
	// (list "port" (+ 8000 80)) -> port = 8080 (evaluated)
	// (list "name" "(+ 1 1)") -> name = "(+ 1 1)" (literal string)
	script := `
		(list
			(list "name" "filo-app")
			(list "port" (+ 8000 80))
			(list "debug" #f)
			(list "retries" (* 2 5)))
	`

	result, _, err := eng.RunScript(context.Background(), script, nil, filo.EvalConfig{})
	if err != nil {
		log.Fatalf("RunScript error: %v", err)
	}

	var cfg3 Config
	err = filo.UnmarshalFromValue(result, &cfg3)
	if err != nil {
		log.Fatalf("Unmarshal error: %v", err)
	}
	fmt.Printf("Config from Filo: %+v\n", cfg3)

	// Example 4: Complex struct with nested fields
	fmt.Println("\n=== Complex struct with nested fields ===")
	complexExample()

	// Example 5: Marshal with Options
	fmt.Println("\n=== Marshal with Options ===")
	optionsExample()

	// Example 6: Marshal and Unmarshal map[string]interface{}
	fmt.Println("\n=== Marshal and Unmarshal map[string]interface{} ===")
	mapExample()
}
