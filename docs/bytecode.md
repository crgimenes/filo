# Filo bytecode — a program as a file

The IR in [ir.md](ir.md) is a tree that lives in the memory of the runtime
that lowered it: 72 bytes a node on a 32-bit target, 16 times the source.
A small machine cannot hold that, and does not need to — it can run code it
did not compile. This document is the format that makes that possible: a
compiler turns the IR into a flat stream of instructions for a stack
machine, and a VM runs the stream in place, from wherever it is stored
(flash, a file, a browser's storage). The machine that runs a program needs
the VM and the builtins the program imports; it never needs the parser.

Both runtimes compile to bytecode and run it. The Go engine compiles
(`Engine.Build`, `StripDebug`, `BuildBundle`) to the same bytes the C
compiler writes for the same program, loads a unit (`Engine.LoadUnit`,
`Engine.LoadBundle`) and runs its entries (`Unit.Run`) on a machine of its
own, and the language does not change: a program compiled here gives the
same value, the same error or success and the same globals as the IR
evaluated by either runtime. The corpus and the Prolog oracle run through
the VM to hold it to that, and every unit they compile to is compiled
again by the Go engine from the same source and must be the same bytes,
and runs again on the Go machine, held to the same result, error and place
(`make govm` in the C repository).

## What differs from the IR

- **Steps.** A step is one executed instruction, not one IR node, so the
  count differs from [ir.md](ir.md)'s. What stays is the purpose: a runaway
  program stops. The corpus keeps wide margins on its step limits, and the
  counts are close (0.93 instructions per IR node, measured statically).
- **Error context.** Messages are the IR's, but the chain of forms around
  them (`in if: in let: …`) is not rebuilt; builtin failures keep
  `in builtin "<name>": `. The corpus asserts that an error occurred, never
  its wording.
- **Line and column** of an error come from the debug section, where the IR
  has them in its nodes; `filo_error_at` gives both, and the corpus holds
  the VM and the IR to the same place for every error but a step limit.
- **When a call is checked.** The IR checks that the callee is a function,
  its arity and the recursion depth before it evaluates the arguments; on a
  stack machine the arguments are already evaluated when `CALL` runs. The
  outcome is the same — the run fails and leaves no globals behind — and
  only a failing call with an argument that has an effect outside the run
  (a builtin that draws or prints) can tell the two apart.

## Units

A **unit** is what shares one set of globals: for a screen of the board,
its common file, its init and its hooks. It holds any number of entry
points (exports) — each program compiled into it is one, with a name — and
the functions they create.

Everything a unit refers to outside itself is by name, resolved when it is
loaded against whatever the loading VM has, wherever a function comes from
— the core, a library in C, the host, or Filo the VM loaded itself:

- **Imports** are the functions the unit calls that were builtins where it
  was compiled, the core ones included, so that adding a builtin to the
  core never renumbers anything. Each resolves to a builtin of that name,
  or else to a global holding a function (the loading VM has it in Filo);
  `CALLB` calls either, and `PUSH_B` pushes either as a value.
- **Globals** are names, resolved to the loading context's symbol table
  (created when the table is open).
- **Externs** are the globals the unit reads and never writes: a value the
  host sets (`W`, `KEY`), or a function the unit did not define. Each must
  be held by the loading context, or be a builtin of that name, which the
  load binds to the global as a function value (the unit was compiled where
  the name was a function in Filo).

What the loading context lacks — an import, an extern, or any global when
its table is sealed — refuses the load, all of it in one message, as many
names as it holds: `missing (3): fg bg W`. The imports and the externs are
therefore the unit's requirements, listed by `filo dump`: a VM runs the
unit when it provides them, whatever else it lacks. A lazy load
(`filo_bc_load_lazy`) leaves an extern the context does not hold to fail
when it is read, as the interpreter fails; the corpus and the fuzzers load
that way, programs being installed do not. The Go engine is handed its
globals when it runs, not when it loads: `Unit.Run` refuses a missing
import as a load does and leaves a missing extern to fail when read, and
`Unit.Missing` lists both for a host that refuses on either.

Inside the unit everything is an index: constants, globals, imports,
functions.

## File layout

All integers are little-endian. Counts, indices and lengths inside sections
are ULEB128 (7 bits a byte, low first, high bit set on every byte but the
last).

    offset  size  field
    0       4     magic 7F 46 42 43 ("\x7fFBC": the first byte is not ASCII,
                  so no source text can start this way)
    4       1     kind: 1 = unit (2 = bundle, below)
    5       1     format version: 1
    6       2     header size, in bytes; a reader skips what it does not know
    8       4     FNV-1a 32 of the whole file, computed with this field zero
    12      2     widest operand stack of any function, in values
    14      2     widest frame of any function, in slots
    16      2     number of sections
    18      2     reserved, zero
    20      12×n  section table: kind (u16), reserved (u16), offset (u32),
                  length (u32) — offsets from the start of the file

The first 20 bytes are the fixed header; the section table follows it and
the header size covers both.

Sections, each at most once; unknown kinds are skipped:

| kind | name | content |
|---:|---|---|
| 1 | imports | count, then names (length, bytes) |
| 2 | globals | count, then names |
| 3 | constants | count, then entries: tag byte, payload |
| 4 | functions | count, then per function: code offset, code length, params, frame slots, max stack |
| 5 | code | the instruction bytes of every function |
| 6 | exports | count, then per export: name, function index |
| 7 | debug | where each instruction came from in the source (below); optional |
| 8 | externs | count, then indices into the globals, rising: the ones the unit reads and never writes; optional |

Constant tags: 1 number (8 bytes, IEEE 754 double), 2 string (length,
bytes), 3 true, 4 false, 5 empty list. A NaN is written as
0x7FF8000000000000 whatever NaN the compiler's arithmetic gave (the C
library's differs between machines, Go's differs from it), so the same
program is the same bytes everywhere.

