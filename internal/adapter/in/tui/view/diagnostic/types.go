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
	KindStreamIncomplete Kind = "stream_incomplete"
	KindEmptyResponse    Kind = "empty_response"
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

// UserCode returns the stable, provider-independent code shown in the TUI.
// Transport status and provider-specific codes remain available through Error.Code,
// Badge, and RawDetails for diagnostics without leaking unstable upstream wording.
func UserCode(kind Kind) string {
	switch kind {
	case KindModelNotFound:
		return "MODEL_NOT_FOUND"
	case KindContextOverflow:
		return "CONTEXT_OVERFLOW"
	case KindAuthentication:
		return "AUTH_FAILED"
	case KindForbidden:
		return "FORBIDDEN"
	case KindRateLimit:
		return "RATE_LIMITED"
	case KindQuotaExceeded:
		return "QUOTA_EXCEEDED"
	case KindServerOverloaded:
		return "PROVIDER_OVERLOADED"
	case KindStreamTimeout:
		return "STREAM_TIMEOUT"
	case KindStreamIncomplete:
		return "STREAM_INCOMPLETE"
	case KindEmptyResponse:
		return "EMPTY_RESPONSE"
	case KindRuntimeTimeout:
		return "OPERATION_TIMEOUT"
	case KindInvalidPrompt:
		return "INVALID_PROMPT"
	case KindMCPFailed:
		return "MCP_FAILED"
	case KindConfigInvalid:
		return "CONFIG_INVALID"
	case KindConfigTypo:
		return "CONFIG_TYPO"
	case KindToolFailed:
		return "TOOL_FAILED"
	case KindToolDispatch:
		return "TOOL_DISPATCH"
	case KindPermissionDenied:
		return "PERMISSION_DENIED"
	case KindCancelled:
		return "CANCELLED"
	default:
		return "ERROR"
	}
}
