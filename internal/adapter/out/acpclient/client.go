// Package acpclient provides a small full-duplex JSON-RPC client for a Protonman
// ACP subprocess. It intentionally owns only transport/lifecycle concerns; ACP
// domain DTOs stay with the protocol layer.
package acpclient

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"sync/atomic"
)

var ErrClosed = errors.New("ACP client is closed")

var ErrMethodNotHandled = errors.New("ACP reverse request method is not handled")

type RPCError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func (e *RPCError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("ACP error %d: %s", e.Code, e.Message)
}

type Event struct {
	ID     json.RawMessage
	Method string
	Params json.RawMessage
}

// Request is a server-to-client JSON-RPC request that requires a response.
type Request struct {
	ID     json.RawMessage
	Method string
	Params json.RawMessage
}

// RequestHandler handles server-to-client ACP requests such as permission prompts.
type RequestHandler func(context.Context, Request) (any, error)

type Client struct {
	ctx    context.Context
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	cancel context.CancelFunc

	writeMu sync.Mutex
	mu      sync.Mutex
	pending map[uint64]chan response
	handler RequestHandler
	nextID  atomic.Uint64
	onEvent func(Event)
	closed  chan struct{}
	once    sync.Once
}

type response struct {
	result json.RawMessage
	err    error
}

type envelope struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

func Start(ctx context.Context, binary string, onEvent func(Event)) (*Client, error) {
	if binary == "" {
		return nil, errors.New("ACP binary is required")
	}
	procCtx, cancel := context.WithCancel(ctx)
	cmd := exec.CommandContext(procCtx, binary, "--acp")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("open ACP stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("open ACP stdout: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("open ACP stderr: %w", err)
	}
	if err := cmd.Start(); err != nil {
		cancel()
		return nil, fmt.Errorf("start %s --acp: %w", binary, err)
	}

	client := &Client{
		ctx: procCtx, cmd: cmd, stdin: stdin, cancel: cancel, onEvent: onEvent,
		pending: make(map[uint64]chan response), closed: make(chan struct{}),
	}
	go client.readLoop(stdout)
	go io.Copy(io.Discard, stderr)
	go func() {
		err := cmd.Wait()
		client.shutdown(err)
	}()
	return client, nil
}

// SetRequestHandler installs the handler for server-to-client JSON-RPC requests.
// It may be replaced while the client is running.
func (c *Client) SetRequestHandler(handler RequestHandler) {
	c.mu.Lock()
	c.handler = handler
	c.mu.Unlock()
}

func (c *Client) Call(ctx context.Context, method string, params any, result any) error {
	if method == "" {
		return errors.New("ACP method is required")
	}
	id := c.nextID.Add(1)
	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return fmt.Errorf("encode %s params: %w", method, err)
	}
	request := envelope{JSONRPC: "2.0", ID: json.RawMessage(fmt.Sprintf("%d", id)), Method: method, Params: paramsJSON}
	ch := make(chan response, 1)

	c.mu.Lock()
	select {
	case <-c.closed:
		c.mu.Unlock()
		return ErrClosed
	default:
	}
	c.pending[id] = ch
	c.mu.Unlock()

	if err := c.write(request); err != nil {
		c.removePending(id)
		return err
	}

	select {
	case <-ctx.Done():
		c.removePending(id)
		return ctx.Err()
	case <-c.closed:
		return ErrClosed
	case res := <-ch:
		if res.err != nil {
			return res.err
		}
		if result == nil || len(res.result) == 0 || string(res.result) == "null" {
			return nil
		}
		if err := json.Unmarshal(res.result, result); err != nil {
			return fmt.Errorf("decode %s result: %w", method, err)
		}
		return nil
	}
}

func (c *Client) Close() error {
	c.shutdown(ErrClosed)
	if c.cmd.Process != nil {
		_ = c.cmd.Process.Kill()
	}
	return nil
}

func (c *Client) write(v any) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	select {
	case <-c.closed:
		return ErrClosed
	default:
	}
	payload, err := json.Marshal(v)
	if err != nil {
		return err
	}
	payload = append(payload, '\n')
	if _, err := c.stdin.Write(payload); err != nil {
		return fmt.Errorf("write ACP request: %w", err)
	}
	return nil
}

func (c *Client) readLoop(r io.Reader) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
	for scanner.Scan() {
		var msg envelope
		if err := json.Unmarshal(scanner.Bytes(), &msg); err != nil {
			continue
		}
		if msg.Method != "" {
			if len(msg.ID) > 0 && string(msg.ID) != "null" {
				request := Request{ID: append(json.RawMessage(nil), msg.ID...), Method: msg.Method, Params: append(json.RawMessage(nil), msg.Params...)}
				go c.handleRequest(request)
				continue
			}
			if c.onEvent != nil {
				c.onEvent(Event{Method: msg.Method, Params: append(json.RawMessage(nil), msg.Params...)})
			}
			continue
		}
		var id uint64
		if len(msg.ID) == 0 || json.Unmarshal(msg.ID, &id) != nil {
			continue
		}
		c.mu.Lock()
		ch := c.pending[id]
		delete(c.pending, id)
		c.mu.Unlock()
		if ch == nil {
			continue
		}
		if msg.Error != nil {
			ch <- response{err: msg.Error}
		} else {
			ch <- response{result: msg.Result}
		}
	}
	if err := scanner.Err(); err != nil {
		c.shutdown(fmt.Errorf("read ACP stream: %w", err))
	} else {
		c.shutdown(io.EOF)
	}
}

func (c *Client) handleRequest(request Request) {
	c.mu.Lock()
	handler := c.handler
	c.mu.Unlock()

	if handler == nil {
		_ = c.write(envelope{
			JSONRPC: "2.0",
			ID:      request.ID,
			Error:   &RPCError{Code: -32601, Message: ErrMethodNotHandled.Error() + ": " + request.Method},
		})
		return
	}

	result, err := handler(c.ctx, request)
	if err != nil {
		_ = c.write(envelope{
			JSONRPC: "2.0",
			ID:      request.ID,
			Error:   &RPCError{Code: -32000, Message: err.Error()},
		})
		return
	}

	resultJSON, err := json.Marshal(result)
	if err != nil {
		_ = c.write(envelope{
			JSONRPC: "2.0",
			ID:      request.ID,
			Error:   &RPCError{Code: -32603, Message: "encode reverse request result: " + err.Error()},
		})
		return
	}
	_ = c.write(envelope{JSONRPC: "2.0", ID: request.ID, Result: resultJSON})
}

func (c *Client) removePending(id uint64) {
	c.mu.Lock()
	delete(c.pending, id)
	c.mu.Unlock()
}

func (c *Client) shutdown(cause error) {
	c.once.Do(func() {
		c.cancel()
		_ = c.stdin.Close()
		close(c.closed)
		if cause == nil {
			cause = ErrClosed
		}
		c.mu.Lock()
		pending := c.pending
		c.pending = make(map[uint64]chan response)
		c.mu.Unlock()
		for _, ch := range pending {
			ch <- response{err: cause}
		}
	})
}