A function's code runs from its offset for its length inside the code
section; a jump or a fall-through that leaves that range is an error. Frame
slots are the parameters first, then one slot for every `let`/`letv`
binding in the function, never reused: a closure created in one `let`
keeps seeing that `let`'s slot, as it keeps seeing that `let`'s frame in
the IR.

## Debug

Section 7 maps instructions back to the source: a line and a column (from
1, the column in bytes, as parse errors count them) for every instruction
where they change, in code order. Each entry is three ULEB128 numbers: the
distance in bytes from the previous entry's pc (the first from 0), the line
as a signed difference from the previous entry's (zigzag: 0, -1, 1, -2, 2 …
written 0, 1, 2, 3, 4 …) and the column as it is. The position of any pc is
that of the last entry at or before it.

The runtime reads it only when an error needs a place (`filo_error_at`); a
unit without it runs the same and its errors have no place. `filo_bc_strip`
writes a unit without it, every other byte kept. It costs a quarter of a
small program's unit and two fifths of the board's screens, whose
instructions change place almost one by one.

## Bundles

A **bundle** is several units in one file, each whole and named: a program
that travels as one file however many units it has (the screens of a board,
one unit each, since each has its own globals). It is the `.jar` to the
unit's `a.out`, from the same family of signatures.

    offset  size  field
    0       4     magic 7F 46 42 43, as a unit's
    4       1     kind: 2 = bundle
    5       1     format version: 1
    6       2     header size, table included
    8       4     FNV-1a 32 of the whole file, computed with this field zero
    12      2     widest operand stack of any member
    14      2     widest frame of any member
    16      2     number of members, 1 to 256
    18      2     reserved, zero
    20      12×n  member table: offset (u32), length (u32) and the offset of
                  its name (u32), all from the start of the file

The names follow the table (length, bytes, as names are in a unit), then the
members, each starting on a multiple of 8 so it can run from memory-mapped
flash as it lies. A member is a unit exactly as the compiler wrote it,
checksum included: cut out, it is a unit file again. Names are unique.

Opening a bundle checks it whole (its checksum, its table, every member
inside the file and aligned); loading a member is loading a unit, with the
unit's own checks. The runtime writes one with `filo_bundle_build` and finds
a member with `filo_bundle_find`. What memory a bundle needs is the most any
one member needs, not the sum: a host runs one unit at a time.

## Instructions

The first byte of an instruction is five bits of opcode and three of
immediate: `opcode << 3 | imm`, the way the Z80 keeps registers and
conditions in the bits of its opcodes (`LD r,r'` is `01 ddd sss`). An
immediate of 0 to 6 is the operand; 7 means the operand follows as a
ULEB128. Where an instruction needs a second operand, it follows as a
ULEB128. Jumps carry a fixed 2-byte signed offset, counted from the end of
the jump instruction.

| op | name | immediate | then | stack | effect |
|---:|---|---|---|---|---|
| 0 | `PUSH_K` | constant | | → v | push the constant |
| 1 | `PUSH_G` | global | | → v | push the global; `undefined global: <name>` when unset |
| 2 | `STORE_G` | global | | v → v | set the global, keep the value |
| 3 | `PUSH_L` | slot | | → v | push a slot of the current frame |
| 4 | `STORE_L` | slot | | v → v | set a slot of the current frame, keep the value |
| 5 | `PUSH_UP` | depth | slot | → v | push a slot of the frame *depth* functions out |
| 6 | `STORE_UP` | depth | slot | v → v | set it, keep the value |
| 7 | `POP` | count | | v… → | drop values |
| 8 | `JMP` | condition | offset (2 bytes) | see below | jump |
| 9 | `CALL` | argc | | f a… → r | call a function value |
| 10 | `CALLB` | argc | import | a… → r | call a builtin |
| 11 | `RET` | 0 return, 1 exit | | v → | leave the function, or end the run |
| 12 | `CLOSURE` | function | | → f | a function value capturing the current frame |
| 13 | `TUPLE` | count | | a… → t | a tuple of the values |
| 14 | `UNPACK` | count | | t → a… | the elements of a tuple of exactly *count*; `letv expects tuple expression` / `letv arity mismatch` |
| 15 | `TRAP` | constant | | | error with the string constant as message |
| 16 | `PUSH_B` | import | | → f | the import as a function value: a builtin (the same value every time it is pushed), or the global holding the function the loading VM has in Filo |

