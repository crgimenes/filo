/* Filo runtime in C. See filo.h for the model and ../docs/ir.md for the
   contract this file implements. The file reads top to bottom: memory,
   values, errors, parser, lowering, evaluation, builtins, public API. */
#include "filo.h"

#include <string.h>

/* ---------------------------------------------------------------- memory */

static void *arena_alloc(filo_arena *a, size_t n) {
    size_t aligned = (n + 7U) & ~(size_t)7U;
    if (aligned > a->cap - a->used) {
        return NULL;
    }
    void *p = a->base + a->used;
    a->used += aligned;
    return p;
}

static void arena_reset(filo_arena *a) {
    a->used = 0;
}

/* Every allocation that fails ends the current operation with this
   message; callers check for NULL and return FILO_ERR. */
static void *ralloc(filo_ctx *ctx, size_t n) {
    void *p = arena_alloc(&ctx->run, n);
    if (p == NULL) {
        (void)filo_fail(ctx, "out of memory");
    }
    return p;
}

static void *palloc(filo_ctx *ctx, size_t n) {
    void *p = arena_alloc(&ctx->persistent, n);
    if (p == NULL) {
        (void)filo_fail(ctx, "out of memory");
    }
    return p;
}

/* --------------------------------------------------------------- errors */

static size_t cstr_copy(char *dst, size_t cap, const char *s) {
    size_t n = strlen(s);
    if (n > cap - 1) {
        n = cap - 1;
    }
    memcpy(dst, s, n);
    dst[n] = '\0';
    return n;
}

int filo_fail(filo_ctx *ctx, const char *msg) {
    (void)cstr_copy(ctx->error, sizeof(ctx->error), msg);
    return FILO_ERR;
}

int filo_fail2(filo_ctx *ctx, const char *msg, const char *detail) {
    size_t n = cstr_copy(ctx->error, sizeof(ctx->error), msg);
    if (detail != NULL) {
        (void)cstr_copy(ctx->error + n, sizeof(ctx->error) - n, detail);
    }
    return FILO_ERR;
}

/* Prefixes the pending error with "in <ctx>: " — the context chain the Go
   engine builds by wrapping errors on the way out. */
static int fail_in(filo_ctx *ctx, const char *where) {
    char inner[FILO_ERROR_MAX];
    (void)cstr_copy(inner, sizeof(inner), ctx->error);
    size_t n = cstr_copy(ctx->error, sizeof(ctx->error), "in ");
    n += cstr_copy(ctx->error + n, sizeof(ctx->error) - n, where);
    n += cstr_copy(ctx->error + n, sizeof(ctx->error) - n, ": ");
    (void)cstr_copy(ctx->error + n, sizeof(ctx->error) - n, inner);
    return FILO_ERR;
}

/* Writes a decimal into a small buffer; used for argument indexes and
   arities in messages. */
static size_t u32_text(char *dst, size_t cap, uint32_t v) {
    char tmp[12];
    size_t n = 0;
    do {
        tmp[n] = (char)('0' + (v % 10U));
        n++;
        v /= 10U;
    } while (v > 0 && n < sizeof(tmp));
    if (n > cap - 1) {
        n = cap - 1;
    }
    for (size_t i = 0; i < n; i++) {
        dst[i] = tmp[n - 1 - i];
    }
    dst[n] = '\0';
    return n;
}

const char *filo_kind_name(uint8_t kind) {
    switch (kind) {
    case FILO_NUMBER:
        return "number";
    case FILO_BOOL:
        return "bool";
    case FILO_STRING:
        return "string";
    case FILO_LIST:
        return "list";
    case FILO_TUPLE:
        return "tuple";
    case FILO_FUNC:
        return "func";
    default:
        return "unknown";
    }
}

static int fail_expected(filo_ctx *ctx, const char *want, const filo_value *got) {
    char msg[FILO_ERROR_MAX];
    size_t n = cstr_copy(msg, sizeof(msg), "expected ");
    n += cstr_copy(msg + n, sizeof(msg) - n, want);
    n += cstr_copy(msg + n, sizeof(msg) - n, ", got ");
    (void)cstr_copy(msg + n, sizeof(msg) - n, filo_kind_name(got->kind));
    return filo_fail(ctx, msg);
}

/* --------------------------------------------------------------- values */

filo_value filo_num(double x) {
    filo_value v = {0};
    memset(&v, 0, sizeof(v));
    v.kind = FILO_NUMBER;
    v.u.num = x;
    return v;
}

filo_value filo_bool(bool b) {
    filo_value v = {0};
    memset(&v, 0, sizeof(v));
    v.kind = FILO_BOOL;
    v.u.b = b;
    return v;
}

filo_value filo_string(const uint8_t *ptr, uint32_t len) {
    filo_value v = {0};
    memset(&v, 0, sizeof(v));
    v.kind = FILO_STRING;
    v.u.str.ptr = ptr;
    v.u.str.len = len;
    return v;
}

filo_value filo_cstring(const char *s) {
    return filo_string((const uint8_t *)s, (uint32_t)strlen(s));
}

static filo_value empty_list(void) {
    filo_value v = {0};
    memset(&v, 0, sizeof(v));
    v.kind = FILO_LIST;
    return v;
}

static int make_seq(filo_ctx *ctx, uint8_t kind, const filo_value *items, uint32_t n,
                    filo_value *out) {
    memset(out, 0, sizeof(*out));
    out->kind = kind;
    out->u.seq.len = n;
    if (n == 0) {
        return FILO_OK;
    }
    filo_value *dst = ralloc(ctx, sizeof(filo_value) * n);
    if (dst == NULL) {
        return FILO_ERR;
    }
    if (items != NULL) {
        memcpy(dst, items, sizeof(filo_value) * n);
    }
    out->u.seq.items = dst;
    return FILO_OK;
}

int filo_list(filo_ctx *ctx, const filo_value *items, uint32_t n, filo_value *out) {
    return make_seq(ctx, FILO_LIST, items, n, out);
}

int filo_tuple(filo_ctx *ctx, const filo_value *items, uint32_t n, filo_value *out) {
    return make_seq(ctx, FILO_TUPLE, items, n, out);
}

bool filo_equal(const filo_value *a, const filo_value *b) {
    if (a->kind != b->kind) {
        return false;
    }
    switch (a->kind) {
    case FILO_NUMBER:
        return a->u.num == b->u.num; /* desvio: comparação exata de float é a semântica de = */
    case FILO_BOOL:
        return a->u.b == b->u.b;
    case FILO_STRING:
        if (a->u.str.len != b->u.str.len) {
            return false;
        }
        if (a->u.str.len == 0) {
            return true;
        }
        return memcmp(a->u.str.ptr, b->u.str.ptr, a->u.str.len) == 0;
    case FILO_LIST:
    case FILO_TUPLE:
        if (a->u.seq.len != b->u.seq.len) {
            return false;
        }
        for (uint32_t i = 0; i < a->u.seq.len; i++) {
            if (!filo_equal(&a->u.seq.items[i], &b->u.seq.items[i])) {
                return false;
            }
        }
        return true;
    case FILO_FUNC:
        return a->u.fn == b->u.fn;
    default:
        return false;
    }
}

/* --------------------------------------------------------------- parser */

/* The parse tree is a temporary in the run arena: lowering reads it and
   only the IR survives, in the persistent arena. */
typedef enum {
    N_NUMBER,
    N_BOOL,
    N_STRING,
    N_SYMBOL,
    N_LIST,
} node_kind;

typedef struct node node;
struct node {
    uint8_t kind;
    double num;
    bool b;
    filo_str text; /* string bytes or symbol name */
    node **elems;  /* list */
    uint32_t nelems;
};

typedef struct {
    filo_ctx *ctx;
    const uint8_t *src;
    size_t len;
    size_t i;
    uint32_t depth;
} parser;

static bool is_ws(uint8_t c) {
    if (c == ' ' || c == '\t' || c == '\n' || c == '\r') {
        return true;
    }
    return false;
}

static bool is_digit(uint8_t c) {
    if (c >= '0' && c <= '9') {
        return true;
    }
    return false;
}

static void skip_ws(parser *p) {
    while (p->i < p->len) {
        uint8_t c = p->src[p->i];
        if (is_ws(c)) {
            p->i++;
            continue;
        }
        if (c == ';') {
            while (p->i < p->len && p->src[p->i] != '\n') {
                p->i++;
            }
            continue;
        }
        return;
    }
}

static node *new_node(parser *p, uint8_t kind) {
    node *n = ralloc(p->ctx, sizeof(node));
    if (n == NULL) {
        return NULL;
    }
    memset(n, 0, sizeof(*n));
    n->kind = kind;
    return n;
}

static int parse_fail(parser *p, const char *msg) {
    return filo_fail2(p->ctx, "parse error: ", msg);
}

static node *read_node(parser *p);

static node *read_list(parser *p) {
    p->depth++;
    if (p->depth > FILO_PARSE_DEPTH_MAX) {
        (void)parse_fail(p, "nesting too deep");
        return NULL;
    }
    node *list = new_node(p, N_LIST);
    if (list == NULL) {
        return NULL;
    }
    /* elements are collected into a growing run-arena block: lists are
       short, so doubling is fine and the waste dies with the run */
    uint32_t cap = 0;
    for (;;) {
        skip_ws(p);
        if (p->i >= p->len) {
            (void)parse_fail(p, "unterminated list");
            return NULL;
        }
        if (p->src[p->i] == ')') {
            p->i++;
            break;
        }
        node *e = read_node(p);
        if (e == NULL) {
            return NULL;
        }
        if (list->nelems == cap) {
            uint32_t ncap = cap == 0 ? 4 : cap * 2;
            node **grown = (node **)ralloc(p->ctx, sizeof(node *) * ncap);
            if (grown == NULL) {
                return NULL;
            }
            if (list->nelems > 0) {
                memcpy((void *)grown, (const void *)list->elems, sizeof(node *) * list->nelems);
            }
            list->elems = grown;
            cap = ncap;
        }
        list->elems[list->nelems] = e;
        list->nelems++;
    }
    p->depth--;
    return list;
}

static node *read_string(parser *p) {
    p->i++; /* opening quote */
    /* decoded bytes go to a run-arena buffer no longer than the source span */
    size_t start = p->i;
    size_t scan = start;
    while (scan < p->len && p->src[scan] != '"') {
        if (p->src[scan] == '\\') {
            scan++;
        }
        scan++;
    }
    if (scan > p->len) {
        (void)parse_fail(p, "unterminated escape sequence");
        return NULL;
    }
    if (scan >= p->len) {
        (void)parse_fail(p, "unterminated string literal");
        return NULL;
    }
    uint8_t *buf = ralloc(p->ctx, scan - start + 1);
    if (buf == NULL) {
        return NULL;
    }
    uint32_t n = 0;
    while (p->i < scan) {
        uint8_t c = p->src[p->i];
        p->i++;
        if (c != '\\') {
            buf[n] = c;
            n++;
            continue;
        }
        uint8_t e = p->src[p->i];
        p->i++;
        uint8_t decoded = 0;
        switch (e) {
        case '"':
            decoded = '"';
            break;
        case 'n':
            decoded = '\n';
            break;
        case 't':
            decoded = '\t';
            break;
        case 'r':
            decoded = '\r';
            break;
        case '\\':
            decoded = '\\';
            break;
        case '0':
            decoded = '\0';
            break;
        case 'a':
            decoded = '\a';
            break;
        case 'b':
            decoded = '\b';
            break;
        case 'f':
            decoded = '\f';
            break;
        case 'v':
            decoded = '\v';
            break;
        default:
            (void)parse_fail(p, "unsupported escape");
            return NULL;
        }
        buf[n] = decoded;
        n++;
    }
    p->i = scan + 1; /* closing quote */
    node *s = new_node(p, N_STRING);
    if (s == NULL) {
        return NULL;
    }
    s->text.ptr = buf;
    s->text.len = n;
    return s;
}

