.PHONY: all build tools install vet test fmt tidy clean cross dist

# Filo is pure Go; the tools cross-compile from any host.
export CGO_ENABLED=0

BUILD_FLAGS := -trimpath -ldflags "-s -w"
TOOLS       := filo filofmt filofix

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
	rm -rf bin dist

# What release.sh publishes (VERSION is its tag, which each tool reports):
# every tool universal for macOS, gzipped for Linux, and for Windows, the
# module's own versions only (GOWORK off), amd64 and arm64.
VERSION  ?= dev
DIST_DIR ?= dist
DIST_LD  := -s -w -X main.Version=$(VERSION)
dist:
	@mkdir -p $(DIST_DIR)/.work
	@for t in $(TOOLS); do \
		for a in amd64 arm64; do \
			GOWORK=off GOOS=darwin GOARCH=$$a go build -trimpath -ldflags "$(DIST_LD)" \
				-o $(DIST_DIR)/.work/$$t-$$a ./cmd/$$t || exit 1; \
			GOWORK=off GOOS=linux GOARCH=$$a go build -trimpath -ldflags "$(DIST_LD)" \
				-o $(DIST_DIR)/$$t-linux-$$a ./cmd/$$t || exit 1; \
			gzip -9f $(DIST_DIR)/$$t-linux-$$a; \
			GOWORK=off GOOS=windows GOARCH=$$a go build -trimpath -ldflags "$(DIST_LD)" \
				-o $(DIST_DIR)/$$t-windows-$$a.exe ./cmd/$$t || exit 1; \
		done; \
		lipo -create -output $(DIST_DIR)/$$t-darwin-universal \
			$(DIST_DIR)/.work/$$t-amd64 $(DIST_DIR)/.work/$$t-arm64 || exit 1; \
	done

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
