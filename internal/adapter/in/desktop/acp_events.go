//go:build desktop

package desktop

import (
	"encoding/json"
	"strings"
	"sync"

	"github.com/phongsathornpt/protonman/internal/adapter/out/acpclient"
)

type sessionUpdateEnvelope struct {
	SessionID string          `json:"sessionId"`
	Update    json.RawMessage `json:"update"`
}

type sessionUpdateHeader struct {
	Kind string `json:"sessionUpdate"`
}

type messageChunkUpdate struct {
	Kind    string          `json:"sessionUpdate"`
	Content json.RawMessage `json:"content"`
}

type toolUpdate struct {
	Kind       string          `json:"sessionUpdate"`
	ToolCallID string          `json:"toolCallId"`
	Title      string          `json:"title"`
	Status     string          `json:"status"`
	Content    json.RawMessage `json:"content"`
}

type configOptionUpdate struct {
	Kind          string                          `json:"sessionUpdate"`
	ConfigOptions []acpclient.SessionConfigOption `json:"configOptions"`
}

type sessionInfoUpdate struct {
	Kind      string  `json:"sessionUpdate"`
	Title     *string `json:"title,omitempty"`
	UpdatedAt *string `json:"updatedAt,omitempty"`
}

type currentModeUpdate struct {
	Kind      string `json:"sessionUpdate"`
	CurrentID string `json:"currentModeId"`
}

type availableCommandUpdate struct {
	Kind              string                       `json:"sessionUpdate"`
	AvailableCommands []acpclient.AvailableCommand `json:"availableCommands"`
}

type usageUpdate struct {
	Kind string `json:"sessionUpdate"`
	Used int64  `json:"used"`
	Size int64  `json:"size"`
	Cost *struct {
		Amount   float64 `json:"amount"`
		Currency string  `json:"currency"`
	} `json:"cost,omitempty"`
}

type subagentUpdate struct {
	Kind    string `json:"sessionUpdate"`
	AgentID string `json:"agentId"`
	Profile string `json:"profile"`
	Task    string `json:"task"`
	Summary string `json:"summary"`
	Status  string `json:"status"`
}

type sessionProtocolState struct {
	Mode     string
	Commands []acpclient.AvailableCommand
	Usage    usageUpdate
}

var sessionProtocolStates sync.Map // map[*application]map[string]sessionProtocolState

func protocolStateMapFor(a *application) map[string]sessionProtocolState {
	if current, ok := sessionProtocolStates.Load(a); ok {
		return current.(map[string]sessionProtocolState)
	}
	created := make(map[string]sessionProtocolState)
	actual, _ := sessionProtocolStates.LoadOrStore(a, created)
	return actual.(map[string]sessionProtocolState)
}

func decodeSessionUpdate(params json.RawMessage) (string, string, json.RawMessage, bool) {
	var envelope sessionUpdateEnvelope
	if json.Unmarshal(params, &envelope) != nil || strings.TrimSpace(envelope.SessionID) == "" || len(envelope.Update) == 0 {
		return "", "", nil, false
	}
	var header sessionUpdateHeader
	if json.Unmarshal(envelope.Update, &header) != nil || strings.TrimSpace(header.Kind) == "" {
		return "", "", nil, false
	}
	return envelope.SessionID, header.Kind, envelope.Update, true
}

func (a *application) applySessionInfoUpdate(sessionID string, update sessionInfoUpdate) {
	if update.Title == nil {
		return
	}
	a.mu.Lock()
	for i := range a.state.Sessions {
		if a.state.Sessions[i].ID == sessionID {
			a.state.Sessions[i].Title = strings.TrimSpace(*update.Title)
			break
		}
	}
	active := a.state.ActiveSessionID == sessionID
	a.rebuildSidebarRowsLocked()
	a.mu.Unlock()
	if a.list != nil {
		a.list.Refresh()
	}
	if active {
		a.renderSessionChrome()
	}
}

func (a *application) applyCurrentModeUpdate(sessionID string, update currentModeUpdate) {
	a.mu.Lock()
	states := protocolStateMapFor(a)
	state := states[sessionID]
	state.Mode = strings.TrimSpace(update.CurrentID)
	states[sessionID] = state
	active := a.state.ActiveSessionID == sessionID
	a.mu.Unlock()
	if active {
		a.renderSessionChrome()
	}
}

func (a *application) applyAvailableCommandsUpdate(sessionID string, update availableCommandUpdate) {
	a.mu.Lock()
	states := protocolStateMapFor(a)
	state := states[sessionID]
	state.Commands = append([]acpclient.AvailableCommand(nil), update.AvailableCommands...)
	states[sessionID] = state
	a.mu.Unlock()
}

func (a *application) applyUsageUpdate(sessionID string, update usageUpdate) {
	a.mu.Lock()
	states := protocolStateMapFor(a)
	state := states[sessionID]
	state.Usage = update
	states[sessionID] = state
	active := a.state.ActiveSessionID == sessionID
	a.mu.Unlock()
	if active {
		a.renderSessionChrome()
	}
}

func (a *application) protocolState(sessionID string) sessionProtocolState {
	a.mu.Lock()
	defer a.mu.Unlock()
	return protocolStateMapFor(a)[sessionID]
}
