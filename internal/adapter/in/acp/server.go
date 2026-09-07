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

	"github.com/projectTHORN/proton/internal/app"
	"github.com/projectTHORN/proton/internal/base/buildinfo"
	"github.com/projectTHORN/proton/internal/core/permission"
	"github.com/projectTHORN/proton/internal/core/session"
	"github.com/projectTHORN/proton/internal/core/tool"
	"github.com/projectTHORN/proton/internal/engine/toolcall"
	sdk "github.com/projectTHORN/proton/proton-sdk"
)

// ErrInvalidServer indicates that the ACP server cannot start.
var ErrInvalidServer = errors.New("invalid ACP server")

// ErrInvalidRequest indicates malformed JSON-RPC or parameters.
var ErrInvalidRequest = errors.New("invalid request")

// Option configures an ACP Server.
type Option func(*Server)

// RunnerFactory creates a model/tool runner bound to one session's service.
type RunnerFactory func(*toolcall.Service) (app.Conversation, error)

// SessionRegistryFactory creates stateful tool bindings for one ACP session.
type SessionRegistryFactory func(sessionID string, cwd string) (tool.Registry, error)

// MCPRegistryConfigurer attaches client-provided MCP servers to a session-local registry.
type MCPRegistryConfigurer func(ctx context.Context, registry tool.Registry, servers []MCPServerConfig) error

// WithSessions sets the application session service for persistence use cases.
func WithSessions(sessions *app.Sessions) Option {
	return func(server *Server) { server.sessionService = sessions }
}

// WithRunnerFactory supplies isolated runners for ACP sessions. The factory
// receives the session-local tool-call service so model-driven calls do not
// share permission state with other sessions.
func WithRunnerFactory(factory RunnerFactory) Option {
	return func(s *Server) {
		s.runnerFactory = factory
	}
}

// WithSessionRegistryFactory binds stateful tools such as todo state to each session.
func WithSessionRegistryFactory(factory SessionRegistryFactory) Option {
	return func(s *Server) { s.sessionRegistryFactory = factory }
}

// WithMCPRegistryConfigurer wires ACP mcpServers into each session-local tool registry.
func WithMCPRegistryConfigurer(configurer MCPRegistryConfigurer) Option {
	return func(s *Server) { s.mcpRegistryConfigurer = configurer }
}

// Server is a full-duplex JSON-RPC 2.0 ACP agent server.
type Server struct {
	service                *toolcall.Service
	registry               tool.Registry
	runnerFactory          RunnerFactory
	sessionRegistryFactory SessionRegistryFactory
	mcpRegistryConfigurer  MCPRegistryConfigurer
	sessionService         *app.Sessions

	mu       sync.Mutex
	writeMu  sync.Mutex
	sessions map[string]*Session
	nextID   uint64
}

// New creates an ACP server over Proton's toolcall service and model runner.
func New(service *toolcall.Service, registry tool.Registry, runner app.Conversation, opts ...Option) (*Server, error) {
	if service == nil {
		return nil, fmt.Errorf("%w: service is required", ErrInvalidServer)
	}
	if registry == nil {
		return nil, fmt.Errorf("%w: registry is required", ErrInvalidServer)
	}
	s := &Server{
		service:  service,
		registry: registry,
		sessions: make(map[string]*Session),
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

// Serve handles incoming JSON-RPC requests from input and writes responses to output.
func (s *Server) Serve(ctx context.Context, input io.Reader, output io.Writer) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("start ACP server: %w", err)
	}
	if input == nil || output == nil {
		return fmt.Errorf("%w: input and output are required", ErrInvalidServer)
	}

	stopInputWatch := watchInputCancellation(ctx, input)
	defer stopInputWatch()

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

		// Asynchronous prompt execution
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
			notifier := func(notification RPCNotification) error {
				return WriteJSON(output, &s.writeMu, notification)
			}
			result, promptErr := sess.ExecutePrompt(ctx, blocks, notifier)
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
		return RPCRequest{}, &RPCResponse{
			JSONRPC: "2.0",
			Error:   &RPCError{Code: CodeParseError, Message: "parse error"},
		}
	}
	if request.Method == "" {
		return RPCRequest{}, &RPCResponse{
			JSONRPC: "2.0",
			ID:      request.ID,
			Error:   &RPCError{Code: CodeInvalidRequest, Message: "invalid request: method required"},
		}
	}
	return request, nil
}

