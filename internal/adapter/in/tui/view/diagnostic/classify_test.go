package diagnostic

import (
	"context"
	"errors"
	"fmt"
	"testing"

	applicationturn "github.com/phongsathornpt/protonman/internal/engine/turn"
	sdk "github.com/phongsathornpt/protonman/proton-sdk"
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

func TestClassifyEmptyModelResponseIsActionable(t *testing.T) {
	err := fmt.Errorf("turn failed: %w", applicationturn.ErrEmptyResponse)
	got := Classify(err, "opencode", "nemotron-3.5-lightning-free")
	if got.Kind != KindEmptyResponse || !got.Retryable {
		t.Fatalf("classification = %+v, want retryable empty response", got)
	}
	if UserCode(got.Kind) != "EMPTY_RESPONSE" {
		t.Fatalf("user code = %q", UserCode(got.Kind))
	}
}

func TestClassifyIncompleteModelStreamIsDistinctFromTimeout(t *testing.T) {
	err := fmt.Errorf("read model stream: %w", sdk.ErrIncompleteStream)
	got := Classify(err, "opencode", "nemotron-3.5-lightning-free")
	if got.Kind != KindStreamIncomplete || !got.Retryable {
		t.Fatalf("classification = %+v, want retryable incomplete stream", got)
	}
	if got.Badge != "STREAM_INCOMPLETE" || UserCode(got.Kind) != "STREAM_INCOMPLETE" {
		t.Fatalf("badge=%q user_code=%q", got.Badge, UserCode(got.Kind))
	}
}

func TestClassifyBoundedIncompleteStreamTimeoutRemainsTimeout(t *testing.T) {
	err := fmt.Errorf("read model stream: %w: opencode free model stream became idle before completion", sdk.ErrIncompleteStream)
	got := Classify(err, "opencode", "nemotron-3.5-lightning-free")
	if got.Kind != KindStreamTimeout || !got.Retryable {
		t.Fatalf("classification = %+v, want retryable stream timeout", got)
	}
	if UserCode(got.Kind) != "STREAM_TIMEOUT" {
		t.Fatalf("user code = %q", UserCode(got.Kind))
	}
}
