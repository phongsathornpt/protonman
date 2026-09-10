package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/phongsathornpt/protonman/internal/core/tool"
	"github.com/phongsathornpt/protonman/internal/engine/prompt"
	sdk "github.com/phongsathornpt/protonman/proton-sdk"
)

type dummyHandler struct {
	def tool.Definition
}

func (d dummyHandler) Definition() tool.Definition {
	return d.def
}

func (d dummyHandler) Execute(_ context.Context, call tool.Call) (tool.Result, error) {
	return tool.Result{CallID: call.ID}, nil
}

type staticRegistry struct {
	handlers map[string]tool.Handler
}

func (s staticRegistry) Lookup(name string) (tool.Handler, bool) {
	h, ok := s.handlers[name]
	return h, ok
}

func (s staticRegistry) Definitions() []tool.Definition {
	defs := make([]tool.Definition, 0, len(s.handlers))
	for _, h := range s.handlers {
		defs = append(defs, h.Definition())
	}
	return defs
}

func TestFilterRegistryForCanonicalProfiles(t *testing.T) {
	baseReg := staticRegistry{handlers: map[string]tool.Handler{
		"read":        dummyHandler{def: tool.Definition{Name: "read", Kind: tool.KindRead}},
		"find":        dummyHandler{def: tool.Definition{Name: "find", Kind: tool.KindRead}},
		"grep":        dummyHandler{def: tool.Definition{Name: "grep", Kind: tool.KindGrep}},
		"web":         dummyHandler{def: tool.Definition{Name: "web", Kind: tool.KindWeb}},
		"edit":        dummyHandler{def: tool.Definition{Name: "edit", Kind: tool.KindEdit}},
		"bash":        dummyHandler{def: tool.Definition{Name: "bash", Kind: tool.KindBash}},
		"subagent":    dummyHandler{def: tool.Definition{Name: "subagent", Kind: tool.KindAgent}},
		"todo":        dummyHandler{def: tool.Definition{Name: "todo", Kind: tool.KindTask}},
		"mcp.read":    dummyHandler{def: tool.Definition{Name: "mcp.read", Kind: tool.KindMCP, Mutability: tool.MutabilityReadOnly}},
		"mcp.write":   dummyHandler{def: tool.Definition{Name: "mcp.write", Kind: tool.KindMCP, Mutability: tool.MutabilityMutating}},
		"mcp.unknown": dummyHandler{def: tool.Definition{Name: "mcp.unknown", Kind: tool.KindMCP}},
	}}

	for _, profile := range []Profile{ProfileStrength, ProfileIntelligence} {
		scoped := FilterRegistryForProfile(baseReg, profile)
		for _, name := range []string{"read", "find", "grep", "web", "edit", "bash", "mcp.read", "mcp.write", "mcp.unknown"} {
			if _, ok := scoped.Lookup(name); !ok {
				t.Errorf("%s missing tool %s", profile, name)
			}
		}
		for _, name := range []string{"subagent", "todo"} {
			if _, ok := scoped.Lookup(name); ok {
				t.Errorf("%s must not expose %s", profile, name)
			}
		}
	}

	intScoped := FilterRegistryForProfile(baseReg, ProfileAgility)
	for _, name := range []string{"read", "find", "grep", "web", "mcp.read"} {
		if _, ok := intScoped.Lookup(name); !ok {
			t.Errorf("int missing tool %s", name)
		}
	}
	for _, name := range []string{"edit", "bash", "subagent", "todo", "mcp.write", "mcp.unknown"} {
		if _, ok := intScoped.Lookup(name); ok {
			t.Errorf("int must not expose %s", name)
		}
	}
}

func TestSystemPromptForProfileBehaviorContracts(t *testing.T) {
	checks := map[Profile][]string{
		ProfileStrength:     {"implementation subagent", "smallest coherent change", "project conventions", "validation"},
		ProfileAgility:      {"fast read-only exploration subagent", "minimum evidence", "Do not modify workspace files", "confidence"},
		ProfileIntelligence: {"deep engineering and reasoning subagent", "invariants and constraints", "Compare viable solutions", "material risks"},
	}
	for profile, markers := range checks {
		prompt := prompt.Render(prompt.Spec{Role: RolePromptForProfile(profile), Profile: string(profile)})
		for _, marker := range markers {
			if !strings.Contains(prompt, marker) {
				t.Errorf("profile %s missing behavior marker %q", profile, marker)
			}
		}
	}
}

func TestProfileSpecsAreCanonicalAndComplete(t *testing.T) {
	want := []Profile{ProfileUniversal, ProfileStrength, ProfileAgility, ProfileIntelligence}
	got := SupportedProfiles()
	if len(got) != len(want) {
		t.Fatalf("supported profiles = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("supported profiles = %v, want %v", got, want)
		}
	}
}

func TestParseProfileRejectsOldNames(t *testing.T) {
	for _, raw := range []string{"pow", "worker", "int", "explorer", "reviewer", "dex"} {
		if _, err := ParseProfile(raw); err == nil {
			t.Fatalf("ParseProfile(%q) error = nil, want error", raw)
		}
	}
}

func TestDefaultSystemPromptGroundsCodingToolUse(t *testing.T) {
	prompt := DefaultSystemPrompt()
	for _, marker := range []string{
		"UNIVERSAL",
		"primary software engineering agent",
		"Never guess workspace contents",
		"Inspect relevant code",
		"perform the edits",
		"Tool and action identifiers are exact",
		"never prefix, rename, qualify, or invent",
		"verify the result",
	} {
		if !strings.Contains(prompt, marker) {
			t.Fatalf("default system prompt missing %q:\n%s", marker, prompt)
		}
	}
}

func TestProfilePromptsIncludeSharedToolContract(t *testing.T) {
	for _, profile := range SupportedProfiles() {
		prompt := prompt.Render(prompt.Spec{Role: RolePromptForProfile(profile), Profile: string(profile)})
		if !strings.Contains(prompt, "Tool and action identifiers are exact") {
			t.Fatalf("profile %q missing shared tool contract", profile)
		}
	}
}

func TestProfileSpecsDeclarePortableReasoningEffort(t *testing.T) {
	want := map[Profile]sdk.ReasoningEffort{
		ProfileUniversal:    sdk.ReasoningMedium,
		ProfileStrength:     sdk.ReasoningMedium,
		ProfileAgility:      sdk.ReasoningMedium,
		ProfileIntelligence: sdk.ReasoningHigh,
	}
	for profile, effort := range want {
		spec, ok := SpecForProfile(profile)
		if !ok || spec.Reasoning != effort {
			t.Fatalf("SpecForProfile(%q).Reasoning = %q, want %q", profile, spec.Reasoning, effort)
		}
	}
}

func TestProfileSpecsRequireWorkspaceGrounding(t *testing.T) {
	for _, profile := range SubagentProfiles() {
		spec, ok := SpecForProfile(profile)
		if !ok {
			t.Fatalf("missing spec for %q", profile)
		}
		if got := spec.GroundingEvidence; got != tool.EvidenceWorkspace {
			t.Fatalf("profile %q grounding = %q, want workspace", profile, got)
		}
	}
}

func TestSubagentProfilesExcludeUniversal(t *testing.T) {
	got := SubagentProfiles()
	want := []Profile{ProfileStrength, ProfileAgility, ProfileIntelligence}
	if len(got) != len(want) {
		t.Fatalf("subagent profiles = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("subagent profiles = %v, want %v", got, want)
		}
	}
}
