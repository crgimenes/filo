#include "filo_nolibc.h"

#include <string.h>

/* Exactly representable as doubles, which is what makes the short path here
   correctly rounded: one multiply or one divide, no accumulated error. */
static const double pow10_tab[23] = {
    1e0,  1e1,  1e2,  1e3,  1e4,  1e5,  1e6,  1e7,  1e8,  1e9,  1e10, 1e11,
    1e12, 1e13, 1e14, 1e15, 1e16, 1e17, 1e18, 1e19, 1e20, 1e21, 1e22,
};

static const double inf_value = 1e308 * 10;

enum {
    POW10_MAX = 22,
    MANT_DIGITS_MAX = 19, /* what fits in a uint64 without overflow */
    SIG_DIGITS_MAX = 17,  /* what a double can carry */
};

static bool is_digit(uint8_t c) {
    if (c >= '0' && c <= '9') {
        return true;
    }
    return false;
}

static uint8_t lower(uint8_t c) {
    if (c >= 'A' && c <= 'Z') {
        return (uint8_t)(c + 32);
    }
    return c;
}

static bool word_at(const uint8_t *s, size_t len, size_t *i, const char *word) {
    size_t k = 0;
    while (word[k] != '\0') {
        if (*i + k >= len || lower(s[*i + k]) != (uint8_t)word[k]) {
            return false;
        }
        k++;
    }
    *i += k;
    return true;
}

/* mant * 10^exp. Exact when the mantissa fits a double and the power is in
   the table; otherwise scaled in steps, which can land one unit off in the
   last place for extreme inputs. */
static double scale(double mant, int exp) {
    if (exp == 0) {
        return mant;
    }
    bool up = exp > 0;
    int n = up ? exp : -exp;
    while (n > POW10_MAX) {
        mant = up ? mant * pow10_tab[POW10_MAX] : mant / pow10_tab[POW10_MAX];
        n -= POW10_MAX;
        if (mant == 0 || mant >= inf_value || mant <= -inf_value) {
            return mant; /* saturated: further steps cannot bring it back */
        }
    }
    return up ? mant * pow10_tab[n] : mant / pow10_tab[n];
}

bool filo_nolibc_str_to_num(void *user, const uint8_t *s, size_t len, double *out) {
    (void)user;
    size_t i = 0;
    while (i < len && (s[i] == ' ' || s[i] == '\t' || s[i] == '\n' || s[i] == '\r')) {
        i++;
    }
    bool neg = false;
    if (i < len && (s[i] == '+' || s[i] == '-')) {
        neg = s[i] == '-';
        i++;
    }
    if (word_at(s, len, &i, "infinity") || word_at(s, len, &i, "inf")) {
        if (i != len) {
            return false;
        }
        *out = neg ? -inf_value : inf_value;
        return true;
    }
    if (word_at(s, len, &i, "nan")) {
        if (i != len) {
            return false;
        }
        *out = inf_value - inf_value;
        return true;
    }

    uint64_t mant = 0;
    int exp10 = 0;
    size_t digits = 0;
    size_t taken = 0;
    bool seen_dot = false;
    while (i < len) {
        uint8_t c = s[i];
        if (c == '_') {
            /* Go allows a separator only between digits */
            if (i == 0 || i + 1 >= len || !is_digit(s[i - 1]) || !is_digit(s[i + 1])) {
                return false;
            }
            i++;
            continue;
        }
        if (c == '.') {
            if (seen_dot) {
                return false;
            }
            seen_dot = true;
            i++;
            continue;
        }
        if (!is_digit(c)) {
            break;
        }
        digits++;
        if (taken < MANT_DIGITS_MAX) {
            mant = (mant * 10) + (uint64_t)(c - '0');
            taken++;
            if (seen_dot) {
                exp10--;
            }
        } else if (!seen_dot) {
            exp10++; /* past the mantissa's reach, only the scale still counts */
        }
        i++;
    }
    if (digits == 0) {
        return false;
    }
    if (i < len && (s[i] == 'e' || s[i] == 'E')) {
        i++;
        bool eneg = false;
        if (i < len && (s[i] == '+' || s[i] == '-')) {
            eneg = s[i] == '-';
            i++;
        }
        size_t edigits = 0;
        int e = 0;
        while (i < len && is_digit(s[i])) {
            if (e < 100000) {
                e = (e * 10) + (s[i] - '0');
            }
            edigits++;
            i++;
        }
        if (edigits == 0) {
            return false;
        }
        exp10 += eneg ? -e : e;
    }
    if (i != len) {
        return false;
    }
    double v = scale((double)mant, exp10);
    *out = neg ? -v : v;
    return true;
}

/* ---- formatting ---- */

static size_t put_u64(char *dst, size_t cap, uint64_t v) {
    char tmp[24];
    size_t n = 0;
    do {
        tmp[n] = (char)('0' + (v % 10));
        n++;
        v /= 10;
    } while (v > 0);
    if (n > cap) {
        return 0;
    }
    size_t k = 0;
    while (k < n) {
        dst[k] = tmp[n - 1 - k];
        k++;
    }
    return n;
}

/* Decimal exponent of x > 0: the e with 10^e <= x < 10^(e+1). */
static int exp10_of(double x) {
    int e = 0;
    while (x >= 1e22) {
        x /= 1e22;
        e += 22;
    }
    while (x < 1) {
        x *= 1e22;
        e -= 22;
    }
    int k = 0;
    while (k < POW10_MAX && x >= pow10_tab[k + 1]) {
        k++;
    }
    return e + k;
}

/* The prec most significant digits of x > 0, as an integer, adjusting the
   exponent when rounding carries (9.99 at two digits becomes 10). */
