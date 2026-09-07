#include "filo_strings.h"

#include <string.h>

static filo_strings_fns host_fns;

/* An empty string may arrive without a pointer; giving it one keeps every
   slice and rune walk below free of NULL arithmetic. */
static const uint8_t empty_bytes[1] = {0};

static filo_str with_ptr(filo_str s) {
    if (s.ptr == NULL) {
        s.ptr = empty_bytes;
        s.len = 0;
    }
    return s;
}

/* ------------------------------------------------------------- helpers */

static size_t u32_text(char *dst, size_t cap, uint32_t v) {
    char tmp[16] = {0};
    size_t n = 0;
    do {
        tmp[n] = (char)('0' + (v % 10));
        n++;
        v /= 10;
    } while (v > 0);
    if (n > cap - 1) {
        n = cap - 1;
    }
    for (size_t i = 0; i < n; i++) {
        dst[i] = tmp[n - 1 - i];
    }
    dst[n] = '\0';
    return n;
}

static void copy_bytes(uint8_t *dst, const uint8_t *src, size_t n) {
    if (n > 0 && src != NULL) {
        memcpy(dst, src, n);
    }
}

/* A result buffer; one byte for an empty result keeps every pointer valid. */
static uint8_t *take(filo_ctx *ctx, size_t n) {
    return filo_alloc(ctx, n > 0 ? n : 1);
}

static size_t cat(char *dst, size_t cap, size_t pos, const char *s) {
    size_t n = strlen(s);
    if (pos >= cap - 1) {
        return pos;
    }
    if (n > cap - 1 - pos) {
        n = cap - 1 - pos;
    }
    memcpy(dst + pos, s, n);
    dst[pos + n] = '\0';
    return pos + n;
}

/* "<a><b><c>" as the error, printf-free. */
static int fail3(filo_ctx *ctx, const char *a, const char *b, const char *c) {
    char msg[FILO_ERROR_MAX] = {0};
    size_t n = cat(msg, sizeof(msg), 0, a);
    n = cat(msg, sizeof(msg), n, b);
    (void)cat(msg, sizeof(msg), n, c);
    return filo_fail(ctx, msg);
}

/* "<name>: <what> must be string: expected string, got <kind>" */
static int str_arg(filo_ctx *ctx, const char *name, const char *what, const filo_value *v,
                   filo_str *out) {
    if (v->kind == FILO_STRING) {
        *out = with_ptr(v->u.str);
        return FILO_OK;
    }
    char msg[FILO_ERROR_MAX] = {0};
    size_t n = cat(msg, sizeof(msg), 0, name);
    n = cat(msg, sizeof(msg), n, ": ");
    n = cat(msg, sizeof(msg), n, what);
    n = cat(msg, sizeof(msg), n, " must be string: expected string, got ");
    (void)cat(msg, sizeof(msg), n, filo_kind_name(v->kind));
    (void)filo_fail(ctx, msg);
    return FILO_ERR; /* literal, so an analyzer sees the caller never continues */
}

static int num_arg(filo_ctx *ctx, const char *name, const char *what, const filo_value *v,
                   double *out) {
    if (v->kind == FILO_NUMBER) {
        *out = v->u.num;
        return FILO_OK;
    }
    char msg[FILO_ERROR_MAX] = {0};
    size_t n = cat(msg, sizeof(msg), 0, name);
    n = cat(msg, sizeof(msg), n, ": ");
    n = cat(msg, sizeof(msg), n, what);
    n = cat(msg, sizeof(msg), n, " must be number: expected number, got ");
    (void)cat(msg, sizeof(msg), n, filo_kind_name(v->kind));
    (void)filo_fail(ctx, msg);
    return FILO_ERR; /* literal, so an analyzer sees the caller never continues */
}

static double trunc_d(double x) {
    if (!(x > -9007199254740992.0 && x < 9007199254740992.0)) {
        return x;
    }
    return (double)(int64_t)x;
}

/* Bytes of the UTF-8 sequence starting at s[i]; a stray or truncated byte
   counts as one, as Go's rune iteration does. */
