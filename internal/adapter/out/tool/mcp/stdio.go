package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
)

const stdioProtocolVersion = "2025-06-18"

type rpcError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

type rpcEnvelope struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcReply struct {
	result json.RawMessage
	rpcErr *rpcError
	err    error
}

// StdioServer runs one MCP server process and speaks newline-delimited JSON-RPC over stdio.
type StdioServer struct {
	name    string
	command string
	args    []string
	env     []string
	cwd     string
	limits  Limits

	startMu sync.Mutex
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	started bool
	closed  bool
	initMu  sync.Mutex
	inited  bool

	writeMu        sync.Mutex
	pendingMu      sync.Mutex
	pending        map[uint64]chan rpcReply
	nextID         atomic.Uint64
	done           chan struct{}
	waitErr        error
	stderr         limitedBuffer
	notifyMu       sync.RWMutex
	onToolsChanged func()
}

// NewStdioServer creates a lazily-started MCP stdio transport.
func NewStdioServer(name, command string, args, env []string, cwd string) (*StdioServer, error) {
	return NewStdioServerWithLimits(name, command, args, env, cwd, DefaultLimits())
}

func NewStdioServerWithLimits(name, command string, args, env []string, cwd string, limits Limits) (*StdioServer, error) {
	if _, err := validServerName(name); err != nil {
		return nil, err
	}
	if err := limits.validate(); err != nil {
		return nil, fmt.Errorf("create MCP stdio server %q: %w", name, err)
	}
	command = strings.TrimSpace(command)
	if command == "" {
		return nil, fmt.Errorf("create MCP stdio server %q: command is required", name)
	}
	return &StdioServer{
		name: name, command: command, limits: limits,
		args: append([]string(nil), args...), env: append([]string(nil), env...), cwd: cwd,
		pending: make(map[uint64]chan rpcReply), done: make(chan struct{}),
		stderr: limitedBuffer{limit: limits.MaxStderrBytes},
	}, nil
}

func (s *StdioServer) Name() string { return s.name }

func (s *StdioServer) SetToolListChangedHandler(handler func()) {
	s.notifyMu.Lock()
	s.onToolsChanged = handler
	s.notifyMu.Unlock()
}

func (s *StdioServer) notifyToolsChanged() {
	s.notifyMu.RLock()
	handler := s.onToolsChanged
	s.notifyMu.RUnlock()
	if handler != nil {
		handler()
	}
}

func (s *StdioServer) ListTools(ctx context.Context) ([]Tool, error) {
	if err := s.ensureInitialized(ctx); err != nil {
		return nil, err
	}
	var response struct {
		Tools []struct {
			Name         string          `json:"name"`
			Description  string          `json:"description"`
			InputSchema  map[string]any  `json:"inputSchema"`
			OutputSchema map[string]any  `json:"outputSchema"`
			Annotations  ToolAnnotations `json:"annotations,omitempty"`
		} `json:"tools"`
	}
	if err := s.request(ctx, "tools/list", map[string]any{}, &response); err != nil {
		return nil, fmt.Errorf("list MCP tools from %q: %w", s.name, err)
	}
	tools := make([]Tool, 0, len(response.Tools))
	for _, item := range response.Tools {
		tools = append(tools, Tool{
			Name: item.Name, Description: item.Description,
			InputSchema: item.InputSchema, OutputSchema: item.OutputSchema,
			Mutability: item.Annotations.declaredMutability(), Annotations: item.Annotations,
		})
	}
	return tools, nil
}

