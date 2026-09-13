.DEFAULT_GOAL := tui

.PHONY: all tui desktop desktop-run run dev build install run-bin clean test test-architecture test-race test-e2e test-install bench bench-cpu bench-mem fmt vet lint tag tag-push help

# Binary configuration
BIN_DIR := bin
BIN_NAME := protonman
BINARY := $(BIN_DIR)/$(BIN_NAME)
DESKTOP_BINARY := $(BIN_DIR)/protonman-desktop
INSTALL_DIR ?= $(HOME)/.local/bin
INSTALL_BINARY := $(INSTALL_DIR)/$(BIN_NAME)
GO_SOURCES := $(shell find cmd internal proton-sdk -type f -name '*.go' ! -name '*_test.go')
VERSION ?= $(shell git describe --tags --always --dirty --match 'v[0-9]*' 2>/dev/null || echo dev)
VERSION_LDFLAGS := -X github.com/phongsathornpt/protonman/internal/base/buildinfo.version=$(VERSION)
BUILD_LDFLAGS := $(strip $(LDFLAGS) $(VERSION_LDFLAGS))
VERSION_KEY := $(subst /,_,$(VERSION))
VERSION_STAMP := $(BIN_DIR)/.version-$(VERSION_KEY)
GO_TMPDIR ?= $(HOME)/.cache/protonman/tmp
GO_ENV := GOTMPDIR="$(GO_TMPDIR)"

## tui: Run Protonman TUI from the cached binary (default)
tui: run

## desktop: Build the Fyne desktop client
# Linux requires the normal Fyne desktop development packages (OpenGL/X11).
desktop: $(DESKTOP_BINARY)

## desktop-run: Build and run the Fyne desktop client against the local CLI
desktop-run: $(BINARY) $(DESKTOP_BINARY)
	PROTONMAN_BINARY="$(abspath $(BINARY))" ./$(DESKTOP_BINARY)

## run: Build Protonman only when sources changed, then run it
run: $(BINARY)
	./$(BINARY)

## dev: Run Protonman through go run (always invokes the Go toolchain)
dev:
	@mkdir -p "$(GO_TMPDIR)"
	$(GO_ENV) go run -ldflags "$(BUILD_LDFLAGS)" ./cmd/protonman

## build: Build the protonman binary when sources or resolved version changed
build: $(BINARY)

$(VERSION_STAMP):
	@mkdir -p "$(BIN_DIR)"
	@rm -f "$(BIN_DIR)"/.version-*
	@touch "$@"

$(BINARY): $(GO_SOURCES) go.mod go.sum Makefile $(VERSION_STAMP)
	@mkdir -p $(BIN_DIR) "$(GO_TMPDIR)"
	$(GO_ENV) go build -trimpath -ldflags "$(BUILD_LDFLAGS)" -o $(BINARY) ./cmd/protonman

$(DESKTOP_BINARY): $(GO_SOURCES) go.mod go.sum Makefile $(VERSION_STAMP)
	@mkdir -p $(BIN_DIR) "$(GO_TMPDIR)"
	$(GO_ENV) go build -tags desktop -trimpath -ldflags "$(BUILD_LDFLAGS)" -o $(DESKTOP_BINARY) ./cmd/protonman-desktop

## install: Build from the current source tree and install into ~/.local/bin by default
install: build
	@mkdir -p "$(INSTALL_DIR)"
	install -m 0755 "$(BINARY)" "$(INSTALL_BINARY)"
	@printf 'installed %s (%s)\n' "$(INSTALL_BINARY)" "$$($(INSTALL_BINARY) --version)"

## run-bin: Alias for run
run-bin: run

## test: Run the complete repository test suite
test:
	go test ./...

## test-architecture: Run architecture and SDK ownership regression guards
test-architecture:
	go test ./test/architecture/...
	go test ./proton-sdk/... -run 'Ownership|Architecture|Contract'

## test-race: Run all tests with race detector
test-race:
	go test -race ./...

## test-e2e: Run end-to-end tests
test-e2e:
	go test -v ./test/e2e/...

## test-install: Run offline installer integration tests
test-install:
	./test/install/install_test.sh

## bench: Run all benchmarks with memory allocation profiling
bench:
	go test -run=^$$ -bench=. -benchmem ./...

## bench-cpu: Run benchmarks with CPU profiling to cpu.pprof
bench-cpu:
	go test -run=^$$ -bench=. -benchmem -cpuprofile=cpu.pprof ./internal/core/permission/...

## bench-mem: Run benchmarks with memory profiling to mem.pprof
bench-mem:
	go test -run=^$$ -bench=. -benchmem -memprofile=mem.pprof ./internal/core/permission/...

## fmt: Format all Go source files
fmt:
	gofmt -w .

## vet: Run go vet linter
vet:
	go vet ./...

## lint: Check formatting and vet code
lint: vet
	@test -z "$$(gofmt -l .)" || (echo "Unformatted files exist:" && gofmt -l . && exit 1)

## clean: Remove built binaries
clean:
	rm -rf $(BIN_DIR)

## tag: Create an annotated release tag (usage: make tag TAG=v1.2.3)
tag:
	@test -n "$(TAG)" || (echo "TAG is required, e.g. make tag TAG=v1.2.3" >&2; exit 1)
	@printf '%s\n' "$(TAG)" | grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z][0-9A-Za-z.-]*)?$$' || (echo "invalid release tag: $(TAG)" >&2; exit 1)
	@test -z "$$(git status --porcelain)" || (echo "working tree must be clean before tagging" >&2; exit 1)
	@! git rev-parse -q --verify "refs/tags/$(TAG)" >/dev/null || (echo "tag already exists: $(TAG)" >&2; exit 1)
	git tag -a "$(TAG)" -m "Protonman $(TAG)"
	@echo "created tag $(TAG)"

## tag-push: Push an existing release tag to origin (usage: make tag-push TAG=v1.2.3)
tag-push:
	@test -n "$(TAG)" || (echo "TAG is required, e.g. make tag-push TAG=v1.2.3" >&2; exit 1)
	@git rev-parse -q --verify "refs/tags/$(TAG)" >/dev/null || (echo "local tag does not exist: $(TAG)" >&2; exit 1)
	git push origin "$(TAG)"

## help: Display this help message
help:
	@echo "Protonman - Go Coding Agent"
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@sed -n 's/^## //p' $(MAKEFILE_LIST) | column -t -s ':'
