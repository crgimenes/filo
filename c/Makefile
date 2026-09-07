# Filo C runtime: host build, QA gate and the corpus check.
CC ?= cc
LLVM ?= /opt/homebrew/opt/llvm/bin
WARN = -Wall -Wextra -Werror -Wshadow -Wconversion -Wdouble-promotion -Wundef
CFLAGS ?= -std=c11 -O2 $(WARN)
CORE = filo.c
HOST = filo_libc.c
CORPUS = ../testdata/corpus/*.txt

.PHONY: all corpus fmt fmt-check tidy check qa clean freestanding

all: build/corpus_runner

build/corpus_runner: $(CORE) $(HOST) corpus_runner.c filo.h filo_libc.h
	@mkdir -p build
	$(CC) $(CFLAGS) -o $@ $(CORE) $(HOST) corpus_runner.c -lm

# The corpus under sanitizers: the C runtime must agree with the Go engine
# on every case, and must do it without a single memory fault.
corpus: $(CORE) $(HOST) corpus_runner.c filo.h filo_libc.h
	@mkdir -p build
	$(CC) -std=c11 -O1 -g -fsanitize=address,undefined -fno-sanitize-recover=all $(WARN) \
		-o build/corpus_runner_san $(CORE) $(HOST) corpus_runner.c -lm
	./build/corpus_runner_san $(CORPUS)

fmt:
	$(LLVM)/clang-format -i *.c *.h

fmt-check:
	$(LLVM)/clang-format --dry-run --Werror *.c *.h

tidy:
	$(LLVM)/clang-tidy --quiet --warnings-as-errors='*' \
		--checks='bugprone-*,cert-*,clang-analyzer-*,readability-*,-readability-magic-numbers,-readability-function-cognitive-complexity,-readability-identifier-length,-readability-braces-around-statements,-bugprone-easily-swappable-parameters,-cert-err33-c,-readability-else-after-return,-readability-avoid-nested-conditional-operator,-readability-math-missing-parentheses,-cert-dcl03-c,-readability-uppercase-literal-suffix' \
		$(CORE) $(HOST) corpus_runner.c bench.c -- -std=c11

check:
	cppcheck --enable=warning,style,performance,portability --inline-suppr \
		--suppress=missingIncludeSystem --error-exitcode=1 $(CORE) $(HOST) corpus_runner.c bench.c

# Proves the core needs nothing from libc but memcpy/memcmp/strlen: the
# same freestanding wasm32 target the msh terminal uses.
freestanding: $(CORE) filo.h
	@mkdir -p build
	$(LLVM)/clang --target=wasm32 -ffreestanding -nostdlib -fno-builtin -c \
		-std=c11 -O2 $(WARN) -Wno-unused-function -isystem ../../bbs/msh/host/wasm/include \
		-o build/filo_wasm.o $(CORE)
	@echo "freestanding wasm32 object: build/filo_wasm.o"

bench: $(CORE) $(HOST) bench.c filo.h filo_libc.h
	@mkdir -p build
	$(CC) $(CFLAGS) -o build/bench $(CORE) $(HOST) bench.c -lm
	./build/bench 2000

qa: all fmt-check corpus tidy check freestanding

clean:
	rm -rf build
