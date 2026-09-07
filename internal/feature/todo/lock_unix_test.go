//go:build unix

package todo

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestFileLockWaitHonorsContextCancellation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "todo.md.lock")
	unlock, err := lockFileContext(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if _, err := lockFileContext(ctx, path); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("second lock error=%v, want deadline exceeded", err)
	}
}

func TestFileLockCanBeReacquiredAfterRelease(t *testing.T) {
	path := filepath.Join(t.TempDir(), "todo.md.lock")
	unlock, err := lockFileContext(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	unlock()
	unlockAgain, err := lockFileContext(context.Background(), path)
	if err != nil {
		t.Fatalf("reacquire lock: %v", err)
	}
	unlockAgain()
}
