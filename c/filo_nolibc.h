/* Number text for hosts without a C library: wasm32, microcontrollers, and
   anything else with no strtod or printf. Pair it with the math pack's host
   table left empty, since the transcendental functions need libm too.

   Formatting follows the same rule as the libc host: the shortest decimal
   that reads back as the same double, written plainly when the exponent
   lands in [-4, 6) and in scientific form otherwise.

   How far the agreement goes, measured by nolibc_test.c against a libc host
   over 300k values: for at most 15 significant digits and a magnitude from
   1e-8 up to 1e22 the text is identical and always reads back as the same
   double. That bound is not arbitrary: inside it the conversion is a single
   multiply or divide by a power of ten that a double holds exactly. Outside
   it (17-digit values, magnitudes past 1e22 or below 1e-8) the last digit
   may differ from a libc host and the round trip may lose a unit in the last
   place. Every value a script realistically holds is inside.

   The pair is exact with itself wherever it can be: num_to_str checks its
   own candidate with str_to_num before returning it, so (number (string x))
   gives back x whenever any decimal spelling does. */
#ifndef FILO_NOLIBC_H
#define FILO_NOLIBC_H

#include "filo.h"
#include "filo_strings.h"

size_t filo_nolibc_num_to_str(void *user, double x, char *dst, size_t cap);
bool filo_nolibc_str_to_num(void *user, const uint8_t *s, size_t len, double *out);

/* %f for str-fmt; no host math functions, so filo_math_register may be given
   a NULL table (or skipped entirely). */
extern const filo_strings_fns filo_nolibc_strings;

extern const filo_host filo_nolibc_host;

#endif