static size_t rune_at(filo_str s, size_t i) {
    uint8_t b = s.ptr[i];
    size_t n = 1;
    if ((b & 0xE0U) == 0xC0U) {
        n = 2;
    } else if ((b & 0xF0U) == 0xE0U) {
        n = 3;
    } else if ((b & 0xF8U) == 0xF0U) {
        n = 4;
    }
    if (n > s.len - i) {
        n = 1;
    }
    return n;
}

static size_t rune_count(filo_str s) {
    size_t n = 0;
    for (size_t i = 0; i < s.len; i += rune_at(s, i)) {
        n++;
    }
    return n;
}

/* Byte offset of the k-th rune; the length when k is past the end. */
static size_t rune_offset(filo_str s, size_t k) {
    size_t i = 0;
    while (k > 0 && i < s.len) {
        i += rune_at(s, i);
        k--;
    }
    return i;
}

static bool find_at(filo_str hay, filo_str needle, size_t from, size_t *at) {
    if (needle.len > hay.len) {
        return false;
    }
    for (size_t i = from; i + needle.len <= hay.len; i++) {
        if (memcmp(hay.ptr + i, needle.ptr, needle.len) == 0) {
            *at = i;
            return true;
        }
    }
    return false;
}

static filo_value slice(filo_str s, size_t from, size_t to) {
    return filo_string(s.ptr + from, (uint32_t)(to - from));
}

static filo_str bytes_of(const uint8_t *p, size_t n) {
    filo_str s = {p, (uint32_t)n};
    return s;
}

/* --------------------------------------------------------- the builtins */

static int s_len(filo_ctx *ctx, const filo_value *args, uint32_t n, filo_value *out) {
    if (n != 1) {
        return filo_fail(ctx, "str-len expects 1 argument (string)");
    }
    filo_str s = {NULL, 0};
    if (str_arg(ctx, "str-len", "argument", &args[0], &s) != FILO_OK) {
        return FILO_ERR;
    }
    *out = filo_num((double)rune_count(s));
    return FILO_OK;
}

static int s_concat(filo_ctx *ctx, const filo_value *args, uint32_t n, filo_value *out) {
    size_t total = 0;
    for (uint32_t i = 0; i < n; i++) {
        filo_str s = {NULL, 0};
        char what[32] = {0};
        size_t k = cat(what, sizeof(what), 0, "argument ");
        (void)u32_text(what + k, sizeof(what) - k, i);
        if (str_arg(ctx, "str-concat", what, &args[i], &s) != FILO_OK) {
            return FILO_ERR;
        }
        total += s.len;
    }
    uint8_t *buf = take(ctx, total);
    if (buf == NULL) {
        return FILO_ERR;
    }
    size_t pos = 0;
    for (uint32_t i = 0; i < n; i++) {
        copy_bytes(buf + pos, args[i].u.str.ptr, args[i].u.str.len);
        pos += args[i].u.str.len;
    }
    *out = filo_string(buf, (uint32_t)total);
    return FILO_OK;
}

static int s_join(filo_ctx *ctx, const filo_value *args, uint32_t n, filo_value *out) {
    if (n != 2) {
        return filo_fail(ctx, "str-join expects 2 arguments (separator, list)");
    }
    filo_str sep = {NULL, 0};
    if (str_arg(ctx, "str-join", "separator", &args[0], &sep) != FILO_OK) {
        return FILO_ERR;
    }
    if (args[1].kind != FILO_LIST) {
        return fail3(ctx, "str-join: second argument must be list: expected list, got ",
                     filo_kind_name(args[1].kind), "");
    }
    filo_seq l = args[1].u.seq;
    size_t total = 0;
    for (uint32_t i = 0; i < l.len; i++) {
        filo_str s = {NULL, 0};
        char what[40] = {0};
        size_t k = cat(what, sizeof(what), 0, "list element ");
        (void)u32_text(what + k, sizeof(what) - k, i);
        if (str_arg(ctx, "str-join", what, &l.items[i], &s) != FILO_OK) {
            return FILO_ERR;
        }
        total += s.len;
    }
    if (l.len > 1) {
        total += (size_t)sep.len * (l.len - 1);
    }
    uint8_t *buf = take(ctx, total);
    if (buf == NULL) {
        return FILO_ERR;
    }
    size_t pos = 0;
    for (uint32_t i = 0; i < l.len; i++) {
        if (i > 0) {
            copy_bytes(buf + pos, sep.ptr, sep.len);
            pos += sep.len;
        }
        copy_bytes(buf + pos, l.items[i].u.str.ptr, l.items[i].u.str.len);
        pos += l.items[i].u.str.len;
    }
    *out = filo_string(buf, (uint32_t)total);
    return FILO_OK;
}

