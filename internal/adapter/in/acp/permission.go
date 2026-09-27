package acp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/phongsathornpt/protonman/internal/core/permission"
)

// requestPermissionMethod is the server-to-client method that asks the ACP client
// to authorize one tool call. Without it, ask/auto permission modes have nowhere
// to prompt and toolcall.Service denies every non-statically-allowed call.
const requestPermissionMethod = "session/request_permission"

// permissionDecision carries the client's answer back to the blocked tool call.
type permissionDecision struct {
	optionID string
	err      error
}

// permissionBroker is the server side of interactive permission handling. The
// Serve loop owns inbound framing, so a server-initiated request is handed a
// delivery function and its reply is routed back by request ID.
//
// Lifetime: one broker per Serve call. Every waiter is registered under s.mu and
// resolved on response, cancellation, or Serve shutdown, so no goroutine blocks
// on a channel that is never written.
type permissionBroker struct {
	server  *Server
	waiters map[uint64]chan permissionDecision
	mu      sync.Mutex
	closed  bool
}

func newPermissionBroker(server *Server) *permissionBroker {
	return &permissionBroker{
		server:  server,
		waiters: make(map[uint64]chan permissionDecision),
	}
}

// prompt builds the toolcall.PermissionPrompt for one session. The returned
// resolver is installed on that session's toolcall service and on its subagents,
// so delegated work asks the same client instead of failing closed.
func (b *permissionBroker) prompt(sessionID string) func(context.Context, permission.Request) (permission.Resolution, error) {
	return func(ctx context.Context, request permission.Request) (permission.Resolution, error) {
		return b.request(ctx, sessionID, request)
	}
}

func (b *permissionBroker) request(
	ctx context.Context,
	sessionID string,
	request permission.Request,
) (permission.Resolution, error) {
	if err := ctx.Err(); err != nil {
		return denyResolution("permission request cancelled before it was sent"), nil
	}
	if b == nil || b.server == nil {
		return denyResolution("no ACP client is attached to this session"), nil
	}

	params := RequestPermissionParams{
		SessionID: sessionID,
		ToolCall:  permissionToolCall(request),
		Options:   permissionOptions(),
	}
	encoded, err := jsonMarshal(params)
	if err != nil {
		return denyResolution("encode permission request"), nil
	}

	id, waiter, err := b.register()
	if err != nil {
		return denyResolution("permission prompting is unavailable"), nil
	}
	defer b.unregister(id)

	if err := WriteJSON(b.server.output, &b.server.writeMu, RPCRequest{
		JSONRPC: "2.0",
		ID:      json.RawMessage(strconv.FormatUint(id, 10)),
		Method:  requestPermissionMethod,
		Params:  encoded,
	}); err != nil {
		return denyResolution("deliver permission request"), nil
	}

	select {
	case <-ctx.Done():
		return denyResolution("permission request was cancelled"), nil
	case decision := <-waiter:
		if decision.err != nil {
			return denyResolution(decision.err.Error()), nil
		}
		return permissionResolution(decision.optionID)
	}
}

// register allocates a request ID and its waiter channel. A closed broker refuses
// new work so shutdown cannot race a late tool call into a blocked wait.
func (b *permissionBroker) register() (uint64, chan permissionDecision, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return 0, nil, errors.New("ACP permission broker is closed")
	}
	id := b.nextPermissionID()
	waiter := make(chan permissionDecision, 1)
	b.waiters[id] = waiter
	return id, waiter, nil
}

func (b *permissionBroker) unregister(id uint64) {
	b.mu.Lock()
	delete(b.waiters, id)
	b.mu.Unlock()
}

// resolve routes a client response to its waiter. Unknown IDs are ignored
// because a late reply can outlive its cancelled request.
func (b *permissionBroker) resolve(id uint64, decision permissionDecision) {
	b.mu.Lock()
	waiter, ok := b.waiters[id]
	if ok {
		delete(b.waiters, id)
	}
	b.mu.Unlock()
	if !ok {
		return
	}
	waiter <- decision
}