static bool at_boundary(const parser *p, size_t i) {
    if (i >= p->len) {
        return true;
    }
    uint8_t c = p->src[i];
    if (is_ws(c) || c == ')' || c == '(' || c == ';' || c == '"') {
        return true;
    }
    return false;
}

static node *read_bool(parser *p) {
    /* exactly #t or #f, then a token boundary: #true is an error */
    if (p->i + 1 < p->len && (p->src[p->i + 1] == 't' || p->src[p->i + 1] == 'f') &&
        at_boundary(p, p->i + 2)) {
        node *b = new_node(p, N_BOOL);
        if (b == NULL) {
            return NULL;
        }
        b->b = p->src[p->i + 1] == 't';
        p->i += 2;
        return b;
    }
    (void)parse_fail(p, "invalid boolean literal");
    return NULL;
}

/* Source grammar of a number literal: [+-]?(digits[.digits*]|.digits)
   ([eE][+-]?digits)?. Any other atom is a symbol, whatever the host's
   str_to_num would accept (inf, nan, 1_000). */
static bool number_shape(const uint8_t *s, size_t len) {
    size_t i = 0;
    if (i < len && (s[i] == '+' || s[i] == '-')) {
        i++;
    }
    size_t digits = 0;
    while (i < len && is_digit(s[i])) {
        i++;
        digits++;
    }
    if (i < len && s[i] == '.') {
        i++;
        while (i < len && is_digit(s[i])) {
            i++;
            digits++;
        }
    }
    if (digits == 0) {
        return false;
    }
    if (i < len && (s[i] == 'e' || s[i] == 'E')) {
        i++;
        if (i < len && (s[i] == '+' || s[i] == '-')) {
            i++;
        }
        size_t exp = 0;
        while (i < len && is_digit(s[i])) {
            i++;
            exp++;
        }
        if (exp == 0) {
            return false;
        }
    }
    return i == len;
}

static node *read_atom(parser *p) {
    size_t start = p->i;
    while (p->i < p->len) {
        uint8_t c = p->src[p->i];
        if (is_ws(c) || c == '(' || c == ')') {
            break;
        }
        p->i++;
    }
    if (p->i == start) {
        (void)parse_fail(p, "expected token");
        return NULL;
    }
    double x = 0;
    if (number_shape(p->src + start, p->i - start) && p->ctx->host.str_to_num != NULL &&
        p->ctx->host.str_to_num(p->ctx->host.user, p->src + start, p->i - start, &x)) {
        node *n = new_node(p, N_NUMBER);
        if (n == NULL) {
            return NULL;
        }
        n->num = x;
        return n;
    }
    node *s = new_node(p, N_SYMBOL);
    if (s == NULL) {
        return NULL;
    }
    s->text.ptr = p->src + start;
    s->text.len = (uint32_t)(p->i - start);
    return s;
}

static node *read_node(parser *p) {
    skip_ws(p);
    if (p->i >= p->len) {
        (void)parse_fail(p, "unexpected end of input");
        return NULL;
    }
    uint8_t c = p->src[p->i];
    if (c == '(') {
        p->i++;
        return read_list(p);
    }
    if (c == ')') {
        (void)parse_fail(p, "expected token");
        return NULL;
    }
    if (c == '"') {
        return read_string(p);
    }
    if (c == '#') {
        return read_bool(p);
    }
    return read_atom(p);
}

/* Parses the whole source. Several top-level expressions are wrapped in an
   implicit (let () ...), exactly as the Go parser does. */
static node *parse_all(filo_ctx *ctx, const uint8_t *src, size_t len) {
    parser p = {ctx, src, len, 0, 0};
    if (len >= 3 && src[0] == 0xEF && src[1] == 0xBB && src[2] == 0xBF) {
        p.i = 3; /* a BOM is not part of the first token */
    }
    node *first = NULL;
    node *wrapper = NULL;
    uint32_t cap = 0;
    for (;;) {
        skip_ws(&p);
        if (p.i >= p.len) {
            break;
        }
        node *n = read_node(&p);
        if (n == NULL) {
            return NULL;
        }
        if (first == NULL) {
            first = n;
            continue;
        }
        if (wrapper == NULL) {
            wrapper = new_node(&p, N_LIST);
            node *let = new_node(&p, N_SYMBOL);
            node *bindings = new_node(&p, N_LIST);
            if (wrapper == NULL || let == NULL || bindings == NULL) {
                return NULL;
            }
            let->text = filo_cstring("let").u.str;
            cap = 8;
            wrapper->elems = (node **)ralloc(ctx, sizeof(node *) * cap);
            if (wrapper->elems == NULL) {
                return NULL;
            }
            wrapper->elems[0] = let;
            wrapper->elems[1] = bindings;
            wrapper->elems[2] = first;
            wrapper->nelems = 3;
        }
        if (wrapper->nelems == cap) {
            uint32_t ncap = cap * 2;
            node **grown = (node **)ralloc(ctx, sizeof(node *) * ncap);
            if (grown == NULL) {
                return NULL;
            }
            memcpy((void *)grown, (const void *)wrapper->elems, sizeof(node *) * wrapper->nelems);
            wrapper->elems = grown;
            cap = ncap;
        }
        wrapper->elems[wrapper->nelems] = n;
        wrapper->nelems++;
    }
    if (first == NULL) {
        (void)parse_fail(&p, "empty script");
        return NULL;
    }
    return wrapper != NULL ? wrapper : first;
}

/* ------------------------------------------------------------------- IR */

typedef enum {
    OP_CONST,
    OP_LOCAL,
    OP_GLOBAL,
    OP_DYNAMIC,
    OP_BUILTIN,
    OP_EMPTY,
    OP_INVALID,
    OP_IF,
    OP_COND,
    OP_DO,
    OP_AND,
    OP_OR,
    OP_LET,
    OP_LETV,
    OP_SET,
    OP_FN,
    OP_DEF,
    OP_TUPLE,
    OP_EXIT,
    OP_RETURN,
    OP_CALLB,
    OP_CALL,
} opcode;

typedef struct {
    bool invalid;
    bool is_else;
    const filo_instr *test;
    const filo_instr *const *body;
    uint32_t nbody;
} clause;

/* One instruction; which fields matter depends on op, exactly as the Go
   Instr (see ../ir.go). Everything it points to is in the persistent arena. */
struct filo_instr {
    uint8_t op;
    uint32_t a;
    uint32_t b;
    const char *name;
    const char *msg;
    filo_value val;
    const char *const *names;
    uint32_t nnames;
    const filo_instr *const *args;
    uint32_t nargs;
    const clause *clauses;
    uint32_t nclauses;
    filo_builtin fn;
};

typedef struct frame frame;
struct frame {
    filo_value *slots;
    uint32_t n;
    frame *parent;
};

struct filo_func {
    const char *const *params;
    uint32_t nparams;
    const filo_instr *const *body;
    uint32_t nbody;
    frame *frame;
};

/* --------------------------------------------------------------- lowering */

/* Copies a name into the persistent arena, NUL-terminated. */
static const char *pstr(filo_ctx *ctx, const uint8_t *ptr, uint32_t len) {
    char *s = palloc(ctx, (size_t)len + 1);
    if (s == NULL) {
        return NULL;
    }
    if (len > 0) {
        memcpy(s, ptr, len);
    }
    s[len] = '\0';
    return s;
}

static bool name_is(const filo_str *s, const char *lit) {
    size_t n = strlen(lit);
    if (s->len != n) {
        return false;
    }
    return memcmp(s->ptr, lit, n) == 0;
}

static bool cname_eq(const char *name, const uint8_t *ptr, uint32_t len) {
    if (strlen(name) != len) {
        return false;
    }
    return memcmp(name, ptr, len) == 0;
}

/* Symbol ids are interned per context and never change, like the Go
   SymbolTable. */
static int32_t symbol_id(filo_ctx *ctx, const uint8_t *ptr, uint32_t len) {
    for (uint32_t i = 0; i < ctx->nsymbols; i++) {
        if (cname_eq(ctx->symbols[i], ptr, len)) {
            return (int32_t)i;
        }
    }
    if (ctx->nsymbols >= FILO_SYMBOLS_MAX) {
        (void)filo_fail(ctx, "too many globals");
        return -1;
    }
    const char *name = pstr(ctx, ptr, len);
    if (name == NULL) {
        return -1;
    }
    ctx->symbols[ctx->nsymbols] = name;
    ctx->nsymbols++;
    return (int32_t)(ctx->nsymbols - 1);
}

static const filo_builtin_entry *builtin_by_name(const filo_ctx *ctx, const uint8_t *ptr,
                                                 uint32_t len) {
    for (uint32_t i = 0; i < ctx->nbuiltins; i++) {
        if (cname_eq(ctx->builtins[i].name, ptr, len)) {
            return &ctx->builtins[i];
        }
    }
    return NULL;
}

/* A lexical scope while lowering; lives in the run arena and dies with the
   compile. */
typedef struct scope scope;
struct scope {
    filo_str *vars;
    uint32_t n;
    uint32_t cap;
    scope *parent;
};

typedef struct {
    filo_ctx *ctx;
    scope *scope;
} lowerer;

static scope *scope_enter(lowerer *lw) {
    scope *s = ralloc(lw->ctx, sizeof(scope));
    if (s == NULL) {
        return NULL;
    }
    memset(s, 0, sizeof(*s));
    s->parent = lw->scope;
    lw->scope = s;
    return s;
}

static void scope_leave(lowerer *lw) {
    lw->scope = lw->scope->parent;
}

static int scope_define(lowerer *lw, filo_str name) {
    scope *s = lw->scope;
    if (s->n == s->cap) {
        uint32_t ncap = s->cap == 0 ? 8 : s->cap * 2;
        filo_str *grown = ralloc(lw->ctx, sizeof(filo_str) * ncap);
        if (grown == NULL) {
            return FILO_ERR;
        }
        if (s->n > 0) {
            memcpy(grown, s->vars, sizeof(filo_str) * s->n);
        }
        s->vars = grown;
        s->cap = ncap;
    }
    s->vars[s->n] = name;
    s->n++;
    return FILO_OK;
}

static bool scope_resolve(const lowerer *lw, const filo_str *name, uint32_t *depth,
                          uint32_t *index) {
    uint32_t d = 0;
    for (const scope *s = lw->scope; s != NULL; s = s->parent) {
        for (uint32_t i = 0; i < s->n; i++) {
            if (s->vars[i].len == name->len && memcmp(s->vars[i].ptr, name->ptr, name->len) == 0) {
                *depth = d;
                *index = i;
                return true;
            }
        }
        d++;
    }
    return false;
}

static filo_instr *new_instr(lowerer *lw, uint8_t op) {
    filo_instr *in = palloc(lw->ctx, sizeof(filo_instr));
    if (in == NULL) {
        return NULL;
    }
    memset(in, 0, sizeof(*in));
    in->op = op;
    return in;
}

static filo_instr *invalid_instr(lowerer *lw, const char *where, const char *msg) {
    filo_instr *in = new_instr(lw, OP_INVALID);
    if (in == NULL) {
        return NULL;
    }
    in->name = where;
    in->msg = msg;
    return in;
}

static const filo_instr *lower(lowerer *lw, const node *n);

