/* Hooks for hosts that have a libc: see filo_libc.c. */
#ifndef FILO_LIBC_H
#define FILO_LIBC_H

#include "filo.h"
#include "filo_math.h"
#include "filo_strings.h"

size_t filo_libc_num_to_str(void *user, double x, char *dst, size_t cap);
bool filo_libc_str_to_num(void *user, const uint8_t *s, size_t len, double *out);

extern const filo_host filo_libc_host;

/* libm-backed tables for the packs. */
extern const filo_math_fns filo_libc_math;
extern const filo_strings_fns filo_libc_strings;
size_t filo_libc_fmt_fixed(double x, uint32_t prec, char *dst, size_t cap);

/* Call once: installs libm-backed pieces (pow). */
void filo_libc_install(void);

#endif
