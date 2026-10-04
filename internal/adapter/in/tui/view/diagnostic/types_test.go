package diagnostic

import "testing"

func TestUserCodeMapping(t *testing.T) {
	tests := map[Kind]string{
		KindModelNotFound: "MODEL_NOT_FOUND", KindContextOverflow: "CONTEXT_OVERFLOW",
		KindAuthentication: "AUTH_FAILED", KindForbidden: "FORBIDDEN",
		KindRateLimit: "RATE_LIMITED", KindQuotaExceeded: "QUOTA_EXCEEDED",
		KindServerOverloaded: "PROVIDER_OVERLOADED", KindStreamTimeout: "STREAM_TIMEOUT",
		KindStreamIncomplete: "STREAM_INCOMPLETE",
		KindEmptyResponse:    "EMPTY_RESPONSE",
		KindRuntimeTimeout:   "OPERATION_TIMEOUT", KindInvalidPrompt: "INVALID_PROMPT",
		KindMCPFailed: "MCP_FAILED", KindConfigInvalid: "CONFIG_INVALID",
		KindConfigTypo: "CONFIG_TYPO", KindToolFailed: "TOOL_FAILED",
		KindToolDispatch: "TOOL_DISPATCH", KindPermissionDenied: "PERMISSION_DENIED",
		KindCancelled: "CANCELLED", KindGeneric: "ERROR",
	}
	for kind, want := range tests {
		if got := UserCode(kind); got != want {
			t.Errorf("UserCode(%q) = %q, want %q", kind, got, want)
		}
	}
}