`JMP` conditions — every conditional one requires a bool
(`expected bool, got <kind>`):

| imm | used by | effect |
|---:|---|---|
| 0 | `if`, `cond` | always jump |
| 1 | `if`, `cond` | pop; jump when false |
| 2 | `and` | jump when false, keeping the value; else pop |
| 3 | `or` | jump when true, keeping the value; else pop |
| 4 | `and`, `or` | check the last operand is a bool; never jump |

`CALL` follows [ir.md](ir.md): the value must be a function
(`attempt to call non-function (got <kind>)`), the recursion depth is
checked, the argument count must match (`function expects <n> arguments,
got <m>`), and the arguments fill the first slots of a new frame whose
parent is the frame the function captured. `RET 0` in a function returns
its value; at the top of an entry point it ends the run, as `RET 1` does
anywhere. An entry point ends with `RET 0`.

Every structural error the IR raises lazily — a malformed form, `if` with
the wrong number of arguments, a `cond` clause that is not a clause — is a
`TRAP` at the point where the IR would raise it, after everything the IR
would have evaluated before it.

## The device build

A machine that only runs units compiles `filo.c` with `FILO_VM_ONLY` defined
(the whole build, `filo.h` included). The parser, the IR, the tree-walking
evaluator and the compiler are left out, and so are `filo_compile`,
`filo_run` and `filo_bc_build`; what stays is the values, the globals, the
builtins, the loader and the VM. Measured with the ESP32-S3's gcc at `-Os`:
16,945 bytes of code and constants against 30,184 for the full runtime.

`make device` holds it to the full build: the full build writes every corpus
and oracle case as a unit, with what the run gave, and the device build,
with the libc-free number host, no libm and a symbol table of 128
(`FILO_SYMBOLS_MAX`), must give the same for each.

## Seeing it run

`filo` (built by `make cli`) takes a program from source to the machine in
view:

    filo show tree examples/constantes.filo   # the tree as read
    filo show folded examples/constantes.filo # once constants fold: 10
    filo show ir examples/dobro.filo        # the IR, frames and slots named
    filo run examples/dobro.filo            # 42, from the IR
    filo run --both examples/erro.filo      # IR and VM side by side
    filo build -o dobro.fbc examples/dobro.filo
    filo dump dobro.fbc                     # the unit, every operand named
    filo run --trace dobro.fbc              # each instruction, with the stack
    filo bundle -o demo.fbb ola.fbc dobro.fbc
    filo run demo.fbb dobro                 # one member of the bundle

The C reader goes from characters straight to the tree, so there is no
token stage to show; `filo_show` shows the three that exist before the
bytecode, each line with where it came from.

The listing reads units with `fbc_dump.c`, written from this document alone
and sharing no code with the loader: a second reading of the format, held to
the loader on every unit the corpus and the oracle compile to. The Go engine
has its own, the package `fbc` (and `filo dump` in its `cmd/filo`), also
written from this document alone; its listing is the C one byte for byte,
on every unit the corpus compiles to (`make govm`), where each unit also
runs stepped one instruction at a time (`Unit.Start`, for a debugger) and
must end as the whole run does. The trace is
the `trace` hook of `filo_host`, which sees each instruction before it runs:
its offset in the code section, the operand stack and how many calls deep
the run is; the host decodes the instruction from the unit's bytes. It costs
a check per instruction, as `should_stop` does (about 3% on a recursive
`fib`), and the IR has no trace. What the examples show is kept in
`testdata/cli`.

## Pausing a run

`filo_bc_start` runs an entry point for at most a budget of instructions and
returns `FILO_PAUSED` when it runs out; `filo_bc_resume` goes on with
another budget, until the run ends as `filo_bc_run` would have ended — the
same value, the same error, the same steps. Calls between bytecode functions
are activation records in the run arena, not C frames, so a pause keeps
them as they are and costs no copy. The one place a run cannot stop is
inside a builtin that calls a function back (`map`, `fold`): that call runs
to its end on the C stack, and the run pauses at its next instruction. A
paused run holds the run arena, so any other run, compile or build on the
same context cancels it, and the globals it wrote go back.

The check costs nothing per instruction: the machine already compares the
steps with the step limit, and it now compares them with the smaller of the
limit and the pause. The corpus and the oracle run paused at every
instruction boundary (`corpus_runner --vm --pause 1`), and the unit fuzzer
runs every unit whole and in slices and requires the same end.

## Loading is a trust boundary

A unit may come from anywhere — a serial line, a card, a download — so the
loader and the VM treat it as untrusted: the checksum is verified, every
section is bounds-checked, and every index, slot, jump and stack access is
checked when it is used. A corrupt or hostile file fails with an error; it
never reads or writes outside what it was given.
