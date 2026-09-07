// Package acp implements the Agent Client Protocol (ACP) v1 specification.
package acp

import (
	"encoding/json"
)

// ProtocolVersion is the ACP protocol version supported by Proton.
const ProtocolVersion = 1

// JSON-RPC 2.0 error codes.
const (
	CodeParseError     = -32700
	CodeInvalidRequest = -32600
	CodeMethodNotFound = -32601
	CodeInvalidParams  = -32602
	CodeInternalError  = -32603
	CodeServerError    = -32000
)

// RPCRequest is a line-delimited JSON-RPC 2.0 request.
type RPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// RPCResponse is a line-delimited JSON-RPC 2.0 response.
type RPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

// RPCError is the error payload for an RPCResponse.
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// RPCNotification is a one-way notification sent over stdio.
type RPCNotification struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  any    `json:"params"`
}

// ImplementationInfo describes the agent or client software.
type ImplementationInfo struct {
	Name    string `json:"name"`
	Title   string `json:"title,omitempty"`
	Version string `json:"version,omitempty"`
}

// ClientCapabilities declares features supported by the editor (e.g. Zed).
type ClientCapabilities struct {
	FS          *ClientFSCapabilities `json:"fs,omitempty"`
	Terminal    bool                  `json:"terminal,omitempty"`
	Elicitation any                   `json:"elicitation,omitempty"`
}

// ClientFSCapabilities indicates client-side filesystem methods.
type ClientFSCapabilities struct {
	ReadTextFile  bool `json:"readTextFile,omitempty"`
	WriteTextFile bool `json:"writeTextFile,omitempty"`
}

// AgentCapabilities advertises capabilities supported by Proton.
type AgentCapabilities struct {
	LoadSession         bool                `json:"loadSession"`
	PromptCapabilities  PromptCapabilities  `json:"promptCapabilities"`
	SessionCapabilities SessionCapabilities `json:"sessionCapabilities"`
	MCPCapabilities     MCPCapabilities     `json:"mcpCapabilities,omitempty"`
}

// PromptCapabilities lists supported prompt content types.
type PromptCapabilities struct {
	Image           bool `json:"image"`
	Audio           bool `json:"audio"`
	EmbeddedContext bool `json:"embeddedContext"`
}

// SessionCapabilities lists optional session capabilities.
type SessionCapabilities struct {
	Resume                *struct{} `json:"resume,omitempty"`
	Delete                *struct{} `json:"delete,omitempty"`
	AdditionalDirectories *struct{} `json:"additionalDirectories,omitempty"`
}

// MCPCapabilities describes supported MCP transports.
type MCPCapabilities struct {
	HTTP bool `json:"http,omitempty"`
	SSE  bool `json:"sse,omitempty"`
}

// InitializeParams are sent by the client upon connecting.
type InitializeParams struct {
	ProtocolVersion    int                 `json:"protocolVersion"`
	ClientCapabilities ClientCapabilities  `json:"clientCapabilities,omitempty"`
	ClientInfo         *ImplementationInfo `json:"clientInfo,omitempty"`
}

// InitializeResult is the agent's response to initialize.
type InitializeResult struct {
	ProtocolVersion   int                `json:"protocolVersion"`
	AgentCapabilities AgentCapabilities  `json:"agentCapabilities"`
	AgentInfo         ImplementationInfo `json:"agentInfo"`
	AuthMethods       []any              `json:"authMethods"`
}

// MCPServerConfig is an MCP server configuration passed by Zed.
type MCPServerConfig struct {
	Name    string   `json:"name"`
	Command string   `json:"command"`
	Args    []string `json:"args,omitempty"`
	Env     []string `json:"env,omitempty"`
}

// SessionNewParams creates a new thread in the given working directory.
type SessionNewParams struct {
	Cwd        string            `json:"cwd,omitempty"`
	MCPServers []MCPServerConfig `json:"mcpServers,omitempty"`
}

// SessionNewResult returns the new session ID and available operating modes.
type SessionNewResult struct {
	SessionID string            `json:"sessionId"`
	Modes     *SessionModeState `json:"modes,omitempty"`
}

// SessionLoadParams loads a previous session and replays history.
type SessionLoadParams struct {
	SessionID  string            `json:"sessionId"`
	Cwd        string            `json:"cwd,omitempty"`
	MCPServers []MCPServerConfig `json:"mcpServers,omitempty"`
}

// SessionResumeParams resumes a session without replaying history.
type SessionResumeParams struct {
	SessionID  string            `json:"sessionId"`
	Cwd        string            `json:"cwd,omitempty"`
	MCPServers []MCPServerConfig `json:"mcpServers,omitempty"`
}

// SessionSetModeParams switches the session mode (e.g. ask, plan, always-approve).
type SessionSetModeParams struct {
	SessionID string `json:"sessionId"`
	ModeID    string `json:"modeId"`
}

// SessionCancelParams cancels an active prompt turn.
type SessionCancelParams struct {
	SessionID string `json:"sessionId"`
}

// SessionListParams lists sessions for a given workspace root.
type SessionListParams struct {
	Cwd string `json:"cwd,omitempty"`
}

