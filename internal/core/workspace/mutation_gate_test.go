package workspace

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestMutationGateSerializesAndHonorsCancellation(t *testing.T) {
	ws, err := New(t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	release, err := ws.AcquireMutation(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := ws.AcquireMutation(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("blocked acquire error = %v, want deadline exceeded", err)
	}
	release()
	if release2, err := ws.AcquireMutation(context.Background()); err != nil {
		t.Fatal(err)
	} else {
		release2()
	}
}

func TestMutationGateIsScopedPerWorkspace(t *testing.T) {
	first, _ := New(t.TempDir(), nil)
	second, _ := New(t.TempDir(), nil)
	releaseFirst, err := first.AcquireMutation(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer releaseFirst()
	releaseSecond, err := second.AcquireMutation(context.Background())
	if err != nil {
		t.Fatalf("different workspace blocked: %v", err)
	}
	releaseSecond()
}
