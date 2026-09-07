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

	"github.com/projectTHORN/proton/internal/buildinfo"
	"github.com/projectTHORN/proton/internal/permission"
	"github.com/projectTHORN/proton/internal/session"
	"github.com/projectTHORN/proton/internal/tool"
	"github.com/projectTHORN/proton/internal/toolcall"
	applicationturn "github.com/projectTHORN/proton/internal/turn"
	sdk "github.com/projectTHORN/proton/proton-sdk"
)

// ErrInvalidServer indicates that the ACP server cannot start.
var ErrInvalidServer = errors.New("invalid ACP server")

// ErrInvalidRequest indicates malformed JSON-RPC or parameters.
var ErrInvalidRequest = errors.New("invalid request")

// Option configures an ACP Server.
type Option func(*Server)

// RunnerFactory creates a model/tool runner bound to one session's service.
type RunnerFactory func(*toolcall.Service) (applicationturn.Runner, error)

// WithStore sets the session store for loading, resuming, and listing sessions.
func WithStore(store session.Repository) Option {
	return func(s *Server) {
		s.store = store
	}
}

// WithRunnerFactory supplies isolated runners for ACP sessions. The factory
// receives the session-local tool-call service so model-driven calls do not
// share permission state with other sessions.
func WithRunnerFactory(factory RunnerFactory) Option {
	return func(s *Server) {
		s.runnerFactory = factory
	}
}

// Server is a full-duplex JSON-RPC 2.0 ACP agent server.
type Server struct {
	service       *toolcall.Service
	registry      tool.Registry
	runnerFactory RunnerFactory
	store         session.Repository

	mu       sync.Mutex
	writeMu  sync.Mutex
	sessions map[string]*Session
	nextID   uint64
}

// New creates an ACP server over Proton's toolcall service and model runner.
func New(service *toolcall.Service, registry tool.Registry, runner applicationturn.Runner, opts ...Option) (*Server, error) {
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
		if loop, ok := runner.(*applicationturn.Loop); ok {
			s.runnerFactory = func(service *toolcall.Service) (applicationturn.Runner, error) {
				return loop.CloneWithTools(service)
			}
		} else {
			return nil, fmt.Errorf("%w: runner factory is required for a configured custom runner", ErrInvalidServer)
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
			_ = json.Unmarshal(request.Params, &params)
		}
		s.mu.Lock()
		s.nextID++
		sessionID := fmt.Sprintf("acp-%d", s.nextID)
		s.mu.Unlock()
		sess, err := s.newSession(sessionID, params.Cwd)
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

		sess, err := s.loadOrCreateSession(ctx, params.SessionID, params.Cwd)
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
		_, err := s.loadOrCreateSession(ctx, params.SessionID, params.Cwd)
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

func (s *Server) loadOrCreateSession(ctx context.Context, sessionID string, cwd string) (*Session, error) {
	s.mu.Lock()
	existing, ok := s.sessions[sessionID]
	s.mu.Unlock()
	if ok {
		return existing, nil
	}

	sess, err := s.newSession(sessionID, cwd)
	if err != nil {
		return nil, err
	}
	if s.store != nil {
		state, found, err := s.store.Load(ctx, sessionID)
		if err != nil {
			return nil, fmt.Errorf("load session state %q: %w", sessionID, err)
		}
		if found {
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

func (s *Server) newSession(sessionID string, cwd string) (*Session, error) {
	service := s.service.Clone()
	if service == nil {
		return nil, fmt.Errorf("%w: clone session tool-call service", ErrInvalidServer)
	}
	var runner applicationturn.Runner
	if s.runnerFactory != nil {
		created, err := s.runnerFactory(service)
		if err != nil {
			return nil, fmt.Errorf("create runner for session %q: %w", sessionID, err)
		}
		runner = created
	}
	return NewSession(sessionID, cwd, service, s.registry, runner, s.store), nil
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

	if s.store != nil {
		storedIDs, err := s.store.List(ctx, "")
		if err != nil {
			return nil, fmt.Errorf("list session state: %w", err)
		}
		for _, id := range storedIDs {
			if seen[id] {
				continue
			}
			list = append(list, SessionInfo{
				SessionID: id,
				Cwd:       cwd,
				Title:     "Session " + id,
			})
		}
	}

	return list, nil
}

func (s *Server) deleteSession(ctx context.Context, sessionID string) error {
	if s.store != nil {
		if err := s.store.Delete(ctx, sessionID); err != nil {
			return fmt.Errorf("delete session state %q: %w", sessionID, err)
		}
	}
	s.mu.Lock()
	delete(s.sessions, sessionID)
	s.mu.Unlock()
	return nil
}
