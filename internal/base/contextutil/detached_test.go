package contextutil

import (
	"context"
	"testing"
	"time"
)

type valueKey struct{}

func TestDetachedTimeoutPreservesValuesWithoutParentCancellation(t *testing.T) {
	parent, cancelParent := context.WithCancel(context.WithValue(context.Background(), valueKey{}, "value"))
	cancelParent()
	ctx, cancel := DetachedTimeout(parent, time.Second)
	defer cancel()
	if err := ctx.Err(); err != nil {
		t.Fatalf("detached context inherited cancellation: %v", err)
	}
	if got := ctx.Value(valueKey{}); got != "value" {
		t.Fatalf("value = %v, want value", got)
	}
}

func TestDetachedTimeoutExpires(t *testing.T) {
	ctx, cancel := DetachedTimeout(context.Background(), time.Millisecond)
	defer cancel()
	select {
	case <-ctx.Done():
		if ctx.Err() != context.DeadlineExceeded {
			t.Fatalf("error = %v, want deadline exceeded", ctx.Err())
		}
	case <-time.After(time.Second):
		t.Fatal("detached context did not expire")
	}
}