/* Go's strings.Split: an empty separator explodes into runes (and an empty
   string into no parts); otherwise n separators make n+1 parts. */
static int s_split(filo_ctx *ctx, const filo_value *args, uint32_t n, filo_value *out) {
    if (n != 2) {
        return filo_fail(ctx, "str-split expects 2 arguments (separator, string)");
    }
    filo_str sep = {NULL, 0};
    filo_str s = {NULL, 0};
    if (str_arg(ctx, "str-split", "separator", &args[0], &sep) != FILO_OK ||
        str_arg(ctx, "str-split", "second argument", &args[1], &s) != FILO_OK) {
        return FILO_ERR;
    }
    uint32_t parts = 0;
    if (sep.len == 0) {
        parts = (uint32_t)rune_count(s);
    } else {
        parts = 1;
        size_t at = 0;
        size_t from = 0;
        while (find_at(s, sep, from, &at)) {
            parts++;
            from = at + sep.len;
        }
    }
    if (filo_list(ctx, NULL, parts, out) != FILO_OK) {
        return FILO_ERR;
    }
    filo_value *items = out->u.seq.items;
    if (sep.len == 0) {
        size_t i = 0;
        for (uint32_t k = 0; k < parts && i < s.len; k++) {
            size_t w = rune_at(s, i);
            items[k] = slice(s, i, i + w);
            i += w;
        }
        return FILO_OK;
    }
    size_t from = 0;
    for (uint32_t k = 0; k + 1 < parts; k++) {
        size_t at = 0;
        (void)find_at(s, sep, from, &at);
        items[k] = slice(s, from, at);
        from = at + sep.len;
    }
    items[parts - 1] = slice(s, from, s.len);
    return FILO_OK;
}

static int s_find(filo_ctx *ctx, const filo_value *args, uint32_t n, filo_value *out) {
    if (n != 2) {
        return filo_fail(ctx, "str-find expects 2 arguments (substring, string)");
    }
    filo_str sub = {NULL, 0};
    filo_str s = {NULL, 0};
    if (str_arg(ctx, "str-find", "substring", &args[0], &sub) != FILO_OK ||
        str_arg(ctx, "str-find", "second argument", &args[1], &s) != FILO_OK) {
        return FILO_ERR;
    }
    size_t at = 0;
    *out = filo_bool(find_at(s, sub, 0, &at));
    return FILO_OK;
}

static bool space_at(filo_str s, size_t i, size_t *w) {
    uint8_t b = s.ptr[i];
    if (b == ' ' || b == '\t' || b == '\n' || b == '\v' || b == '\f' || b == '\r') {
        *w = 1;
        return true;
    }
    if (b == 0xC2 && i + 1 < s.len && (s.ptr[i + 1] == 0x85 || s.ptr[i + 1] == 0xA0)) {
        *w = 2;
        return true;
    }
    return false;
}

static int s_trim(filo_ctx *ctx, const filo_value *args, uint32_t n, filo_value *out) {
    if (n != 1) {
        return filo_fail(ctx, "str-trim expects 1 argument (string)");
    }
    filo_str s = {NULL, 0};
    if (str_arg(ctx, "str-trim", "argument", &args[0], &s) != FILO_OK) {
        return FILO_ERR;
    }
    size_t from = 0;
    size_t w = 0;
    while (from < s.len && space_at(s, from, &w)) {
        from += w;
    }
    size_t to = s.len;
    while (to > from) {
        size_t back = 1;
        if (to >= 2 && s.ptr[to - 2] == 0xC2) {
            back = 2;
        }
        if (!space_at(s, to - back, &w) || w != back) {
            break;
        }
        to -= back;
    }
    *out = slice(s, from, to);
    return FILO_OK;
}

