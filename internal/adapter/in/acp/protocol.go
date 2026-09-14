// Package acp implements the Agent Client Protocol (ACP) v1 specification.
package acp

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

// ProtocolVersion is the ACP protocol version supported by Protonman.
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

type RPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type RPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

type RPCNotification struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  any    `json:"params"`
}

type ImplementationInfo struct {
	MetaCarrier
	Name    string `json:"name"`
	Title   string `json:"title,omitempty"`
	Version string `json:"version,omitempty"`
}

type ClientCapabilities struct {
	MetaCarrier
	FS          *ClientFSCapabilities `json:"fs,omitempty"`
	Terminal    bool                  `json:"terminal,omitempty"`
	Elicitation any                   `json:"elicitation,omitempty"`
}

type ClientFSCapabilities struct {
	MetaCarrier
	ReadTextFile  bool `json:"readTextFile,omitempty"`
	WriteTextFile bool `json:"writeTextFile,omitempty"`
}

type AgentCapabilities struct {
	MetaCarrier
	LoadSession         bool                `json:"loadSession"`
	PromptCapabilities  PromptCapabilities  `json:"promptCapabilities"`
	SessionCapabilities SessionCapabilities `json:"sessionCapabilities"`
	MCPCapabilities     MCPCapabilities     `json:"mcpCapabilities,omitempty"`
}

type PromptCapabilities struct {
	MetaCarrier
	Image           bool `json:"image"`
	Audio           bool `json:"audio"`
	EmbeddedContext bool `json:"embeddedContext"`
}

type SessionCapabilities struct {
	MetaCarrier
	Resume                *struct{} `json:"resume,omitempty"`
	Delete                *struct{} `json:"delete,omitempty"`
	Close                 *struct{} `json:"close,omitempty"`
	AdditionalDirectories *struct{} `json:"additionalDirectories,omitempty"`
}

type MCPCapabilities struct {
	MetaCarrier
	HTTP bool `json:"http,omitempty"`
	SSE  bool `json:"sse,omitempty"`
}

type InitializeParams struct {
	MetaCarrier
	ProtocolVersion    int                 `json:"protocolVersion"`
	ClientCapabilities ClientCapabilities  `json:"clientCapabilities,omitempty"`
	ClientInfo         *ImplementationInfo `json:"clientInfo,omitempty"`
}

type InitializeResult struct {
	MetaCarrier
	ProtocolVersion   int                `json:"protocolVersion"`
	AgentCapabilities AgentCapabilities  `json:"agentCapabilities"`
	AgentInfo         ImplementationInfo `json:"agentInfo"`
	AuthMethods       []any              `json:"authMethods"`
}

type MCPServerConfig struct {
	MetaCarrier
	Name    string   `json:"name"`
	Command string   `json:"command"`
	Args    []string `json:"args,omitempty"`
	Env     []string `json:"env,omitempty"`
}

func validateMCPServerConfigs(configs []MCPServerConfig) error {
	seen := make(map[string]struct{}, len(configs))
	for i, config := range configs {
		name := strings.TrimSpace(config.Name)
		if name == "" || name != config.Name {
			return fmt.Errorf("MCP server %d name must be non-empty and trimmed", i)
		}
		command := strings.TrimSpace(config.Command)
		if command == "" || command != config.Command {
			return fmt.Errorf("MCP server %q command must be non-empty and trimmed", name)
		}
		if _, exists := seen[name]; exists {
			return fmt.Errorf("duplicate MCP server %q", name)
		}
		seen[name] = struct{}{}
	}
	return nil
}

func cloneMCPServerConfigs(configs []MCPServerConfig) []MCPServerConfig {
	out := make([]MCPServerConfig, len(configs))
	for i, config := range configs {
		out[i] = config
		out[i].Args = append([]string(nil), config.Args...)
		out[i].Env = append([]string(nil), config.Env...)
		out[i].Meta = cloneMeta(config.Meta)
	}
	return out
}

func sameMCPServerConfigs(left, right []MCPServerConfig) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i].Name != right[i].Name || left[i].Command != right[i].Command ||
			!slices.Equal(left[i].Args, right[i].Args) || !slices.Equal(left[i].Env, right[i].Env) {
			return false
		}
	}
	return true
}

type SessionNewParams struct {
	MetaCarrier
	Cwd                   string            `json:"cwd,omitempty"`
	AdditionalDirectories []string          `json:"additionalDirectories,omitempty"`
	MCPServers            []MCPServerConfig `json:"mcpServers,omitempty"`
}

type SessionNewResult struct {
	MetaCarrier
	SessionID string            `json:"sessionId"`
	Modes     *SessionModeState `json:"modes,omitempty"`
}

type SessionLoadParams struct {
	MetaCarrier
	SessionID             string            `json:"sessionId"`
	Cwd                   string            `json:"cwd,omitempty"`
	AdditionalDirectories []string          `json:"additionalDirectories,omitempty"`
	MCPServers            []MCPServerConfig `json:"mcpServers,omitempty"`
}

