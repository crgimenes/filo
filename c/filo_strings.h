/* Strings pack for the C runtime: the builtins of the Go filostrings
   package. Strings are UTF-8 and str-len/str-sub count runes. Case mapping
   covers ASCII and the Latin-1 letters (U+00C0-U+00FF); str-trim removes
   ASCII whitespace, U+0085 and U+00A0; other code points pass through.
   str-fmt's %f needs a host formatter (filo_libc.c provides one). */
#ifndef FILO_STRINGS_H
#define FILO_STRINGS_H

#include "filo.h"

typedef struct {
    /* Renders x with prec decimals, as printf's "%.<prec>f"; returns the
       length written, 0 when it does not fit. NULL makes %f an error. */
    size_t (*fmt_fixed)(double x, uint32_t prec, char *dst, size_t cap);
} filo_strings_fns;

/* fns may be NULL. */
int filo_strings_register(filo_ctx *ctx, const filo_strings_fns *fns);

#endif
