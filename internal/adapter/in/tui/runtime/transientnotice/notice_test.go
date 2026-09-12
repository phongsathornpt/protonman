package transientnotice

import (
	"testing"
	"time"
)

func TestExpireAfterReturnsMatchingID(t *testing.T) {
	msg := ExpireAfter(42, time.Millisecond)()
	expired, ok := msg.(Expired)
	if !ok || expired.ID != 42 {
		t.Fatalf("expired = %#v", msg)
	}
}
