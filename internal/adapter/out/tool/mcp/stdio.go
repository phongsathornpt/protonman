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

const (
	stdioProtocolVersion = "2025-06-18"
	stdioMaxMessageBytes = 10 * 1024 * 1024
	stdioMaxStderrBytes  = 256 * 1024
)

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

	startMu sync.Mutex
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	started bool
	closed  bool
	initMu  sync.Mutex
	inited  bool

	writeMu   sync.Mutex
	pendingMu sync.Mutex
	pending   map[uint64]chan rpcReply
	nextID    atomic.Uint64
	done      chan struct{}
	waitErr   error
	stderr    limitedBuffer
}

// NewStdioServer creates a lazily-started MCP stdio transport.
func NewStdioServer(name, command string, args, env []string, cwd string) (*StdioServer, error) {
	if _, err := validServerName(name); err != nil {
		return nil, err
	}
	command = strings.TrimSpace(command)
	if command == "" {
		return nil, fmt.Errorf("create MCP stdio server %q: command is required", name)
	}
	return &StdioServer{
		name: name, command: command,
		args: append([]string(nil), args...), env: append([]string(nil), env...), cwd: cwd,
		pending: make(map[uint64]chan rpcReply), done: make(chan struct{}),
		stderr: limitedBuffer{limit: stdioMaxStderrBytes},
	}, nil
}

func (s *StdioServer) Name() string { return s.name }

func (s *StdioServer) ListTools(ctx context.Context) ([]Tool, error) {
	if err := s.ensureInitialized(ctx); err != nil {
		return nil, err
	}
	var response struct {
		Tools []struct {
			Name         string         `json:"name"`
			Description  string         `json:"description"`
			InputSchema  map[string]any `json:"inputSchema"`
			OutputSchema map[string]any `json:"outputSchema"`
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
		})
	}
	return tools, nil
}

func (s *StdioServer) CallTool(ctx context.Context, name string, arguments json.RawMessage) (Result, error) {
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
	for _, item := range response.Content {
		if item.Type == "text" && item.Text != "" {
			parts = append(parts, item.Text)
		}
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
		return ctx.Err()
	case reply := <-ch:
		if reply.err != nil {
			return reply.err
		}
		if reply.rpcErr != nil {
			return fmt.Errorf("MCP JSON-RPC error %d: %s", reply.rpcErr.Code, reply.rpcErr.Message)
		}
		if out == nil || len(reply.result) == 0 {
			return nil
		}
		if err := json.Unmarshal(reply.result, out); err != nil {
			return fmt.Errorf("decode MCP response for %s: %w", method, err)
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
	scanner.Buffer(make([]byte, 64*1024), stdioMaxMessageBytes)
	for scanner.Scan() {
		var message rpcEnvelope
		if err := json.Unmarshal(scanner.Bytes(), &message); err != nil {
			s.failPending(fmt.Errorf("decode MCP JSON-RPC message: %w", err))
			continue
		}
		if len(message.ID) == 0 {
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
		s.failPending(fmt.Errorf("read MCP stdio: %w", err))
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
	s.failPending(fmt.Errorf("MCP server %q exited: %w; stderr: %s", s.name, err, strings.TrimSpace(s.stderr.String())))
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
