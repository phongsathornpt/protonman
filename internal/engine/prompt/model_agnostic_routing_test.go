package prompt

import (
	"strings"
	"testing"
)

func TestDelegationProtocolIsModelAgnostic(t *testing.T) {
	section := strings.ToLower(delegationSection(Spec{}))
	assertNoModelSpecificPromptGuidance(t, section)
}

func TestCanonicalPromptIsModelAgnostic(t *testing.T) {
	got := strings.ToLower(Render(Spec{
		AvailableTools: []string{"read", "bash", "todo", "subagent"},
		Capabilities:   ToolCapabilities{Tasks: true, Agents: true, MCP: true},
		Mutations:      MutationCapabilities{Source: true},
	}))
	assertNoModelSpecificPromptGuidance(t, got)
	if strings.Contains(got, "# model guidance") {
		t.Fatalf("canonical prompt contains model guidance section:\n%s", got)
	}
}

func assertNoModelSpecificPromptGuidance(t *testing.T, text string) {
	t.Helper()
	for _, forbidden := range []string{
		"gemini",
		"gpt",
		"qwen",
		"claude",
		"deepseek",
	} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("model-facing prompt contains model-specific guidance %q:\n%s", forbidden, text)
		}
	}
}