/* Go's ReplaceAll: an empty old matches before every rune and at the end. */
static int s_replace(filo_ctx *ctx, const filo_value *args, uint32_t n, filo_value *out) {
    if (n != 3) {
        return filo_fail(ctx, "str-replace expects 3 arguments (old, new, string)");
    }
    filo_str old = {NULL, 0};
    filo_str rep = {NULL, 0};
    filo_str s = {NULL, 0};
    if (str_arg(ctx, "str-replace", "old", &args[0], &old) != FILO_OK ||
        str_arg(ctx, "str-replace", "new", &args[1], &rep) != FILO_OK ||
        str_arg(ctx, "str-replace", "third argument", &args[2], &s) != FILO_OK) {
        return FILO_ERR;
    }
    size_t hits = 0;
    if (old.len == 0) {
        hits = rune_count(s) + 1;
    } else {
        size_t at = 0;
        size_t from = 0;
        while (find_at(s, old, from, &at)) {
            hits++;
            from = at + old.len;
        }
    }
    size_t total = s.len - hits * old.len + hits * rep.len;
    uint8_t *buf = take(ctx, total);
    if (buf == NULL) {
        return FILO_ERR;
    }
    size_t pos = 0;
    size_t i = 0;
    if (old.len == 0) {
        while (i < s.len) {
            copy_bytes(buf + pos, rep.ptr, rep.len);
            pos += rep.len;
            size_t w = rune_at(s, i);
            copy_bytes(buf + pos, s.ptr + i, w);
            pos += w;
            i += w;
        }
        copy_bytes(buf + pos, rep.ptr, rep.len);
        *out = filo_string(buf, (uint32_t)total);
        return FILO_OK;
    }
    size_t at = 0;
    while (find_at(s, old, i, &at)) {
        copy_bytes(buf + pos, s.ptr + i, at - i);
        pos += at - i;
        copy_bytes(buf + pos, rep.ptr, rep.len);
        pos += rep.len;
        i = at + old.len;
    }
    copy_bytes(buf + pos, s.ptr + i, s.len - i);
    *out = filo_string(buf, (uint32_t)total);
    return FILO_OK;
}

/* ASCII and Latin-1 letters keep their byte length under case mapping, so
   the result is a same-sized copy. */
static int map_case(filo_ctx *ctx, const char *name, const filo_value *args, uint32_t n, bool upper,
                    filo_value *out) {
    if (n != 1) {
        return fail3(ctx, name, " expects 1 argument (string)", "");
    }
    filo_str s = {NULL, 0};
    if (str_arg(ctx, name, "argument", &args[0], &s) != FILO_OK) {
        return FILO_ERR;
    }
    uint8_t *buf = take(ctx, s.len);
    if (buf == NULL) {
        return FILO_ERR;
    }
    copy_bytes(buf, s.ptr, s.len);
    for (size_t i = 0; i < s.len; i++) {
        uint8_t b = buf[i];
        if (upper && b >= 'a' && b <= 'z') {
            buf[i] = (uint8_t)(b - 32);
            continue;
        }
        if (!upper && b >= 'A' && b <= 'Z') {
            buf[i] = (uint8_t)(b + 32);
            continue;
        }
        if (b == 0xC3 && i + 1 < s.len) {
            uint8_t c = buf[i + 1];
            if (upper && c == 0xBF) { /* ÿ -> Ÿ */
                buf[i] = 0xC5;
                buf[i + 1] = 0xB8;
            } else if (upper && c >= 0xA0 && c <= 0xBE && c != 0xB7) {
                buf[i + 1] = (uint8_t)(c - 0x20);
            } else if (!upper && c >= 0x80 && c <= 0x9E && c != 0x97) {
                buf[i + 1] = (uint8_t)(c + 0x20);
            }
            i++;
            continue;
        }
        if (!upper && b == 0xC5 && i + 1 < s.len && buf[i + 1] == 0xB8) { /* Ÿ -> ÿ */
            buf[i] = 0xC3;
            buf[i + 1] = 0xBF;
            i++;
        }
    }
    *out = filo_string(buf, s.len);
    return FILO_OK;
}

static int s_upper(filo_ctx *ctx, const filo_value *args, uint32_t n, filo_value *out) {
    return map_case(ctx, "str-upper", args, n, true, out);
}

