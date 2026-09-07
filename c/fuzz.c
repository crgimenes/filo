/* libFuzzer entry: any byte string is a script. The runtime must never fault,
   whatever the input; it may only fail with an error. Limits are tight so
   a run finishes fast and the arenas are small so exhaustion is exercised. */
#include <stddef.h>
#include <stdint.h>
#include <string.h>

#include "filo.h"
#include "filo_libc.h"

static uint8_t persistent_mem[256U << 10U];
static uint8_t run_mem[1U << 20U];
static filo_ctx ctx;

int LLVMFuzzerTestOneInput(const uint8_t *data, size_t size);

int LLVMFuzzerTestOneInput(const uint8_t *data, size_t size) {
    filo_libc_install();
    filo_init(&ctx, &filo_libc_host, persistent_mem, sizeof(persistent_mem), run_mem,
              sizeof(run_mem));
    (void)filo_math_register(&ctx, &filo_libc_math);
    (void)filo_strings_register(&ctx, &filo_libc_strings);
    filo_prog prog;
    if (filo_compile(&ctx, data, size, &prog) != FILO_OK) {
        return 0;
    }
    filo_limits limits = {20000U, 64U};
    filo_value v;
    if (filo_run(&ctx, &prog, &limits, &v) == FILO_OK) {
        char buf[256];
        size_t n = 0;
        (void)filo_value_text(&ctx, &v, buf, sizeof(buf), &n);
        /* a second run on the same program must be just as safe */
        (void)filo_run(&ctx, &prog, &limits, &v);
    }
    return 0;
}
