package diagnostic

import (
	"context"
	"errors"
	"testing"
)

func TestClassifyRuntimeDeadlineIsNotNetworkTimeout(t *testing.T) {
	got := Classify(context.DeadlineExceeded, "", "")
	if got.Kind != KindRuntimeTimeout {
		t.Fatalf("kind = %q, want %q", got.Kind, KindRuntimeTimeout)
	}
	if got.Title == "Connection / Stream Timeout" {
		t.Fatalf("runtime deadline misclassified as network timeout: %+v", got)
	}
}

func TestClassifyWrappedRuntimeDeadlineIsNotNetworkTimeout(t *testing.T) {
	got := Classify(errors.New("worker wait failed: context deadline exceeded"), "", "")
	if got.Kind != KindRuntimeTimeout {
		t.Fatalf("kind = %q, want %q", got.Kind, KindRuntimeTimeout)
	}
}

func TestClassifyProviderStreamTimeoutRemainsNetworkTimeout(t *testing.T) {
	got := Classify(errors.New("ProviderHeaderTimeoutError: upstream response timeout"), "", "")
	if got.Kind != KindStreamTimeout {
		t.Fatalf("kind = %q, want %q", got.Kind, KindStreamTimeout)
	}
}
