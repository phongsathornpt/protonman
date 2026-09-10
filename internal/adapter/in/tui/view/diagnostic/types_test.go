package diagnostic

import "testing"

func TestUserCodeMapping(t *testing.T) {
	tests := map[Kind]string{
		KindModelNotFound: "MODEL_NOT_FOUND", KindContextOverflow: "CONTEXT_OVERFLOW",
		KindAuthentication: "AUTH_FAILED", KindForbidden: "FORBIDDEN",
		KindRateLimit: "RATE_LIMITED", KindQuotaExceeded: "QUOTA_EXCEEDED",
		KindServerOverloaded: "PROVIDER_OVERLOADED", KindStreamTimeout: "STREAM_TIMEOUT",
		KindRuntimeTimeout: "OPERATION_TIMEOUT", KindInvalidPrompt: "INVALID_PROMPT",
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

func TestFormatSummaryUsesStableCodeInsteadOfUpstreamBadge(t *testing.T) {
	err := Error{Kind: KindServerOverloaded, Title: "Provider Server Overloaded", Badge: "503 SERVER_ERROR", Message: "temporarily unavailable", Code: "503"}
	got := FormatSummary(err)
	want := "[PROVIDER_OVERLOADED] Provider Server Overloaded: temporarily unavailable"
	if got != want {
		t.Fatalf("FormatSummary() = %q, want %q", got, want)
	}
}
