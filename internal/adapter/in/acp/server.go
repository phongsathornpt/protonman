// Package acp implements the Agent Client Protocol (ACP) v1 specification.
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

	"github.com/phongsathornpt/protonman/internal/app"
	"github.com/phongsathornpt/protonman/internal/core/tool"
	"github.com/phongsathornpt/protonman/internal/engine/toolcall"
)

var ErrInvalidServer = errors.New("invalid ACP server")
var ErrInvalidRequest = errors.New("invalid request")

type Option func(*Server)
type RunnerFactory func(*toolcall.Service) (app.Conversation, error)
type SessionRegistryFactory func(sessionID string, cwd string, additionalDirectories []string) (tool.Registry, error)
type MCPRegistryConfigurer func(ctx context.Context, cwd string, registry tool.Registry, servers []MCPServerConfig) (io.Closer, error)

func WithSessions(sessions *app.Sessions) Option {
	return func(server *Server) { server.sessionService = sessions }
}
func WithMemories(memories *app.Memories) Option {
	return func(server *Server) { server.memories = memories }
}
func WithAgents(agents app.Agents) Option { return func(server *Server) { server.agents = agents } }
func WithRunnerFactory(factory RunnerFactory) Option {
	return func(s *Server) { s.runnerFactory = factory }
}
func WithSessionRegistryFactory(factory SessionRegistryFactory) Option {
	return func(s *Server) { s.sessionRegistryFactory = factory }
}
func WithMCPRegistryConfigurer(configurer MCPRegistryConfigurer) Option {
	return func(s *Server) { s.mcpRegistryConfigurer = configurer }
}

type Server struct {
	service                *toolcall.Service
	registry               tool.Registry
	runnerFactory          RunnerFactory
	sessionRegistryFactory SessionRegistryFactory
	mcpRegistryConfigurer  MCPRegistryConfigurer
	sessionService         *app.Sessions
	memories               *app.Memories
	agents                 app.Agents
	mu                     sync.Mutex
	writeMu                sync.Mutex
	sessions               map[string]*Session
	sessionDirectories     map[string][]string
	nextID                 uint64
}

func New(service *toolcall.Service, registry tool.Registry, runner app.Conversation, opts ...Option) (*Server, error) {
	if service == nil {
		return nil, fmt.Errorf("%w: service is required", ErrInvalidServer)
	}
	if registry == nil {
		return nil, fmt.Errorf("%w: registry is required", ErrInvalidServer)
	}
	s := &Server{
		service:            service,
		registry:           registry,
		sessions:           make(map[string]*Session),
		sessionDirectories: make(map[string][]string),
	}
	for _, opt := range opts {
		if opt != nil {
			opt(s)
		}
	}
	if s.runnerFactory == nil && runner != nil {
		s.runnerFactory = func(service *toolcall.Service) (app.Conversation, error) {
			return app.CloneConversationWithTools(runner, service)
		}
		if _, err := s.runnerFactory(service); err != nil {
			return nil, fmt.Errorf("%w: runner factory is required for a configured custom runner: %v", ErrInvalidServer, err)
		}
	}
	return s, nil
}

func (s *Server) Serve(ctx context.Context, input io.Reader, output io.Writer) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("start ACP server: %w", err)
	}
	if input == nil || output == nil {
		return fmt.Errorf("%w: input and output are required", ErrInvalidServer)
	}
	stopInputWatch := watchInputCancellation(ctx, input)
	defer stopInputWatch()
	defer s.closeSessions()
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
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
		request, parseErr := decodeRequest(line)
		if parseErr != nil {
			if err := WriteJSON(output, &s.writeMu, parseErr); err != nil {
				return err
			}
			continue
		}
		if request.Method != "session/prompt" {
			if err := s.handleRequest(ctx, request, output); err != nil {
				return err
			}
			continue
		}
		var params SessionPromptParams
		if err := json.Unmarshal(request.Params, &params); err != nil {
			if err := s.writeResponse(output, request.ID, nil, nil, fmt.Errorf("%w: %v", ErrInvalidRequest, err)); err != nil {
				return err
			}
			continue
		}
		params.SessionID = strings.TrimSpace(params.SessionID)
		if params.SessionID == "" {
			if err := s.writeResponse(output, request.ID, nil, nil, errors.New("sessionId is required")); err != nil {
				return err
			}
			continue
		}
		sess, ok := s.lookupSession(params.SessionID)
		if !ok {
			if err := s.writeResponse(output, request.ID, nil, nil, fmt.Errorf("unknown session %q", params.SessionID)); err != nil {
				return err
			}
			continue
		}
		prompts.Add(1)
		go func(req RPCRequest, sess *Session, blocks []ContentBlock) {
			defer prompts.Done()
			notifier := func(notification RPCNotification) error { return WriteJSON(output, &s.writeMu, notification) }
			result, promptErr := sess.ExecutePrompt(ctx, blocks, notifier)
			if err := s.writeSessionInfoNotification(output, sess); err != nil {
				select {
				case asyncErrors <- err:
				default:
				}
				return
			}
			if err := s.writeSessionUsageNotification(output, sess); err != nil {
				select {
				case asyncErrors <- err:
				default:
				}
				return
			}
			if err := s.writeResponse(output, req.ID, result, nil, promptErr); err != nil {
				select {
				case asyncErrors <- err:
				default:
				}
			}
		}(request, sess, params.Prompt)
	}
	scanErr := scanner.Err()
	prompts.Wait()
	if err := ctx.Err(); err != nil {
		return err
	}
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

func watchInputCancellation(ctx context.Context, input io.Reader) func() {
	closer, ok := input.(io.Closer)
	if !ok {
		return func() {}
	}
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = closer.Close()
		case <-done:
		}
	}()
	return func() { close(done) }
}

func decodeRequest(line []byte) (RPCRequest, *RPCResponse) {
	var request RPCRequest
	if err := json.Unmarshal(line, &request); err != nil {
		return RPCRequest{}, &RPCResponse{JSONRPC: "2.0", Error: &RPCError{Code: CodeParseError, Message: "parse error"}}
	}
	if request.Method == "" {
		return RPCRequest{}, &RPCResponse{JSONRPC: "2.0", ID: request.ID, Error: &RPCError{Code: CodeInvalidRequest, Message: "invalid request: method required"}}
	}
	return request, nil
}

func (s *Server) handleRequest(ctx context.Context, request RPCRequest, output io.Writer) error {
	if result, handled, err := s.dispatchSessionRuntime(ctx, request); handled {
		return s.writeResponse(output, request.ID, result, nil, err)
	}
	result, notify, err := s.dispatch(ctx, request, output)
	return s.writeResponse(output, request.ID, result, notify, err)
}

func (s *Server) writeResponse(output io.Writer, id json.RawMessage, result any, notify *RPCNotification, requestErr error) error {
	if requestErr != nil {
		if len(id) == 0 || string(id) == "null" {
			return nil
		}
		return WriteJSON(output, &s.writeMu, RPCResponse{JSONRPC: "2.0", ID: id, Error: &RPCError{Code: CodeServerError, Message: requestErr.Error()}})
	}
	if notify != nil {
		if err := WriteJSON(output, &s.writeMu, notify); err != nil {
			return err
		}
	}
	if len(id) == 0 || string(id) == "null" {
		return nil
	}
	return WriteJSON(output, &s.writeMu, RPCResponse{JSONRPC: "2.0", ID: id, Result: result})
}
