/* Filo runtime in C: the same language as the Go engine, lowered to the IR
   described in ../docs/ir.md and evaluated by the same rules, so a script's
   outcome is identical on both. Two files (filo.h, filo.c), no libc beyond
   memcpy/memcmp/strlen, no allocation after init: the host hands over two
   memory blocks and everything lives in them.

   Memory model. The persistent arena holds compiled programs, the symbol
   table and the globals; it only grows. The run arena holds everything a
   single run creates (frames, lists, strings, closures) and is reset at the
   start of the next run. Globals written by a run are copied out into the
   persistent arena when the run ends, so they survive; the run's result value
   stays valid until the next run or compile. A script that exhausts either
   arena fails with an error, never corrupts memory. */
#ifndef FILO_H
#define FILO_H

#include <stdbool.h>
#include <stddef.h>
#include <stdint.h>

enum {
    FILO_OK = 0,
    FILO_ERR = 1, /* filo_error() describes it */
};

enum {
    FILO_ERROR_MAX = 256,
    FILO_BUILTINS_MAX = 96,
    FILO_SYMBOLS_MAX = 512,
    FILO_STEP_LIMIT_DEFAULT = 100000,
    FILO_RECURSION_LIMIT_DEFAULT = 128,
    FILO_RANGE_MAX = 1048576,   /* 2^20 elements; same ceiling as the Go runtime */
    FILO_PARSE_DEPTH_MAX = 256, /* the Go engine allows 4096; this is a
                                   configuration, not part of the language */
    FILO_EVAL_DEPTH_MAX = 512,  /* nesting of eval() calls: bounds the C stack */
};

typedef enum {
    FILO_NUMBER = 0,
    FILO_BOOL,
    FILO_STRING,
    FILO_LIST,
    FILO_TUPLE,
    FILO_FUNC,
} filo_kind;

typedef struct filo_value filo_value;
typedef struct filo_func filo_func;
typedef struct filo_instr filo_instr;
typedef struct filo_ctx filo_ctx;

/* Strings are byte sequences with a length: a script may contain \0. */
typedef struct {
    const uint8_t *ptr;
    uint32_t len;
} filo_str;

typedef struct {
    filo_value *items;
    uint32_t len;
} filo_seq;

struct filo_value {
    uint8_t kind; /* filo_kind */
    union {
        double num;
        bool b;
        filo_str str;
        filo_seq seq; /* list and tuple */
        filo_func *fn;
    } u;
};

/* A builtin receives evaluated arguments and writes its result to out. On
   failure it sets the message with filo_fail() and returns FILO_ERR. */
typedef int (*filo_builtin)(filo_ctx *ctx, const filo_value *args, uint32_t n, filo_value *out);

/* Host hooks. Number formatting and parsing are hooks because doing them
   exactly (shortest round-trip text, Go's ParseFloat rules) needs either libc
   or a deliberate algorithm; filo_libc.c provides both on a libc host.
   should_stop, when set, is polled at every step and ends the run with an
   error when it returns true — the host's timeout or cancellation. */
typedef struct {
    void *user;
    /* writes the shortest round-trip text of x; returns the length or 0 */
    size_t (*num_to_str)(void *user, double x, char *dst, size_t cap);
    /* parses the whole of s; returns false when it is not a number */
    bool (*str_to_num)(void *user, const uint8_t *s, size_t len, double *out);
    bool (*should_stop)(void *user);
} filo_host;

typedef struct {
    uint8_t *base;
    size_t cap;
    size_t used;
} filo_arena;

typedef struct {
    const char *name;
    filo_builtin fn;
} filo_builtin_entry;

/* A compiled script: IR in the persistent arena, bound to the context that
   compiled it. */
typedef struct {
    const filo_instr *root;
} filo_prog;

typedef struct {
    uint32_t step_limit;
    uint32_t recursion_limit;
} filo_limits;

/* The whole runtime state. Allocate one per independent interpreter — as a
   static, on the stack, wherever — and hand it two memory blocks. Nothing
   here is global. */
