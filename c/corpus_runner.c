/* Runs the language corpus (the .txt files under testdata/corpus) against the C runtime.
   Same format the Go runner reads (see testdata/corpus/README.md); a file
   that names a pack this runtime does not have is skipped (math and strings
   are registered on demand). Exit status is
   the number of failing cases, capped at 255. */
#include <math.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#include "filo.h"
#include "filo_libc.h"
#include "filo_nolibc.h"

enum {
    MEM_PERSISTENT = 4U << 20U,
    MEM_RUN = 8U << 20U,
    LINE_MAX_LEN = 4096,
    TEXT_MAX = 65536,
    BINDINGS_MAX = 16,
};

typedef struct {
    char name[256];
    char expr[1024];
} binding;

typedef struct {
    char name[256];
    int line;
    binding given[BINDINGS_MAX];
    int ngiven;
    binding globals[BINDINGS_MAX];
    int nglobals;
    filo_limits limits;
    bool has_limits;
    bool needs_pow;
    bool needs_math;
    char script[TEXT_MAX];
    char want[TEXT_MAX];
    bool want_err;
} corpus_case;

static uint8_t persistent_mem[MEM_PERSISTENT];
static uint8_t run_mem[MEM_RUN];

/* the packs the current file asked for */
static bool want_math = false;
static bool want_strings = false;

/* The same corpus against the libc-free host: a runtime that answers
   differently depending on who formats its numbers is two runtimes. */
static bool use_nolibc = false;

static void init_ctx(filo_ctx *ctx, void *p, size_t pcap, void *r, size_t rcap) {
    filo_init(ctx, use_nolibc ? &filo_nolibc_host : &filo_libc_host, p, pcap, r, rcap);
    if (want_math) {
        (void)filo_math_register(ctx, use_nolibc ? NULL : &filo_libc_math);
    }
    if (want_strings) {
        (void)filo_strings_register(ctx, use_nolibc ? &filo_nolibc_strings : &filo_libc_strings);
    }
}

/* Reads a "packs:" line; false when it names a pack this runtime lacks. */
static bool select_packs(const char *list) {
    want_math = false;
    want_strings = false;
    char buf[256];
    snprintf(buf, sizeof(buf), "%s", list);
    for (const char *tok = strtok(buf, " \t,"); tok != NULL; tok = strtok(NULL, " \t,")) {
        if (strcmp(tok, "math") == 0) {
            want_math = true;
        } else if (strcmp(tok, "strings") == 0) {
            want_strings = true;
        } else {
            return false;
        }
    }
    return true;
}

static bool same_value(const filo_value *a, const filo_value *b) {
    if (a->kind != b->kind) {
        return false;
    }
    if (a->kind == FILO_NUMBER && isnan(a->u.num) && isnan(b->u.num)) {
        return true;
    }
    if (a->kind == FILO_LIST || a->kind == FILO_TUPLE) {
        if (a->u.seq.len != b->u.seq.len) {
            return false;
        }
        for (uint32_t i = 0; i < a->u.seq.len; i++) {
            if (!same_value(&a->u.seq.items[i], &b->u.seq.items[i])) {
                return false;
            }
        }
        return true;
    }
    return filo_equal(a, b);
}

static void show(filo_ctx *ctx, const filo_value *v, char *buf, size_t cap) {
    size_t n = 0;
    if (filo_value_text(ctx, v, buf, cap, &n) != FILO_OK) {
        snprintf(buf, cap, "<%s>", filo_error(ctx));
    }
}

/* Evaluates an expression on a fresh context; the value is copied into the
   caller's context as a global so it survives the callee's memory. */
static bool eval_into(filo_ctx *dst, const char *global, const char *expr, char *why, size_t cap) {
    static uint8_t p2[1U << 20U];
    static uint8_t r2[1U << 20U];
    filo_ctx *tmp = malloc(sizeof(filo_ctx));
    if (tmp == NULL) {
        snprintf(why, cap, "out of memory");
        return false;
    }
    init_ctx(tmp, p2, sizeof(p2), r2, sizeof(r2));
    filo_prog prog;
    filo_value v;
    bool ok = false;
    if (filo_compile(tmp, (const uint8_t *)expr, strlen(expr), &prog) == FILO_OK &&
        filo_run(tmp, &prog, NULL, &v) == FILO_OK) {
        ok = true;
    }
    if (!ok) {
        snprintf(why, cap, "\"%s\" does not evaluate: %s", expr, filo_error(tmp));
        free(tmp);
        return false;
    }
    ok = filo_set_global(dst, global, v) == FILO_OK;
    if (!ok) {
        snprintf(why, cap, "%s", filo_error(dst));
    }
    free(tmp);
    return ok;
}