func (s *StdioServer) CallTool(ctx context.Context, name string, arguments json.RawMessage) (Result, error) {
	if len(arguments) > s.limits.MaxArgumentsBytes {
		return Result{}, fmt.Errorf("MCP tool arguments are %d bytes; limit is %d", len(arguments), s.limits.MaxArgumentsBytes)
	}
	if err := s.ensureInitialized(ctx); err != nil {
		return Result{}, err
	}
	var response struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text,omitempty"`
		} `json:"content"`
		Structured json.RawMessage `json:"structuredContent,omitempty"`
		IsError    bool            `json:"isError,omitempty"`
	}
	var args any = map[string]any{}
	if len(bytes.TrimSpace(arguments)) > 0 {
		if err := json.Unmarshal(arguments, &args); err != nil {
			return Result{}, fmt.Errorf("decode MCP tool arguments: %w", err)
		}
	}
	if err := s.request(ctx, "tools/call", map[string]any{"name": name, "arguments": args}, &response); err != nil {
		return Result{}, fmt.Errorf("call MCP tool %s.%s: %w", s.name, name, err)
	}
	parts := make([]string, 0, len(response.Content))
	textBytes := 0
	for _, item := range response.Content {
		if item.Type == "text" && item.Text != "" {
			textBytes += len(item.Text)
			if textBytes > s.limits.MaxTextOutputBytes {
				return Result{}, fmt.Errorf("MCP tool text output exceeds %d bytes", s.limits.MaxTextOutputBytes)
			}
			parts = append(parts, item.Text)
		}
	}
	if len(response.Structured) > s.limits.MaxStructuredBytes {
		return Result{}, fmt.Errorf("MCP tool structured output is %d bytes; limit is %d", len(response.Structured), s.limits.MaxStructuredBytes)
	}
	return Result{Output: strings.Join(parts, "\n"), StructuredOutput: append(json.RawMessage(nil), response.Structured...), IsError: response.IsError}, nil
}

func (s *StdioServer) ensureInitialized(ctx context.Context) error {
	if err := s.ensureStarted(); err != nil {
		return err
	}
	s.initMu.Lock()
	defer s.initMu.Unlock()
	if s.inited {
		return nil
	}
	var response struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	params := map[string]any{
		"protocolVersion": stdioProtocolVersion,
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "proton", "version": "dev"},
	}
	if err := s.request(ctx, "initialize", params, &response); err != nil {
		return fmt.Errorf("initialize MCP server %q: %w", s.name, err)
	}
	if strings.TrimSpace(response.ProtocolVersion) == "" {
		return fmt.Errorf("initialize MCP server %q: missing protocolVersion", s.name)
	}
	if err := s.notify("notifications/initialized", map[string]any{}); err != nil {
		return fmt.Errorf("initialize MCP server %q: %w", s.name, err)
	}
	s.inited = true
	return nil
}

func (s *StdioServer) ensureStarted() error {
	s.startMu.Lock()
	defer s.startMu.Unlock()
	if s.closed {
		return errors.New("MCP stdio server is closed")
	}
	if s.started {
		return nil
	}
	cmd := exec.Command(s.command, s.args...)
	cmd.Env = append(os.Environ(), s.env...)
	if strings.TrimSpace(s.cwd) != "" {
		cmd.Dir = s.cwd
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("open MCP stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return fmt.Errorf("open MCP stdout: %w", err)
	}
	cmd.Stderr = &s.stderr
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		return fmt.Errorf("start MCP server %q: %w", s.name, err)
	}
	s.cmd, s.stdin, s.started = cmd, stdin, true
	go s.readLoop(stdout)
	go s.waitLoop()
	return nil
}

func (s *StdioServer) request(ctx context.Context, method string, params any, out any) error {
	id := s.nextID.Add(1)
	ch := make(chan rpcReply, 1)
	s.pendingMu.Lock()
	s.pending[id] = ch
	s.pendingMu.Unlock()
	if err := s.writeEnvelope(rpcEnvelope{JSONRPC: "2.0", ID: json.RawMessage(fmt.Sprintf("%d", id)), Method: method, Params: mustRaw(params)}); err != nil {
		s.removePending(id)
		return err
	}
	select {
	case <-ctx.Done():
		s.removePending(id)
		return classifyFailure(FailureTransport, s.name, "", method, ctx.Err())
	case reply := <-ch:
		if reply.err != nil {
			return reply.err
		}
		if reply.rpcErr != nil {
			toolName := ""
			if method == "tools/call" {
				if values, ok := params.(map[string]any); ok {
					toolName, _ = values["name"].(string)
				}
			}
			return rpcFailure(s.name, toolName, method, reply.rpcErr)
		}
		if out == nil || len(reply.result) == 0 {
			return nil
		}
		if err := json.Unmarshal(reply.result, out); err != nil {
			return classifyFailure(FailureProtocol, s.name, "", method, fmt.Errorf("decode response: %w", err))
		}
		return nil
	}
}

