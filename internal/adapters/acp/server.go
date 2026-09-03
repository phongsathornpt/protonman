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
	"sync"

	"github.com/projectTHORN/proton/internal/adapters/headless"
	"github.com/projectTHORN/proton/internal/application/toolcall"
	applicationturn "github.com/projectTHORN/proton/internal/application/turn"
	"github.com/projectTHORN/proton/internal/tool"
)

const protocolVersion = 1

// ErrInvalidServer indicates that the ACP server cannot start.
var ErrInvalidServer = errors.New("invalid ACP server")

// Server is a line-delimited JSON-RPC 2.0 ACP agent.
type Server struct {
	service  *toolcall.Service
	registry tool.Registry
	runner   applicationturn.Runner

	mu       sync.Mutex
	sessions map[string]*acpSession
	nextID   uint64
}

type acpSession struct {
	runner *headless.Runner

	mu        sync.Mutex
	active    bool
	cancelled bool
	cancel    context.CancelFunc
}

type promptExecution struct {
	session   *acpSession
	ctx       context.Context
	sessionID string
	prompt    string
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
		sessions: make(map[string]*acpSession),
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

type cancelParams struct {
	SessionID string `json:"sessionId"`
}

// StopReason represents the outcome of an ACP prompt turn.
type StopReason string

const (
	StopReasonCancelled StopReason = "cancelled"
	StopReasonEndTurn   StopReason = "end_turn"
)

// BlockType identifies the content type within an ACP prompt block.
type BlockType string

const (
	BlockTypeText BlockType = "text"
)

const (
	codeParseError     = -32700
	codeInvalidRequest = -32600
	codeServerError    = -32000
)

type promptBlock struct {
	Type BlockType  `json:"type"`
	Text string     `json:"text"`
}

type promptResult struct {
	StopReason StopReason `json:"stopReason"`
}

type lockedWriter struct {
	mu     sync.Mutex
	writer io.Writer
}

func (w *lockedWriter) Write(payload []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.writer.Write(payload)
}

// Serve reads JSON-RPC requests until the input closes. Prompt turns run
// independently from input scanning so a session/cancel notification can be
// processed while model or tool work is still active.
func (s *Server) Serve(ctx context.Context, input io.Reader, output io.Writer) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("start ACP server: %w", err)
	}
	if input == nil || output == nil {
		return fmt.Errorf("%w: input and output are required", ErrInvalidServer)
	}

	writer := &lockedWriter{writer: output}
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var prompts sync.WaitGroup
	asyncErrors := make(chan error, 1)

	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return err
		}
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		request, invalid := decodeRequest(line)
		if invalid != nil {
			if err := writeJSON(writer, invalid); err != nil {
				return err
			}
			continue
		}

		if request.Method != "session/prompt" {
			if err := s.handleRequest(ctx, request, writer); err != nil {
				return err
			}
			continue
		}

		execution, err := s.preparePrompt(ctx, request.Params)
		if err != nil {
			if err := writeRequestResult(writer, request.ID, nil, nil, err); err != nil {
				return err
			}
			continue
		}
		prompts.Add(1)
		go func(request rpcRequest, execution promptExecution) {
			defer prompts.Done()
			result, notify, promptErr := s.executePrompt(execution)
			if err := writeRequestResult(writer, request.ID, result, notify, promptErr); err != nil {
				select {
				case asyncErrors <- err:
				default:
				}
			}
		}(request, execution)
	}

	scanErr := scanner.Err()
	prompts.Wait()
	select {
	case err := <-asyncErrors:
		return err
	default:
	}
	if scanErr != nil {
		return fmt.Errorf("read ACP input: %w", scanErr)
	}
	return nil
}

func decodeRequest(line []byte) (rpcRequest, *rpcResponse) {
	var request rpcRequest
	if err := json.Unmarshal(line, &request); err != nil {
		return rpcRequest{}, &rpcResponse{
			JSONRPC: "2.0",
			Error:   &rpcError{Code: codeParseError, Message: "parse error"},
		}
	}
	if request.Method == "" {
		return rpcRequest{}, &rpcResponse{
			JSONRPC: "2.0",
			ID:      request.ID,
			Error:   &rpcError{Code: codeInvalidRequest, Message: "invalid request"},
		}
	}
	return request, nil
}

func (s *Server) handleLine(ctx context.Context, line []byte, output io.Writer) error {
	request, invalid := decodeRequest(bytes.TrimSpace(line))
	if invalid != nil {
		return writeJSON(output, invalid)
	}
	return s.handleRequest(ctx, request, output)
}

func (s *Server) handleRequest(ctx context.Context, request rpcRequest, output io.Writer) error {
	result, notify, err := s.dispatch(ctx, request)
	return writeRequestResult(output, request.ID, result, notify, err)
}