static bool run_case(const corpus_case *c, char *why, size_t cap) {
    filo_ctx *ctx = malloc(sizeof(filo_ctx));
    if (ctx == NULL) {
        snprintf(why, cap, "out of memory");
        return false;
    }
    init_ctx(ctx, persistent_mem, sizeof(persistent_mem), run_mem, sizeof(run_mem));
    for (int i = 0; i < c->ngiven; i++) {
        if (!eval_into(ctx, c->given[i].name, c->given[i].expr, why, cap)) {
            free(ctx);
            return false;
        }
    }
    filo_prog prog;
    filo_value got;
    bool failed = true;
    if (filo_compile(ctx, (const uint8_t *)c->script, strlen(c->script), &prog) == FILO_OK &&
        filo_run(ctx, &prog, c->has_limits ? &c->limits : NULL, &got) == FILO_OK) {
        failed = false;
    }
    if (c->want_err) {
        bool ok = true;
        if (!failed) {
            char buf[512];
            show(ctx, &got, buf, sizeof(buf));
            snprintf(why, cap, "expected an error, got %s", buf);
            ok = false;
        }
        free(ctx);
        return ok;
    }
    if (failed) {
        snprintf(why, cap, "unexpected error: %s", filo_error(ctx));
        free(ctx);
        return false;
    }
    /* the expectation is evaluated with its own memory, then compared */
    if (!eval_into(ctx, "__want", c->want, why, cap)) {
        free(ctx);
        return false;
    }
    filo_value want;
    (void)filo_get_global(ctx, "__want", &want);
    if (!same_value(&got, &want)) {
        char g[512];
        char w[512];
        show(ctx, &got, g, sizeof(g));
        show(ctx, &want, w, sizeof(w));
        snprintf(why, cap, "got %s, want %s", g, w);
        free(ctx);
        return false;
    }
    for (int i = 0; i < c->nglobals; i++) {
        filo_value v;
        if (!filo_get_global(ctx, c->globals[i].name, &v)) {
            snprintf(why, cap, "global %s not set after the run", c->globals[i].name);
            free(ctx);
            return false;
        }
        if (!eval_into(ctx, "__want", c->globals[i].expr, why, cap)) {
            free(ctx);
            return false;
        }
        (void)filo_get_global(ctx, "__want", &want);
        if (!same_value(&v, &want)) {
            char g[512];
            char w[512];
            show(ctx, &v, g, sizeof(g));
            show(ctx, &want, w, sizeof(w));
            snprintf(why, cap, "global %s is %s, want %s", c->globals[i].name, g, w);
            free(ctx);
            return false;
        }
    }
    free(ctx);
    return true;
}

/* ---- the file format ---- */

static void rstrip(char *s) {
    size_t n = strlen(s);
    while (n > 0 && (s[n - 1] == '\n' || s[n - 1] == '\r' || s[n - 1] == ' ' || s[n - 1] == '\t')) {
        s[n - 1] = '\0';
        n--;
    }
}

static bool parse_binding(const char *s, binding *b) {
    const char *eq = strchr(s, '=');
    if (eq == NULL) {
        return false;
    }
    size_t nl = (size_t)(eq - s);
    while (nl > 0 && (s[nl - 1] == ' ' || s[nl - 1] == '\t')) {
        nl--;
    }
    const char *e = eq + 1;
    while (*e == ' ' || *e == '\t') {
        e++;
    }
    if (nl == 0 || nl >= sizeof(b->name) || *e == '\0' || strlen(e) >= sizeof(b->expr)) {
        return false;
    }
    memcpy(b->name, s, nl);
    b->name[nl] = '\0';
    strcpy(b->expr, e);
    rstrip(b->expr);
    return true;
}

static bool parse_limits(const char *s, filo_limits *l) {
    memset(l, 0, sizeof(*l));
    char buf[256];
    snprintf(buf, sizeof(buf), "%s", s);
    for (char *tok = strtok(buf, " \t"); tok != NULL; tok = strtok(NULL, " \t")) {
        char *eq = strchr(tok, '=');
        if (eq == NULL) {
            return false;
        }
        *eq = '\0';
        long v = strtol(eq + 1, NULL, 10);
        if (v <= 0) {
            return false;
        }
        if (strcmp(tok, "steps") == 0) {
            l->step_limit = (uint32_t)v;
        } else if (strcmp(tok, "recursion") == 0) {
            l->recursion_limit = (uint32_t)v;
        } else {
            return false;
        }
    }
    return true;
}

typedef enum { SEC_NONE, SEC_SCRIPT, SEC_WANT, SEC_GLOBALS } section;

static int failures = 0;
static int passed = 0;

static int skipped = 0;

