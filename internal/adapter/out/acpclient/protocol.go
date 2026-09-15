package acpclient

// ImplementationInfo describes an ACP peer implementation.
type ImplementationInfo struct {
	Name    string `json:"name"`
	Title   string `json:"title,omitempty"`
	Version string `json:"version,omitempty"`
}

// AgentCapabilities is the subset of stable ACP v1 agent capabilities consumed
// by Protonman Desktop. Keep this transport-facing shape independent from the
// inbound ACP server adapter so Desktop can interoperate with other ACP agents.
type AgentCapabilities struct {
	LoadSession         bool                `json:"loadSession"`
	PromptCapabilities  PromptCapabilities  `json:"promptCapabilities"`
	SessionCapabilities SessionCapabilities `json:"sessionCapabilities"`
	MCPCapabilities     MCPCapabilities     `json:"mcpCapabilities,omitempty"`
}

type PromptCapabilities struct {
	Image           bool `json:"image"`
	Audio           bool `json:"audio"`
	EmbeddedContext bool `json:"embeddedContext"`
}

type SessionCapabilities struct {
	Resume                *struct{} `json:"resume,omitempty"`
	Delete                *struct{} `json:"delete,omitempty"`
	Close                 *struct{} `json:"close,omitempty"`
	AdditionalDirectories *struct{} `json:"additionalDirectories,omitempty"`
}

type MCPCapabilities struct {
	HTTP bool `json:"http,omitempty"`
	SSE  bool `json:"sse,omitempty"`
}

// InitializeResult is returned by ACP initialize.
type InitializeResult struct {
	ProtocolVersion   int               `json:"protocolVersion"`
	AgentCapabilities AgentCapabilities `json:"agentCapabilities"`
	AgentInfo         ImplementationInfo `json:"agentInfo"`
	AuthMethods       []any             `json:"authMethods"`
}

// SessionConfigOption is one ACP session configuration control advertised by
// the agent. Desktop renders these dynamically instead of hard-coding provider
// specific runtime RPCs.
type SessionConfigOption struct {
	ID           string                      `json:"id"`
	Name         string                      `json:"name"`
	Description  string                      `json:"description,omitempty"`
	Category     string                      `json:"category,omitempty"`
	Type         string                      `json:"type"`
	CurrentValue string                      `json:"currentValue"`
	Options      []SessionConfigSelectOption `json:"options,omitempty"`
}

type SessionConfigSelectOption struct {
	Value string `json:"value"`
	Name  string `json:"name"`
}

type SessionNewResult struct {
	SessionID     string                `json:"sessionId"`
	ConfigOptions []SessionConfigOption `json:"configOptions,omitempty"`
}

type SessionResumeResult struct {
	ConfigOptions []SessionConfigOption `json:"configOptions,omitempty"`
}

type SessionSetConfigOptionResult struct {
	ConfigOptions []SessionConfigOption `json:"configOptions,omitempty"`
}
