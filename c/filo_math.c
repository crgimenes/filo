#include "filo_math.h"

#include <string.h>

static filo_math_fns host_fns;

/* Truncation without libm: doubles beyond 2^53 are already integral, and
   NaN or an infinity fails both comparisons and comes back unchanged. */
static double trunc_d(double x) {
    if (!(x > -9007199254740992.0 && x < 9007199254740992.0)) {
        return x;
    }
    return (double)(int64_t)x;
}

static int one_num(filo_ctx *ctx, const char *name, const filo_value *args, uint32_t n, double *x) {
    if (n != 1) {
        return filo_fail2(ctx, name, " expects 1 argument");
    }
    return filo_arg_num(ctx, &args[0], x);
}

static int m_abs(filo_ctx *ctx, const filo_value *args, uint32_t n, filo_value *out) {
    double x = 0;
    if (one_num(ctx, "abs", args, n, &x) != FILO_OK) {
        return FILO_ERR;
    }
    if (x <= 0) {
        x = 0.0 - x; /* turns -0 into +0 as well */
    }
    *out = filo_num(x);
    return FILO_OK;
}

static int m_floor(filo_ctx *ctx, const filo_value *args, uint32_t n, filo_value *out) {
    double x = 0;
    if (one_num(ctx, "floor", args, n, &x) != FILO_OK) {
        return FILO_ERR;
    }
    double t = trunc_d(x);
    if (t > x) {
        t -= 1;
    }
    *out = filo_num(t);
    return FILO_OK;
}

static int m_ceil(filo_ctx *ctx, const filo_value *args, uint32_t n, filo_value *out) {
    double x = 0;
    if (one_num(ctx, "ceil", args, n, &x) != FILO_OK) {
        return FILO_ERR;
    }
    double t = trunc_d(x);
    if (t < x) {
        t += 1;
    }
    *out = filo_num(t);
    return FILO_OK;
}

/* Half away from zero, as Go's math.Round. */
static int m_round(filo_ctx *ctx, const filo_value *args, uint32_t n, filo_value *out) {
    double x = 0;
    if (one_num(ctx, "round", args, n, &x) != FILO_OK) {
        return FILO_ERR;
    }
    double t = trunc_d(x);
    double d = x - t;
    if (d >= 0.5) {
        t += 1;
    } else if (d <= -0.5) {
        t -= 1;
    }
    *out = filo_num(t);
    return FILO_OK;
}

static int m_to_int(filo_ctx *ctx, const filo_value *args, uint32_t n, filo_value *out) {
    double x = 0;
    if (one_num(ctx, "to-int", args, n, &x) != FILO_OK) {
        return FILO_ERR;
    }
    *out = filo_num(trunc_d(x));
    return FILO_OK;
}

static int via_host(filo_ctx *ctx, const char *name, double (*fn)(double), const filo_value *args,
                    uint32_t n, filo_value *out) {
    double x = 0;
    if (one_num(ctx, name, args, n, &x) != FILO_OK) {
        return FILO_ERR;
    }
    *out = filo_num(fn(x));
    return FILO_OK;
}

static int m_sqrt(filo_ctx *ctx, const filo_value *args, uint32_t n, filo_value *out) {
    if (n == 1 && args[0].kind == FILO_NUMBER && args[0].u.num < 0) {
        return filo_fail(ctx, "sqrt of negative number");
    }
    return via_host(ctx, "sqrt", host_fns.sqrt, args, n, out);
}

static int m_sin(filo_ctx *ctx, const filo_value *args, uint32_t n, filo_value *out) {
    return via_host(ctx, "sin", host_fns.sin, args, n, out);
}

static int m_cos(filo_ctx *ctx, const filo_value *args, uint32_t n, filo_value *out) {
    return via_host(ctx, "cos", host_fns.cos, args, n, out);
}

static int m_tan(filo_ctx *ctx, const filo_value *args, uint32_t n, filo_value *out) {
    return via_host(ctx, "tan", host_fns.tan, args, n, out);
}

static int m_log(filo_ctx *ctx, const filo_value *args, uint32_t n, filo_value *out) {
    if (n == 1 && args[0].kind == FILO_NUMBER && args[0].u.num <= 0) {
        return filo_fail(ctx, "log of non-positive number");
    }
    return via_host(ctx, "log", host_fns.log, args, n, out);
}

