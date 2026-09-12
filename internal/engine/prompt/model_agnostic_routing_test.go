package prompt

import (
	"strings"
	"testing"
)

func TestDelegationProtocolIsModelAgnostic(t *testing.T) {
	section := strings.ToLower(delegationSection(Spec{}))
	for _, forbidden := range []string{
		"gemini",
		"gpt",
		"qwen",
		"claude",
		"deepseek",
	} {
		if strings.Contains(section, forbidden) {
			t.Fatalf("delegation protocol contains model-specific guidance %q:\n%s", forbidden, section)
		}
	}
}