// SessionInfo describes a discovered session.
type SessionInfo struct {
	SessionID string `json:"sessionId"`
	Cwd       string `json:"cwd,omitempty"`
	Title     string `json:"title,omitempty"`
	UpdatedAt string `json:"updatedAt,omitempty"`
}

// SessionListResult returns the discovered sessions.
type SessionListResult struct {
	Sessions []SessionInfo `json:"sessions"`
}

// SessionDeleteParams deletes a session from history.
type SessionDeleteParams struct {
	SessionID string `json:"sessionId"`
}

// BlockType identifies the content type within a ContentBlock.
type BlockType string

const (
	BlockTypeText         BlockType = "text"
	BlockTypeImage        BlockType = "image"
	BlockTypeResource     BlockType = "resource"
	BlockTypeResourceLink BlockType = "resource_link"
	BlockTypeAudio        BlockType = "audio"
)

// ContentBlock represents one displayable content element.
type ContentBlock struct {
	Type     BlockType             `json:"type"`
	Text     string                `json:"text,omitempty"`
	MIMEType string                `json:"mimeType,omitempty"`
	Data     string                `json:"data,omitempty"`     // base64 image or audio
	URI      string                `json:"uri,omitempty"`      // resource or resource_link URI
	Name     string                `json:"name,omitempty"`     // resource_link name
	Resource *EmbeddedTextResource `json:"resource,omitempty"` // embedded context
}

// EmbeddedTextResource represents text contents embedded directly in a message.
type EmbeddedTextResource struct {
	URI      string `json:"uri"`
	MIMEType string `json:"mimeType,omitempty"`
	Text     string `json:"text,omitempty"`
}

// SessionPromptParams is sent to run a user prompt.
type SessionPromptParams struct {
	SessionID string         `json:"sessionId"`
	Prompt    []ContentBlock `json:"prompt"`
}

// StopReason represents the outcome of an ACP prompt turn.
type StopReason string

const (
	StopReasonEndTurn         StopReason = "end_turn"
	StopReasonCancelled       StopReason = "cancelled"
	StopReasonMaxTokens       StopReason = "max_tokens"
	StopReasonMaxTurnRequests StopReason = "max_turn_requests"
	StopReasonRefusal         StopReason = "refusal"
)

// SessionPromptResult completes a prompt turn.
type SessionPromptResult struct {
	StopReason StopReason `json:"stopReason"`
}

// ToolKind categorizes tool calls for UI rendering in Zed.
type ToolKind string

const (
	ToolKindRead    ToolKind = "read"
	ToolKindEdit    ToolKind = "edit"
	ToolKindDelete  ToolKind = "delete"
	ToolKindMove    ToolKind = "move"
	ToolKindSearch  ToolKind = "search"
	ToolKindExecute ToolKind = "execute"
	ToolKindThink   ToolKind = "think"
	ToolKindFetch   ToolKind = "fetch"
	ToolKindOther   ToolKind = "other"
)

// ToolCallStatus represents the execution state of a tool call.
type ToolCallStatus string

const (
	ToolCallStatusPending    ToolCallStatus = "pending"
	ToolCallStatusInProgress ToolCallStatus = "in_progress"
	ToolCallStatusCompleted  ToolCallStatus = "completed"
	ToolCallStatusFailed     ToolCallStatus = "failed"
)

// ToolCallLocation points to a file location affected by a tool call.
type ToolCallLocation struct {
	Path string `json:"path"`
	Line int    `json:"line,omitempty"`
}

// SessionMode represents an operating mode (ask, plan, always-approve).
type SessionMode struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// SessionModeState records the active mode and available modes.
type SessionModeState struct {
	CurrentModeID  string        `json:"currentModeId"`
	AvailableModes []SessionMode `json:"availableModes"`
}

// AvailableCommand defines a slash command advertised to Zed.
type AvailableCommand struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Input       *AvailableCommandInput `json:"input,omitempty"`
}

// AvailableCommandInput describes input hints for a slash command.
type AvailableCommandInput struct {
	Hint string `json:"hint"`
}

// PermissionOptionKind indicates the semantic meaning of an approval choice.
type PermissionOptionKind string

const (
	PermissionOptionAllowOnce    PermissionOptionKind = "allow_once"
	PermissionOptionAllowAlways  PermissionOptionKind = "allow_always"
	PermissionOptionRejectOnce   PermissionOptionKind = "reject_once"
	PermissionOptionRejectAlways PermissionOptionKind = "reject_always"
)

// PermissionOption is one choice presented to the user in Zed.
type PermissionOption struct {
	OptionID string               `json:"optionId"`
	Name     string               `json:"name"`
	Kind     PermissionOptionKind `json:"kind"`
}

// RequestPermissionParams is sent from Agent to Client to prompt for tool permission.
type RequestPermissionParams struct {
	SessionID string             `json:"sessionId"`
	ToolCall  map[string]any     `json:"toolCall"`
	Options   []PermissionOption `json:"options"`
}

// RequestPermissionResult contains the user's decision from Zed.
type RequestPermissionResult struct {
	Outcome struct {
		Outcome  string `json:"outcome"` // "selected" or "cancelled"
		OptionID string `json:"optionId,omitempty"`
	} `json:"outcome"`
}
