package mcp

import (
	"context"
	"errors"
	"fmt"

	"github.com/phongsathornpt/proton/internal/core/tool"
)

type FailureKind string

const (
	FailureTransport    FailureKind = "transport"
	FailureProtocol     FailureKind = "protocol"
	FailureTimeout      FailureKind = "timeout"
	FailureDisconnected FailureKind = "disconnected"
	FailureTool         FailureKind = "tool"
	FailureContract     FailureKind = "contract"
)

// FailureError preserves MCP failure semantics through generic tool execution wrappers.
type FailureError struct {
	Kind   FailureKind
	Server string
	Tool   string
	Op     string
	Err    error
}

func (e *FailureError) Error() string {
	if e == nil {
		return "MCP failure"
	}
	target := e.Server
	if e.Tool != "" {
		target += "." + e.Tool
	}
	prefix := "MCP " + string(e.Kind)
	if e.Op != "" {
		prefix += " " + e.Op
	}
	if target != "" {
		prefix += " for " + target
	}
	if e.Err != nil {
		return prefix + ": " + e.Err.Error()
	}
	return prefix
}

func (e *FailureError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func (e *FailureError) FailureCode() tool.ErrorCode {
	if e == nil {
		return tool.ErrorCodeExecution
	}
	switch e.Kind {
	case FailureTimeout:
		return tool.ErrorCodeDeadlineExceeded
	case FailureTransport, FailureDisconnected:
		return tool.ErrorCodeNetworkUnavailable
	case FailureContract:
		return tool.ErrorCodeInvalidOutput
	default:
		return tool.ErrorCodeExecution
	}
}

func classifyFailure(kind FailureKind, server, toolName, op string, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.DeadlineExceeded) {
		kind = FailureTimeout
	}
	return &FailureError{Kind: kind, Server: server, Tool: toolName, Op: op, Err: err}
}

func rpcFailure(server, toolName, method string, rpcErr *rpcError) error {
	if rpcErr == nil {
		return nil
	}
	kind := FailureProtocol
	if method == "tools/call" {
		kind = FailureTool
	}
	return classifyFailure(kind, server, toolName, method, fmt.Errorf("JSON-RPC error %d: %s", rpcErr.Code, rpcErr.Message))
}
