package acp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/phongsathornpt/protonman/internal/base/buildinfo"
	"github.com/phongsathornpt/protonman/internal/core/permission"
	"github.com/phongsathornpt/protonman/internal/core/session"
)

func (s *Server) advertisedSessionCapabilities() SessionCapabilities {
	capabilities := SessionCapabilities{
		List:   &struct{}{},
		Resume: &struct{}{},
		Delete: &struct{}{},
		Close:  &struct{}{},
	}
	if feature, ok := stableFeature("session/additional_directories"); ok && feature.Advertised && s.sessionRegistryFactory != nil {
		capabilities.AdditionalDirectories = &struct{}{}
	}
	return capabilities
}

func (s *Server) dispatch(ctx context.Context, request RPCRequest, output io.Writer) (any, *RPCNotification, error) {
	switch request.Method {
	case "initialize":
		var params InitializeParams
		if len(request.Params) > 0 {
			if err := json.Unmarshal(request.Params, &params); err != nil {
				return nil, nil, fmt.Errorf("decode initialize: %w", err)
			}
		}
		s.mu.Lock()
		s.clientCapabilities = params.ClientCapabilities
		s.mu.Unlock()
		return InitializeResult{
			ProtocolVersion: ProtocolVersion,
			AgentCapabilities: AgentCapabilities{
				LoadSession:         true,
				PromptCapabilities:  PromptCapabilities{Image: true, Audio: false, EmbeddedContext: true},
				SessionCapabilities: s.advertisedSessionCapabilities(),
				MCPCapabilities:     MCPCapabilities{HTTP: true, SSE: false},
			},
			AgentInfo:   ImplementationInfo{Name: "proton", Title: "Protonman AI Coding Agent", Version: buildinfo.Version()},
			AuthMethods: []any{},
		}, nil, nil
	case "session/new":
		var params SessionNewParams
		if len(request.Params) > 0 {
			if err := json.Unmarshal(request.Params, &params); err != nil {
				return nil, nil, fmt.Errorf("decode session/new: %w", err)
			}
		}
		cwd, directories, err := normalizeSessionDirectories(params.Cwd, params.AdditionalDirectories)
		if err != nil {
			return nil, nil, fmt.Errorf("session/new workspace roots: %w", err)
		}
		if err := validateMCPServerConfigs(params.MCPServers); err != nil {
			return nil, nil, fmt.Errorf("session/new MCP servers: %w", err)
		}
		sessionID := session.NewID(cwd)
		sess, err := s.newSession(ctx, sessionID, cwd, directories, params.MCPServers)
		if err != nil {
			return nil, nil, err
		}
		s.mu.Lock()
		s.sessions[sessionID] = sess
		s.sessionDirectories[sessionID] = cloneDirectories(directories)
		s.mu.Unlock()
		notify := &RPCNotification{JSONRPC: "2.0", Method: "session/update", Params: map[string]any{"sessionId": sessionID, "update": map[string]any{"sessionUpdate": "available_commands_update", "availableCommands": DefaultAvailableCommands()}}}
		return SessionNewResult{SessionID: sessionID, Modes: DefaultSessionModes(sess.service.Mode().String()), ConfigOptions: s.sessionConfigOptions(ctx, sess)}, notify, nil
	case "session/load":
		var params SessionLoadParams
		if err := json.Unmarshal(request.Params, &params); err != nil {
			return nil, nil, fmt.Errorf("decode session/load: %w", err)
		}
		params.SessionID = strings.TrimSpace(params.SessionID)
		if params.SessionID == "" {
			return nil, nil, errors.New("sessionId is required")
		}
		cwd, directories, err := normalizeSessionDirectories(params.Cwd, params.AdditionalDirectories)
		if err != nil {
			return nil, nil, fmt.Errorf("session/load workspace roots: %w", err)
		}
		if err := validateMCPServerConfigs(params.MCPServers); err != nil {
			return nil, nil, fmt.Errorf("session/load MCP servers: %w", err)
		}
		loadStart := time.Now()
		sess, timings, err := s.loadSession(ctx, params.SessionID, cwd, directories, params.MCPServers)
		if err != nil {
			return nil, nil, err
		}
		timings.sessionLoad = time.Since(loadStart)
		replayStart := time.Now()
		replayMessages := 0
		if err := sess.ReplayHistory(func(notification RPCNotification) error {
			replayMessages++
			return WriteJSON(output, &s.writeMu, notification)
		}); err != nil {
			return nil, nil, err
		}
		timings.replay = time.Since(replayStart)
		timings.replayMessages = replayMessages
		deferModelDiscovery := sessionLoadDefersModelDiscovery(params.Meta)
		configStart := time.Now()
		configOptions := s.sessionConfigOptionsWithModelDiscovery(ctx, sess, !deferModelDiscovery)
		timings.configOptions = time.Since(configStart)
		timings.total = time.Since(loadStart)
		result := SessionLoadResult{
			Modes:         DefaultSessionModes(sess.service.Mode().String()),
			ConfigOptions: configOptions,
		}
		if timingEnabled() {
			result.Meta = timings.meta()
		}
		return result, nil, nil
	case "session/resume":
		var params SessionResumeParams
		if err := json.Unmarshal(request.Params, &params); err != nil {
			return nil, nil, fmt.Errorf("decode session/resume: %w", err)
		}
		params.SessionID = strings.TrimSpace(params.SessionID)
		if params.SessionID == "" {
			return nil, nil, errors.New("sessionId is required")
		}
		cwd, directories, err := normalizeSessionDirectories(params.Cwd, params.AdditionalDirectories)
		if err != nil {
			return nil, nil, fmt.Errorf("session/resume workspace roots: %w", err)
		}
		if err := validateMCPServerConfigs(params.MCPServers); err != nil {
			return nil, nil, fmt.Errorf("session/resume MCP servers: %w", err)
		}
		sess, err := s.loadOrCreateSession(ctx, params.SessionID, cwd, directories, params.MCPServers)
		if err != nil {
			return nil, nil, err
		}
		return SessionResumeResult{Modes: DefaultSessionModes(sess.service.Mode().String()), ConfigOptions: s.sessionConfigOptions(ctx, sess)}, nil, nil
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
		sess.agents.SetPermissionMode(mode)
		sess.mu.Lock()
		settings := sessionRuntimeForLocked(sess)
		modeID := mode.String()
		if mode == permission.ModeDeny {
			modeID = "plan"
		}
		settings.PermissionMode = modeID
		storeSessionRuntime(sess, settings)
		sess.mu.Unlock()
		if err := sess.saveStateDetached(ctx); err != nil {
			return nil, nil, fmt.Errorf("save session %q: %w", params.SessionID, err)
		}
		notify := &RPCNotification{JSONRPC: "2.0", Method: "session/update", Params: map[string]any{"sessionId": params.SessionID, "update": map[string]any{"sessionUpdate": "current_mode_update", "modeId": modeID}}}
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
	case "session/close":
		var params SessionCloseParams
		if err := json.Unmarshal(request.Params, &params); err != nil {
			return nil, nil, fmt.Errorf("decode session/close: %w", err)
		}
		params.SessionID = strings.TrimSpace(params.SessionID)
		if params.SessionID == "" {
			return nil, nil, errors.New("sessionId is required")
		}
		if err := s.closeSession(ctx, params.SessionID); err != nil {
			return nil, nil, err
		}
		return map[string]any{}, nil, nil
	case "session/list":
		var params SessionListParams
		if len(request.Params) > 0 {
			if err := json.Unmarshal(request.Params, &params); err != nil {
				return nil, nil, fmt.Errorf("decode session/list: %w", err)
			}
		}
		sessions, err := s.listSessions(ctx, params.Cwd)
		if err != nil {
			return nil, nil, err
		}
		return SessionListResult{Sessions: sessions}, nil, nil
	case methodSessionContext:
		var params ProtonmanSessionContextParams
		if err := json.Unmarshal(request.Params, &params); err != nil {
			return nil, nil, fmt.Errorf("decode %s: %w", methodSessionContext, err)
		}
		result, err := s.sessionContext(ctx, params.SessionID)
		return result, nil, err
	case methodSessionMemory:
		var params ProtonmanSessionMemoryParams
		if err := json.Unmarshal(request.Params, &params); err != nil {
			return nil, nil, fmt.Errorf("decode %s: %w", methodSessionMemory, err)
		}
		result, err := s.sessionMemory(ctx, params.SessionID)
		return result, nil, err
	case methodSessionMemoryForget:
		var params ProtonmanSessionMemoryForgetParams
		if err := json.Unmarshal(request.Params, &params); err != nil {
			return nil, nil, fmt.Errorf("decode %s: %w", methodSessionMemoryForget, err)
		}
		result, err := s.sessionMemoryForget(ctx, params)
		return result, nil, err
	case methodSessionSkills:
		var params ProtonmanSessionSkillsParams
		if err := json.Unmarshal(request.Params, &params); err != nil {
			return nil, nil, fmt.Errorf("decode %s: %w", methodSessionSkills, err)
		}
		result, err := s.sessionSkills(ctx, params.SessionID)
		return result, nil, err
	case methodSessionSkillsToggle:
		var params ProtonmanSessionSkillToggleParams
		if err := json.Unmarshal(request.Params, &params); err != nil {
			return nil, nil, fmt.Errorf("decode %s: %w", methodSessionSkillsToggle, err)
		}
		result, err := s.sessionSkillToggle(ctx, params)
		return result, nil, err
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
		return nil, nil, fmt.Errorf("%w: method %q is not supported", ErrMethodNotSupported, request.Method)
	}
}

func sessionLoadDefersModelDiscovery(meta Meta) bool {
	// The desktop refreshes runtime options asynchronously after transcript load.
	// Let it opt out of network model discovery on this latency-sensitive request.
	var options struct {
		DeferModelDiscovery bool `json:"deferModelDiscovery"`
	}
	return json.Unmarshal(meta["protonman"], &options) == nil && options.DeferModelDiscovery
}
