/* Math pack for the C runtime: the builtins of the Go filomath package.
   abs, floor, ceil, round, to-int, math-min, math-max, pi and e need no
   libm; the transcendental ones come from the host through filo_math_fns.
   An entry the host leaves NULL is not registered at all, so a script that
   names it fails to compile with "undefined symbol" instead of running until
   the call. filo_libc.c provides the libm-backed table. */
#ifndef FILO_MATH_H
#define FILO_MATH_H

#include "filo.h"

typedef struct {
    double (*sqrt)(double);
    double (*sin)(double);
    double (*cos)(double);
    double (*tan)(double);
    double (*log)(double);
    double (*log10)(double);
    double (*exp)(double);
} filo_math_fns;

/* fns may be NULL: every host-backed builtin then fails when called. */
int filo_math_register(filo_ctx *ctx, const filo_math_fns *fns);

#endif