static uint64_t sig_digits(double x, int prec, int *e10) {
    double scaled = scale(x, prec - 1 - *e10);
    uint64_t n = (uint64_t)(scaled + 0.5);
    uint64_t limit = 1;
    int k = 0;
    while (k < prec) {
        limit *= 10;
        k++;
    }
    if (n >= limit) {
        n /= 10;
        *e10 += 1;
    }
    return n;
}

/* Lays out digits/exponent as text: plain when the exponent lands in
   [-4, 6), scientific otherwise, which is Go's rule for %g. */
static size_t render(bool neg, const char *digits, size_t nd, int e10, char *dst, size_t cap) {
    size_t out = 0;
    if (neg) {
        dst[out] = '-';
        out++;
    }
    if (e10 < -4 || e10 >= 6) {
        dst[out] = digits[0];
        out++;
        if (nd > 1) {
            dst[out] = '.';
            out++;
            memcpy(dst + out, digits + 1, nd - 1);
            out += nd - 1;
        }
        dst[out] = 'e';
        out++;
        int e = e10;
        if (e < 0) {
            dst[out] = '-';
            e = -e;
        } else {
            dst[out] = '+';
        }
        out++;
        if (e < 10) {
            dst[out] = '0';
            out++;
        }
        return out + put_u64(dst + out, cap - out, (uint64_t)e);
    }
    if (e10 >= 0) {
        size_t whole = (size_t)e10 + 1;
        size_t k = 0;
        while (k < whole) {
            dst[out] = k < nd ? digits[k] : '0';
            out++;
            k++;
        }
        if (nd > whole) {
            dst[out] = '.';
            out++;
            memcpy(dst + out, digits + whole, nd - whole);
            out += nd - whole;
        }
        return out;
    }
    dst[out] = '0';
    out++;
    dst[out] = '.';
    out++;
    int z = -e10 - 1;
    while (z > 0) {
        dst[out] = '0';
        out++;
        z--;
    }
    memcpy(dst + out, digits, nd);
    return out + nd;
}

size_t filo_nolibc_num_to_str(void *user, double x, char *dst, size_t cap) {
    (void)user;
    if (cap < 32) {
        return 0;
    }
    if (x != x) {
        memcpy(dst, "NaN", 3);
        return 3;
    }
    if (x >= inf_value) {
        memcpy(dst, "+Inf", 4);
        return 4;
    }
    if (x <= -inf_value) {
        memcpy(dst, "-Inf", 4);
        return 4;
    }
    bool neg = false;
    if (x < 0 || (x == 0 && 1 / x < 0)) {
        neg = true;
        x = -x;
    }
    if (x == 0) {
        size_t n = 0;
        if (neg) {
            dst[0] = '-';
            n = 1;
        }
        dst[n] = '0';
        return n + 1;
    }

    /* The shortest run of digits that THIS host reads back as the same
       double. Checking with our own parser is what makes the pair exact
       together: (number (string x)) is x, whatever a libc would have
       printed. Beyond 15 significant digits a double cannot carry the
       arithmetic, so the last digit may differ from a libc host's. */
    int base_e10 = exp10_of(x);
    static const int nudge[3] = {0, 1, -1};
    char digits[SIG_DIGITS_MAX + 1];
    size_t out = 0;
    double want = neg ? -x : x;
    int prec = 1;
    while (prec <= SIG_DIGITS_MAX) {
        size_t k = 0;
        while (k < 3) {
            int e10 = base_e10 + nudge[k];
            uint64_t n = sig_digits(x, prec, &e10);
            size_t nd = put_u64(digits, sizeof(digits), n);
            while (nd > 1 && digits[nd - 1] == '0') {
                nd--; /* trailing zeros carry no information */
            }
            out = render(neg, digits, nd, e10, dst, cap);
            double back = 0;
            if (filo_nolibc_str_to_num(NULL, (const uint8_t *)dst, out, &back) && back == want) {
                return out;
            }
            k++;
        }
        prec++;
    }
    return out;
}

/* %f without printf: round to prec decimals and lay the digits out. */
static size_t nolibc_fmt_fixed(double x, uint32_t prec, char *dst, size_t cap) {
    if (cap < 32 || x != x || x >= inf_value || x <= -inf_value) {
        return 0;
    }
    bool neg = false;
    if (x < 0) {
        neg = true;
        x = -x;
    }
    if (prec > 17) {
        prec = 17;
    }
    double shifted = scale(x, (int)prec);
    if (shifted >= 1e19) {
        return 0; /* beyond what the integer path can carry */
    }
    uint64_t n = (uint64_t)shifted;
    double frac = shifted - (double)n;
    if (frac > 0.5 || (frac == 0.5 && (n & 1U) == 1U)) {
        n++;
    }
    char digits[24];
    size_t nd = put_u64(digits, sizeof(digits), n);
    size_t out = 0;
    if (neg) {
        dst[out] = '-';
        out++;
    }
    if (nd <= prec) {
        dst[out] = '0';
        out++;
        if (prec > 0) {
            dst[out] = '.';
            out++;
            size_t pad = prec - nd;
            while (pad > 0) {
                dst[out] = '0';
                out++;
                pad--;
            }
            memcpy(dst + out, digits, nd);
            out += nd;
        }
        return out;
    }
    size_t whole = nd - prec;
    memcpy(dst + out, digits, whole);
    out += whole;
    if (prec > 0) {
        dst[out] = '.';
        out++;
        memcpy(dst + out, digits + whole, prec);
        out += prec;
    }
    return out;
}

const filo_strings_fns filo_nolibc_strings = {nolibc_fmt_fixed};

const filo_host filo_nolibc_host = {
    NULL,
    filo_nolibc_num_to_str,
    filo_nolibc_str_to_num,
    NULL,
};
