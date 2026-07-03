# filo-repl

Interactive REPL (Read-Evaluate-Print Loop) for the Filo language.

## Features

- **Interactive mode**: Line editing, command history, multi-line input
- **Batch mode**: Pipe-friendly execution when stdin is not a TTY
- **Multi-line expressions**: Automatically continues reading until parentheses are balanced
- **Editor integration**: `.edit` (or `.e`) opens `$EDITOR` to edit complex expressions
- **Extension packages**: Load math, rand, string, or print builtins

## Building

```bash
go build -o filo-repl ./cmd/filo-repl
```

## Usage

### Interactive REPL

```bash
./filo-repl
```

You'll see:

```
Filo REPL - Type expressions to evaluate. Ctrl+D to exit.

filo> (+ 1 2)
3
filo> (if (> 10 5)
...     "yes"
...     "no")
yes
filo>
```

### Batch Mode (Pipes)

```bash
echo '(+ 1 2 3)' | ./filo-repl
# Output: 6

cat script.filo | ./filo-repl --filo-package math
```

### With Extension Packages

```bash
# Math functions
./filo-repl --filo-package math
filo> (sqrt 16)
4

# String functions
./filo-repl --filo-package str
filo> (str-upper "hello")
HELLO

# Multiple packages
./filo-repl --filo-package math,str,rand
```

## REPL Commands

| Command | Description |
|---------|-------------|
| `.help` | Show help in pager ($PAGER or less) |
| `.edit` | Open $EDITOR to edit input |
| `.clear` | Clear current input buffer |
| `.quit` | Exit REPL (also: `.exit`, `.q`) |

## Keyboard Shortcuts

| Key | Action |
|-----|--------|
| Enter | Execute if balanced, continue if not |
| Ctrl+D | Exit (empty) or force execute (with content) |
| Up/Down | Navigate command history |

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--filo-package` | (none) | Comma-separated: `math`, `rand`, `str`, `print`, `json` |
| `--step-limit` | 100000 | Maximum evaluation steps |
| `--recursion-limit` | 128 | Maximum recursion depth |
| `--timeout` | 30 | Timeout in seconds |
| `--fold-const` | false | Fold constant expressions as you type |

## Extension Packages

| Package | Description |
|---------|-------------|
| `math` | sqrt, sin, cos, log, abs, floor, ceil, round, etc. |
| `rand` | rand-float, rand-int, uuid-v4 |
| `str` | str-upper, str-lower, str-trim, str-split, str-join, etc. |
| `print` | print, println, printf |
| `json` | json-marshal, json-unmarshal, json-null |

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | Error |
