// Package acp implements a JSON-RPC Agent Client Protocol stdio transport.
package acp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/projectTHORN/proton/internal/adapters/headless"
	"github.com/projectTHORN/proton/internal/application/toolcall"
	applicationturn "github.com/projectTHORN/proton/internal/application/turn"
	"github.com/projectTHORN/proton/internal/domain/tool"
)

const protocolVersion = 1

// ErrInvalidServer indicates that the ACP server cannot start.
var ErrInvalidServer = errors.New("invalid ACP server")

// Server is a line-delimited JSON-RPC 2.0 ACP agent.
type Server struct {
	service  *toolcall.Service
	registry tool.Registry
	runner   applicationturn.Runner
	sessions map[string]*headless.Runner
	nextID   uint64
}

// New creates an ACP agent over Proton's existing tool-call service.
func New(service *toolcall.Service, registry tool.Registry, runner applicationturn.Runner) (*Server, error) {
	if service == nil {
		return nil, fmt.Errorf("%w: service is required", ErrInvalidServer)
	}
	if registry == nil {
		return nil, fmt.Errorf("%w: registry is required", ErrInvalidServer)
	}
	return &Server{
		service:  service,
		registry: registry,
		runner:   runner,
		sessions: make(map[string]*headless.Runner),
	}, nil
}

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type rpcNotification struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  any    `json:"params"`
}

type sessionNewResult struct {
	SessionID string `json:"sessionId"`
}

type promptParams struct {
	SessionID string        `json:"sessionId"`
	Prompt    []promptBlock `json:"prompt"`
}

type promptBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type promptResult struct {
	StopReason string `json:"stopReason"`
}

// Serve reads JSON-RPC requests until the input closes.
func (s *Server) Serve(ctx context.Context, input io.Reader, output io.Writer) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("start ACP server: %w", err)
	}
	if input == nil || output == nil {
		return fmt.Errorf("%w: input and output are required", ErrInvalidServer)
	}
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return err
		}
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		if err := s.handleLine(ctx, line, output); err != nil {
			return err
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read ACP input: %w", err)
	}
	return nil
}

func (s *Server) handleLine(ctx context.Context, line []byte, output io.Writer) error {
	var request rpcRequest
	if err := json.Unmarshal(line, &request); err != nil {
		return writeJSON(output, rpcResponse{
			JSONRPC: "2.0",
			Error:   &rpcError{Code: -32700, Message: "parse error"},
		})
	}
	if request.Method == "" {
		return writeJSON(output, rpcResponse{
			JSONRPC: "2.0",
			ID:      request.ID,
			Error:   &rpcError{Code: -32600, Message: "invalid request"},
		})
	}
	result, notify, err := s.dispatch(ctx, request)
	if err != nil {
		return writeJSON(output, rpcResponse{
			JSONRPC: "2.0",
			ID:      request.ID,
			Error:   &rpcError{Code: -32000, Message: err.Error()},
		})
	}
	if notify != nil {
		if err := writeJSON(output, notify); err != nil {
			return err
		}
	}
	if len(request.ID) == 0 || string(request.ID) == "null" {
		return nil
	}
	return writeJSON(output, rpcResponse{
		JSONRPC: "2.0",
		ID:      request.ID,
		Result:  result,
	})
}

func (s *Server) dispatch(ctx context.Context, request rpcRequest) (any, *rpcNotification, error) {
	switch request.Method {
	case "initialize":
		return map[string]any{
			"protocolVersion": protocolVersion,
			"agentCapabilities": map[string]any{
				"loadSession": false,
			},
			"agentInfo": map[string]any{
				"name":    "proton",
				"version": "dev",
			},
		}, nil, nil
	case "session/new":
		s.nextID++
		sessionID := fmt.Sprintf("acp-%d", s.nextID)
		runner, err := headless.New(s.service, s.registry, s.runner)
		if err != nil {
			return nil, nil, err
		}
		s.sessions[sessionID] = runner
		return sessionNewResult{SessionID: sessionID}, nil, nil
	case "session/prompt":
		return s.prompt(ctx, request.Params)
	case "session/cancel":
		return map[string]any{}, nil, nil
	default:
		return nil, nil, fmt.Errorf("method %q is not supported", request.Method)
	}
}

func (s *Server) prompt(ctx context.Context, raw json.RawMessage) (any, *rpcNotification, error) {
	var params promptParams
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil, nil, fmt.Errorf("decode session/prompt: %w", err)
	}
	runner, ok := s.sessions[params.SessionID]
	if !ok {
		return nil, nil, fmt.Errorf("unknown session %q", params.SessionID)
	}
	var builder strings.Builder
	for _, block := range params.Prompt {
		builder.WriteString(block.Text)
	}
	prompt := strings.TrimSpace(builder.String())
	var buffer bytes.Buffer
	if err := runner.Run(ctx, prompt, &buffer, headless.FormatText); err != nil {
		return nil, nil, err
	}
	text := strings.TrimRight(buffer.String(), "\n")
	notify := &rpcNotification{
		JSONRPC: "2.0",
		Method:  "session/update",
		Params: map[string]any{
			"sessionId": params.SessionID,
			"update": map[string]any{
				"sessionUpdate": "agent_message_chunk",
				"content": map[string]any{
					"type": "text",
					"text": text,
				},
			},
		},
	}
	return promptResult{StopReason: "end_turn"}, notify, nil
}

func writeJSON(output io.Writer, value any) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode ACP message: %w", err)
	}
	_, err = fmt.Fprintf(output, "%s\n", payload)
	return err
}
