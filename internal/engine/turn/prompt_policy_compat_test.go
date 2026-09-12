package turn

import (
	"testing"

	"github.com/phongsathornpt/protonman/internal/engine/prompt"
)

func TestWithSystemPromptSpecDiscardsLegacyModelPromptHints(t *testing.T) {
	loop := &Loop{}
	option := WithSystemPromptSpec(prompt.Spec{
		ModelPromptHints: []string{"provider-specific prose"},
		AvailableTools:   []string{"read"},
	})
	if err := option(loop); err != nil {
		t.Fatalf("WithSystemPromptSpec() error = %v", err)
	}
	if loop.promptSpec == nil {
		t.Fatal("prompt spec was not stored")
	}
	if len(loop.promptSpec.ModelPromptHints) != 0 {
		t.Fatalf("legacy model prompt hints survived turn option: %v", loop.promptSpec.ModelPromptHints)
	}
	if len(loop.promptSpec.AvailableTools) != 1 || loop.promptSpec.AvailableTools[0] != "read" {
		t.Fatalf("unrelated prompt spec fields changed: %+v", loop.promptSpec)
	}
}
