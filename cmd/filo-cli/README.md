# filo-cli

Command-line tool for running Filo scripts.

## Building

```bash
go build -o filo-cli ./cmd/filo-cli
```

## Usage

```bash
filo-cli [options]
```

### Basic Usage

Simple scripts run without any database configuration:

```bash
# Simple expression via stdin
echo '(+ 1 2 3)' | ./filo-cli
# Output: 6

# With math package
echo '(sqrt 16)' | ./filo-cli --filo-package math
# Output: 4

# From file
./filo-cli --script-file calculate.filo
```

## Flags

### Script Input

| Flag | Default | Description |
|------|---------|-------------|
| `--script-file` | (stdin) | Path to Filo script file |

### Extension Packages

| Flag | Default | Description |
|------|---------|-------------|
| `--filo-package` | (none) | Comma-separated list: `math` |

### Execution Limits

| Flag | Default | Description |
|------|---------|-------------|
| `--step-limit` | 100000 | Maximum evaluation steps |
| `--recursion-limit` | 128 | Maximum recursion depth |
| `--timeout` | 30 | Timeout in seconds |

## Extension Packages

| Package | Description |
|---------|-------------|
| `math` | Advanced math: sqrt, sin, cos, log, etc. |

## Examples

### Pure Filo (No Dependencies)

```bash
# Arithmetic
echo '(* 2 (+ 3 4))' | ./filo-cli
# Output: 14

# Conditionals
echo '(if (> 10 5) "yes" "no")' | ./filo-cli
# Output: yes

# Lists
echo '(head (list 1 2 3))' | ./filo-cli
# Output: 1
```

### With Math Package

```bash
# Trigonometry
echo '(sin (/ (pi) 2))' | ./filo-cli --filo-package math
# Output: 1

# Pythagorean theorem
echo '(sqrt (+ (pow 3 2) (pow 4 2)))' | ./filo-cli --filo-package math
# Output: 5

# Rounding
echo '(round 3.7)' | ./filo-cli --filo-package math
# Output: 4
```

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | Error |

## Error Handling

Errors are printed to stderr:

```bash
echo '(undefined-function)' | ./filo-cli
# stderr: error: script execution failed: unknown symbol: undefined-function
# exit code: 1
```
