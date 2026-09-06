package protonsdk

import (
	"errors"
	"fmt"
	"testing"
)

func TestProviderErrorClassification(t *testing.T) {
	for _, tc := range []struct {
		status int
		code   string
		want   ErrorKind
	}{
		{401, "authentication_error", ErrorAuthentication},
		{403, "permission_error", ErrorPermission},
		{429, "rate_limit_error", ErrorRateLimit},
		{404, "model_not_found", ErrorModelNotFound},
		{401, "ModelError", ErrorModelNotFound},
		{400, "context_length_exceeded", ErrorContextLength},
		{529, "overloaded_error", ErrorOverloaded},
	} {
		err := NewProviderError("test", tc.status, tc.code, "message")
		if err.Kind != tc.want {
			t.Fatalf("status=%d code=%q kind=%q want=%q", tc.status, tc.code, err.Kind, tc.want)
		}
	}
}

func TestTransportErrorUnwrapsCause(t *testing.T) {
	cause := fmt.Errorf("network down")
	err := NewTransportError("test", cause)
	if err.Kind != ErrorTransport || !err.Retryable || !errors.Is(err, cause) {
		t.Fatalf("transport error = %#v", err)
	}
}
