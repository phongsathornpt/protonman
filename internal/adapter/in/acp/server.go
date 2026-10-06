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
	"github.com/phongsathornpt/protonman/internal/feature/skill"
)

var ErrInvalidServer = errors.New("invalid ACP server")
var ErrInvalidRequest = errors.New("invalid request")
var ErrMethodNotSupported = errors.New("method is not supported")

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
	skillRegistry          *skill.Registry
	skillSaver             SkillSaver
	mu                     sync.Mutex
	writeMu                sync.Mutex
	sessions               map[string]*Session
	sessionDirectories     map[string][]string
	permissionSeq          uint64
	// output is the transport used for server-initiated requests such as
	// session/request_permission. It is set for the duration of Serve.
	output io.Writer
	// permissions routes client answers to blocked tool calls. It is non-nil
	// only while Serve is running.
	permissions *permissionBroker
	// questions routes client answers to interactive clarifying questions. It is non-nil
	// only while Serve is running.
	questions          *questionBroker
	clientCapabilities ClientCapabilities
}

func (s *Server) supportsFormElicitation() bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.clientCapabilities.Elicitation != nil && s.clientCapabilities.Elicitation.Form != nil
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

	// Interactive permission needs a reverse channel: the loop below only reads
	// client-to-server traffic, so tool calls blocked in ask mode are resolved
	// by matching inbound responses to the requests this server emitted.
	broker := newPermissionBroker(s)
	qBroker := newQuestionBroker(s)
	s.mu.Lock()
	s.output = output
	s.permissions = broker
	s.questions = qBroker
	s.mu.Unlock()
	defer func() {
		broker.close()
		qBroker.close()
		s.mu.Lock()
		s.output = nil
		s.permissions = nil
		s.questions = nil
		s.mu.Unlock()
	}()

	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
	serveCtx, cancelServe := context.WithCancel(ctx)
	defer cancelServe()
	dispatcher := newRequestDispatcher(serveCtx, s, output)
	// stop unwinds every admitted request before Serve returns, so no handler
	// outlives the output/broker state torn down by the deferred cleanup above.
	stop := func(err error) error {
		cancelServe()
		dispatcher.wait()
		return err
	}
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return stop(err)
		}
		if err := dispatcher.failed(); err != nil {
			return stop(err)
		}
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		// A payload with an id but no method is a response to a request this
		// server sent, such as session/request_permission. It must be consumed
		// before decodeRequest, which rejects method-less frames.
		if handled := s.handleServerResponse(broker, line); handled {
			continue
		}
		request, parseErr := decodeRequest(line)
		if parseErr != nil {
			if err := WriteJSON(output, &s.writeMu, parseErr); err != nil {
				return stop(err)
			}
			continue
		}
		if request.Method == methodCancelRequest {
			dispatcher.cancelRequest(request.Params)
			continue
		}
		if request.Method != "session/prompt" {
			if err := dispatcher.submit(request); err != nil {
				return stop(err)
			}
			continue
		}
		var params SessionPromptParams
		if err := json.Unmarshal(request.Params, &params); err != nil {
			if err := s.writeResponse(output, request.ID, nil, nil, fmt.Errorf("%w: %v", ErrInvalidRequest, err)); err != nil {
				return stop(err)
			}
			continue
		}
		params.SessionID = strings.TrimSpace(params.SessionID)
		if params.SessionID == "" {
			if err := s.writeResponse(output, request.ID, nil, nil, errors.New("sessionId is required")); err != nil {
				return stop(err)
			}
			continue
		}
		sess, ok := s.lookupSession(params.SessionID)
		if !ok {
			if err := s.writeResponse(output, request.ID, nil, nil, fmt.Errorf("unknown session %q", params.SessionID)); err != nil {
				return stop(err)
			}
			continue
		}
		if err := dispatcher.submitPrompt(request, sess, params.Prompt); err != nil {
			return stop(err)
		}
	}
	scanErr := scanner.Err()
	// Input ended: requests already admitted run to completion so a piped
	// script still receives every response.
	dispatcher.wait()
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := dispatcher.failed(); err != nil {
		return err
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

// route resolves one request to its result, optional trailing notification, and
// error without writing anything, so callers decide how the outcome is framed.
func (s *Server) route(ctx context.Context, request RPCRequest, output io.Writer) (any, *RPCNotification, error) {
	if result, notify, handled, err := s.dispatchSessionConfig(ctx, request); handled {
		return result, notify, err
	}
	if result, handled, err := s.dispatchSessionRuntime(ctx, request); handled {
		return result, nil, err
	}
	if result, handled, err := s.dispatchProviders(ctx, request); handled {
		return result, nil, err
	}
	return s.dispatch(ctx, request, output)
}

func (s *Server) handleRequest(ctx context.Context, request RPCRequest, output io.Writer) error {
	result, notify, err := s.route(ctx, request, output)
	return s.writeResponse(output, request.ID, result, notify, err)
}

func (s *Server) writeResponse(output io.Writer, id json.RawMessage, result any, notify *RPCNotification, requestErr error) error {
	if requestErr != nil {
		if len(id) == 0 || string(id) == "null" {
			return nil
		}
		if errors.Is(requestErr, ErrMethodNotSupported) {
			return WriteJSON(output, &s.writeMu, RPCResponse{JSONRPC: "2.0", ID: id, Error: &RPCError{Code: CodeMethodNotFound, Message: requestErr.Error()}})
		}
		if errors.Is(requestErr, errRequestCancelled) {
			return WriteJSON(output, &s.writeMu, RPCResponse{JSONRPC: "2.0", ID: id, Error: &RPCError{Code: CodeRequestCancelled, Message: "Request cancelled"}})
		}
		var invalid invalidRequestError
		if errors.As(requestErr, &invalid) {
			return WriteJSON(output, &s.writeMu, RPCResponse{JSONRPC: "2.0", ID: id, Error: &RPCError{Code: CodeInvalidRequest, Message: invalid.Error()}})
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