type SessionResumeParams struct {
	MetaCarrier
	SessionID             string            `json:"sessionId"`
	Cwd                   string            `json:"cwd,omitempty"`
	AdditionalDirectories []string          `json:"additionalDirectories,omitempty"`
	MCPServers            []MCPServerConfig `json:"mcpServers,omitempty"`
}

type SessionSetModeParams struct {
	MetaCarrier
	SessionID string `json:"sessionId"`
	ModeID    string `json:"modeId"`
}

type SessionCancelParams struct {
	MetaCarrier
	SessionID string `json:"sessionId"`
}

type SessionCloseParams struct {
	MetaCarrier
	SessionID string `json:"sessionId"`
}

type SessionListParams struct {
	MetaCarrier
	Cwd string `json:"cwd,omitempty"`
}

type SessionInfo struct {
	MetaCarrier
	SessionID             string   `json:"sessionId"`
	Cwd                   string   `json:"cwd,omitempty"`
	AdditionalDirectories []string `json:"additionalDirectories,omitempty"`
	Title                 string   `json:"title,omitempty"`
	UpdatedAt             string   `json:"updatedAt,omitempty"`
	WorkspaceKey          string   `json:"workspaceKey,omitempty"`
	WorkspaceName         string   `json:"workspaceName,omitempty"`
}

type SessionListResult struct {
	MetaCarrier
	Sessions []SessionInfo `json:"sessions"`
}

type SessionDeleteParams struct {
	MetaCarrier
	SessionID string `json:"sessionId"`
}

type BlockType string

const (
	BlockTypeText         BlockType = "text"
	BlockTypeImage        BlockType = "image"
	BlockTypeResource     BlockType = "resource"
	BlockTypeResourceLink BlockType = "resource_link"
	BlockTypeAudio        BlockType = "audio"
)

type ContentBlock struct {
	MetaCarrier
	Type     BlockType             `json:"type"`
	Text     string                `json:"text,omitempty"`
	MIMEType string                `json:"mimeType,omitempty"`
	Data     string                `json:"data,omitempty"`
	URI      string                `json:"uri,omitempty"`
	Name     string                `json:"name,omitempty"`
	Resource *EmbeddedTextResource `json:"resource,omitempty"`
}

type EmbeddedTextResource struct {
	MetaCarrier
	URI      string `json:"uri"`
	MIMEType string `json:"mimeType,omitempty"`
	Text     string `json:"text,omitempty"`
}

type SessionPromptParams struct {
	MetaCarrier
	SessionID string         `json:"sessionId"`
	Prompt    []ContentBlock `json:"prompt"`
}

type StopReason string

const (
	StopReasonEndTurn         StopReason = "end_turn"
	StopReasonCancelled       StopReason = "cancelled"
	StopReasonMaxTokens       StopReason = "max_tokens"
	StopReasonMaxTurnRequests StopReason = "max_turn_requests"
	StopReasonRefusal         StopReason = "refusal"
)

type SessionPromptResult struct {
	MetaCarrier
	StopReason StopReason `json:"stopReason"`
}

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

type ToolCallStatus string

const (
	ToolCallStatusPending    ToolCallStatus = "pending"
	ToolCallStatusInProgress ToolCallStatus = "in_progress"
	ToolCallStatusCompleted  ToolCallStatus = "completed"
	ToolCallStatusFailed     ToolCallStatus = "failed"
)

type ToolCallLocation struct {
	MetaCarrier
	Path string `json:"path"`
	Line int    `json:"line,omitempty"`
}

type SessionMode struct {
	MetaCarrier
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

type SessionModeState struct {
	MetaCarrier
	CurrentModeID  string        `json:"currentModeId"`
	AvailableModes []SessionMode `json:"availableModes"`
}

type AvailableCommand struct {
	MetaCarrier
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Input       *AvailableCommandInput `json:"input,omitempty"`
}

type AvailableCommandInput struct {
	MetaCarrier
	Hint string `json:"hint"`
}

type PermissionOptionKind string

const (
	PermissionOptionAllowOnce    PermissionOptionKind = "allow_once"
	PermissionOptionAllowAlways  PermissionOptionKind = "allow_always"
	PermissionOptionRejectOnce   PermissionOptionKind = "reject_once"
	PermissionOptionRejectAlways PermissionOptionKind = "reject_always"
)

type PermissionOption struct {
	MetaCarrier
	OptionID string               `json:"optionId"`
	Name     string               `json:"name"`
	Kind     PermissionOptionKind `json:"kind"`
}

type RequestPermissionParams struct {
	MetaCarrier
	SessionID string             `json:"sessionId"`
	ToolCall  map[string]any     `json:"toolCall"`
	Options   []PermissionOption `json:"options"`
}

type RequestPermissionResult struct {
	MetaCarrier
	Outcome struct {
		Outcome  string `json:"outcome"`
		OptionID string `json:"optionId,omitempty"`
	} `json:"outcome"`
}