func (s *Server) handleRequest(ctx context.Context, request RPCRequest, output io.Writer) error {
	result, notify, err := s.dispatch(ctx, request, output)
	return s.writeResponse(output, request.ID, result, notify, err)
}

func (s *Server) writeResponse(output io.Writer, id json.RawMessage, result any, notify *RPCNotification, requestErr error) error {
	if requestErr != nil {
		if len(id) == 0 || string(id) == "null" {
			return nil
		}
		return WriteJSON(output, &s.writeMu, RPCResponse{
			JSONRPC: "2.0",
			ID:      id,
			Error:   &RPCError{Code: CodeServerError, Message: requestErr.Error()},
		})
	}
	if notify != nil {
		if err := WriteJSON(output, &s.writeMu, notify); err != nil {
			return err
		}
	}
	if len(id) == 0 || string(id) == "null" {
		return nil
	}
	return WriteJSON(output, &s.writeMu, RPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	})
}

func (s *Server) dispatch(ctx context.Context, request RPCRequest, output io.Writer) (any, *RPCNotification, error) {
	switch request.Method {
	case "initialize":
		res := InitializeResult{
			ProtocolVersion: ProtocolVersion,
			AgentCapabilities: AgentCapabilities{
				LoadSession: true,
				PromptCapabilities: PromptCapabilities{
					Image:           true,
					Audio:           false,
					EmbeddedContext: true,
				},
				SessionCapabilities: SessionCapabilities{
					Resume:                &struct{}{},
					Delete:                &struct{}{},
					AdditionalDirectories: &struct{}{},
				},
				MCPCapabilities: MCPCapabilities{
					HTTP: true,
					SSE:  false,
				},
			},
			AgentInfo: ImplementationInfo{
				Name:    "proton",
				Title:   "Proton AI Coding Agent",
				Version: buildinfo.Version(),
			},
			AuthMethods: []any{},
		}
		return res, nil, nil

	case "session/new":
		var params SessionNewParams
		if len(request.Params) > 0 {
			if err := json.Unmarshal(request.Params, &params); err != nil {
				return nil, nil, fmt.Errorf("decode session/new: %w", err)
			}
		}
		if err := validateMCPServerConfigs(params.MCPServers); err != nil {
			return nil, nil, fmt.Errorf("session/new MCP servers: %w", err)
		}
		sessionID := session.NewID(params.Cwd)
		sess, err := s.newSession(ctx, sessionID, params.Cwd, params.MCPServers)
		if err != nil {
			return nil, nil, err
		}
		s.mu.Lock()
		s.sessions[sessionID] = sess
		s.mu.Unlock()

		notify := &RPCNotification{
			JSONRPC: "2.0",
			Method:  "session/update",
			Params: map[string]any{
				"sessionId": sessionID,
				"update": map[string]any{
					"sessionUpdate":     "available_commands_update",
					"availableCommands": DefaultAvailableCommands(),
				},
			},
		}

		currentMode := sess.service.Mode().String()
		return SessionNewResult{
			SessionID: sessionID,
			Modes:     DefaultSessionModes(currentMode),
		}, notify, nil

	case "session/load":
		var params SessionLoadParams
		if err := json.Unmarshal(request.Params, &params); err != nil {
			return nil, nil, fmt.Errorf("decode session/load: %w", err)
		}
		params.SessionID = strings.TrimSpace(params.SessionID)
		if params.SessionID == "" {
			return nil, nil, errors.New("sessionId is required")
		}

		if err := validateMCPServerConfigs(params.MCPServers); err != nil {
			return nil, nil, fmt.Errorf("session/load MCP servers: %w", err)
		}
		sess, err := s.loadOrCreateSession(ctx, params.SessionID, params.Cwd, params.MCPServers)
		if err != nil {
			return nil, nil, err
		}

		// Replay past messages to client
		if err := sess.ReplayHistory(func(notification RPCNotification) error {
			return WriteJSON(output, &s.writeMu, notification)
		}); err != nil {
			return nil, nil, err
		}
		return nil, nil, nil

	case "session/resume":
		var params SessionResumeParams
		if err := json.Unmarshal(request.Params, &params); err != nil {
			return nil, nil, fmt.Errorf("decode session/resume: %w", err)
		}
		params.SessionID = strings.TrimSpace(params.SessionID)
		if params.SessionID == "" {
			return nil, nil, errors.New("sessionId is required")
		}
		if err := validateMCPServerConfigs(params.MCPServers); err != nil {
			return nil, nil, fmt.Errorf("session/resume MCP servers: %w", err)
		}
		_, err := s.loadOrCreateSession(ctx, params.SessionID, params.Cwd, params.MCPServers)
		if err != nil {
			return nil, nil, err
		}
		return nil, nil, nil

	case "session/set_mode":
		var params SessionSetModeParams
		if err := json.Unmarshal(request.Params, &params); err != nil {
			return nil, nil, fmt.Errorf("decode session/set_mode: %w", err)
		}
		params.SessionID = strings.TrimSpace(params.SessionID)
		if params.SessionID == "" {
			return nil, nil, errors.New("sessionId is required")
		}
		sess, ok := s.lookupSession(params.SessionID)
		if !ok {
			return nil, nil, fmt.Errorf("unknown session %q", params.SessionID)
		}
		mode, err := permission.ParseMode(params.ModeID)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid mode %q: %w", params.ModeID, err)
		}
		if err := sess.service.SetMode(mode); err != nil {
			return nil, nil, fmt.Errorf("set session mode: %w", err)
		}
		if err := sess.saveStateDetached(ctx); err != nil {
			return nil, nil, fmt.Errorf("save session %q: %w", params.SessionID, err)
		}

		notify := &RPCNotification{
			JSONRPC: "2.0",
			Method:  "session/update",
			Params: map[string]any{
				"sessionId": params.SessionID,
				"update": map[string]any{
					"sessionUpdate": "current_mode_update",
					"modeId":        mode.String(),
				},
			},
		}
		return nil, notify, nil

	case "session/cancel":
		var params SessionCancelParams
		if err := json.Unmarshal(request.Params, &params); err != nil {
			return nil, nil, fmt.Errorf("decode session/cancel: %w", err)
		}
		params.SessionID = strings.TrimSpace(params.SessionID)
		if params.SessionID == "" {
			return nil, nil, errors.New("sessionId is required")
		}
		sess, ok := s.lookupSession(params.SessionID)
		if !ok {
			return nil, nil, fmt.Errorf("unknown session %q", params.SessionID)
		}
		sess.Cancel()
		return map[string]any{}, nil, nil

	case "session/list":
		var params SessionListParams
		if len(request.Params) > 0 {
			_ = json.Unmarshal(request.Params, &params)
		}
		sessions, err := s.listSessions(ctx, params.Cwd)
		if err != nil {
			return nil, nil, err
		}
		return SessionListResult{Sessions: sessions}, nil, nil

	case "session/delete":
		var params SessionDeleteParams
		if err := json.Unmarshal(request.Params, &params); err != nil {
			return nil, nil, fmt.Errorf("decode session/delete: %w", err)
		}
		params.SessionID = strings.TrimSpace(params.SessionID)
		if params.SessionID == "" {
			return nil, nil, errors.New("sessionId is required")
		}
		if err := s.deleteSession(ctx, params.SessionID); err != nil {
			return nil, nil, err
		}
		return nil, nil, nil

	default:
		return nil, nil, fmt.Errorf("method %q is not supported", request.Method)
	}
}