static int s_lower(filo_ctx *ctx, const filo_value *args, uint32_t n, filo_value *out) {
    return map_case(ctx, "str-lower", args, n, false, out);
}

/* Rune-indexed, clamped to the string; a negative or missing end means the
   end of the string, as in the Go pack. */
static int s_sub(filo_ctx *ctx, const filo_value *args, uint32_t n, filo_value *out) {
    if (n != 2 && n != 3) {
        return filo_fail(ctx, "str-sub expects 2 or 3 arguments (string, start, [end])");
    }
    filo_str s = {NULL, 0};
    double start = 0;
    if (str_arg(ctx, "str-sub", "first argument", &args[0], &s) != FILO_OK ||
        num_arg(ctx, "str-sub", "start", &args[1], &start) != FILO_OK) {
        return FILO_ERR;
    }
    if (trunc_d(start) != start) {
        return filo_fail(ctx, "str-sub: start must be an integer");
    }
    double end = -1;
    if (n == 3) {
        if (num_arg(ctx, "str-sub", "end", &args[2], &end) != FILO_OK) {
            return FILO_ERR;
        }
        if (trunc_d(end) != end) {
            return filo_fail(ctx, "str-sub: end must be an integer");
        }
    }
    double count = (double)rune_count(s);
    if (start < 0) {
        start = 0;
    }
    if (start > count) {
        start = count;
    }
    if (end < 0 || end > count) {
        end = count;
    }
    if (start > end) {
        *out = filo_string(NULL, 0);
        return FILO_OK;
    }
    size_t from = rune_offset(s, (size_t)start);
    size_t to = rune_offset(s, (size_t)end);
    *out = slice(s, from, to);
    return FILO_OK;
}

/* ------------------------------------------------------------- str-fmt */

typedef struct {
    uint8_t *dst;
    size_t cap;
    size_t pos;
} fsink;

static void sput(fsink *s, const uint8_t *b, size_t n) {
    if (s->dst != NULL && s->pos + n <= s->cap) {
        memcpy(s->dst + s->pos, b, n);
    }
    s->pos += n;
}

static void sfill(fsink *s, uint8_t c, size_t n) {
    for (size_t i = 0; i < n; i++) {
        sput(s, &c, 1);
    }
}

typedef struct {
    bool left;
    bool zero;
    bool plus;
    uint32_t width;
    bool has_prec;
    uint32_t prec;
} fspec;

/* Width counts runes and a '0' pads after the sign of a number, as fmt. */
static void pad_out(fsink *s, filo_str body, const fspec *sp, bool numeric) {
    size_t vis = rune_count(body);
    size_t padn = 0;
    if (sp->width > vis) {
        padn = sp->width - vis;
    }
    if (sp->left) {
        sput(s, body.ptr, body.len);
        sfill(s, ' ', padn);
        return;
    }
    if (sp->zero) {
        if (numeric && body.len > 0 && (body.ptr[0] == '-' || body.ptr[0] == '+')) {
            sput(s, body.ptr, 1);
            sfill(s, '0', padn);
            sput(s, body.ptr + 1, body.len - 1);
            return;
        }
        sfill(s, '0', padn);
        sput(s, body.ptr, body.len);
        return;
    }
    sfill(s, ' ', padn);
    sput(s, body.ptr, body.len);
}

static uint32_t digits_at(filo_str f, size_t *i) {
    uint32_t v = 0;
    while (*i < f.len && f.ptr[*i] >= '0' && f.ptr[*i] <= '9') {
        v = v * 10 + (uint32_t)(f.ptr[*i] - '0');
        (*i)++;
    }
    return v;
}

/* Text of the value for %s (strings verbatim) or %v (source form), cut to
   the precision in runes. Rendered values live in the run arena. */