struct filo_ctx {
    filo_host host;
    filo_arena persistent;
    filo_arena run;

    filo_builtin_entry builtins[FILO_BUILTINS_MAX];
    uint32_t nbuiltins;

    const char *symbols[FILO_SYMBOLS_MAX]; /* id -> name, in the persistent arena */
    uint32_t nsymbols;
    filo_value globals[FILO_SYMBOLS_MAX];
    bool defined[FILO_SYMBOLS_MAX];
    /* a run writes globals in place and marks them; the run end copies the
       marked ones out, or puts the saved ones back when the run failed */
    bool dirty[FILO_SYMBOLS_MAX];
    filo_value saved[FILO_SYMBOLS_MAX];
    bool saved_defined[FILO_SYMBOLS_MAX];

    filo_limits limits;

    /* per-run state */
    uint32_t steps;
    uint32_t recursion;
    uint32_t depth;
    void *frame; /* current local scope (internal type) */
    char error[FILO_ERROR_MAX];
    uint8_t signal;      /* internal: exit/return unwinding */
    filo_value signaled; /* value carried by the signal */
};

/* ---- lifecycle ---- */

/* Initializes ctx with the host hooks (may be NULL: no formatting, no stop
   polling) and the two memory blocks. Registers the core builtins. */
int filo_init(filo_ctx *ctx, const filo_host *host, void *persistent, size_t persistent_cap,
              void *run, size_t run_cap);

/* Registers a builtin under name. Fails when the table is full or the name
   is already taken. */
int filo_register_builtin(filo_ctx *ctx, const char *name, filo_builtin fn);

/* Parses and lowers src; the program lives in the persistent arena. */
int filo_compile(filo_ctx *ctx, const uint8_t *src, size_t len, filo_prog *out);

/* Runs prog. Resets the run arena first; copies surviving globals out at the
   end. result may be NULL. The default limits apply when limits is NULL. */
int filo_run(filo_ctx *ctx, const filo_prog *prog, const filo_limits *limits, filo_value *result);

/* The message of the last failed call. */
const char *filo_error(const filo_ctx *ctx);

/* ---- globals ---- */

/* Copies v into the persistent arena under name (creating the symbol). */
int filo_set_global(filo_ctx *ctx, const char *name, filo_value v);

/* Reads a global; false when it was never set. */
bool filo_get_global(const filo_ctx *ctx, const char *name, filo_value *out);

/* ---- values ---- */

filo_value filo_num(double x);
filo_value filo_bool(bool b);
/* The bytes are referenced, not copied: they must outlive their use. Values
   the host keeps should go through filo_set_global, which copies. */
filo_value filo_string(const uint8_t *ptr, uint32_t len);
filo_value filo_cstring(const char *s);

/* Builds a list or tuple of n items in the run arena (for builtins). */
int filo_list(filo_ctx *ctx, const filo_value *items, uint32_t n, filo_value *out);
int filo_tuple(filo_ctx *ctx, const filo_value *items, uint32_t n, filo_value *out);

/* Deep, exact equality: NaN is not equal to NaN, kinds must match. */
bool filo_equal(const filo_value *a, const filo_value *b);

/* Renders v the way (string v) does: strings verbatim, everything else in
   source form. Needs the num_to_str hook for numbers. */
int filo_value_text(filo_ctx *ctx, const filo_value *v, char *dst, size_t cap, size_t *len);

/* ---- for builtins ---- */

/* Calls a func value with evaluated arguments (map, fold and filter do). */
int filo_call(filo_ctx *ctx, const filo_value *fn, const filo_value *args, uint32_t n,
              filo_value *out);

/* Records an error message (printf-free: message and an optional detail). */
int filo_fail(filo_ctx *ctx, const char *msg);
int filo_fail2(filo_ctx *ctx, const char *msg, const char *detail);

const char *filo_kind_name(uint8_t kind);

/* pow with a fractional exponent needs libm; a host that has it installs
   it here (filo_libc.c does). NULL restores the core's integral-only pow. */
void filo_set_pow(double (*fn)(double, double));

#endif
