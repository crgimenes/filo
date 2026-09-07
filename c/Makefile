# Filo C runtime: host build, QA gate and the corpus check.
CC ?= cc
LLVM ?= /opt/homebrew/opt/llvm/bin
WARN = -Wall -Wextra -Werror -Wshadow -Wconversion -Wdouble-promotion -Wundef
CFLAGS ?= -std=c11 -O2 $(WARN)
CORE = filo.c
PACKS = filo_math.c filo_strings.c
HOST = filo_libc.c
HDRS = filo.h filo_libc.h filo_math.h filo_strings.h
CORPUS = ../testdata/corpus/*.txt

.PHONY: all corpus fmt fmt-check tidy check qa clean freestanding fuzz bench

all: build/corpus_runner

build/corpus_runner: $(CORE) $(PACKS) $(HOST) corpus_runner.c $(HDRS)
	@mkdir -p build
	$(CC) $(CFLAGS) -o $@ $(CORE) $(PACKS) $(HOST) corpus_runner.c -lm

# The corpus under sanitizers: the C runtime must agree with the Go engine
# on every case, and must do it without a single memory fault.
corpus: $(CORE) $(PACKS) $(HOST) corpus_runner.c $(HDRS)
	@mkdir -p build
	$(CC) -std=c11 -O1 -g -fsanitize=address,undefined -fno-sanitize-recover=all $(WARN) \
		-o build/corpus_runner_san $(CORE) $(PACKS) $(HOST) corpus_runner.c -lm
	./build/corpus_runner_san $(CORPUS)

fmt:
	$(LLVM)/clang-format -i *.c *.h

fmt-check:
	$(LLVM)/clang-format --dry-run --Werror *.c *.h

tidy:
	$(LLVM)/clang-tidy --quiet --warnings-as-errors='*' \
		--checks='bugprone-*,cert-*,clang-analyzer-*,readability-*,-readability-magic-numbers,-readability-function-cognitive-complexity,-readability-identifier-length,-readability-braces-around-statements,-bugprone-easily-swappable-parameters,-cert-err33-c,-readability-else-after-return,-readability-avoid-nested-conditional-operator,-readability-math-missing-parentheses,-cert-dcl03-c,-readability-uppercase-literal-suffix' \
		$(CORE) $(PACKS) $(HOST) corpus_runner.c bench.c fuzz.c -- -std=c11

check:
	cppcheck --enable=warning,style,performance,portability --inline-suppr \
		--suppress=missingIncludeSystem --error-exitcode=1 $(CORE) $(PACKS) $(HOST) corpus_runner.c bench.c fuzz.c

# Proves the core and the packs need nothing from libc but memcpy/memcmp/
# strlen/strchr: the same freestanding wasm32 target the msh terminal uses.
freestanding: $(CORE) $(PACKS) $(HDRS)
	@mkdir -p build
	@for f in $(CORE) $(PACKS); do \
		$(LLVM)/clang --target=wasm32 -ffreestanding -nostdlib -fno-builtin -c \
			-std=c11 -O2 $(WARN) -Wno-unused-function -isystem ../../bbs/msh/host/wasm/include \
			-o build/$${f%.c}_wasm.o $$f || exit 1; \
		echo "freestanding wasm32 object: build/$${f%.c}_wasm.o"; \
	done

FUZZ_SECONDS ?= 15

# Any byte string is a script and the runtime must not fault on any of them.
# Seeds are the corpus scripts; a finding is written as build/fuzz_crash_*.
fuzz: $(CORE) $(PACKS) $(HOST) fuzz.c fuzz.dict $(HDRS)
	@mkdir -p build/fuzz_seeds build/fuzz_corpus
	@awk '/^=== /{n++; f=sprintf("build/fuzz_seeds/%04d", n); next} \
		/^--- /{if (f != "") close(f); f=""; next} \
		/^(given |limits )/{next} f!=""{print > f}' $(CORPUS)
	$(LLVM)/clang -std=c11 -O1 -g -fsanitize=fuzzer,address,undefined -fno-sanitize-recover=all $(WARN) \
		-o build/fuzz $(CORE) $(PACKS) $(HOST) fuzz.c -lm
	@./build/fuzz -max_total_time=$(FUZZ_SECONDS) -max_len=2048 -dict=fuzz.dict \
		-artifact_prefix=build/fuzz_crash_ build/fuzz_corpus build/fuzz_seeds > build/fuzz.log 2>&1 \
		|| { tail -30 build/fuzz.log; exit 1; }
	@tail -2 build/fuzz.log

bench: $(CORE) $(PACKS) $(HOST) bench.c $(HDRS)
	@mkdir -p build
	$(CC) $(CFLAGS) -o build/bench $(CORE) $(PACKS) $(HOST) bench.c -lm
	./build/bench 2000

qa: all fmt-check corpus tidy check freestanding fuzz

clean:
	rm -rf build