/* Lowers nodes[from..) into a persistent array. */
static int lower_all(lowerer *lw, node *const *nodes, uint32_t from, uint32_t to,
                     const filo_instr *const **out, uint32_t *nout) {
    uint32_t n = to > from ? to - from : 0;
    *nout = n;
    *out = NULL;
    if (n == 0) {
        return FILO_OK;
    }
    const filo_instr **arr = (const filo_instr **)palloc(lw->ctx, sizeof(filo_instr *) * n);
    if (arr == NULL) {
        return FILO_ERR;
    }
    for (uint32_t i = 0; i < n; i++) {
        arr[i] = lower(lw, nodes[from + i]);
        if (arr[i] == NULL) {
            return FILO_ERR;
        }
    }
    *out = arr;
    return FILO_OK;
}

static const filo_instr *lower_symbol(lowerer *lw, const filo_str *name) {
    uint32_t depth = 0;
    uint32_t index = 0;
    if (scope_resolve(lw, name, &depth, &index)) {
        filo_instr *in = new_instr(lw, OP_LOCAL);
        if (in == NULL) {
            return NULL;
        }
        in->a = depth;
        in->b = index;
        in->name = pstr(lw->ctx, name->ptr, name->len);
        return in->name != NULL ? in : NULL;
    }
    const filo_builtin_entry *bi = builtin_by_name(lw->ctx, name->ptr, name->len);
    if (bi != NULL) {
        filo_instr *in = new_instr(lw, OP_BUILTIN);
        if (in == NULL) {
            return NULL;
        }
        in->name = bi->name;
        in->fn = bi->fn;
        return in;
    }
    int32_t id = symbol_id(lw->ctx, name->ptr, name->len);
    if (id < 0) {
        return NULL;
    }
    filo_instr *in = new_instr(lw, OP_GLOBAL);
    if (in == NULL) {
        return NULL;
    }
    in->a = (uint32_t)id;
    in->name = lw->ctx->symbols[id];
    return in;
}

static const filo_instr *lower_simple(lowerer *lw, uint8_t op, const char *name, const node *list) {
    filo_instr *in = new_instr(lw, op);
    if (in == NULL) {
        return NULL;
    }
    in->name = name;
    if (lower_all(lw, list->elems, 1, list->nelems, &in->args, &in->nargs) != FILO_OK) {
        return NULL;
    }
    return in;
}

static const filo_instr *lower_cond(lowerer *lw, const node *list) {
    filo_instr *in = new_instr(lw, OP_COND);
    if (in == NULL) {
        return NULL;
    }
    uint32_t n = list->nelems - 1;
    clause *cls = NULL;
    if (n > 0) {
        cls = palloc(lw->ctx, sizeof(clause) * n);
        if (cls == NULL) {
            return NULL;
        }
        memset(cls, 0, sizeof(clause) * n);
    }
    for (uint32_t i = 0; i < n; i++) {
        const node *c = list->elems[i + 1];
        if (c->kind != N_LIST || c->nelems < 2) {
            cls[i].invalid = true;
            continue;
        }
        const node *head = c->elems[0];
        if (head->kind == N_SYMBOL && name_is(&head->text, "else")) {
            cls[i].is_else = true;
        } else {
            cls[i].test = lower(lw, head);
            if (cls[i].test == NULL) {
                return NULL;
            }
        }
        if (lower_all(lw, c->elems, 1, c->nelems, &cls[i].body, &cls[i].nbody) != FILO_OK) {
            return NULL;
        }
    }
    in->clauses = cls;
    in->nclauses = n;
    return in;
}

static const filo_instr *lower_let(lowerer *lw, const node *list) {
    uint32_t nargs = list->nelems - 1;
    if (nargs == 0) {
        return invalid_instr(lw, "let", "let expects bindings and body");
    }
    const node *bindings = list->elems[1];
    if (bindings->kind != N_LIST) {
        if (nargs < 2) {
            return invalid_instr(lw, "let", "let expects bindings and body");
        }
        return invalid_instr(lw, "let", "let expects binding list");
    }
    if (scope_enter(lw) == NULL) {
        return NULL;
    }
    uint32_t nb = bindings->nelems;
    uint32_t total = nb + (nargs - 1);
    const filo_instr **arr =
        (const filo_instr **)palloc(lw->ctx, sizeof(filo_instr *) * (total + 1));
    if (arr == NULL) {
        return NULL;
    }
    /* sequential like let*: each value sees the bindings before it */
    for (uint32_t i = 0; i < nb; i++) {
        const node *pair = bindings->elems[i];
        if (pair->kind != N_LIST || pair->nelems != 2) {
            (void)filo_fail(lw->ctx, "invalid let binding");
            return NULL;
        }
        if (pair->elems[0]->kind != N_SYMBOL) {
            (void)filo_fail(lw->ctx, "let binding name must be symbol");
            return NULL;
        }
        arr[i] = lower(lw, pair->elems[1]);
        if (arr[i] == NULL) {
            return NULL;
        }
        if (scope_define(lw, pair->elems[0]->text) != FILO_OK) {
            return NULL;
        }
    }
    for (uint32_t i = 2; i < list->nelems; i++) {
        arr[nb + i - 2] = lower(lw, list->elems[i]);
        if (arr[nb + i - 2] == NULL) {
            return NULL;
        }
    }
    scope_leave(lw);
    if (nargs < 2) {
        return invalid_instr(lw, "let", "let expects bindings and body");
    }
    filo_instr *in = new_instr(lw, OP_LET);
    if (in == NULL) {
        return NULL;
    }
    in->a = nb;
    in->args = arr;
    in->nargs = total;
    return in;
}

static const filo_instr *lower_letv(lowerer *lw, const node *list) {
    uint32_t nargs = list->nelems - 1;
    if (nargs < 2) {
        return invalid_instr(lw, "letv", "letv expects bindings and body");
    }
    const node *names = list->elems[1];
    if (names->kind != N_LIST) {
        return invalid_instr(lw, "letv", "letv expects name list");
    }
    /* the tuple is evaluated outside the new scope, so it is lowered there */
    const filo_instr *tuple = lower(lw, list->elems[2]);
    if (tuple == NULL) {
        return NULL;
    }
    if (scope_enter(lw) == NULL) {
        return NULL;
    }
    const char **nm = NULL;
    if (names->nelems > 0) {
        nm = (const char **)palloc(lw->ctx, sizeof(char *) * names->nelems);
        if (nm == NULL) {
            return NULL;
        }
    }
    for (uint32_t i = 0; i < names->nelems; i++) {
        if (names->elems[i]->kind != N_SYMBOL) {
            (void)filo_fail(lw->ctx, "letv names must be symbols");
            return NULL;
        }
        nm[i] = pstr(lw->ctx, names->elems[i]->text.ptr, names->elems[i]->text.len);
        if (nm[i] == NULL || scope_define(lw, names->elems[i]->text) != FILO_OK) {
            return NULL;
        }
    }
    uint32_t nbody = list->nelems - 3;
    const filo_instr **arr =
        (const filo_instr **)palloc(lw->ctx, sizeof(filo_instr *) * (nbody + 1));
    if (arr == NULL) {
        return NULL;
    }
    arr[0] = tuple;
    for (uint32_t i = 0; i < nbody; i++) {
        arr[i + 1] = lower(lw, list->elems[i + 3]);
        if (arr[i + 1] == NULL) {
            return NULL;
        }
    }
    scope_leave(lw);
    filo_instr *in = new_instr(lw, OP_LETV);
    if (in == NULL) {
        return NULL;
    }
    in->names = nm;
    in->nnames = names->nelems;
    in->args = arr;
    in->nargs = nbody + 1;
    return in;
}

static const filo_instr *lower_fn(lowerer *lw, const node *list) {
    uint32_t nargs = list->nelems - 1;
    if (nargs == 0) {
        return invalid_instr(lw, "fn", "fn expects parameters and body");
    }
    const node *params = list->elems[1];
    if (params->kind != N_LIST) {
        if (nargs < 2) {
            return invalid_instr(lw, "fn", "fn expects parameters and body");
        }
        return invalid_instr(lw, "fn", "fn expects parameter list");
    }
    if (scope_enter(lw) == NULL) {
        return NULL;
    }
    const char **nm = NULL;
    if (params->nelems > 0) {
        nm = (const char **)palloc(lw->ctx, sizeof(char *) * params->nelems);
        if (nm == NULL) {
            return NULL;
        }
    }
    for (uint32_t i = 0; i < params->nelems; i++) {
        if (params->elems[i]->kind != N_SYMBOL) {
            (void)filo_fail(lw->ctx, "fn params must be symbols");
            return NULL;
        }
        nm[i] = pstr(lw->ctx, params->elems[i]->text.ptr, params->elems[i]->text.len);
        if (nm[i] == NULL || scope_define(lw, params->elems[i]->text) != FILO_OK) {
            return NULL;
        }
    }
    const filo_instr *const *body = NULL;
    uint32_t nbody = 0;
    if (lower_all(lw, list->elems, 2, list->nelems, &body, &nbody) != FILO_OK) {
        return NULL;
    }
    scope_leave(lw);
    if (nbody == 0) {
        return invalid_instr(lw, "fn", "fn expects parameters and body");
    }
    filo_instr *in = new_instr(lw, OP_FN);
    if (in == NULL) {
        return NULL;
    }
    in->names = nm;
    in->nnames = params->nelems;
    in->args = body;
    in->nargs = nbody;
    return in;
}

static const filo_instr *lower_def(lowerer *lw, const node *list) {
    if (list->nelems != 3) {
        return invalid_instr(lw, "def", "def expects name and expression");
    }
    const filo_instr *value = lower(lw, list->elems[2]);
    if (value == NULL) {
        return NULL;
    }
    filo_instr *in = new_instr(lw, OP_DEF);
    if (in == NULL) {
        return NULL;
    }
    const filo_instr **arr = (const filo_instr **)palloc(lw->ctx, sizeof(filo_instr *));
    if (arr == NULL) {
        return NULL;
    }
    arr[0] = value;
    in->args = arr;
    in->nargs = 1;
    if (list->elems[1]->kind != N_SYMBOL) {
        in->msg = "def name must be symbol";
        return in;
    }
    in->name = pstr(lw->ctx, list->elems[1]->text.ptr, list->elems[1]->text.len);
    return in->name != NULL ? in : NULL;
}

static const filo_instr *lower_list(lowerer *lw, const node *list) {
    if (list->nelems == 0) {
        return new_instr(lw, OP_EMPTY);
    }
    const node *head = list->elems[0];
    if (head->kind == N_SYMBOL) {
        const filo_str *h = &head->text;
        if (name_is(h, "if")) {
            return lower_simple(lw, OP_IF, NULL, list);
        }
        if (name_is(h, "do")) {
            return lower_simple(lw, OP_DO, NULL, list);
        }
        if (name_is(h, "and")) {
            return lower_simple(lw, OP_AND, NULL, list);
        }
        if (name_is(h, "or")) {
            return lower_simple(lw, OP_OR, NULL, list);
        }
        if (name_is(h, "set")) {
            return lower_simple(lw, OP_SET, NULL, list);
        }
        if (name_is(h, "exit")) {
            return lower_simple(lw, OP_EXIT, "exit", list);
        }
        if (name_is(h, "return")) {
            return lower_simple(lw, OP_RETURN, "return", list);
        }
        if (name_is(h, "values")) {
            return lower_simple(lw, OP_TUPLE, "values", list);
        }
        if (name_is(h, "tuple")) {
            return lower_simple(lw, OP_TUPLE, "tuple", list);
        }
        if (name_is(h, "cond")) {
            return lower_cond(lw, list);
        }
        if (name_is(h, "let")) {
            return lower_let(lw, list);
        }
        if (name_is(h, "letv")) {
            return lower_letv(lw, list);
        }
        if (name_is(h, "fn")) {
            return lower_fn(lw, list);
        }
        if (name_is(h, "def")) {
            return lower_def(lw, list);
        }
    }
    const filo_instr *const *elems = NULL;
    uint32_t n = 0;
    if (lower_all(lw, list->elems, 0, list->nelems, &elems, &n) != FILO_OK) {
        return NULL;
    }
    if (elems[0]->op == OP_BUILTIN) {
        filo_instr *in = new_instr(lw, OP_CALLB);
        if (in == NULL) {
            return NULL;
        }
        in->name = elems[0]->name;
        in->fn = elems[0]->fn;
        in->args = elems + 1;
        in->nargs = n - 1;
        return in;
    }
    filo_instr *in = new_instr(lw, OP_CALL);
    if (in == NULL) {
        return NULL;
    }
    in->args = elems;
    in->nargs = n;
    return in;
}