static int m_log10(filo_ctx *ctx, const filo_value *args, uint32_t n, filo_value *out) {
    if (n == 1 && args[0].kind == FILO_NUMBER && args[0].u.num <= 0) {
        return filo_fail(ctx, "log10 of non-positive number");
    }
    return via_host(ctx, "log10", host_fns.log10, args, n, out);
}

static int m_exp(filo_ctx *ctx, const filo_value *args, uint32_t n, filo_value *out) {
    return via_host(ctx, "exp", host_fns.exp, args, n, out);
}

/* The Go loop compares with < or >, so a NaN never replaces the running
   value and a leading NaN is kept. */
static int extreme(filo_ctx *ctx, const char *name, bool want_max, const filo_value *args,
                   uint32_t n, filo_value *out) {
    if (n == 0) {
        return filo_fail2(ctx, name, " expects at least 1 argument");
    }
    double r = 0;
    if (filo_arg_num(ctx, &args[0], &r) != FILO_OK) {
        return FILO_ERR;
    }
    for (uint32_t i = 1; i < n; i++) {
        double x = 0;
        if (filo_arg_num(ctx, &args[i], &x) != FILO_OK) {
            return FILO_ERR;
        }
        if (want_max && x > r) {
            r = x;
        }
        if (!want_max && x < r) {
            r = x;
        }
    }
    *out = filo_num(r);
    return FILO_OK;
}

static int m_min(filo_ctx *ctx, const filo_value *args, uint32_t n, filo_value *out) {
    return extreme(ctx, "math-min", false, args, n, out);
}

static int m_max(filo_ctx *ctx, const filo_value *args, uint32_t n, filo_value *out) {
    return extreme(ctx, "math-max", true, args, n, out);
}

static int m_pi(filo_ctx *ctx, const filo_value *args, uint32_t n, filo_value *out) {
    (void)args;
    if (n != 0) {
        return filo_fail(ctx, "pi expects 0 arguments");
    }
    *out = filo_num(3.14159265358979323846);
    return FILO_OK;
}

static int m_e(filo_ctx *ctx, const filo_value *args, uint32_t n, filo_value *out) {
    (void)args;
    if (n != 0) {
        return filo_fail(ctx, "e expects 0 arguments");
    }
    *out = filo_num(2.71828182845904523536);
    return FILO_OK;
}

int filo_math_register(filo_ctx *ctx, const filo_math_fns *fns) {
    memset(&host_fns, 0, sizeof(host_fns));
    if (fns != NULL) {
        host_fns = *fns;
    }
    static const struct {
        const char *name;
        filo_builtin fn;
    } plain[] = {
        {"abs", m_abs},      {"floor", m_floor},   {"ceil", m_ceil},
        {"round", m_round},  {"to-int", m_to_int}, {"math-min", m_min},
        {"math-max", m_max}, {"pi", m_pi},         {"e", m_e},
    };
    for (size_t i = 0; i < sizeof(plain) / sizeof(plain[0]); i++) {
        if (filo_register_builtin(ctx, plain[i].name, plain[i].fn) != FILO_OK) {
            return FILO_ERR;
        }
    }
    /* Only what the host actually supplies: a function it does not have
       should be an undefined symbol when the script compiles, not a surprise
       at the moment of the call. */
    const struct {
        const char *name;
        filo_builtin fn;
        const void *have;
    } hosted[] = {
        {"sqrt", m_sqrt, (const void *)host_fns.sqrt},
        {"sin", m_sin, (const void *)host_fns.sin},
        {"cos", m_cos, (const void *)host_fns.cos},
        {"tan", m_tan, (const void *)host_fns.tan},
        {"log", m_log, (const void *)host_fns.log},
        {"log10", m_log10, (const void *)host_fns.log10},
        {"exp", m_exp, (const void *)host_fns.exp},
    };
    for (size_t i = 0; i < sizeof(hosted) / sizeof(hosted[0]); i++) {
        if (hosted[i].have == NULL) {
            continue;
        }
        if (filo_register_builtin(ctx, hosted[i].name, hosted[i].fn) != FILO_OK) {
            return FILO_ERR;
        }
    }
    return FILO_OK;
}