static int verb_text(filo_ctx *ctx, const filo_value *v, bool repr, const fspec *sp,
                     filo_str *out) {
    filo_str text;
    if (!repr && v->kind == FILO_STRING) {
        text = with_ptr(v->u.str);
    } else {
        size_t n = 0;
        int rc = repr ? filo_value_repr(ctx, v, NULL, 0, &n) : filo_value_text(ctx, v, NULL, 0, &n);
        if (rc != FILO_OK) {
            return FILO_ERR;
        }
        char *buf = NULL;
        if (n > 0) {
            buf = filo_alloc(ctx, n + 1);
            if (buf == NULL) {
                return FILO_ERR;
            }
            rc = repr ? filo_value_repr(ctx, v, buf, n + 1, &n)
                      : filo_value_text(ctx, v, buf, n + 1, &n);
            if (rc != FILO_OK) {
                return FILO_ERR;
            }
        }
        text = bytes_of((const uint8_t *)buf, n);
    }
    if (sp->has_prec) {
        size_t cut = rune_offset(text, sp->prec);
        text.len = (uint32_t)cut;
    }
    *out = text;
    return FILO_OK;
}

static int verb_int(filo_ctx *ctx, const filo_value *v, const fspec *sp, fsink *s) {
    if (v->kind != FILO_NUMBER) {
        return fail3(ctx, "str-fmt: expected number, got ", filo_kind_name(v->kind), "");
    }
    double x = v->u.num;
    if (trunc_d(x) != x || !(x > -9.2e18 && x < 9.2e18)) {
        char shown[48] = {0};
        size_t n = 0;
        if (filo_value_text(ctx, v, shown, sizeof(shown), &n) != FILO_OK) {
            return FILO_ERR;
        }
        return fail3(ctx, "str-fmt: %d expects an integer, got ", shown, "");
    }
    bool neg = x < 0;
    uint64_t mag = (uint64_t)(neg ? -x : x);
    char tmp[32] = {0};
    size_t n = 0;
    do {
        tmp[n] = (char)('0' + (mag % 10));
        n++;
        mag /= 10;
    } while (mag > 0);
    uint8_t body[64] = {0};
    size_t len = 0;
    if (neg) {
        body[len++] = '-';
    } else if (sp->plus) {
        body[len++] = '+';
    }
    size_t zeros = 0;
    if (sp->has_prec && sp->prec > n) {
        zeros = sp->prec - n;
    }
    while (zeros > 0 && len < sizeof(body)) {
        body[len++] = '0';
        zeros--;
    }
    while (n > 0 && len < sizeof(body)) {
        n--;
        body[len++] = (uint8_t)tmp[n];
    }
    fspec adj = *sp;
    if (sp->has_prec) {
        adj.zero = false; /* fmt ignores the 0 flag when a precision is given */
    }
    pad_out(s, bytes_of(body, len), &adj, true);
    return FILO_OK;
}

static int verb_fixed(filo_ctx *ctx, const filo_value *v, const fspec *sp, fsink *s) {
    if (v->kind != FILO_NUMBER) {
        return fail3(ctx, "str-fmt: expected number, got ", filo_kind_name(v->kind), "");
    }
    if (host_fns.fmt_fixed == NULL) {
        return filo_fail(ctx, "str-fmt: %f is not available on this host");
    }
    uint32_t prec = sp->has_prec ? sp->prec : 6;
    char buf[400] = {0};
    size_t n = host_fns.fmt_fixed(v->u.num, prec, buf + 1, sizeof(buf) - 1);
    if (n == 0) {
        return filo_fail(ctx, "str-fmt: number formatting failed");
    }
    const uint8_t *body = (const uint8_t *)buf + 1;
    if (sp->plus && buf[1] != '-') {
        buf[0] = '+';
        body = (const uint8_t *)buf;
        n++;
    }
    pad_out(s, bytes_of(body, n), sp, true);
    return FILO_OK;
}