static const filo_instr *lower(lowerer *lw, const node *n) {
    switch (n->kind) {
    case N_NUMBER: {
        filo_instr *in = new_instr(lw, OP_CONST);
        if (in == NULL) {
            return NULL;
        }
        in->val = filo_num(n->num);
        return in;
    }
    case N_BOOL: {
        filo_instr *in = new_instr(lw, OP_CONST);
        if (in == NULL) {
            return NULL;
        }
        in->val = filo_bool(n->b);
        return in;
    }
    case N_STRING: {
        filo_instr *in = new_instr(lw, OP_CONST);
        if (in == NULL) {
            return NULL;
        }
        /* the bytes were decoded into the run arena; constants must outlive it */
        const char *copy = pstr(lw->ctx, n->text.ptr, n->text.len);
        if (copy == NULL) {
            return NULL;
        }
        in->val = filo_string((const uint8_t *)copy, n->text.len);
        return in;
    }
    case N_SYMBOL:
        return lower_symbol(lw, &n->text);
    case N_LIST:
        return lower_list(lw, n);
    default:
        (void)filo_fail(lw->ctx, "unknown node");
        return NULL;
    }
}

/* ------------------------------------------------------------- evaluation */

enum {
    SIG_NONE = 0,
    SIG_EXIT,
    SIG_RETURN,
};

static int eval(filo_ctx *ctx, const filo_instr *in, filo_value *out);

/* Adds the "in <where>: " context unless a signal is unwinding: a signal is
   not an error and never gains context. */
static int wrap(filo_ctx *ctx, const char *where, int rc) {
    if (rc == FILO_OK || ctx->signal != SIG_NONE) {
        return rc;
    }
    return fail_in(ctx, where);
}

static int prefix_error(filo_ctx *ctx, const char *prefix) {
    char inner[FILO_ERROR_MAX];
    (void)cstr_copy(inner, sizeof(inner), ctx->error);
    size_t n = cstr_copy(ctx->error, sizeof(ctx->error), prefix);
    (void)cstr_copy(ctx->error + n, sizeof(ctx->error) - n, inner);
    return FILO_ERR;
}

/* "argument <i>: " */
static int fail_argument(filo_ctx *ctx, uint32_t i) {
    char prefix[32];
    size_t n = cstr_copy(prefix, sizeof(prefix), "argument ");
    n += u32_text(prefix + n, sizeof(prefix) - n, i);
    (void)cstr_copy(prefix + n, sizeof(prefix) - n, ": ");
    return prefix_error(ctx, prefix);
}

static int eval_args(filo_ctx *ctx, const filo_instr *const *args, uint32_t n, filo_value **out) {
    *out = NULL;
    if (n == 0) {
        return FILO_OK;
    }
    filo_value *vals = ralloc(ctx, sizeof(filo_value) * n);
    if (vals == NULL) {
        return FILO_ERR;
    }
    for (uint32_t i = 0; i < n; i++) {
        if (eval(ctx, args[i], &vals[i]) != FILO_OK) {
            if (ctx->signal == SIG_NONE) {
                (void)fail_argument(ctx, i);
            }
            return FILO_ERR;
        }
    }
    *out = vals;
    return FILO_OK;
}

static int eval_body(filo_ctx *ctx, const filo_instr *const *body, uint32_t n, filo_value *out) {
    if (n == 0) {
        return filo_fail(ctx, "empty body");
    }
    for (uint32_t i = 0; i < n; i++) {
        if (eval(ctx, body[i], out) != FILO_OK) {
            return FILO_ERR;
        }
    }
    return FILO_OK;
}

static int as_bool(filo_ctx *ctx, const filo_value *v, bool *out) {
    if (v->kind != FILO_BOOL) {
        return fail_expected(ctx, "bool", v);
    }
    *out = v->u.b;
    return FILO_OK;
}

static int call_builtin(filo_ctx *ctx, const char *name, filo_builtin fn,
                        const filo_instr *const *args, uint32_t n, filo_value *out) {
    filo_value *vals = NULL;
    if (eval_args(ctx, args, n, &vals) != FILO_OK) {
        if (ctx->signal == SIG_NONE) {
            char prefix[FILO_ERROR_MAX];
            size_t k = cstr_copy(prefix, sizeof(prefix), "while evaluating arguments for \"");
            k += cstr_copy(prefix + k, sizeof(prefix) - k, name);
            (void)cstr_copy(prefix + k, sizeof(prefix) - k, "\": ");
            (void)prefix_error(ctx, prefix);
        }
        return FILO_ERR;
    }
    if (fn(ctx, vals, n, out) != FILO_OK) {
        if (ctx->signal == SIG_NONE) {
            char prefix[FILO_ERROR_MAX];
            size_t k = cstr_copy(prefix, sizeof(prefix), "in builtin \"");
            k += cstr_copy(prefix + k, sizeof(prefix) - k, name);
            (void)cstr_copy(prefix + k, sizeof(prefix) - k, "\": ");
            (void)prefix_error(ctx, prefix);
        }
        return FILO_ERR;
    }
    return FILO_OK;
}

/* Runs fn's body in a frame of slots whose parent is the captured frame; the
   recursion counter was incremented by the caller. A return signal becomes
   the value. */
static int run_func(filo_ctx *ctx, const filo_func *fn, filo_value *slots, uint32_t n,
                    filo_value *out) {
    frame *f = ralloc(ctx, sizeof(frame));
    if (f == NULL) {
        ctx->recursion--;
        return FILO_ERR;
    }
    f->slots = slots;
    f->n = n;
    f->parent = fn->frame;
    frame *old = ctx->frame;
    ctx->frame = f;
    int rc = eval_body(ctx, fn->body, fn->nbody, out);
    ctx->frame = old;
    ctx->recursion--;
    if (rc != FILO_OK && ctx->signal == SIG_RETURN) {
        *out = ctx->signaled;
        ctx->signal = SIG_NONE;
        return FILO_OK;
    }
    return rc;
}

static int fail_arity(filo_ctx *ctx, uint32_t want, uint32_t got) {
    char msg[64];
    size_t n = cstr_copy(msg, sizeof(msg), "function expects ");
    n += u32_text(msg + n, sizeof(msg) - n, want);
    n += cstr_copy(msg + n, sizeof(msg) - n, " arguments, got ");
    (void)u32_text(msg + n, sizeof(msg) - n, got);
    return filo_fail(ctx, msg);
}

static int enter_call(filo_ctx *ctx) {
    ctx->recursion++;
    if (ctx->limits.recursion_limit > 0 && ctx->recursion > ctx->limits.recursion_limit) {
        ctx->recursion--;
        return filo_fail(ctx, "recursion limit exceeded");
    }
    return FILO_OK;
}

int filo_call(filo_ctx *ctx, const filo_value *fnv, const filo_value *args, uint32_t n,
              filo_value *out) {
    if (fnv->kind != FILO_FUNC) {
        return fail_expected(ctx, "func", fnv);
    }
    const filo_func *fn = fnv->u.fn;
    if (fn->nparams != n) {
        return fail_arity(ctx, fn->nparams, n);
    }
    if (enter_call(ctx) != FILO_OK) {
        return FILO_ERR;
    }
    filo_value *slots = NULL;
    if (n > 0) {
        slots = ralloc(ctx, sizeof(filo_value) * n);
        if (slots == NULL) {
            ctx->recursion--;
            return FILO_ERR;
        }
        memcpy(slots, args, sizeof(filo_value) * n);
    }
    return run_func(ctx, fn, slots, n, out);
}

static int eval_call(filo_ctx *ctx, const filo_instr *in, filo_value *out) {
    const filo_instr *head = in->args[0];
    if (head->op == OP_DYNAMIC) {
        const filo_builtin_entry *bi =
            builtin_by_name(ctx, (const uint8_t *)head->name, (uint32_t)strlen(head->name));
        if (bi != NULL) {
            return call_builtin(ctx, bi->name, bi->fn, in->args + 1, in->nargs - 1, out);
        }
    }
    filo_value fnv = {0};
    if (eval(ctx, head, &fnv) != FILO_OK) {
        return wrap(ctx, "call", FILO_ERR);
    }
    if (fnv.kind != FILO_FUNC) {
        char msg[64];
        size_t k = cstr_copy(msg, sizeof(msg), "attempt to call non-function (got ");
        k += cstr_copy(msg + k, sizeof(msg) - k, filo_kind_name(fnv.kind));
        (void)cstr_copy(msg + k, sizeof(msg) - k, ")");
        return filo_fail(ctx, msg);
    }
    const filo_func *fn = fnv.u.fn;
    uint32_t n = in->nargs - 1;
    if (fn->nparams != n) {
        (void)fail_arity(ctx, fn->nparams, n);
        return wrap(ctx, "function call", FILO_ERR);
    }
    if (enter_call(ctx) != FILO_OK) {
        return wrap(ctx, "function call", FILO_ERR);
    }
    filo_value *slots = NULL;
    if (n > 0) {
        slots = ralloc(ctx, sizeof(filo_value) * n);
        if (slots == NULL) {
            ctx->recursion--;
            return FILO_ERR;
        }
    }
    for (uint32_t i = 0; i < n; i++) {
        if (eval(ctx, in->args[i + 1], &slots[i]) != FILO_OK) {
            ctx->recursion--;
            if (ctx->signal == SIG_NONE) {
                (void)fail_argument(ctx, i);
                (void)prefix_error(ctx, "in call arguments: ");
            }
            return wrap(ctx, "function call", FILO_ERR);
        }
    }
    return wrap(ctx, "function call", run_func(ctx, fn, slots, n, out));
}

static int eval_if(filo_ctx *ctx, const filo_instr *in, filo_value *out) {
    if (in->nargs < 2 || in->nargs > 3) {
        return filo_fail(ctx, "if expects 2 or 3 arguments (condition then [else])");
    }
    filo_value c = {0};
    if (eval(ctx, in->args[0], &c) != FILO_OK) {
        return FILO_ERR;
    }
    bool b = false;
    if (as_bool(ctx, &c, &b) != FILO_OK) {
        return FILO_ERR;
    }
    if (b) {
        return eval(ctx, in->args[1], out);
    }
    if (in->nargs == 2) {
        *out = empty_list();
        return FILO_OK;
    }
    return eval(ctx, in->args[2], out);
}

static int eval_cond(filo_ctx *ctx, const filo_instr *in, filo_value *out) {
    for (uint32_t i = 0; i < in->nclauses; i++) {
        const clause *c = &in->clauses[i];
        if (c->invalid) {
            return filo_fail(ctx, "cond clause must be a list of a test and a body");
        }
        if (c->is_else) {
            return eval_body(ctx, c->body, c->nbody, out);
        }
        filo_value t = {0};
        if (eval(ctx, c->test, &t) != FILO_OK) {
            return FILO_ERR;
        }
        bool b = false;
        if (as_bool(ctx, &t, &b) != FILO_OK) {
            return FILO_ERR;
        }
        if (b) {
            return eval_body(ctx, c->body, c->nbody, out);
        }
    }
    *out = empty_list();
    return FILO_OK;
}

