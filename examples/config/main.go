package main

import (
	"context"
	"fmt"
	"time"

	"github.com/crgimenes/filo"
	"github.com/crgimenes/filo/filostrings"
)

// This example shows how Filo can be used for dynamic configuration.
// Common use case: environment-specific settings, feature flags, A/B testing.

func main() {
	eng := filo.NewEngine()
	filostrings.RegisterBuiltins(eng)

	ctx := context.Background()
	cfg := filo.EvalConfig{StepLimit: 256, RecursionLimit: 16, Timeout: time.Second}

	// Configuration script that adapts based on environment
	configScript := `
(do
  ; Base configuration
  (set api-url "http://localhost:3000")
  (set debug-mode #t)
  (set max-connections 10)
  (set cache-ttl 60)

  ; Environment-specific overrides
  (if (= env "production")
    (do
      (set api-url "https://api.example.com")
      (set debug-mode #f)
      (set max-connections 100)
      (set cache-ttl 3600)))

  (if (= env "staging")
    (do
      (set api-url "https://staging-api.example.com")
      (set max-connections 50)))

  ; Feature flags based on user tier
  (set feature-advanced (or (= tier "pro") (= tier "enterprise")))
  (set feature-api-access (= tier "enterprise"))

  ; Return computed config as a list of key-value pairs
  (list
    (list "api-url" api-url)
    (list "debug-mode" debug-mode)
    (list "max-connections" max-connections)
    (list "cache-ttl" cache-ttl)
    (list "feature-advanced" feature-advanced)
    (list "feature-api-access" feature-api-access)))
`

	environments := []map[string]filo.Value{
		{"env": filo.VString("development"), "tier": filo.VString("free")},
		{"env": filo.VString("production"), "tier": filo.VString("pro")},
		{"env": filo.VString("staging"), "tier": filo.VString("enterprise")},
	}

	for _, globals := range environments {
		envName, _ := globals["env"].AsString()
		tierName, _ := globals["tier"].AsString()
		fmt.Printf("=== Environment: %s, Tier: %s ===\n", envName, tierName)

		result, _, err := eng.RunScript(ctx, configScript, globals, cfg)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			continue
		}

		configList, _ := result.AsList()
		for _, item := range configList {
			pair, _ := item.AsList()
			key, _ := pair[0].AsString()
			fmt.Printf("  %s = %s\n", key, pair[1].String())
		}
		fmt.Println()
	}
}
