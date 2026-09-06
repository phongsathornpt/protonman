.DEFAULT_GOAL := tui

.PHONY: all tui run dev build run-bin clean test test-race test-e2e bench bench-cpu bench-mem fmt vet lint help

# Binary configuration
BIN_DIR := bin
BIN_NAME := proton
BINARY := $(BIN_DIR)/$(BIN_NAME)
GO_SOURCES := $(shell find cmd internal -type f -name '*.go' ! -name '*_test.go')

## tui: Run Proton TUI from the cached binary (default)
tui: run

## run: Build Proton only when sources changed, then run it
run: $(BINARY)
	./$(BINARY)

## dev: Run Proton through go run (always invokes the Go toolchain)
dev:
	go run ./cmd/proton

## build: Build the proton binary only when sources changed
build: $(BINARY)

$(BINARY): $(GO_SOURCES) go.mod go.sum
	@mkdir -p $(BIN_DIR)
	go build -o $(BINARY) ./cmd/proton

## run-bin: Alias for run
run-bin: run

## test: Run unit and adapter tests
test:
	go test ./cmd/... ./internal/...

## test-race: Run all tests with race detector
test-race:
	go test -race ./...

## test-e2e: Run end-to-end tests
test-e2e:
	go test -v ./test/e2e/...

## bench: Run all benchmarks with memory allocation profiling
bench:
	go test -run=^$$ -bench=. -benchmem ./...

## bench-cpu: Run benchmarks with CPU profiling to cpu.pprof
bench-cpu:
	go test -run=^$$ -bench=. -benchmem -cpuprofile=cpu.pprof ./internal/permission/...

## bench-mem: Run benchmarks with memory profiling to mem.pprof
bench-mem:
	go test -run=^$$ -bench=. -benchmem -memprofile=mem.pprof ./internal/permission/...

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

## help: Display this help message
help:
	@echo "Proton - Go Coding Agent"
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@sed -n 's/^## //p' $(MAKEFILE_LIST) | column -t -s ':'