static int eval_do(filo_ctx *ctx, const filo_instr *in, filo_value *out) {
    if (in->nargs == 0) {
        return filo_fail(ctx, "do expects at least 1 expression");
    }
    return eval_body(ctx, in->args, in->nargs, out);
}

static int eval_logic(filo_ctx *ctx, const filo_instr *in, bool is_and, filo_value *out) {
    for (uint32_t i = 0; i < in->nargs; i++) {
        filo_value v = {0};
        if (eval(ctx, in->args[i], &v) != FILO_OK) {
            return FILO_ERR;
        }
        bool b = false;
        if (as_bool(ctx, &v, &b) != FILO_OK) {
            return FILO_ERR;
        }
        if (b != is_and) { /* and stops at the first false, or at the first true */
            *out = filo_bool(b);
            return FILO_OK;
        }
    }
    *out = filo_bool(is_and);
    return FILO_OK;
}

static int eval_let(filo_ctx *ctx, const filo_instr *in, filo_value *out) {
    uint32_t n = in->a;
    if (in->nargs <= n) {
        return filo_fail(ctx, "let expects bindings and body");
    }
    frame *f = ralloc(ctx, sizeof(frame));
    if (f == NULL) {
        return FILO_ERR;
    }
    f->slots = NULL;
    f->n = n;
    if (n > 0) {
        f->slots = ralloc(ctx, sizeof(filo_value) * n);
        if (f->slots == NULL) {
            return FILO_ERR;
        }
        memset(f->slots, 0, sizeof(filo_value) * n);
    }
    f->parent = ctx->frame;
    frame *old = ctx->frame;
    ctx->frame = f; /* active while the values run: a binding may read earlier ones */
    for (uint32_t i = 0; i < n; i++) {
        if (eval(ctx, in->args[i], &f->slots[i]) != FILO_OK) {
            ctx->frame = old;
            return FILO_ERR;
        }
    }
    int rc = eval_body(ctx, in->args + n, in->nargs - n, out);
    ctx->frame = old;
    return rc;
}

static int eval_letv(filo_ctx *ctx, const filo_instr *in, filo_value *out) {
    filo_value t = {0};
    if (eval(ctx, in->args[0], &t) != FILO_OK) { /* in the outer scope */
        return FILO_ERR;
    }
    if (t.kind != FILO_TUPLE) {
        return filo_fail(ctx, "letv expects tuple expression");
    }
    if (t.u.seq.len != in->nnames) {
        return filo_fail(ctx, "letv arity mismatch");
    }
    frame *f = ralloc(ctx, sizeof(frame));
    if (f == NULL) {
        return FILO_ERR;
    }
    f->n = in->nnames;
    f->slots = NULL;
    if (f->n > 0) {
        /* a copy: a later set must not write into the tuple */
        f->slots = ralloc(ctx, sizeof(filo_value) * f->n);
        if (f->slots == NULL) {
            return FILO_ERR;
        }
        memcpy(f->slots, t.u.seq.items, sizeof(filo_value) * f->n);
    }
    f->parent = ctx->frame;
    frame *old = ctx->frame;
    ctx->frame = f;
    int rc = eval_body(ctx, in->args + 1, in->nargs - 1, out);
    ctx->frame = old;
    return rc;
}

static void set_global_id(filo_ctx *ctx, uint32_t id, filo_value v);

static int eval_set(filo_ctx *ctx, const filo_instr *in, filo_value *out) {
    if (in->nargs != 2) {
        return filo_fail(ctx, "set expects name and expression");
    }
    filo_value v = {0};
    if (eval(ctx, in->args[1], &v) != FILO_OK) {
        return FILO_ERR;
    }
    const filo_instr *target = in->args[0];
    switch (target->op) {
    case OP_LOCAL: {
        frame *f = ctx->frame;
        for (uint32_t i = 0; i < target->a; i++) {
            f = f->parent;
        }
        f->slots[target->b] = v;
        *out = v;
        return FILO_OK;
    }
    case OP_GLOBAL:
        set_global_id(ctx, target->a, v);
        *out = v;
        return FILO_OK;
    case OP_DYNAMIC: {
        int32_t id = symbol_id(ctx, (const uint8_t *)target->name, (uint32_t)strlen(target->name));
        if (id < 0) {
            return FILO_ERR;
        }
        set_global_id(ctx, (uint32_t)id, v);
        *out = v;
        return FILO_OK;
    }
    default:
        return filo_fail(ctx, "set name must be symbol");
    }
}

static int eval_fn(filo_ctx *ctx, const filo_instr *in, filo_value *out) {
    if (in->nargs == 0) {
        return filo_fail(ctx, "fn expects parameters and body");
    }
    filo_func *fn = ralloc(ctx, sizeof(filo_func));
    if (fn == NULL) {
        return FILO_ERR;
    }
    fn->params = in->names;
    fn->nparams = in->nnames;
    fn->body = in->args;
    fn->nbody = in->nargs;
    fn->frame = ctx->frame;
    memset(out, 0, sizeof(*out));
    out->kind = FILO_FUNC;
    out->u.fn = fn;
    return FILO_OK;
}

static int eval_def(filo_ctx *ctx, const filo_instr *in, filo_value *out) {
    if (in->msg != NULL) {
        return filo_fail(ctx, in->msg);
    }
    filo_value v = {0};
    if (eval(ctx, in->args[0], &v) != FILO_OK) {
        return FILO_ERR;
    }
    int32_t id = symbol_id(ctx, (const uint8_t *)in->name, (uint32_t)strlen(in->name));
    if (id < 0) {
        return FILO_ERR;
    }
    set_global_id(ctx, (uint32_t)id, v);
    *out = v;
    return FILO_OK;
}

static int eval_signal(filo_ctx *ctx, const filo_instr *in, uint8_t sig) {
    if (in->nargs > 1) {
        return filo_fail2(ctx, in->name, " expects 0 or 1 argument");
    }
    filo_value v = empty_list();
    if (in->nargs == 1 && eval(ctx, in->args[0], &v) != FILO_OK) {
        return FILO_ERR;
    }
    ctx->signal = sig;
    ctx->signaled = v;
    return FILO_ERR;
}

static int eval(filo_ctx *ctx, const filo_instr *in, filo_value *out) {
    if (ctx->host.should_stop != NULL && ctx->host.should_stop(ctx->host.user)) {
        return filo_fail(ctx, "execution cancelled");
    }
    ctx->steps++;
    if (ctx->limits.step_limit > 0 && ctx->steps > ctx->limits.step_limit) {
        return filo_fail(ctx, "step limit exceeded");
    }
    if (ctx->depth >= FILO_EVAL_DEPTH_MAX) {
        return filo_fail(ctx, "evaluation too deep");
    }
    ctx->depth++;
    int rc = FILO_ERR;
    switch (in->op) {
    case OP_CONST:
        *out = in->val;
        rc = FILO_OK;
        break;
    case OP_LOCAL: {
        frame *f = ctx->frame;
        for (uint32_t i = 0; i < in->a; i++) {
            f = f->parent;
        }
        *out = f->slots[in->b];
        rc = FILO_OK;
        break;
    }
    case OP_GLOBAL:
        if (!ctx->defined[in->a]) {
            rc = filo_fail2(ctx, "undefined global: ", in->name);
            break;
        }
        *out = ctx->globals[in->a];
        rc = FILO_OK;
        break;
    case OP_DYNAMIC:
        rc = filo_fail2(ctx, "undefined symbol: ", in->name);
        for (uint32_t i = 0; i < ctx->nsymbols; i++) {
            if (strcmp(ctx->symbols[i], in->name) == 0 && ctx->defined[i]) {
                *out = ctx->globals[i];
                rc = FILO_OK;
                break;
            }
        }
        break;
    case OP_BUILTIN:
        rc = filo_fail2(ctx, "builtin cannot be used as value: ", in->name);
        break;
    case OP_EMPTY:
        rc = filo_fail(ctx, "empty list expression");
        break;
    case OP_INVALID:
        (void)filo_fail(ctx, in->msg);
        rc = fail_in(ctx, in->name);
        break;
    case OP_IF:
        rc = wrap(ctx, "if", eval_if(ctx, in, out));
        break;
    case OP_COND:
        rc = wrap(ctx, "cond", eval_cond(ctx, in, out));
        break;
    case OP_DO:
        rc = wrap(ctx, "do", eval_do(ctx, in, out));
        break;
    case OP_AND:
        rc = wrap(ctx, "and", eval_logic(ctx, in, true, out));
        break;
    case OP_OR:
        rc = wrap(ctx, "or", eval_logic(ctx, in, false, out));
        break;
    case OP_LET:
        rc = wrap(ctx, "let", eval_let(ctx, in, out));
        break;
    case OP_LETV:
        rc = wrap(ctx, "letv", eval_letv(ctx, in, out));
        break;
    case OP_SET:
        rc = wrap(ctx, "set", eval_set(ctx, in, out));
        break;
    case OP_FN:
        rc = wrap(ctx, "fn", eval_fn(ctx, in, out));
        break;
    case OP_DEF:
        rc = wrap(ctx, "def", eval_def(ctx, in, out));
        break;
    case OP_TUPLE: {
        filo_value *vals = NULL;
        rc = eval_args(ctx, in->args, in->nargs, &vals);
        if (rc == FILO_OK) {
            rc = filo_tuple(ctx, vals, in->nargs, out);
        }
        rc = wrap(ctx, in->name, rc);
        break;
    }
    case OP_EXIT:
        rc = eval_signal(ctx, in, SIG_EXIT);
        break;
    case OP_RETURN:
        rc = eval_signal(ctx, in, SIG_RETURN);
        break;
    case OP_CALLB:
        rc = call_builtin(ctx, in->name, in->fn, in->args, in->nargs, out);
        break;
    case OP_CALL:
        rc = eval_call(ctx, in, out);
        break;
    default:
        rc = filo_fail(ctx, "unknown instruction");
        break;
    }
    ctx->depth--;
    return rc;
}

/* ---------------------------------------------------------------- globals */

static bool in_persistent(const filo_ctx *ctx, const void *p) {
    const uint8_t *b = (const uint8_t *)p;
    if (b < ctx->persistent.base) {
        return false;
    }
    return b < ctx->persistent.base + ctx->persistent.cap;
}

/* Frames reachable from a persisted closure may form cycles (a closure
   stored in the frame it captured); the memo breaks them and bounds the
   copy. */
enum { COPY_MEMO_MAX = 128 };

typedef struct {
    const frame *from[COPY_MEMO_MAX];
    frame *to[COPY_MEMO_MAX];
    uint32_t n;
} copy_memo;

static int copy_value(filo_ctx *ctx, const filo_value *src, filo_value *dst, copy_memo *memo);

static frame *copy_frame(filo_ctx *ctx, frame *old, copy_memo *memo) {
    if (old == NULL || in_persistent(ctx, old)) {
        return old;
    }
    for (uint32_t i = 0; i < memo->n; i++) {
        if (memo->from[i] == old) {
            return memo->to[i];
        }
    }
    if (memo->n >= COPY_MEMO_MAX) {
        (void)filo_fail(ctx, "cannot persist function: too many frames");
        return NULL;
    }
    frame *f = palloc(ctx, sizeof(frame));
    if (f == NULL) {
        return NULL;
    }
    memo->from[memo->n] = old;
    memo->to[memo->n] = f;
    memo->n++;
    f->n = old->n;
    f->slots = NULL;
    if (f->n > 0) {
        f->slots = palloc(ctx, sizeof(filo_value) * f->n);
        if (f->slots == NULL) {
            return NULL;
        }
        for (uint32_t i = 0; i < f->n; i++) {
            if (copy_value(ctx, &old->slots[i], &f->slots[i], memo) != FILO_OK) {
                return NULL;
            }
        }
    }
    f->parent = copy_frame(ctx, old->parent, memo);
    if (old->parent != NULL && f->parent == NULL) {
        return NULL;
    }
    return f;
}

