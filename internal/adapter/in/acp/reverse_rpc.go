package acp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"sync"
)

type reverseRPCResponse struct {
	result json.RawMessage
	err    *RPCError
}

type reverseRPCState struct {
	mu      sync.Mutex
	nextID  uint64
	output  io.Writer
	pending map[string]chan reverseRPCResponse
}

var reverseRPCStates sync.Map // map[*Server]*reverseRPCState

func reverseRPCStateFor(server *Server) *reverseRPCState {
	if server == nil {
		return nil
	}
	state := &reverseRPCState{pending: make(map[string]chan reverseRPCResponse)}
	actual, _ := reverseRPCStates.LoadOrStore(server, state)
	return actual.(*reverseRPCState)
}

func (s *Server) bindReverseRPCOutput(output io.Writer) func() {
	state := reverseRPCStateFor(s)
	if state == nil {
		return func() {}
	}
	state.mu.Lock()
	previous := state.output
	state.output = output
	state.mu.Unlock()
	return func() {
		state.mu.Lock()
		if state.output == output {
			state.output = previous
		}
		state.mu.Unlock()
	}
}

func (s *Server) requestClient(ctx context.Context, method string, params any, result any) error {
	if s == nil {
		return fmt.Errorf("ACP server is required")
	}
	if method == "" {
		return fmt.Errorf("ACP client method is required")
	}
	state := reverseRPCStateFor(s)
	state.mu.Lock()
	if state.output == nil {
		state.mu.Unlock()
		return fmt.Errorf("ACP client transport is unavailable")
	}
	state.nextID++
	id := state.nextID
	idRaw := json.RawMessage(strconv.FormatUint(id, 10))
	key := string(idRaw)
	ch := make(chan reverseRPCResponse, 1)
	state.pending[key] = ch
	output := state.output
	state.mu.Unlock()

	paramsJSON, err := json.Marshal(params)
	if err != nil {
		s.removeReverseRPCPending(key)
		return fmt.Errorf("encode %s params: %w", method, err)
	}
	request := RPCRequest{JSONRPC: "2.0", ID: idRaw, Method: method, Params: paramsJSON}
	if err := WriteJSON(output, &s.writeMu, request); err != nil {
		s.removeReverseRPCPending(key)
		return err
	}

	select {
	case <-ctx.Done():
		s.removeReverseRPCPending(key)
		_ = WriteJSON(output, &s.writeMu, RPCNotification{
			JSONRPC: "2.0",
			Method:  methodCancelRequest,
			Params:  CancelRequestParams{RequestID: idRaw},
		})
		return ctx.Err()
	case response := <-ch:
		if response.err != nil {
			return fmt.Errorf("ACP client error %d: %s", response.err.Code, response.err.Message)
		}
		if result == nil || len(response.result) == 0 || string(response.result) == "null" {
			return nil
		}
		if err := json.Unmarshal(response.result, result); err != nil {
			return fmt.Errorf("decode %s result: %w", method, err)
		}
		return nil
	}
}

func (s *Server) removeReverseRPCPending(key string) {
	state := reverseRPCStateFor(s)
	if state == nil {
		return
	}
	state.mu.Lock()
	delete(state.pending, key)
	state.mu.Unlock()
}

func (s *Server) handleClientResponseLine(line []byte) bool {
	var response struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id,omitempty"`
		Method  string          `json:"method,omitempty"`
		Result  json.RawMessage `json:"result,omitempty"`
		Error   *RPCError       `json:"error,omitempty"`
	}
	if err := json.Unmarshal(line, &response); err != nil || response.Method != "" || len(response.ID) == 0 {
		return false
	}
	if len(response.Result) == 0 && response.Error == nil {
		return false
	}
	key, err := requestIDKey(response.ID)
	if err != nil {
		return true
	}
	state := reverseRPCStateFor(s)
	state.mu.Lock()
	ch := state.pending[key]
	delete(state.pending, key)
	state.mu.Unlock()
	if ch != nil {
		ch <- reverseRPCResponse{result: append(json.RawMessage(nil), response.Result...), err: response.Error}
	}
	return true
}