func (s *Server) lookupSession(sessionID string) (*Session, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[sessionID]
	return sess, ok
}

func (s *Server) loadOrCreateSession(ctx context.Context, sessionID string, cwd string, mcpServers []MCPServerConfig) (*Session, error) {
	s.mu.Lock()
	existing, ok := s.sessions[sessionID]
	s.mu.Unlock()
	if ok {
		if err := existing.matchMCPServers(mcpServers); err != nil {
			return nil, err
		}
		return existing, nil
	}

	sess, err := s.newSession(ctx, sessionID, cwd, mcpServers)
	if err != nil {
		return nil, err
	}
	if s.sessionService != nil {
		state, found, err := s.sessionService.Load(ctx, sessionID)
		if err != nil {
			return nil, fmt.Errorf("load session state %q: %w", sessionID, err)
		}
		if found {
			if cwd != "" && state.WorkspaceKey != "" && state.WorkspaceKey != session.WorkspaceKey(cwd) {
				return nil, fmt.Errorf("session %q belongs to another workspace", sessionID)
			}
			if state.WorkspaceKey != "" {
				sess.workspaceKey = state.WorkspaceKey
			}
			if state.WorkspaceName != "" {
				sess.workspaceName = state.WorkspaceName
			}
			sess.stateRevision = state.Revision
			sess.SetMessages(session.ToModelMessages(state.Messages))
			mode, err := permission.ParseMode(state.PermissionMode)
			if err != nil {
				return nil, fmt.Errorf("session permission mode %q: %w", sessionID, err)
			}
			if err := sess.service.SetMode(mode); err != nil {
				return nil, fmt.Errorf("restore session mode %q: %w", sessionID, err)
			}
			if strings.TrimSpace(state.ReasoningEffort) != "" {
				effort, parseErr := sdk.ParseReasoningEffort(state.ReasoningEffort)
				if parseErr != nil {
					return nil, fmt.Errorf("restore session reasoning %q: %w", sessionID, parseErr)
				}
				if err := sess.SetReasoningEffort(effort); err != nil {
					return nil, fmt.Errorf("restore session reasoning %q: %w", sessionID, err)
				}
			}
		}
	}

	s.mu.Lock()
	s.sessions[sessionID] = sess
	s.mu.Unlock()
	return sess, nil
}

