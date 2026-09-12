package turn

import (
	"testing"

	sdk "github.com/phongsathornpt/protonman/proton-sdk"
)

func TestPromptCacheHitPercent(t *testing.T) {
	tests := []struct {
		name  string
		usage sdk.Usage
		want  float64
	}{
		{name: "no input", usage: sdk.Usage{}, want: 0},
		{name: "no cache hit", usage: sdk.Usage{InputTokens: 100}, want: 0},
		{name: "partial hit", usage: sdk.Usage{InputTokens: 100, CachedInputTokens: 75}, want: 75},
		{name: "fractional hit", usage: sdk.Usage{InputTokens: 3, CachedInputTokens: 1}, want: 100.0 / 3.0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := promptCacheHitPercent(tt.usage); got != tt.want {
				t.Fatalf("promptCacheHitPercent(%#v) = %v, want %v", tt.usage, got, tt.want)
			}
		})
	}
}