static int fmt_run(filo_ctx *ctx, filo_str f, const filo_value *args, uint32_t nargs, fsink *s) {
    uint32_t next = 0;
    size_t i = 0;
    while (i < f.len) {
        if (f.ptr[i] != '%') {
            sput(s, f.ptr + i, 1);
            i++;
            continue;
        }
        i++;
        size_t spec_start = i;
        fspec sp;
        memset(&sp, 0, sizeof(sp));
        while (i < f.len && (f.ptr[i] == '-' || f.ptr[i] == '+' || f.ptr[i] == '0')) {
            if (f.ptr[i] == '-') {
                sp.left = true;
            } else if (f.ptr[i] == '+') {
                sp.plus = true;
            } else {
                sp.zero = true;
            }
            i++;
        }
        sp.width = digits_at(f, &i);
        if (i < f.len && f.ptr[i] == '.') {
            i++;
            sp.has_prec = true;
            sp.prec = digits_at(f, &i);
        }
        if (i >= f.len) {
            return filo_fail(ctx, "str-fmt: incomplete verb at end of format");
        }
        uint8_t verb = f.ptr[i];
        char shown[40] = {0};
        size_t k = cat(shown, sizeof(shown), 0, "%");
        size_t spec_len = i - spec_start;
        if (spec_len > sizeof(shown) - 3) {
            spec_len = sizeof(shown) - 3;
        }
        memcpy(shown + k, f.ptr + spec_start, spec_len);
        k += spec_len;
        shown[k] = (char)verb;
        shown[k + 1] = '\0';
        i++;
        if (verb == '%' && spec_len == 0) {
            sput(s, (const uint8_t *)"%", 1);
            continue;
        }
        if (strchr("-+0123456789.", (char)verb) != NULL) {
            return fail3(ctx, "str-fmt: bad verb ", shown, "");
        }
        if (verb != 's' && verb != 'v' && verb != 'd' && verb != 'f') {
            return fail3(ctx, "str-fmt: unknown verb ", shown, "");
        }
        if (next >= nargs) {
            return fail3(ctx, "str-fmt: missing argument for ", shown, "");
        }
        const filo_value *arg = &args[next];
        next++;
        int rc = FILO_OK;
        if (verb == 's' || verb == 'v') {
            filo_str text = {NULL, 0};
            rc = verb_text(ctx, arg, verb == 'v', &sp, &text);
            if (rc == FILO_OK) {
                pad_out(s, text, &sp, false);
            }
        } else if (verb == 'd') {
            rc = verb_int(ctx, arg, &sp, s);
        } else {
            rc = verb_fixed(ctx, arg, &sp, s);
        }
        if (rc != FILO_OK) {
            return FILO_ERR;
        }
    }
    if (next < nargs) {
        char count[16] = {0};
        (void)u32_text(count, sizeof(count), nargs - next);
        return fail3(ctx, "str-fmt: ", count, " extra arguments");
    }
    return FILO_OK;
}

static int s_fmt(filo_ctx *ctx, const filo_value *args, uint32_t n, filo_value *out) {
    if (n < 1) {
        return filo_fail(ctx, "str-fmt expects at least 1 argument (format string)");
    }
    filo_str f = {NULL, 0};
    if (str_arg(ctx, "str-fmt", "format", &args[0], &f) != FILO_OK) {
        return FILO_ERR;
    }
    fsink count = {NULL, 0, 0};
    if (fmt_run(ctx, f, args + 1, n - 1, &count) != FILO_OK) {
        return FILO_ERR;
    }
    uint8_t *buf = take(ctx, count.pos);
    if (buf == NULL) {
        return FILO_ERR;
    }
    fsink w = {buf, count.pos, 0};
    if (fmt_run(ctx, f, args + 1, n - 1, &w) != FILO_OK) {
        return FILO_ERR;
    }
    *out = filo_string(buf, (uint32_t)count.pos);
    return FILO_OK;
}

int filo_strings_register(filo_ctx *ctx, const filo_strings_fns *fns) {
    memset(&host_fns, 0, sizeof(host_fns));
    if (fns != NULL) {
        host_fns = *fns;
    }
    static const struct {
        const char *name;
        filo_builtin fn;
    } table[] = {
        {"str-join", s_join},   {"str-split", s_split},     {"str-find", s_find},
        {"str-trim", s_trim},   {"str-replace", s_replace}, {"str-upper", s_upper},
        {"str-lower", s_lower}, {"str-concat", s_concat},   {"str-len", s_len},
        {"str-sub", s_sub},     {"str-fmt", s_fmt},
    };
    for (size_t i = 0; i < sizeof(table) / sizeof(table[0]); i++) {
        if (filo_register_builtin(ctx, table[i].name, table[i].fn) != FILO_OK) {
            return FILO_ERR;
        }
    }
    return FILO_OK;
}