func (s *Server) newSession(ctx context.Context, sessionID string, cwd string, mcpServers []MCPServerConfig) (*Session, error) {
	registry := s.registry
	if s.sessionRegistryFactory != nil {
		created, err := s.sessionRegistryFactory(sessionID, cwd)
		if err != nil {
			return nil, fmt.Errorf("create registry for session %q: %w", sessionID, err)
		}
		registry = created
	}
	if len(mcpServers) > 0 {
		if s.mcpRegistryConfigurer == nil {
			return nil, fmt.Errorf("configure MCP servers for session %q: MCP server configuration is not available", sessionID)
		}
		if err := s.mcpRegistryConfigurer(ctx, registry, cloneMCPServerConfigs(mcpServers)); err != nil {
			return nil, fmt.Errorf("configure MCP servers for session %q: %w", sessionID, err)
		}
	}
	service, err := s.service.CloneWithRegistry(registry)
	if err != nil {
		return nil, fmt.Errorf("%w: clone session tool-call service: %v", ErrInvalidServer, err)
	}
	var runner app.Conversation
	if s.runnerFactory != nil {
		created, err := s.runnerFactory(service)
		if err != nil {
			return nil, fmt.Errorf("create runner for session %q: %w", sessionID, err)
		}
		runner = created
	}
	sess := NewSession(sessionID, cwd, service, registry, runner, s.sessionService)
	sess.mcpServers = cloneMCPServerConfigs(mcpServers)
	return sess, nil
}

func (s *Server) listSessions(ctx context.Context, cwd string) ([]SessionInfo, error) {
	s.mu.Lock()
	seen := make(map[string]bool)
	list := make([]SessionInfo, 0, len(s.sessions))
	for id, sess := range s.sessions {
		if cwd != "" && sess.cwd != "" && sess.cwd != cwd {
			continue
		}
		seen[id] = true
		list = append(list, SessionInfo{
			SessionID: id,
			Cwd:       sess.cwd,
			Title:     "Session " + id,
		})
	}
	s.mu.Unlock()

	if s.sessionService != nil {
		options := app.SessionListOptions{}
		if cwd != "" {
			options.WorkspaceKey = session.WorkspaceKey(cwd)
		}
		summaries, err := s.sessionService.ListSummaries(ctx, options)
		if err != nil {
			return nil, fmt.Errorf("list session state: %w", err)
		}
		for _, summary := range summaries {
			if seen[summary.ID] {
				continue
			}
			title := "Session " + summary.ID
			if summary.WorkspaceName != "" {
				title = summary.WorkspaceName + " · " + summary.ID
			}
			list = append(list, SessionInfo{SessionID: summary.ID, Cwd: cwd, Title: title})
		}
	}

	return list, nil
}

func (s *Server) deleteSession(ctx context.Context, sessionID string) error {
	if s.sessionService != nil {
		if err := s.sessionService.Delete(ctx, sessionID); err != nil {
			return fmt.Errorf("delete session state %q: %w", sessionID, err)
		}
	}
	s.mu.Lock()
	delete(s.sessions, sessionID)
	s.mu.Unlock()
	return nil
}
