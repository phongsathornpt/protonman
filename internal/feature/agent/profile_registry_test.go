package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/phongsathornpt/protonman/internal/core/tool"
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
		"read_file":     dummyHandler{def: tool.Definition{Name: "read_file", Kind: tool.KindRead}},
		"find_files":    dummyHandler{def: tool.Definition{Name: "find_files", Kind: tool.KindRead}},
		"grep":          dummyHandler{def: tool.Definition{Name: "grep", Kind: tool.KindGrep}},
		"web":           dummyHandler{def: tool.Definition{Name: "web", Kind: tool.KindWeb}},
		"write_file":    dummyHandler{def: tool.Definition{Name: "write_file", Kind: tool.KindEdit}},
		"bash":          dummyHandler{def: tool.Definition{Name: "bash", Kind: tool.KindBash}},
		"delegate_task": dummyHandler{def: tool.Definition{Name: "delegate_task", Kind: tool.KindAgent}},
		"update_todo":   dummyHandler{def: tool.Definition{Name: "update_todo", Kind: tool.KindTask}},
		"mcp.read":      dummyHandler{def: tool.Definition{Name: "mcp.read", Kind: tool.KindMCP, Mutability: tool.MutabilityReadOnly}},
		"mcp.write":     dummyHandler{def: tool.Definition{Name: "mcp.write", Kind: tool.KindMCP, Mutability: tool.MutabilityMutating}},
		"mcp.unknown":   dummyHandler{def: tool.Definition{Name: "mcp.unknown", Kind: tool.KindMCP}},
	}}

	for _, profile := range []Profile{ProfileStrength, ProfileIntelligence} {
		scoped := FilterRegistryForProfile(baseReg, profile)
		for _, name := range []string{"read_file", "find_files", "grep", "web", "write_file", "bash", "mcp.read", "mcp.write", "mcp.unknown"} {
			if _, ok := scoped.Lookup(name); !ok {
				t.Errorf("%s missing tool %s", profile, name)
			}
		}
		for _, name := range []string{"delegate_task", "update_todo"} {
			if _, ok := scoped.Lookup(name); ok {
				t.Errorf("%s must not expose %s", profile, name)
			}
		}
	}

	intScoped := FilterRegistryForProfile(baseReg, ProfileAgility)
	for _, name := range []string{"read_file", "find_files", "grep", "web", "mcp.read"} {
		if _, ok := intScoped.Lookup(name); !ok {
			t.Errorf("int missing tool %s", name)
		}
	}
	for _, name := range []string{"write_file", "bash", "delegate_task", "update_todo", "mcp.write", "mcp.unknown"} {
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
		prompt := SystemPromptForProfile(profile)
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

func TestParseProfileNormalizesLegacyAliases(t *testing.T) {
	cases := map[string]Profile{"pow": ProfileStrength, "worker": ProfileStrength, "int": ProfileAgility, "explorer": ProfileAgility, "reviewer": ProfileAgility, "dex": ProfileIntelligence}
	for raw, want := range cases {
		got, err := ParseProfile(raw)
		if err != nil || got != want {
			t.Fatalf("ParseProfile(%q) = %q, %v; want %q", raw, got, err, want)
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
		"Tool identifiers are exact",
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
		prompt := SystemPromptForProfile(profile)
		if !strings.Contains(prompt, "Tool identifiers are exact") {
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
