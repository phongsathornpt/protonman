package acp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
)

const methodCancelRequest = "$/cancel_request"

type CancelRequestParams struct {
	MetaCarrier
	RequestID json.RawMessage `json:"requestId"`
}

func requestIDKey(raw json.RawMessage) (string, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return "", fmt.Errorf("requestId is required")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return "", fmt.Errorf("decode requestId: %w", err)
	}
	switch typed := value.(type) {
	case string:
	case json.Number:
		if _, err := typed.Int64(); err != nil {
			return "", fmt.Errorf("requestId must be an integer: %w", err)
		}
	default:
		return "", fmt.Errorf("requestId must be a string or integer")
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, raw); err != nil {
		return "", fmt.Errorf("compact requestId: %w", err)
	}
	return compact.String(), nil
}

func (s *Server) registerRequestCancellation(id json.RawMessage, cancel context.CancelFunc) func() {
	if s == nil || cancel == nil {
		return func() {}
	}
	key, err := requestIDKey(id)
	if err != nil {
		return func() {}
	}
	s.mu.Lock()
	if s.requestCancels == nil {
		s.requestCancels = make(map[string]context.CancelFunc)
	}
	s.requestCancels[key] = cancel
	s.mu.Unlock()
	return func() {
		s.mu.Lock()
		delete(s.requestCancels, key)
		s.mu.Unlock()
	}
}

func (s *Server) handleCancelRequestNotification(request RPCRequest) error {
	if s == nil {
		return nil
	}
	var params CancelRequestParams
	if err := json.Unmarshal(request.Params, &params); err != nil {
		return fmt.Errorf("decode %s: %w", methodCancelRequest, err)
	}
	key, err := requestIDKey(params.RequestID)
	if err != nil {
		return err
	}
	s.mu.Lock()
	cancel := s.requestCancels[key]
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	return nil
}
