.PHONY: all build tools install vet test fmt tidy clean cross

# Filo is pure Go; the tools cross-compile from any host.
export CGO_ENABLED=0

BUILD_FLAGS := -trimpath -ldflags "-s -w"
TOOLS       := filo-repl filofmt filofix

# Default: the checks CI runs, plus the tool binaries in ./bin.
all: build vet test tools

build:
	go build ./...

# Build the command-line utilities into ./bin.
tools:
	@mkdir -p bin
	@for t in $(TOOLS); do \
		echo "  bin/$$t"; \
		go build $(BUILD_FLAGS) -o bin/$$t ./cmd/$$t; \
	done

# Install the utilities into GOPATH/bin.
install:
	go install $(BUILD_FLAGS) ./cmd/...

vet:
	go vet ./...

test:
	go test -count=1 -timeout 120s ./...

fmt:
	gofmt -w .

tidy:
	go mod tidy

clean:
	rm -rf bin

# Compile-check every supported target without running. Catches build-tag
# breakage that a single-host `go build` misses.
cross:
	GOOS=darwin  GOARCH=arm64 go build ./...
	GOOS=darwin  GOARCH=amd64 go build ./...
	GOOS=windows GOARCH=amd64 go build ./...
	GOOS=windows GOARCH=arm64 go build ./...
	GOOS=linux   GOARCH=amd64 go build ./...
	GOOS=linux   GOARCH=arm64 go build ./...
	GOOS=freebsd GOARCH=amd64 go build ./...
	GOOS=freebsd GOARCH=arm64 go build ./...
