package mcp

import (
	"context"
	"errors"
	"testing"

	"github.com/phongsathornpt/protonman/internal/core/tool"
)

func TestMCPFailureClassification(t *testing.T) {
	tests := []struct {
		kind  FailureKind
		err   error
		code  tool.ErrorCode
		retry bool
	}{
		{FailureTransport, errors.New("network"), tool.ErrorCodeNetworkUnavailable, true},
		{FailureDisconnected, errors.New("eof"), tool.ErrorCodeNetworkUnavailable, true},
		{FailureProtocol, errors.New("bad json"), tool.ErrorCodeExecution, false},
		{FailureTool, errors.New("rejected"), tool.ErrorCodeExecution, false},
		{FailureContract, errors.New("bad result"), tool.ErrorCodeInvalidOutput, false},
		{FailureTransport, context.DeadlineExceeded, tool.ErrorCodeDeadlineExceeded, true},
	}
	for _, tt := range tests {
		err := classifyFailure(tt.kind, "server", "tool", "op", tt.err)
		failure := tool.FailureFromError(err)
		if failure.Code != tt.code || failure.Retryable != tt.retry {
			t.Fatalf("kind %s => %#v, want code=%s retry=%v", tt.kind, failure, tt.code, tt.retry)
		}
	}
}
