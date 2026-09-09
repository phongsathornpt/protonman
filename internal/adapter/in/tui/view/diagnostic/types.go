package diagnostic

// Kind classifies provider/runtime failures for user-facing presentation.
type Kind string

const (
	KindModelNotFound    Kind = "model_not_found"
	KindContextOverflow  Kind = "context_overflow"
	KindAuthentication   Kind = "authentication"
	KindForbidden        Kind = "forbidden"
	KindRateLimit        Kind = "rate_limit"
	KindQuotaExceeded    Kind = "quota_exceeded"
	KindServerOverloaded Kind = "server_overloaded"
	KindStreamTimeout    Kind = "stream_timeout"
	KindRuntimeTimeout   Kind = "runtime_timeout"
	KindInvalidPrompt    Kind = "invalid_prompt"
	KindMCPFailed        Kind = "mcp_failed"
	KindConfigInvalid    Kind = "config_invalid"
	KindConfigTypo       Kind = "config_typo"
	KindToolFailed       Kind = "tool_failed"
	KindToolDispatch     Kind = "tool_dispatch"
	KindPermissionDenied Kind = "permission_denied"
	KindCancelled        Kind = "cancelled"
	KindGeneric          Kind = "generic"
)

// Error is a structured, user-friendly failure with actionable guidance.
type Error struct {
	Kind        Kind
	Title       string
	Badge       string
	Message     string
	Suggestions []string
	RawDetails  string
	Code        string
	Retryable   bool
}