static void finish_case(const char *file, corpus_case *c) {
    if (c->name[0] == '\0') {
        return;
    }
    if (use_nolibc && (c->needs_pow || c->needs_math)) {
        skipped++; /* this host cannot compute it, which is not a disagreement */
        memset(c, 0, sizeof(*c));
        return;
    }
    rstrip(c->script);
    rstrip(c->want);
    char why[1024] = {0};
    if (run_case(c, why, sizeof(why))) {
        passed++;
    } else {
        failures++;
        printf("FAIL %s/%s (line %d): %s\n", file, c->name, c->line, why);
    }
    memset(c, 0, sizeof(*c));
}

static void append_line(char *dst, size_t cap, const char *line) {
    size_t n = strlen(dst);
    size_t l = strlen(line);
    if (n + l + 2 > cap) {
        return;
    }
    if (n > 0) {
        dst[n] = '\n';
        n++;
    }
    memcpy(dst + n, line, l + 1);
}

static bool run_file(const char *path) {
    want_math = false;
    want_strings = false;
    FILE *f = fopen(path, "r");
    if (f == NULL) {
        printf("cannot open %s\n", path);
        return false;
    }
    const char *base = strrchr(path, '/');
    base = base != NULL ? base + 1 : path;
    corpus_case *c = calloc(1, sizeof(corpus_case));
    if (c == NULL) {
        fclose(f);
        return false;
    }
    section sec = SEC_NONE;
    bool in_case = false;
    char line[LINE_MAX_LEN];
    int n = 0;
    while (fgets(line, sizeof(line), f) != NULL) {
        n++;
        rstrip(line);
        if (strncmp(line, "=== ", 4) == 0) {
            finish_case(base, c);
            snprintf(c->name, sizeof(c->name), "%s", line + 4);
            c->line = n;
            sec = SEC_SCRIPT;
            in_case = true;
            continue;
        }
        if (strncmp(line, "--- ", 4) == 0) {
            const char *s = line + 4;
            if (strcmp(s, "want") == 0) {
                sec = SEC_WANT;
            } else if (strcmp(s, "error") == 0) {
                c->want_err = true;
                sec = SEC_NONE;
            } else if (strcmp(s, "globals") == 0) {
                sec = SEC_GLOBALS;
            }
            continue;
        }
        if (!in_case) {
            if (strncmp(line, "packs:", 6) == 0 && !select_packs(line + 6)) {
                printf("skip %s: needs packs%s\n", base, line + 6);
                free(c);
                fclose(f);
                return true;
            }
            continue;
        }
        switch (sec) {
        case SEC_SCRIPT:
            if (c->script[0] == '\0' && strncmp(line, "given ", 6) == 0 &&
                c->ngiven < BINDINGS_MAX) {
                (void)parse_binding(line + 6, &c->given[c->ngiven]);
                c->ngiven++;
                continue;
            }
            if (c->script[0] == '\0' && strncmp(line, "needs ", 6) == 0) {
                if (strcmp(line + 6, "host-pow") == 0) {
                    c->needs_pow = true;
                } else if (strcmp(line + 6, "host-math") == 0) {
                    c->needs_math = true;
                }
                continue;
            }
            if (c->script[0] == '\0' && strncmp(line, "limits ", 7) == 0) {
                c->has_limits = parse_limits(line + 7, &c->limits);
                continue;
            }
            append_line(c->script, sizeof(c->script), line);
            break;
        case SEC_WANT:
            if (line[0] == '\0') {
                if (c->want[0] != '\0') {
                    sec = SEC_NONE; /* the expectation ended; only comments may follow */
                }
                break;
            }
            append_line(c->want, sizeof(c->want), line);
            break;
        case SEC_GLOBALS:
            if (line[0] != '\0' && line[0] != '#' && c->nglobals < BINDINGS_MAX &&
                parse_binding(line, &c->globals[c->nglobals])) {
                c->nglobals++;
            }
            break;
        case SEC_NONE:
            break;
        }
    }
    finish_case(base, c);
    free(c);
    fclose(f);
    return true;
}

int main(int argc, char **argv) {
    if (argc < 2) {
        printf("usage: corpus_runner [--nolibc] FILE...\n");
        return 2;
    }
    int first = 1;
    if (strcmp(argv[1], "--nolibc") == 0) {
        use_nolibc = true;
        first = 2;
    } else {
        filo_libc_install(); /* pow with a fractional exponent needs libm */
    }
    for (int i = first; i < argc; i++) {
        (void)run_file(argv[i]);
    }
    if (skipped > 0) {
        printf("%d passed, %d failed, %d skipped (host cannot compute them)\n", passed, failures,
               skipped);
        return failures > 255 ? 255 : failures;
    }
    printf("%d passed, %d failed\n", passed, failures);
    return failures > 255 ? 255 : failures;
}