/* Deep-copies a value into the persistent arena. Anything already there is
   shared, not copied: string constants come from the IR, and a persisted
   list only ever holds persisted values. */
static int copy_value(filo_ctx *ctx, const filo_value *src, filo_value *dst, copy_memo *memo) {
    *dst = *src;
    switch (src->kind) {
    case FILO_STRING: {
        if (src->u.str.len == 0 || in_persistent(ctx, src->u.str.ptr)) {
            return FILO_OK;
        }
        uint8_t *copy = palloc(ctx, src->u.str.len);
        if (copy == NULL) {
            return FILO_ERR;
        }
        memcpy(copy, src->u.str.ptr, src->u.str.len);
        dst->u.str.ptr = copy;
        return FILO_OK;
    }
    case FILO_LIST:
    case FILO_TUPLE: {
        if (src->u.seq.len == 0 || in_persistent(ctx, src->u.seq.items)) {
            return FILO_OK;
        }
        filo_value *items = palloc(ctx, sizeof(filo_value) * src->u.seq.len);
        if (items == NULL) {
            return FILO_ERR;
        }
        for (uint32_t i = 0; i < src->u.seq.len; i++) {
            if (copy_value(ctx, &src->u.seq.items[i], &items[i], memo) != FILO_OK) {
                return FILO_ERR;
            }
        }
        dst->u.seq.items = items;
        return FILO_OK;
    }
    case FILO_FUNC: {
        if (in_persistent(ctx, src->u.fn)) {
            return FILO_OK;
        }
        filo_func *fn = palloc(ctx, sizeof(filo_func));
        if (fn == NULL) {
            return FILO_ERR;
        }
        *fn = *src->u.fn;
        fn->frame = copy_frame(ctx, src->u.fn->frame, memo);
        if (src->u.fn->frame != NULL && fn->frame == NULL) {
            return FILO_ERR;
        }
        dst->u.fn = fn;
        return FILO_OK;
    }
    default:
        return FILO_OK;
    }
}

/* During a run a global holds run-arena data and is marked dirty; the run
   end copies dirty globals out (or restores them when the run failed). */
static void set_global_id(filo_ctx *ctx, uint32_t id, filo_value v) {
    if (!ctx->dirty[id]) {
        ctx->dirty[id] = true;
        ctx->saved[id] = ctx->globals[id];
        ctx->saved_defined[id] = ctx->defined[id];
    }
    ctx->globals[id] = v;
    ctx->defined[id] = true;
}

static int commit_globals(filo_ctx *ctx) {
    copy_memo memo;
    memo.n = 0;
    int rc = FILO_OK;
    for (uint32_t i = 0; i < ctx->nsymbols; i++) {
        if (!ctx->dirty[i]) {
            continue;
        }
        ctx->dirty[i] = false;
        if (rc != FILO_OK) {
            continue;
        }
        filo_value persisted = {0};
        if (copy_value(ctx, &ctx->globals[i], &persisted, &memo) != FILO_OK) {
            rc = FILO_ERR;
            continue;
        }
        ctx->globals[i] = persisted;
    }
    return rc;
}

static void rollback_globals(filo_ctx *ctx) {
    for (uint32_t i = 0; i < ctx->nsymbols; i++) {
        if (!ctx->dirty[i]) {
            continue;
        }
        ctx->dirty[i] = false;
        ctx->globals[i] = ctx->saved[i];
        ctx->defined[i] = ctx->saved_defined[i];
    }
}

int filo_set_global(filo_ctx *ctx, const char *name, filo_value v) {
    int32_t id = symbol_id(ctx, (const uint8_t *)name, (uint32_t)strlen(name));
    if (id < 0) {
        return FILO_ERR;
    }
    copy_memo memo;
    memo.n = 0;
    filo_value persisted = {0};
    if (copy_value(ctx, &v, &persisted, &memo) != FILO_OK) {
        return FILO_ERR;
    }
    ctx->globals[id] = persisted;
    ctx->defined[id] = true;
    return FILO_OK;
}

bool filo_get_global(const filo_ctx *ctx, const char *name, filo_value *out) {
    size_t len = strlen(name);
    for (uint32_t i = 0; i < ctx->nsymbols; i++) {
        if (cname_eq(ctx->symbols[i], (const uint8_t *)name, (uint32_t)len)) {
            if (!ctx->defined[i]) {
                return false;
            }
            *out = ctx->globals[i];
            return true;
        }
    }
    return false;
}

/* ------------------------------------------------------------- rendering */

/* A sink that either counts or writes, so a value is rendered twice: once to
   size the run-arena buffer, once into it. */
typedef struct {
    char *dst;
    size_t cap;
    size_t pos;
} sink;

static void sink_put(sink *s, const char *bytes, size_t n) {
    if (s->dst != NULL && s->pos < s->cap) {
        size_t room = s->cap - s->pos;
        memcpy(s->dst + s->pos, bytes, n < room ? n : room);
    }
    s->pos += n;
}

static void sink_puts(sink *s, const char *str) {
    sink_put(s, str, strlen(str));
}

/* Quotes like Go's %q: printable bytes verbatim, the usual escapes named,
   other control bytes as \x.. */
static void sink_quoted(sink *s, filo_str str) {
    static const char hex[] = "0123456789abcdef";
    sink_puts(s, "\"");
    for (uint32_t i = 0; i < str.len; i++) {
        uint8_t c = str.ptr[i];
        char esc[5] = {'\\', 0, 0, 0, 0};
        switch (c) {
        case '"':
            esc[1] = '"';
            sink_put(s, esc, 2);
            break;
        case '\\':
            esc[1] = '\\';
            sink_put(s, esc, 2);
            break;
        case '\n':
            esc[1] = 'n';
            sink_put(s, esc, 2);
            break;
        case '\t':
            esc[1] = 't';
            sink_put(s, esc, 2);
            break;
        case '\r':
            esc[1] = 'r';
            sink_put(s, esc, 2);
            break;
        default:
            if (c < 0x20 || c == 0x7F) {
                esc[1] = 'x';
                esc[2] = hex[(unsigned)c >> 4U];
                esc[3] = hex[(unsigned)c & 0xFU];
                sink_put(s, esc, 4);
            } else {
                sink_put(s, (const char *)&str.ptr[i], 1);
            }
            break;
        }
    }
    sink_puts(s, "\"");
}

static int render(filo_ctx *ctx, const filo_value *v, sink *s, bool quote) {
    switch (v->kind) {
    case FILO_NUMBER: {
        char buf[48];
        if (ctx->host.num_to_str == NULL) {
            return filo_fail(ctx, "number formatting unavailable on this host");
        }
        size_t n = ctx->host.num_to_str(ctx->host.user, v->u.num, buf, sizeof(buf));
        if (n == 0) {
            return filo_fail(ctx, "number formatting failed");
        }
        sink_put(s, buf, n);
        return FILO_OK;
    }
    case FILO_BOOL:
        sink_puts(s, v->u.b ? "#t" : "#f");
        return FILO_OK;
    case FILO_STRING:
        if (quote) {
            sink_quoted(s, v->u.str);
        } else {
            sink_put(s, (const char *)v->u.str.ptr, v->u.str.len);
        }
        return FILO_OK;
    case FILO_LIST:
    case FILO_TUPLE:
        sink_puts(s, v->kind == FILO_LIST ? "(list" : "(tuple");
        for (uint32_t i = 0; i < v->u.seq.len; i++) {
            sink_puts(s, " ");
            if (render(ctx, &v->u.seq.items[i], s, true) != FILO_OK) {
                return FILO_ERR;
            }
        }
        sink_puts(s, ")");
        return FILO_OK;
    case FILO_FUNC:
        return filo_fail(ctx, "string: cannot convert a function");
    default:
        return filo_fail(ctx, "cannot render value");
    }
}

int filo_value_text(filo_ctx *ctx, const filo_value *v, char *dst, size_t cap, size_t *len) {
    sink s = {dst, cap, 0};
    if (render(ctx, v, &s, false) != FILO_OK) {
        return FILO_ERR;
    }
    *len = s.pos;
    if (dst != NULL && s.pos < cap) {
        dst[s.pos] = '\0';
    }
    return FILO_OK;
}

/* Renders into a fresh run-arena string value — what (string v) yields. */
static int render_to_value(filo_ctx *ctx, const filo_value *v, filo_value *out) {
    sink count = {NULL, 0, 0};
    if (render(ctx, v, &count, false) != FILO_OK) {
        return FILO_ERR;
    }
    char *buf = NULL;
    if (count.pos > 0) {
        buf = ralloc(ctx, count.pos);
        if (buf == NULL) {
            return FILO_ERR;
        }
        sink w = {buf, count.pos, 0};
        (void)render(ctx, v, &w, false);
    }
    *out = filo_string((const uint8_t *)buf, (uint32_t)count.pos);
    return FILO_OK;
}

/* ------------------------------------------------------------ public API */

static void register_core(filo_ctx *ctx);

int filo_init(filo_ctx *ctx, const filo_host *host, void *persistent, size_t persistent_cap,
              void *run, size_t run_cap) {
    memset(ctx, 0, sizeof(*ctx));
    if (host != NULL) {
        ctx->host = *host;
    }
    ctx->persistent.base = persistent;
    ctx->persistent.cap = persistent_cap;
    ctx->run.base = run;
    ctx->run.cap = run_cap;
    ctx->limits.step_limit = FILO_STEP_LIMIT_DEFAULT;
    ctx->limits.recursion_limit = FILO_RECURSION_LIMIT_DEFAULT;
    register_core(ctx);
    return FILO_OK;
}

int filo_register_builtin(filo_ctx *ctx, const char *name, filo_builtin fn) {
    if (name == NULL || name[0] == '\0' || fn == NULL) {
        return filo_fail(ctx, "builtin needs a name and a function");
    }
    if (builtin_by_name(ctx, (const uint8_t *)name, (uint32_t)strlen(name)) != NULL) {
        return filo_fail2(ctx, "builtin already registered: ", name);
    }
    if (ctx->nbuiltins >= FILO_BUILTINS_MAX) {
        return filo_fail(ctx, "too many builtins");
    }
    ctx->builtins[ctx->nbuiltins].name = name;
    ctx->builtins[ctx->nbuiltins].fn = fn;
    ctx->nbuiltins++;
    return FILO_OK;
}

int filo_compile(filo_ctx *ctx, const uint8_t *src, size_t len, filo_prog *out) {
    /* the parse tree and the lowering scopes are run-arena temporaries */
    arena_reset(&ctx->run);
    ctx->error[0] = '\0';
    const node *tree = parse_all(ctx, src, len);
    if (tree == NULL) {
        return FILO_ERR;
    }
    lowerer lw = {ctx, NULL};
    if (scope_enter(&lw) == NULL) {
        return FILO_ERR;
    }
    const filo_instr *root = lower(&lw, tree);
    if (root == NULL) {
        return FILO_ERR;
    }
    out->root = root;
    return FILO_OK;
}