func writeRequestResult(output io.Writer, id json.RawMessage, result any, notify *rpcNotification, requestErr error) error {
	if requestErr != nil {
		if len(id) == 0 || string(id) == "null" {
			return nil
		}
		return writeJSON(output, rpcResponse{
			JSONRPC: "2.0",
			ID:      id,
			Error:   &rpcError{Code: codeServerError, Message: requestErr.Error()},
		})
	}
	if notify != nil {
		if err := writeJSON(output, notify); err != nil {
			return err
		}
	}
	if len(id) == 0 || string(id) == "null" {
		return nil
	}
	return writeJSON(output, rpcResponse{
		JSONRPC: "2.0",
		ID:      id,
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
		runner, err := headless.New(s.service, s.registry, s.runner)
		if err != nil {
			return nil, nil, err
		}
		s.mu.Lock()
		s.nextID++
		sessionID := fmt.Sprintf("acp-%d", s.nextID)
		s.sessions[sessionID] = &acpSession{runner: runner}
		s.mu.Unlock()
		return sessionNewResult{SessionID: sessionID}, nil, nil
	case "session/prompt":
		execution, err := s.preparePrompt(ctx, request.Params)
		if err != nil {
			return nil, nil, err
		}
		return s.executePrompt(execution)
	case "session/cancel":
		if err := s.cancelPrompt(request.Params); err != nil {
			return nil, nil, err
		}
		return map[string]any{}, nil, nil
	default:
		return nil, nil, fmt.Errorf("method %q is not supported", request.Method)
	}
}

func (s *Server) preparePrompt(ctx context.Context, raw json.RawMessage) (promptExecution, error) {
	var params promptParams
	if err := json.Unmarshal(raw, &params); err != nil {
		return promptExecution{}, fmt.Errorf("decode session/prompt: %w", err)
	}
	params.SessionID = strings.TrimSpace(params.SessionID)
	if params.SessionID == "" {
		return promptExecution{}, fmt.Errorf("session/prompt sessionId is required")
	}
	session, ok := s.lookupSession(params.SessionID)
	if !ok {
		return promptExecution{}, fmt.Errorf("unknown session %q", params.SessionID)
	}

	var builder strings.Builder
	for _, block := range params.Prompt {
		builder.WriteString(block.Text)
	}
	prompt := strings.TrimSpace(builder.String())

	session.mu.Lock()
	defer session.mu.Unlock()
	if session.active {
		return promptExecution{}, fmt.Errorf("session %q already has an active prompt", params.SessionID)
	}
	promptContext, cancel := context.WithCancel(ctx)
	session.active = true
	session.cancelled = false
	session.cancel = cancel
	return promptExecution{
		session:   session,
		ctx:       promptContext,
		sessionID: params.SessionID,
		prompt:    prompt,
	}, nil
}

func (s *Server) executePrompt(execution promptExecution) (any, *rpcNotification, error) {
	defer execution.finish()
	var buffer bytes.Buffer
	err := execution.session.runner.Run(execution.ctx, execution.prompt, &buffer, headless.FormatText)
	if execution.wasCancelled() {
		return promptResult{StopReason: StopReasonCancelled}, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	text := strings.TrimRight(buffer.String(), "\n")
	notify := &rpcNotification{
		JSONRPC: "2.0",
		Method:  "session/update",
		Params: map[string]any{
			"sessionId": execution.sessionID,
			"update": map[string]any{
				"sessionUpdate": "agent_message_chunk",
				"content": map[string]any{
					"type": string(BlockTypeText),
					"text": text,
				},
			},
		},
	}
	return promptResult{StopReason: StopReasonEndTurn}, notify, nil
}

func (s *Server) cancelPrompt(raw json.RawMessage) error {
	var params cancelParams
	if err := json.Unmarshal(raw, &params); err != nil {
		return fmt.Errorf("decode session/cancel: %w", err)
	}
	params.SessionID = strings.TrimSpace(params.SessionID)
	if params.SessionID == "" {
		return fmt.Errorf("session/cancel sessionId is required")
	}
	session, ok := s.lookupSession(params.SessionID)
	if !ok {
		return fmt.Errorf("unknown session %q", params.SessionID)
	}

	session.mu.Lock()
	cancel := session.cancel
	if session.active && cancel != nil {
		session.cancelled = true
	}
	session.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	return nil
}

func (s *Server) lookupSession(sessionID string) (*acpSession, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.sessions[sessionID]
	return session, ok
}

func (e promptExecution) wasCancelled() bool {
	e.session.mu.Lock()
	defer e.session.mu.Unlock()
	return e.session.cancelled
}

func (e promptExecution) finish() {
	e.session.mu.Lock()
	cancel := e.session.cancel
	e.session.active = false
	e.session.cancelled = false
	e.session.cancel = nil
	e.session.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func writeJSON(output io.Writer, value any) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode ACP message: %w", err)
	}
	payload = append(payload, '\n')
	_, err = output.Write(payload)
	return err
}
