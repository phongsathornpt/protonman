package failure

import (
	"testing"

	sdk "github.com/phongsathornpt/proton/proton-sdk"
)

func TestClassifyProviderUsesCentralModelCodes(t *testing.T) {
	tests := []struct {
		err  error
		code Code
		text string
	}{
		{sdk.NewProviderError("openai", 400, "invalid_request", "unsupported field"), CodeModelInvalidRequest, "invalid model request"},
		{sdk.NewProviderError("openai", 429, "rate_limit", "slow down"), CodeModelRateLimited, "rate limited"},
		{sdk.NewProviderError("openai", 404, "model_not_found", "missing"), CodeModelUnavailable, "model unavailable"},
	}
	for _, tt := range tests {
		got, ok := ClassifyProvider(tt.err)
		if !ok || got.Code != tt.code {
			t.Fatalf("ClassifyProvider(%v) = %+v, %v", tt.err, got, ok)
		}
		if Summary(got.Code) != tt.text {
			t.Fatalf("Summary(%q) = %q, want %q", got.Code, Summary(got.Code), tt.text)
		}
	}
}