int filo_run(filo_ctx *ctx, const filo_prog *prog, const filo_limits *limits, filo_value *result) {
    arena_reset(&ctx->run);
    ctx->error[0] = '\0';
    ctx->steps = 0;
    ctx->recursion = 0;
    ctx->depth = 0;
    ctx->frame = NULL;
    ctx->signal = SIG_NONE;
    filo_limits saved = ctx->limits;
    if (limits != NULL) {
        ctx->limits = *limits;
        if (ctx->limits.step_limit == 0) {
            ctx->limits.step_limit = FILO_STEP_LIMIT_DEFAULT;
        }
        if (ctx->limits.recursion_limit == 0) {
            ctx->limits.recursion_limit = FILO_RECURSION_LIMIT_DEFAULT;
        }
    }
    filo_value v = {0};
    memset(&v, 0, sizeof(v));
    int rc = eval(ctx, prog->root, &v);
    ctx->limits = saved;
    if (rc != FILO_OK && ctx->signal != SIG_NONE) {
        /* exit ends the run with its value; a top-level return acts alike */
        v = ctx->signaled;
        ctx->signal = SIG_NONE;
        ctx->error[0] = '\0';
        rc = FILO_OK;
    }
    if (rc != FILO_OK) {
        rollback_globals(ctx); /* a failed run leaves no trace, as in Go */
        return FILO_ERR;
    }
    if (commit_globals(ctx) != FILO_OK) {
        return FILO_ERR;
    }
    if (result != NULL) {
        *result = v;
    }
    return FILO_OK;
}

const char *filo_error(const filo_ctx *ctx) {
    return ctx->error;
}

/* --------------------------------------------------------------- builtins */

static int as_num(filo_ctx *ctx, const filo_value *v, double *out) {
    if (v->kind != FILO_NUMBER) {
        return fail_expected(ctx, "number", v);
    }
    *out = v->u.num;
    return FILO_OK;
}

static int as_list(filo_ctx *ctx, const filo_value *v, filo_seq *out) {
    if (v->kind != FILO_LIST) {
        return fail_expected(ctx, "list", v);
    }
    *out = v->u.seq;
    return FILO_OK;
}

static int b_add(filo_ctx *ctx, const filo_value *args, uint32_t n, filo_value *out) {
    double sum = 0;
    for (uint32_t i = 0; i < n; i++) {
        double x = 0;
        if (as_num(ctx, &args[i], &x) != FILO_OK) {
            return FILO_ERR;
        }
        sum += x;
    }
    *out = filo_num(sum);
    return FILO_OK;
}

static int b_sub(filo_ctx *ctx, const filo_value *args, uint32_t n, filo_value *out) {
    if (n == 0) {
        return filo_fail(ctx, "- expects at least 1 argument");
    }
    double r = 0;
    if (as_num(ctx, &args[0], &r) != FILO_OK) {
        return FILO_ERR;
    }
    if (n == 1) {
        *out = filo_num(-r);
        return FILO_OK;
    }
    for (uint32_t i = 1; i < n; i++) {
        double x = 0;
        if (as_num(ctx, &args[i], &x) != FILO_OK) {
            return FILO_ERR;
        }
        r -= x;
    }
    *out = filo_num(r);
    return FILO_OK;
}

static int b_mul(filo_ctx *ctx, const filo_value *args, uint32_t n, filo_value *out) {
    double r = 1;
    for (uint32_t i = 0; i < n; i++) {
        double x = 0;
        if (as_num(ctx, &args[i], &x) != FILO_OK) {
            return FILO_ERR;
        }
        r *= x;
    }
    *out = filo_num(r);
    return FILO_OK;
}

static int b_div(filo_ctx *ctx, const filo_value *args, uint32_t n, filo_value *out) {
    if (n == 0) {
        return filo_fail(ctx, "/ expects at least 1 argument");
    }
    double r = 0;
    if (as_num(ctx, &args[0], &r) != FILO_OK) {
        return FILO_ERR;
    }
    if (n == 1) {
        /* reciprocal, the counterpart of (- x) being negation */
        if (r == 0) {
            return filo_fail(ctx, "division by zero");
        }
        *out = filo_num(1 / r);
        return FILO_OK;
    }
    for (uint32_t i = 1; i < n; i++) {
        double x = 0;
        if (as_num(ctx, &args[i], &x) != FILO_OK) {
            return FILO_ERR;
        }
        if (x == 0) {
            return filo_fail(ctx, "division by zero");
        }
        r /= x;
    }
    *out = filo_num(r);
    return FILO_OK;
}

/* fmod without libm: r = a - trunc(a/b)*b, computed with the same rounding
   a C library fmod would give for the magnitudes scripts use. */
static double fmod_trunc(double a, double b) {
    double q = a / b;
    double t = (double)(int64_t)q; /* desvio: truncamento explícito é o algoritmo */
    if (q < 0 && t > q) {
        t -= 1; /* the cast rounded toward zero from the negative side */
    }
    if (q > 0 && t > q) {
        t -= 1;
    }
    return a - t * b;
}

static int b_mod(filo_ctx *ctx, const filo_value *args, uint32_t n, filo_value *out) {
    if (n != 2) {
        return filo_fail(ctx, "% expects 2 arguments");
    }
    double a = 0;
    double b = 0;
    if (as_num(ctx, &args[0], &a) != FILO_OK || as_num(ctx, &args[1], &b) != FILO_OK) {
        return FILO_ERR;
    }
    if (b == 0) {
        return filo_fail(ctx, "modulo by zero");
    }
    /* floored, as in Lua: the result takes the divisor's sign */
    double r = fmod_trunc(a, b);
    if (r != 0 && (r < 0) != (b < 0)) {
        r += b;
    }
    *out = filo_num(r);
    return FILO_OK;
}

/* pow is a host concern in a freestanding build; the core keeps an exact
   path for integral exponents and asks the host (through num hooks being
   present) only implicitly. Fractional exponents use the identity below via
   exp/log approximations would drift from Go, so this runtime declares pow
   for integral exponents and NaN-free bases; the libc host overrides it. */
static double core_pow(double a, double b) {
    if (b == 0) {
        return 1;
    }
    if (b == (double)(int64_t)b && b > -1e6 && b < 1e6) {
        int64_t se = (int64_t)b;
        bool neg = se < 0;
        uint64_t e = (uint64_t)(neg ? -se : se);
        double r = 1;
        double base = a;
        while (e > 0) {
            if ((e & 1U) != 0) {
                r *= base;
            }
            base *= base;
            e >>= 1U;
        }
        return neg ? 1 / r : r;
    }
    return __builtin_nan(""); /* not representable without libm; the libc host installs pow */
}

static double (*pow_hook)(double, double) = core_pow;

static int b_pow(filo_ctx *ctx, const filo_value *args, uint32_t n, filo_value *out) {
    if (n != 2) {
        return filo_fail(ctx, "pow expects 2 arguments");
    }
    double a = 0;
    double b = 0;
    if (as_num(ctx, &args[0], &a) != FILO_OK || as_num(ctx, &args[1], &b) != FILO_OK) {
        return FILO_ERR;
    }
    *out = filo_num(pow_hook(a, b));
    return FILO_OK;
}

void filo_set_pow(double (*fn)(double, double)) {
    pow_hook = fn != NULL ? fn : core_pow;
}

static int b_eq(filo_ctx *ctx, const filo_value *args, uint32_t n, filo_value *out) {
    if (n < 2) {
        return filo_fail(ctx, "= expects at least 2 arguments");
    }
    for (uint32_t i = 1; i < n; i++) {
        if (args[i].kind != args[0].kind) {
            return filo_fail(ctx, "expected values of the same kind");
        }
    }
    for (uint32_t i = 1; i < n; i++) {
        if (!filo_equal(&args[0], &args[i])) {
            *out = filo_bool(false);
            return FILO_OK;
        }
    }
    *out = filo_bool(true);
    return FILO_OK;
}

static int b_ne(filo_ctx *ctx, const filo_value *args, uint32_t n, filo_value *out) {
    if (b_eq(ctx, args, n, out) != FILO_OK) {
        return FILO_ERR;
    }
    bool equal = out->u.b;
    out->u.b = true;
    if (equal) {
        out->u.b = false;
    }
    return FILO_OK;
}

typedef enum { CMP_LT, CMP_LE, CMP_GT, CMP_GE } cmp_op;

static int compare_chain(filo_ctx *ctx, const filo_value *args, uint32_t n, cmp_op op,
                         const char *arity_msg, filo_value *out) {
    if (n < 2) {
        return filo_fail(ctx, arity_msg);
    }
    for (uint32_t i = 1; i < n; i++) {
        double l = 0;
        double r = 0;
        if (as_num(ctx, &args[i - 1], &l) != FILO_OK || as_num(ctx, &args[i], &r) != FILO_OK) {
            return FILO_ERR;
        }
        bool ok = false;
        switch (op) {
        case CMP_LT:
            ok = l < r;
            break;
        case CMP_LE:
            ok = l <= r;
            break;
        case CMP_GT:
            ok = l > r;
            break;
        default:
            ok = l >= r;
            break;
        }
        if (!ok) {
            *out = filo_bool(false);
            return FILO_OK;
        }
    }
    *out = filo_bool(true);
    return FILO_OK;
}

static int b_lt(filo_ctx *ctx, const filo_value *a, uint32_t n, filo_value *out) {
    return compare_chain(ctx, a, n, CMP_LT, "< expects at least 2 arguments", out);
}

static int b_le(filo_ctx *ctx, const filo_value *a, uint32_t n, filo_value *out) {
    return compare_chain(ctx, a, n, CMP_LE, "<= expects at least 2 arguments", out);
}

static int b_gt(filo_ctx *ctx, const filo_value *a, uint32_t n, filo_value *out) {
    return compare_chain(ctx, a, n, CMP_GT, "> expects at least 2 arguments", out);
}

static int b_ge(filo_ctx *ctx, const filo_value *a, uint32_t n, filo_value *out) {
    return compare_chain(ctx, a, n, CMP_GE, ">= expects at least 2 arguments", out);
}

static int b_not(filo_ctx *ctx, const filo_value *args, uint32_t n, filo_value *out) {
    if (n != 1) {
        return filo_fail(ctx, "not expects 1 argument");
    }
    bool b = false;
    if (as_bool(ctx, &args[0], &b) != FILO_OK) {
        return FILO_ERR;
    }
    bool negated = true;
    if (b) {
        negated = false;
    }
    *out = filo_bool(negated);
    return FILO_OK;
}

static int b_string(filo_ctx *ctx, const filo_value *args, uint32_t n, filo_value *out) {
    if (n != 1) {
        return filo_fail(ctx, "string expects 1 argument");
    }
    return render_to_value(ctx, &args[0], out);
}

static bool is_space(uint8_t c) {
    if (c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\v' || c == '\f') {
        return true;
    }
    return false;
}

static int b_number(filo_ctx *ctx, const filo_value *args, uint32_t n, filo_value *out) {
    if (n != 1) {
        return filo_fail(ctx, "number expects 1 argument");
    }
    const filo_value *v = &args[0];
    if (v->kind == FILO_NUMBER) {
        *out = *v;
        return FILO_OK;
    }
    if (v->kind != FILO_STRING) {
        return filo_fail2(ctx, "number expects a number or a numeric string, got ",
                          filo_kind_name(v->kind));
    }
    const uint8_t *s = v->u.str.ptr;
    uint32_t len = v->u.str.len;
    while (len > 0 && is_space(s[0])) {
        s++;
        len--;
    }
    while (len > 0 && is_space(s[len - 1])) {
        len--;
    }
    double x = 0;
    if (ctx->host.str_to_num == NULL || !ctx->host.str_to_num(ctx->host.user, s, len, &x)) {
        return filo_fail(ctx, "number: cannot parse");
    }
    *out = filo_num(x);
    return FILO_OK;
}

static int b_type_of(filo_ctx *ctx, const filo_value *args, uint32_t n, filo_value *out) {
    if (n != 1) {
        return filo_fail(ctx, "type-of expects 1 argument");
    }
    *out = filo_cstring(filo_kind_name(args[0].kind));
    return FILO_OK;
}

