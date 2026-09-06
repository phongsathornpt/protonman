package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/projectTHORN/proton/internal/tool"
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

func TestFilterRegistryForProfile(t *testing.T) {
	baseHandlers := map[string]tool.Handler{
		"read_file":     dummyHandler{def: tool.Definition{Name: "read_file", Kind: tool.KindRead, Description: "read"}},
		"list_dir":      dummyHandler{def: tool.Definition{Name: "list_dir", Kind: tool.KindRead, Description: "list"}},
		"grep":          dummyHandler{def: tool.Definition{Name: "grep", Kind: tool.KindGrep, Description: "grep"}},
		"git_status":    dummyHandler{def: tool.Definition{Name: "git_status", Kind: tool.KindRead, Description: "git"}},
		"web_fetch":     dummyHandler{def: tool.Definition{Name: "web_fetch", Kind: tool.KindWebFetch, Description: "fetch"}},
		"write_file":    dummyHandler{def: tool.Definition{Name: "write_file", Kind: tool.KindEdit, Description: "write"}},
		"apply_patch":   dummyHandler{def: tool.Definition{Name: "apply_patch", Kind: tool.KindEdit, Description: "patch"}},
		"bash":          dummyHandler{def: tool.Definition{Name: "bash", Kind: tool.KindBash, Description: "bash"}},
		"delegate_task": dummyHandler{def: tool.Definition{Name: "delegate_task", Kind: tool.KindAgent, Description: "delegate"}},
		"wait_agent":    dummyHandler{def: tool.Definition{Name: "wait_agent", Kind: tool.KindAgent, Description: "wait"}},
		"update_todo":   dummyHandler{def: tool.Definition{Name: "update_todo", Kind: tool.KindTask, Description: "todo"}},
		"future_admin":  dummyHandler{def: tool.Definition{Name: "future_admin", Kind: tool.KindMCP, Description: "future"}},
	}
	baseReg := staticRegistry{handlers: baseHandlers}

	t.Run("explorer profile scoping", func(t *testing.T) {
		scoped := FilterRegistryForProfile(baseReg, ProfileExplorer)
		defs := scoped.Definitions()

		// Explorer should have read_file, list_dir, grep, git_status, web_fetch (5 tools)
		allowedTools := map[string]bool{
			"read_file":  true,
			"list_dir":   true,
			"grep":       true,
			"git_status": true,
			"web_fetch":  true,
		}
		for _, d := range defs {
			if !allowedTools[d.Name] {
				t.Errorf("explorer has unauthorized tool: %s", d.Name)
			}
		}
		// Must not have mutating tools or delegate_task
		for _, blocked := range []string{"write_file", "apply_patch", "bash", "delegate_task", "wait_agent", "update_todo"} {
			if _, ok := scoped.Lookup(blocked); ok {
				t.Errorf("explorer lookup for %q succeeded, want blocked", blocked)
			}
		}
	})

	t.Run("reviewer profile scoping", func(t *testing.T) {
		scoped := FilterRegistryForProfile(baseReg, ProfileReviewer)

		// Reviewer must NOT have web_fetch (no network access), mutating tools, or delegate_task
		for _, blocked := range []string{"web_fetch", "write_file", "apply_patch", "bash", "delegate_task", "wait_agent", "update_todo"} {
			if _, ok := scoped.Lookup(blocked); ok {
				t.Errorf("reviewer lookup for %q succeeded, want blocked", blocked)
			}
		}
		// Reviewer must have read_file, list_dir, grep, git_status
		for _, allowed := range []string{"read_file", "list_dir", "grep", "git_status"} {
			if _, ok := scoped.Lookup(allowed); !ok {
				t.Errorf("reviewer missing tool: %s", allowed)
			}
		}
	})

	t.Run("worker profile scoping", func(t *testing.T) {
		scoped := FilterRegistryForProfile(baseReg, ProfileWorker)

		// Worker can edit, read, and run bash
		for _, allowed := range []string{"read_file", "write_file", "apply_patch", "bash"} {
			if _, ok := scoped.Lookup(allowed); !ok {
				t.Errorf("worker missing tool: %s", allowed)
			}
		}
		// Worker at depth 1 MUST NOT have delegate_task
		for _, name := range []string{"delegate_task", "wait_agent", "update_todo", "future_admin"} {
			if _, ok := scoped.Lookup(name); ok {
				t.Errorf("worker should not have %s", name)
			}
		}
	})

	t.Run("pow profile scoping", func(t *testing.T) {
		scoped := FilterRegistryForProfile(baseReg, ProfilePOW)
		for _, allowed := range []string{"read_file", "write_file", "apply_patch", "bash"} {
			if _, ok := scoped.Lookup(allowed); !ok {
				t.Errorf("pow missing tool: %s", allowed)
			}
		}
		for _, name := range []string{"delegate_task", "wait_agent", "update_todo", "future_admin"} {
			if _, ok := scoped.Lookup(name); ok {
				t.Errorf("pow should not have %s", name)
			}
		}
	})

	t.Run("dex profile scoping", func(t *testing.T) {
		scoped := FilterRegistryForProfile(baseReg, ProfileDEX)
		for _, allowed := range []string{"read_file", "write_file", "apply_patch", "bash"} {
			if _, ok := scoped.Lookup(allowed); !ok {
				t.Errorf("dex missing tool: %s", allowed)
			}
		}
		for _, name := range []string{"delegate_task", "wait_agent", "update_todo", "future_admin"} {
			if _, ok := scoped.Lookup(name); ok {
				t.Errorf("dex should not have %s", name)
			}
		}
	})

	t.Run("int profile scoping", func(t *testing.T) {
		scoped := FilterRegistryForProfile(baseReg, ProfileINT)
		for _, allowed := range []string{"read_file", "list_dir", "grep", "git_status", "web_fetch"} {
			if _, ok := scoped.Lookup(allowed); !ok {
				t.Errorf("int missing tool: %s", allowed)
			}
		}
		for _, blocked := range []string{"write_file", "apply_patch", "bash", "delegate_task", "wait_agent", "update_todo"} {
			if _, ok := scoped.Lookup(blocked); ok {
				t.Errorf("int lookup for %q succeeded, want blocked", blocked)
			}
		}
	})
}

