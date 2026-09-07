/* Hooks for hosts that have a libc: see filo_libc.c. */
#ifndef FILO_LIBC_H
#define FILO_LIBC_H

#include "filo.h"

size_t filo_libc_num_to_str(void *user, double x, char *dst, size_t cap);
bool filo_libc_str_to_num(void *user, const uint8_t *s, size_t len, double *out);

extern const filo_host filo_libc_host;

/* Call once: installs libm-backed pieces (pow). */
void filo_libc_install(void);

#endif