func (s *StdioServer) notify(method string, params any) error {
	return s.writeEnvelope(rpcEnvelope{JSONRPC: "2.0", Method: method, Params: mustRaw(params)})
}

func (s *StdioServer) writeEnvelope(message rpcEnvelope) error {
	data, err := json.Marshal(message)
	if err != nil {
		return err
	}
	if len(data) > s.limits.MaxMessageBytes {
		return fmt.Errorf("MCP JSON-RPC message is %d bytes; limit is %d", len(data), s.limits.MaxMessageBytes)
	}
	data = append(data, '\n')
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if s.stdin == nil {
		return errors.New("MCP stdio stdin is not available")
	}
	_, err = s.stdin.Write(data)
	return err
}

func (s *StdioServer) readLoop(reader io.Reader) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), s.limits.MaxMessageBytes)
	for scanner.Scan() {
		var message rpcEnvelope
		if err := json.Unmarshal(scanner.Bytes(), &message); err != nil {
			s.failPending(classifyFailure(FailureProtocol, s.name, "", "read", fmt.Errorf("decode JSON-RPC message: %w", err)))
			continue
		}
		if len(message.ID) == 0 {
			if message.Method == "notifications/tools/list_changed" {
				s.notifyToolsChanged()
			}
			continue
		}
		var id uint64
		if _, err := fmt.Sscanf(string(message.ID), "%d", &id); err != nil {
			continue
		}
		s.pendingMu.Lock()
		ch := s.pending[id]
		delete(s.pending, id)
		s.pendingMu.Unlock()
		if ch != nil {
			ch <- rpcReply{result: append(json.RawMessage(nil), message.Result...), rpcErr: message.Error}
		}
	}
	if err := scanner.Err(); err != nil {
		s.failPending(classifyFailure(FailureTransport, s.name, "", "read", err))
	}
}

func (s *StdioServer) waitLoop() {
	err := s.cmd.Wait()
	s.startMu.Lock()
	s.waitErr = err
	select {
	case <-s.done:
	default:
		close(s.done)
	}
	s.startMu.Unlock()
	if err == nil {
		err = io.EOF
	}
	s.failPending(classifyFailure(FailureDisconnected, s.name, "", "wait", fmt.Errorf("%w; stderr: %s", err, strings.TrimSpace(s.stderr.String()))))
}

func (s *StdioServer) removePending(id uint64) {
	s.pendingMu.Lock()
	delete(s.pending, id)
	s.pendingMu.Unlock()
}

func (s *StdioServer) failPending(err error) {
	s.pendingMu.Lock()
	pending := s.pending
	s.pending = make(map[uint64]chan rpcReply)
	s.pendingMu.Unlock()
	for _, ch := range pending {
		select {
		case ch <- rpcReply{err: err}:
		default:
		}
	}
}

// Close stops the child process and releases its pipes. It is safe to call repeatedly.
func (s *StdioServer) Close() error {
	s.startMu.Lock()
	if s.closed {
		s.startMu.Unlock()
		return nil
	}
	s.closed = true
	stdin := s.stdin
	cmd := s.cmd
	started := s.started
	s.startMu.Unlock()
	if stdin != nil {
		_ = stdin.Close()
	}
	if !started || cmd == nil || cmd.Process == nil {
		return nil
	}
	select {
	case <-s.done:
		return nil
	default:
	}
	if err := cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return fmt.Errorf("stop MCP server %q: %w", s.name, err)
	}
	<-s.done
	return nil
}

func mustRaw(value any) json.RawMessage {
	data, _ := json.Marshal(value)
	return data
}

type limitedBuffer struct {
	mu    sync.Mutex
	limit int
	data  []byte
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	original := len(p)
	if remaining := b.limit - len(b.data); remaining > 0 {
		if len(p) > remaining {
			p = p[:remaining]
		}
		b.data = append(b.data, p...)
	}
	return original, nil
}

func (b *limitedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return string(append([]byte(nil), b.data...))
}

var _ Server = (*StdioServer)(nil)
var _ ToolListChangeSource = (*StdioServer)(nil)