func TestSystemPromptForProfile(t *testing.T) {
	profiles := []Profile{
		ProfileExplorer,
		ProfileReviewer,
		ProfileWorker,
		ProfilePOW,
		ProfileDEX,
		ProfileINT,
	}

	for _, p := range profiles {
		prompt := SystemPromptForProfile(p)
		if len(prompt) == 0 {
			t.Errorf("SystemPromptForProfile(%q) returned empty prompt", p)
		}
		if prompt == "You are a helpful assistant." {
			t.Errorf("SystemPromptForProfile(%q) fell back to default prompt", p)
		}
	}

	// Verify principles and J-Space cognitive architecture markers in pow, dex, int
	powPrompt := SystemPromptForProfile(ProfilePOW)
	if !strings.Contains(powPrompt, "POW Mode") || !strings.Contains(powPrompt, "High Velocity") {
		t.Errorf("pow prompt missing POW Mode marker: %s", powPrompt)
	}
	for _, marker := range []string{"Capacity Limit", "1-Line", "Action-First"} {
		if !strings.Contains(powPrompt, marker) {
			t.Errorf("pow prompt missing cognitive marker %q: %s", marker, powPrompt)
		}
	}

	dexPrompt := SystemPromptForProfile(ProfileDEX)
	if !strings.Contains(dexPrompt, "DEX Mode") || !strings.Contains(dexPrompt, "Defensive Engineering") {
		t.Errorf("dex prompt missing DEX Mode marker: %s", dexPrompt)
	}
	for _, marker := range []string{"Empirical Escape", "Named Verifier", "Metacognitive"} {
		if !strings.Contains(dexPrompt, marker) {
			t.Errorf("dex prompt missing cognitive marker %q: %s", marker, dexPrompt)
		}
	}

	intPrompt := SystemPromptForProfile(ProfileINT)
	if !strings.Contains(intPrompt, "INT Mode") || !strings.Contains(intPrompt, "YAGNI") {
		t.Errorf("int prompt missing YAGNI or INT Mode marker: %s", intPrompt)
	}
	for _, marker := range []string{"Broadcast Hub", "Bridge-Before-Conclusion", "Architectural De-escalation"} {
		if !strings.Contains(intPrompt, marker) {
			t.Errorf("int prompt missing cognitive marker %q: %s", marker, intPrompt)
		}
	}
}

func TestProfileSpecsAreCanonicalAndComplete(t *testing.T) {
	seen := map[Profile]bool{}
	for _, profile := range SupportedProfiles() {
		if seen[profile] {
			t.Fatalf("duplicate profile spec for %q", profile)
		}
		seen[profile] = true
		spec, ok := SpecForProfile(profile)
		if !ok || spec.Profile != profile {
			t.Fatalf("SpecForProfile(%q) = %+v, %v", profile, spec, ok)
		}
		if strings.TrimSpace(spec.Description) == "" {
			t.Fatalf("profile %q has empty description", profile)
		}
		if strings.TrimSpace(SystemPromptForProfile(profile)) == "" {
			t.Fatalf("profile %q has empty system prompt", profile)
		}
	}
	if got, want := len(seen), 6; got != want {
		t.Fatalf("profile spec count = %d, want %d", got, want)
	}
}
