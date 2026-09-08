/* The libc-free number host, checked against the libc one on a machine that
   has both. The promise it has to keep is stated in filo_nolibc.h: exact,
   and identical to a libc host, while the value is an integer mantissa of at
   most 15 digits times a power of ten within 22 either way. That is one
   exact scaling step, and it covers every value a script realistically
   holds. Outside it the last digit may differ, which this test measures and
   reports without failing. */
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#include "filo_libc.h"
#include "filo_nolibc.h"

static int failures = 0;
static int shown = 0;

#define CHECK(cond)                                                                                \
    do {                                                                                           \
        if (!(cond)) {                                                                             \
            printf("FAIL %s:%d: %s\n", __FILE__, __LINE__, #cond);                                 \
            failures++;                                                                            \
        }                                                                                          \
    } while (0)

static uint64_t rng = 0x9E3779B97F4A7C15ULL;

static uint64_t next_rand(void) {
    rng += 0x9E3779B97F4A7C15ULL;
    uint64_t z = rng;
    z = (z ^ (z >> 30U)) * 0xBF58476D1CE4E5B9ULL;
    z = (z ^ (z >> 27U)) * 0x94D049BB133111EBULL;
    return z ^ (z >> 31U);
}

static size_t fmt(double x, char *dst, size_t cap) {
    size_t n = filo_nolibc_num_to_str(NULL, x, dst, cap);
    dst[n] = '\0';
    return n;
}

static bool same_as_libc(double x) {
    char a[64];
    char b[64];
    size_t na = filo_libc_num_to_str(NULL, x, a, sizeof(a));
    a[na] = '\0';
    size_t nb = fmt(x, b, sizeof(b));
    if (na == nb && memcmp(a, b, na) == 0) {
        return true;
    }
    if (shown < 10) {
        printf("     libc=%-26s nolibc=%-26s (%a)\n", a, b, x);
        shown++;
    }
    return false;
}

static bool round_trips(double x) {
    char b[64];
    size_t nb = fmt(x, b, sizeof(b));
    double back = 0;
    if (!filo_nolibc_str_to_num(NULL, (const uint8_t *)b, nb, &back)) {
        return false;
    }
    if (back != x) {
        if (shown < 10) {
            printf("     %s did not read back (%a)\n", b, x);
            shown++;
        }
        return false;
    }
    return true;
}

static void test_everyday_values_match_libc(void) {
    const double v[] = {
        0,
        1,
        2,
        10,
        100,
        999999,
        1000000,
        1e6,
        1e-4,
        1e-5,
        0.1,
        0.2,
        0.3,
        0.5,
        1.5,
        3.14159,
        42,
        42.5,
        1e15,
        1e21,
        1e22,
        1e-22,
        123,
        0.001,
        0.0001,
        0.00001,
        1234.5,
        99.99,
        2.5,
        0.125,
        1e9,
        65536,
        9007199254740992.0,
        0.3333333333333333,
        1e-8,
        1.0 / 3,
        2.0 / 3,
    };
    for (size_t i = 0; i < sizeof(v) / sizeof(v[0]); i++) {
        CHECK(same_as_libc(v[i]));
        CHECK(same_as_libc(-v[i]));
        CHECK(round_trips(v[i]));
    }
    char b[64];
    fmt(0.0, b, sizeof(b));
    CHECK(strcmp(b, "0") == 0);
    fmt(-0.0, b, sizeof(b));
    CHECK(strcmp(b, "-0") == 0);
    fmt(1e308 * 10, b, sizeof(b));
    CHECK(strcmp(b, "+Inf") == 0);
    fmt(-1e308 * 10, b, sizeof(b));
    CHECK(strcmp(b, "-Inf") == 0);
    fmt((1e308 * 10) - (1e308 * 10), b, sizeof(b));
    CHECK(strcmp(b, "NaN") == 0);
    /* Go's placement rule: plain up to a million, scientific from it */
    fmt(999999, b, sizeof(b));
    CHECK(strcmp(b, "999999") == 0);
    fmt(1000000, b, sizeof(b));
    CHECK(strcmp(b, "1e+06") == 0);
    fmt(0.0001, b, sizeof(b));
    CHECK(strcmp(b, "0.0001") == 0);
    fmt(0.00001, b, sizeof(b));
    CHECK(strcmp(b, "1e-05") == 0);
}

static void test_parse_matches_libc(void) {
    const char *s[] = {
        "0",     "1",   "-1",   "1.5",  "0.1",    "1e10",  "1E10",  "1e-10", "-2.5e3",
        "  7  ", "+5",  ".5",   "5.",   "1_000",  "1_0.5", "0x10",  "inf",   "-inf",
        "Inf",   "nan", "NaN",  "",     "abc",    "1e",    "1.2.3", "1e999", "-1e999",
        "12abc", "1 2", "1e+3", "1e-3", "000123", "0.0",   "-0",    "1__0",  "_1",
    };
    for (size_t i = 0; i < sizeof(s) / sizeof(s[0]); i++) {
        double a = 0;
        double b = 0;
        bool oka = filo_libc_str_to_num(NULL, (const uint8_t *)s[i], strlen(s[i]), &a);
        bool okb = filo_nolibc_str_to_num(NULL, (const uint8_t *)s[i], strlen(s[i]), &b);
        if (oka != okb) {
            printf("FAIL parse %-10s libc=%d nolibc=%d\n", s[i], oka ? 1 : 0, okb ? 1 : 0);
            failures++;
            continue;
        }
        if (oka && a != b && !(a != a && b != b)) {
            printf("FAIL parse %-10s libc=%.17g nolibc=%.17g\n", s[i], a, b);
            failures++;
        }
    }
}

/* At most 15 significant digits, magnitude from 1e-8 up to 1e22: the range
   the header promises, where the scaling the formatter needs is a single
   exact step. */
static double in_range_value(void) {
    int k = 1 + (int)(next_rand() % 15);
    char txt[48];
    int p = 0;
    txt[p] = (char)('1' + (next_rand() % 9));
    p++;
    for (int d = 1; d < k; d++) {
        txt[p] = (char)('0' + (next_rand() % 10));
        p++;
    }
    int e10 = (int)(next_rand() % 31) - 8; /* magnitude exponent in [-8, 22] */
    p += snprintf(txt + p, sizeof(txt) - (size_t)p, "e%d", e10 - (k - 1));
    txt[p] = '\0';
    double x = strtod(txt, NULL);
    if (next_rand() % 2 == 0) {
        x = -x;
    }
    return x;
}

static void test_in_range_is_exact(int rounds) {
    int checked = 0;
    int trip = 0;
    int diff = 0;
    for (int i = 0; i < rounds; i++) {
        double x = in_range_value();
        if (x == 0 || x != x) {
            continue;
        }
        checked++;
        if (!round_trips(x)) {
            trip++;
        }
        if (!same_as_libc(x)) {
            diff++;
        }
    }
    printf("in range: %d values, %d round-trip failures, %d differ from libc\n", checked, trip,
           diff);
    CHECK(trip == 0);
    CHECK(diff == 0);
}

/* Outside the promised range the pair degrades in the last digit. Measured
   here so the header's claim stays honest, not asserted. */
static void report_out_of_range(int rounds) {
    int checked = 0;
    int trip = 0;
    int diff = 0;
    int quiet = shown;
    shown = 100; /* stop printing: these are expected */
    for (int i = 0; i < rounds; i++) {
        uint64_t bits = next_rand();
        double x = 0;
        memcpy(&x, &bits, sizeof(x));
        if (x != x || x > 1e308 || x < -1e308 || x == 0) {
            continue;
        }
        checked++;
        if (!round_trips(x)) {
            trip++;
        }
        if (!same_as_libc(x)) {
            diff++;
        }
    }
    shown = quiet;
    printf("arbitrary doubles: %d values, %d round-trip failures, %d differ from libc\n", checked,
           trip, diff);
}

static void test_fixed_point_matches_libc(void) {
    const double v[] = {0, 1, 1.5, 3.14159, 2, -2.5, 0.125, 1234.5678, 99.995, 1e6, -0.0001};
    for (size_t i = 0; i < sizeof(v) / sizeof(v[0]); i++) {
        for (uint32_t prec = 0; prec <= 6; prec++) {
            char want[64];
            char got[64];
            int n = snprintf(want, sizeof(want), "%.*f", (int)prec, v[i]);
            size_t g = filo_nolibc_strings.fmt_fixed(v[i], prec, got, sizeof(got));
            got[g] = '\0';
            if ((size_t)n != g || memcmp(want, got, g) != 0) {
                printf("FAIL %%.%uf of %g: libc=%s nolibc=%s\n", prec, v[i], want, got);
                failures++;
            }
        }
    }
}

int main(int argc, char **argv) {
    long asked = argc > 1 ? strtol(argv[1], NULL, 10) : 0;
    int rounds = asked > 0 && asked < 100000000L ? (int)asked : 200000;
    test_everyday_values_match_libc();
    test_parse_matches_libc();
    test_fixed_point_matches_libc();
    test_in_range_is_exact(rounds);
    report_out_of_range(rounds / 4);
    if (failures > 0) {
        printf("%d failure(s)\n", failures);
        return 1;
    }
    printf("nolibc tests passed\n");
    return 0;
}
