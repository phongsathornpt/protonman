package anthropic

import (
	"errors"
	"io"
	"strings"
	"sync/atomic"
	"testing"

	sdk "github.com/phongsathornpt/protonman/proton-sdk"
)

// TestStreamCloseIsIdempotent verifies repeated Close calls close the
// underlying body once and return the same result.
func TestStreamCloseIsIdempotent(t *testing.T) {
	calls := atomic.Int32{}
	body := &countCloseReadCloser{Reader: strings.NewReader(""), onClose: func() { calls.Add(1) }}
	stream := newStream(body, sdk.ProviderMetadata{}, false)
	if err := stream.Close(); err != nil {
		t.Fatalf("first Close() error = %v", err)
	}
	if err := stream.Close(); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("underlying Close calls = %d, want 1", got)
	}
}

// TestStreamCloseReturnsFirstError verifies the first close error wins over
// later calls.
func TestStreamCloseReturnsFirstError(t *testing.T) {
	want := errors.New("close failed")
	body := &countCloseReadCloser{Reader: strings.NewReader(""), err: want}
	stream := newStream(body, sdk.ProviderMetadata{}, false)
	if err := stream.Close(); !errors.Is(err, want) {
		t.Fatalf("first Close() = %v, want %v", err, want)
	}
	if err := stream.Close(); !errors.Is(err, want) {
		t.Fatalf("second Close() = %v, want %v", err, want)
	}
}

type countCloseReadCloser struct {
	io.Reader
	err     error
	onClose func()
}

func (c *countCloseReadCloser) Close() error {
	if c.onClose != nil {
		c.onClose()
	}
	return c.err
}
