package sandbox

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"time"
)

func logSandboxSelected(ctx context.Context, startedAt time.Time, launcher string, path string) {
	slog.DebugContext(ctx, "sandbox command selected",
		"launcher", launcher,
		"executable", filepath.Base(path),
		"duration_ms", time.Since(startedAt).Milliseconds(),
	)
}

func logSandboxFailure(ctx context.Context, startedAt time.Time, phase string, err error) {
	slog.DebugContext(ctx, "sandbox command rejected",
		"phase", phase,
		"duration_ms", time.Since(startedAt).Milliseconds(),
		"error_type", fmt.Sprintf("%T", err),
		"context_error", ctx.Err() != nil,
	)
}
