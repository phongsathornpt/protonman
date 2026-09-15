//go:build desktop

package desktop

import (
	"encoding/json"
	"strings"

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
