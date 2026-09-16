package main

import (
	"context"
	"fmt"

	"github.com/phongsathornpt/protonman/internal/adapter/out/memoryfs"
	"github.com/phongsathornpt/protonman/internal/app"
	"github.com/phongsathornpt/protonman/internal/platform/appdirs"
)

// buildACPMemories composes the read-only durable-memory application facade for
// ACP clients. It intentionally opens the same file-backed repository through
// the composition root instead of letting the inbound ACP adapter know about
// filesystem persistence.
func buildACPMemories(_ context.Context) (*app.Memories, error) {
	layout, err := appdirs.ResolveRuntimeLayout("", "")
	if err != nil {
		return nil, fmt.Errorf("resolve ACP memory layout: %w", err)
	}
	store, err := memoryfs.NewFileStore(layout.User.Memory)
	if err != nil {
		return nil, fmt.Errorf("open ACP memory store: %w", err)
	}
	return app.NewMemories(store), nil
}