// close releases every blocked tool call so Serve shutdown cannot leave a turn
// waiting on a permission answer that can no longer arrive.
func (b *permissionBroker) close() {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return
	}
	b.closed = true
	waiters := b.waiters
	b.waiters = make(map[uint64]chan permissionDecision)
	b.mu.Unlock()
	for _, waiter := range waiters {
		waiter <- permissionDecision{err: errors.New("ACP connection closed before the permission request was answered")}
	}
}

// permissionOptions are the choices offered to the client. Session-scoped allow is
// only offered for calls that are safe to reuse; the toolcall service applies its
// own grant-reuse guard, and anything uncertain must be approved one call at a
// time.
func permissionOptions() []PermissionOption {
	return []PermissionOption{
		{OptionID: "allow_once", Name: "Allow once", Kind: PermissionOptionAllowOnce},
		{OptionID: "allow_session", Name: "Allow for this session", Kind: PermissionOptionAllowAlways},
		{OptionID: "reject_once", Name: "Deny", Kind: PermissionOptionRejectOnce},
	}
}

// permissionToolCall renders the domain request into the ACP tool-call payload the
// desktop permission panel displays.
func permissionToolCall(request permission.Request) map[string]any {
	toolCall := map[string]any{
		"toolCallId": request.CallID,
		"title":      permissionTitle(request),
		"kind":       permissionKindLabel(request),
	}
	if request.Detail != "" {
		toolCall["rawInput"] = map[string]any{"detail": request.Detail}
	}
	if len(request.Arguments) > 0 {
		var arguments map[string]any
		if err := json.Unmarshal(request.Arguments, &arguments); err == nil {
			toolCall["rawInput"] = arguments
		}
	}
	return toolCall
}

func permissionTitle(request permission.Request) string {
	title := strings.TrimSpace(request.ToolName)
	if request.Detail == "" {
		return title
	}
	return title + " · " + request.Detail
}

func permissionKindLabel(request permission.Request) string {
	kind := strings.TrimSpace(string(request.ToolKind))
	if kind == "" {
		return "other"
	}
	return kind
}

// permissionResolution maps the client's choice onto the domain decision.
// An unrecognized or absent option fails closed.
func permissionResolution(optionID string) (permission.Resolution, error) {
	switch strings.TrimSpace(optionID) {
	case "allow_once":
		return permission.Resolution{
			Action: permission.ActionAllow,
			Scope:  permission.GrantScopeOnce,
			Reason: "user allowed one call",
		}, nil
	case "allow_session":
		return permission.Resolution{
			Action: permission.ActionAllow,
			Scope:  permission.GrantScopeSession,
			Reason: "user allowed this exact request for the session",
		}, nil
	default:
		return denyResolution("user denied the request"), nil
	}
}

func denyResolution(reason string) permission.Resolution {
	return permission.Resolution{Action: permission.ActionDeny, Reason: reason}
}

func (b *permissionBroker) nextPermissionID() uint64 {
	b.server.mu.Lock()
	defer b.server.mu.Unlock()
	b.server.permissionSeq++
	return b.server.permissionSeq
}

// handleServerResponse routes an inbound frame to a blocked permission request.
// It reports whether the frame was consumed; anything else belongs to the normal
// request path.
func (s *Server) handleServerResponse(broker *permissionBroker, line []byte) bool {
	var response RPCResponse
	if err := json.Unmarshal(line, &response); err != nil {
		return false
	}
	if len(response.ID) == 0 || string(response.ID) == "null" || response.Result == nil && response.Error == nil {
		return false
	}
	// Notifications carry no id; a frame without one is not a response.
	id, err := strconv.ParseUint(strings.TrimSpace(string(response.ID)), 10, 64)
	if err != nil {
		return false
	}
	decision := permissionDecision{err: errors.New("client returned an empty permission outcome")}
	switch {
	case response.Error != nil:
		decision.err = fmt.Errorf("client rejected the permission request: %s", response.Error.Message)
	case response.Result != nil:
		encoded, err := json.Marshal(response.Result)
		if err != nil {
			decision.err = fmt.Errorf("encode permission response: %w", err)
			break
		}
		var result RequestPermissionResult
		if err := json.Unmarshal(encoded, &result); err != nil {
			decision.err = fmt.Errorf("decode permission response: %w", err)
			break
		}
		decision = permissionDecision{optionID: strings.TrimSpace(result.Outcome.OptionID)}
	}
	broker.resolve(id, decision)
	return true
}
