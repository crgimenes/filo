/* The Go engine's BenchmarkRecursiveCall, on the C runtime: fib 15 per run,
   reported as ns/op so the two numbers compare directly. */
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <time.h>

#include "filo.h"
#include "filo_libc.h"

static uint8_t persistent_mem[1U << 20U];
static uint8_t run_mem[8U << 20U];

int main(int argc, char **argv) {
    uint32_t iterations = 2000;
    if (argc > 1) {
        unsigned long asked = strtoul(argv[1], NULL, 10);
        if (asked > 0 && asked < 1000000UL) {
            iterations = (uint32_t)asked;
        }
    }
    const char *src =
        "(do (def fib (fn (n) (if (< n 2) n (+ (fib (- n 1)) (fib (- n 2)))))) (fib 15))";
    filo_libc_install();
    filo_ctx *ctx = malloc(sizeof(filo_ctx));
    if (ctx == NULL) {
        return 1;
    }
    filo_init(ctx, &filo_libc_host, persistent_mem, sizeof(persistent_mem), run_mem,
              sizeof(run_mem));
    filo_prog prog;
    if (filo_compile(ctx, (const uint8_t *)src, strlen(src), &prog) != FILO_OK) {
        printf("compile: %s\n", filo_error(ctx));
        return 1;
    }
    filo_limits limits = {10000000U, 10000U};
    filo_value v = {0};
    struct timespec t0;
    struct timespec t1;
    clock_gettime(CLOCK_MONOTONIC, &t0);
    for (uint32_t i = 0; i < iterations; i++) {
        if (filo_run(ctx, &prog, &limits, &v) != FILO_OK) {
            printf("run: %s\n", filo_error(ctx));
            return 1;
        }
    }
    clock_gettime(CLOCK_MONOTONIC, &t1);
    double elapsed = (double)(t1.tv_sec - t0.tv_sec) * 1e9 + (double)(t1.tv_nsec - t0.tv_nsec);
    double ns = elapsed / (double)iterations;
    printf("BenchmarkRecursiveCall-C  %u iterations  %.0f ns/op  (fib 15 = %g)\n", iterations, ns,
           v.u.num);
    free(ctx);
    return 0;
}