static int b_is_empty(filo_ctx *ctx, const filo_value *args, uint32_t n, filo_value *out) {
    if (n != 1) {
        return filo_fail(ctx, "is-empty expects 1 argument");
    }
    const filo_value *v = &args[0];
    bool empty = false;
    if (v->kind == FILO_STRING) {
        empty = v->u.str.len == 0;
    }
    if (v->kind == FILO_LIST || v->kind == FILO_TUPLE) {
        empty = v->u.seq.len == 0;
    }
    *out = filo_bool(empty);
    return FILO_OK;
}

static int b_is_nil(filo_ctx *ctx, const filo_value *args, uint32_t n, filo_value *out) {
    if (n != 1) {
        return filo_fail(ctx, "is-nil expects 1 argument");
    }
    bool nil = false;
    if (args[0].kind == FILO_LIST && args[0].u.seq.len == 0) {
        nil = true;
    }
    *out = filo_bool(nil);
    return FILO_OK;
}

static int b_list(filo_ctx *ctx, const filo_value *args, uint32_t n, filo_value *out) {
    return filo_list(ctx, args, n, out);
}

static int b_length(filo_ctx *ctx, const filo_value *args, uint32_t n, filo_value *out) {
    if (n != 1) {
        return filo_fail(ctx, "length expects 1 argument");
    }
    if (args[0].kind != FILO_LIST && args[0].kind != FILO_TUPLE) {
        return filo_fail(ctx, "length expects list or tuple");
    }
    *out = filo_num((double)args[0].u.seq.len);
    return FILO_OK;
}

static int b_head(filo_ctx *ctx, const filo_value *args, uint32_t n, filo_value *out) {
    if (n != 1) {
        return filo_fail(ctx, "head expects 1 argument");
    }
    filo_seq l;
    if (as_list(ctx, &args[0], &l) != FILO_OK) {
        return FILO_ERR;
    }
    if (l.len == 0) {
        return filo_fail(ctx, "head of empty list");
    }
    *out = l.items[0];
    return FILO_OK;
}

static int b_tail(filo_ctx *ctx, const filo_value *args, uint32_t n, filo_value *out) {
    if (n != 1) {
        return filo_fail(ctx, "tail expects 1 argument");
    }
    filo_seq l;
    if (as_list(ctx, &args[0], &l) != FILO_OK) {
        return FILO_ERR;
    }
    if (l.len == 0) {
        return filo_fail(ctx, "tail of empty list");
    }
    return filo_list(ctx, l.items + 1, l.len - 1, out);
}

/* Integral and exactly representable (|x| <= 2^53); NaN and infinities fail,
   as does anything a 64-bit cast could not round-trip. */
static bool is_integral(double x) {
    if (!(x >= -9007199254740992.0 && x <= 9007199254740992.0)) {
        return false;
    }
    return (double)(int64_t)x == x;
}

static int b_nth(filo_ctx *ctx, const filo_value *args, uint32_t n, filo_value *out) {
    if (n != 2) {
        return filo_fail(ctx, "nth expects 2 arguments");
    }
    filo_seq l;
    double idx = 0;
    if (as_list(ctx, &args[0], &l) != FILO_OK || as_num(ctx, &args[1], &idx) != FILO_OK) {
        return FILO_ERR;
    }
    if (!is_integral(idx)) {
        return filo_fail(ctx, "nth expects an integer index");
    }
    if (idx < 0 || idx >= (double)l.len) {
        return filo_fail(ctx, "index out of range");
    }
    *out = l.items[(uint32_t)idx];
    return FILO_OK;
}

static int b_list_append(filo_ctx *ctx, const filo_value *args, uint32_t n, filo_value *out) {
    if (n != 2) {
        return filo_fail(ctx, "list-append expects 2 arguments (list, value)");
    }
    filo_seq l;
    if (as_list(ctx, &args[0], &l) != FILO_OK) {
        return prefix_error(ctx, "list-append: first argument must be list: ");
    }
    if (make_seq(ctx, FILO_LIST, NULL, l.len + 1, out) != FILO_OK) {
        return FILO_ERR;
    }
    if (l.len > 0) {
        memcpy(out->u.seq.items, l.items, sizeof(filo_value) * l.len);
    }
    out->u.seq.items[l.len] = args[1];
    return FILO_OK;
}

static int b_list_concat(filo_ctx *ctx, const filo_value *args, uint32_t n, filo_value *out) {
    if (n < 2) {
        return filo_fail(ctx, "list-concat expects at least 2 arguments");
    }
    uint32_t total = 0;
    for (uint32_t i = 0; i < n; i++) {
        if (args[i].kind != FILO_LIST) {
            char msg[64];
            size_t k = cstr_copy(msg, sizeof(msg), "list-concat: argument ");
            k += u32_text(msg + k, sizeof(msg) - k, i);
            (void)cstr_copy(msg + k, sizeof(msg) - k, " is not a list");
            return filo_fail(ctx, msg);
        }
        total += args[i].u.seq.len;
    }
    if (make_seq(ctx, FILO_LIST, NULL, total, out) != FILO_OK) {
        return FILO_ERR;
    }
    uint32_t at = 0;
    for (uint32_t i = 0; i < n; i++) {
        if (args[i].u.seq.len > 0) {
            memcpy(out->u.seq.items + at, args[i].u.seq.items,
                   sizeof(filo_value) * args[i].u.seq.len);
            at += args[i].u.seq.len;
        }
    }
    return FILO_OK;
}

static int b_map(filo_ctx *ctx, const filo_value *args, uint32_t n, filo_value *out) {
    if (n != 2) {
        return filo_fail(ctx, "map expects function and list");
    }
    if (args[0].kind != FILO_FUNC) {
        return filo_fail(ctx, "map expects function as first argument");
    }
    filo_seq l;
    if (as_list(ctx, &args[1], &l) != FILO_OK) {
        return FILO_ERR;
    }
    if (make_seq(ctx, FILO_LIST, NULL, l.len, out) != FILO_OK) {
        return FILO_ERR;
    }
    for (uint32_t i = 0; i < l.len; i++) {
        if (filo_call(ctx, &args[0], &l.items[i], 1, &out->u.seq.items[i]) != FILO_OK) {
            return FILO_ERR;
        }
    }
    return FILO_OK;
}

static int b_fold(filo_ctx *ctx, const filo_value *args, uint32_t n, filo_value *out) {
    if (n != 3) {
        return filo_fail(ctx, "fold expects function, initial value, and list");
    }
    if (args[0].kind != FILO_FUNC) {
        return filo_fail(ctx, "fold expects function as first argument");
    }
    filo_seq l;
    if (as_list(ctx, &args[2], &l) != FILO_OK) {
        return FILO_ERR;
    }
    filo_value acc = args[1];
    for (uint32_t i = 0; i < l.len; i++) {
        filo_value pair[2] = {acc, l.items[i]};
        if (filo_call(ctx, &args[0], pair, 2, &acc) != FILO_OK) {
            return FILO_ERR;
        }
    }
    *out = acc;
    return FILO_OK;
}

static int b_filter(filo_ctx *ctx, const filo_value *args, uint32_t n, filo_value *out) {
    if (n != 2) {
        return filo_fail(ctx, "filter expects function and list");
    }
    if (args[0].kind != FILO_FUNC) {
        return filo_fail(ctx, "filter expects function as first argument");
    }
    filo_seq l;
    if (as_list(ctx, &args[1], &l) != FILO_OK) {
        return FILO_ERR;
    }
    if (make_seq(ctx, FILO_LIST, NULL, l.len, out) != FILO_OK) {
        return FILO_ERR;
    }
    uint32_t kept = 0;
    for (uint32_t i = 0; i < l.len; i++) {
        filo_value v = {0};
        if (filo_call(ctx, &args[0], &l.items[i], 1, &v) != FILO_OK) {
            return FILO_ERR;
        }
        bool keep = false;
        if (as_bool(ctx, &v, &keep) != FILO_OK) {
            return prefix_error(ctx, "filter predicate must return a bool: ");
        }
        if (keep) {
            out->u.seq.items[kept] = l.items[i];
            kept++;
        }
    }
    out->u.seq.len = kept;
    return FILO_OK;
}

static int b_reverse(filo_ctx *ctx, const filo_value *args, uint32_t n, filo_value *out) {
    if (n != 1) {
        return filo_fail(ctx, "reverse expects 1 argument");
    }
    filo_seq l;
    if (as_list(ctx, &args[0], &l) != FILO_OK) {
        return FILO_ERR;
    }
    if (make_seq(ctx, FILO_LIST, NULL, l.len, out) != FILO_OK) {
        return FILO_ERR;
    }
    for (uint32_t i = 0; i < l.len; i++) {
        out->u.seq.items[l.len - 1 - i] = l.items[i];
    }
    return FILO_OK;
}

static int b_range(filo_ctx *ctx, const filo_value *args, uint32_t n, filo_value *out) {
    if (n != 1 && n != 2) {
        return filo_fail(ctx, "range expects 1 or 2 arguments");
    }
    double start = 0;
    double end = 0;
    if (n == 1) {
        if (as_num(ctx, &args[0], &end) != FILO_OK) {
            return FILO_ERR;
        }
    } else if (as_num(ctx, &args[0], &start) != FILO_OK || as_num(ctx, &args[1], &end) != FILO_OK) {
        return FILO_ERR;
    }
    if (!is_integral(start) || !is_integral(end)) {
        return filo_fail(ctx, "range expects integer bounds");
    }
    int64_t lo = (int64_t)start;
    int64_t hi = (int64_t)end;
    if (hi <= lo) {
        *out = empty_list();
        return FILO_OK;
    }
    if (hi - lo > (int64_t)FILO_RANGE_MAX) {
        return filo_fail(ctx, "range too large");
    }
    uint32_t count = (uint32_t)(hi - lo);
    if (make_seq(ctx, FILO_LIST, NULL, count, out) != FILO_OK) {
        return FILO_ERR;
    }
    for (uint32_t i = 0; i < count; i++) {
        out->u.seq.items[i] = filo_num((double)(lo + (int64_t)i));
    }
    return FILO_OK;
}

static int b_error(filo_ctx *ctx, const filo_value *args, uint32_t n, filo_value *out) {
    (void)out;
    if (n != 1) {
        return filo_fail(ctx, "error expects 1 argument (a message string)");
    }
    if (args[0].kind != FILO_STRING) {
        (void)fail_expected(ctx, "string", &args[0]);
        return prefix_error(ctx, "error expects a string message: ");
    }
    /* the message is script text: bounded copy, no format directives */
    size_t len = args[0].u.str.len;
    if (len > sizeof(ctx->error) - 1) {
        len = sizeof(ctx->error) - 1;
    }
    memcpy(ctx->error, args[0].u.str.ptr, len);
    ctx->error[len] = '\0';
    return FILO_ERR;
}

static void register_core(filo_ctx *ctx) {
    static const filo_builtin_entry core[] = {
        {"+", b_add},
        {"-", b_sub},
        {"*", b_mul},
        {"/", b_div},
        {"%", b_mod},
        {"pow", b_pow},
        {"=", b_eq},
        {"!=", b_ne},
        {"<", b_lt},
        {"<=", b_le},
        {">", b_gt},
        {">=", b_ge},
        {"not", b_not},
        {"string", b_string},
        {"number", b_number},
        {"type-of", b_type_of},
        {"is-empty", b_is_empty},
        {"is-nil", b_is_nil},
        {"list", b_list},
        {"length", b_length},
        {"head", b_head},
        {"tail", b_tail},
        {"nth", b_nth},
        {"list-append", b_list_append},
        {"list-concat", b_list_concat},
        {"map", b_map},
        {"fold", b_fold},
        {"filter", b_filter},
        {"reverse", b_reverse},
        {"range", b_range},
        {"error", b_error},
    };
    for (size_t i = 0; i < sizeof(core) / sizeof(core[0]); i++) {
        (void)filo_register_builtin(ctx, core[i].name, core[i].fn);
    }
}
