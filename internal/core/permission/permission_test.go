package permission

import (
	"testing"

	"github.com/phongsathornpt/protonman/internal/core/tool"
)

func TestPolicyPrecedence(t *testing.T) {
	tests := []struct {
		name       string
		rules      []Rule
		request    Request
		wantAction Action
	}{
		{
			name: "deny beats allow regardless of rule order",
			rules: []Rule{
				{Action: ActionAllow, Tool: ToolBash, Pattern: "git *"},
				{Action: ActionDeny, Tool: ToolBash, Pattern: "git push *"},
			},
			request:    Request{ToolName: "bash", ToolKind: ToolBash, Detail: "git push origin main"},
			wantAction: ActionDeny,
		},
		{
			name: "ask beats allow",
			rules: []Rule{
				{Action: ActionAllow, Tool: ToolRead},
				{Action: ActionAsk, Tool: ToolAny, Pattern: "*.env"},
			},
			request:    Request{ToolName: "read", ToolKind: ToolRead, Detail: ".env"},
			wantAction: ActionAsk,
		},
		{
			name: "allow applies when no stronger rule matches",
			rules: []Rule{
				{Action: ActionAllow, Tool: ToolRead, Pattern: "*.md"},
			},
			request:    Request{ToolName: "read", ToolKind: ToolRead, Detail: "README.md"},
			wantAction: ActionAllow,
		},
		{
			name: "default asks",
			request: Request{
				ToolName: "bash",
				ToolKind: ToolBash,
				Detail:   "pwd",
			},
			wantAction: ActionAsk,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			policy, err := NewPolicy(Config{Rules: test.rules})
			if err != nil {
				t.Fatalf("NewPolicy() error = %v", err)
			}
			decision := policy.Evaluate(test.request)
			if decision.Action != test.wantAction {
				t.Fatalf("Evaluate().Action = %s, want %s", decision.Action, test.wantAction)
			}
		})
	}
}

func TestPolicyGlobMatchesCommandPaths(t *testing.T) {
	policy, err := NewPolicy(Config{
		Rules: []Rule{
			{Action: ActionDeny, Tool: ToolBash, Pattern: "rm *"},
		},
	})
	if err != nil {
		t.Fatalf("NewPolicy() error = %v", err)
	}

	decision := policy.Evaluate(Request{
		ToolName: "bash",
		ToolKind: ToolBash,
		Detail:   "rm -rf ./build/cache",
	})
	if decision.Action != ActionDeny {
		t.Fatalf("Evaluate().Action = %s, want deny", decision.Action)
	}
}

func TestPolicyDomainPatternNormalizesHost(t *testing.T) {
	policy, err := NewPolicy(Config{
		Rules: []Rule{
			{
				Action:      ActionAllow,
				Tool:        ToolWeb,
				Pattern:     "*.example.com",
				PatternMode: PatternModeDomain,
			},
		},
	})
	if err != nil {
		t.Fatalf("NewPolicy() error = %v", err)
	}

	decision := policy.Evaluate(Request{
		ToolName: "web",
		ToolKind: ToolWeb,
		Detail:   "https://API.Example.COM/v1/status",
	})
	if decision.Action != ActionAllow {
		t.Fatalf("Evaluate().Action = %s, want allow", decision.Action)
	}
}

func TestPolicyDomainPatternIsCaseInsensitive(t *testing.T) {
	policy, err := NewPolicy(Config{
		Rules: []Rule{
			{
				Action:      ActionDeny,
				Tool:        ToolWeb,
				Pattern:     "*.Example.COM",
				PatternMode: PatternModeDomain,
			},
		},
		Default: ActionAllow,
	})
	if err != nil {
		t.Fatalf("NewPolicy() error = %v", err)
	}

	decision := policy.Evaluate(Request{
		ToolName: "web",
		ToolKind: ToolWeb,
		Detail:   "https://api.example.com/v1/status",
	})
	if decision.Action != ActionDeny {
		t.Fatalf("Evaluate().Action = %s, want deny", decision.Action)
	}
}

func TestNewPolicyRejectsInvalidRule(t *testing.T) {
	_, err := NewPolicy(Config{
		Rules: []Rule{{Action: ActionUnknown, Tool: ToolBash}},
	})
	if err == nil {
		t.Fatal("NewPolicy() error = nil, want invalid config error")
	}
}

func TestNewPolicyDefaultsEmptyToolToAny(t *testing.T) {
	policy, err := NewPolicy(Config{
		Rules: []Rule{{Action: ActionAllow, Tool: ""}},
	})
	if err != nil {
		t.Fatalf("NewPolicy() error = %v", err)
	}
	dec := policy.Evaluate(Request{ToolName: "bash", ToolKind: ToolBash})
	if dec.Action != ActionAllow {
		t.Fatalf("Evaluate().Action = %s, want allow", dec.Action)
	}
}

func TestActionEnumAndTextMarshaling(t *testing.T) {
	actions := []Action{ActionAllow, ActionDeny, ActionAsk}
	for _, a := range actions {
		if !a.Valid() {
			t.Fatalf("expected action %s to be valid", a)
		}
		text, err := a.MarshalText()
		if err != nil {
			t.Fatalf("MarshalText() error = %v", err)
		}
		var decoded Action
		if err := decoded.UnmarshalText(text); err != nil {
			t.Fatalf("UnmarshalText() error = %v", err)
		}
		if decoded != a {
			t.Fatalf("round-trip failed: got %s, want %s", decoded, a)
		}
	}
	if ActionUnknown.Valid() {
		t.Fatal("ActionUnknown should not be valid")
	}
	if Action(99).Valid() {
		t.Fatal("Action(99) should not be valid")
	}
	var invalid Action
	if err := invalid.UnmarshalText([]byte("invalid")); err == nil {
		t.Fatal("UnmarshalText(invalid) error = nil, want error")
	}
}

func TestModeEnumAndTextMarshaling(t *testing.T) {
	modes := []Mode{ModeAsk, ModeAuto, ModeAlwaysApprove, ModeDeny}
	for _, m := range modes {
		if !m.Valid() {
			t.Fatalf("expected mode %s to be valid", m)
		}
		text, err := m.MarshalText()
		if err != nil {
			t.Fatalf("MarshalText() error = %v", err)
		}
		var decoded Mode
		if err := decoded.UnmarshalText(text); err != nil {
			t.Fatalf("UnmarshalText() error = %v", err)
		}
		if decoded != m {
			t.Fatalf("round-trip failed: got %s, want %s", decoded, m)
		}
	}
	if ModeUnknown.Valid() {
		t.Fatal("ModeUnknown should not be valid")
	}
	if Mode(99).Valid() {
		t.Fatal("Mode(99) should not be valid")
	}
	var invalid Mode
	if err := invalid.UnmarshalText([]byte("invalid")); err == nil {
		t.Fatal("UnmarshalText(invalid) error = nil, want error")
	}
}

func TestPatternModeEnumAndTextMarshaling(t *testing.T) {
	patternModes := []PatternMode{PatternModeGlob, PatternModeDomain}
	for _, pm := range patternModes {
		if !pm.Valid() {
			t.Fatalf("expected pattern mode %s to be valid", pm)
		}
		text, err := pm.MarshalText()
		if err != nil {
			t.Fatalf("MarshalText() error = %v", err)
		}
		var decoded PatternMode
		if err := decoded.UnmarshalText(text); err != nil {
			t.Fatalf("UnmarshalText() error = %v", err)
		}
		if decoded != pm {
			t.Fatalf("round-trip failed: got %s, want %s", decoded, pm)
		}
	}
	if PatternModeUnknown.Valid() {
		t.Fatal("PatternModeUnknown should not be valid")
	}
	if PatternMode(99).Valid() {
		t.Fatal("PatternMode(99) should not be valid")
	}
	var invalid PatternMode
	if err := invalid.UnmarshalText([]byte("invalid")); err == nil {
		t.Fatal("UnmarshalText(invalid) error = nil, want error")
	}
}

func TestGrantScopeEnumAndTextMarshaling(t *testing.T) {
	scopes := []GrantScope{GrantScopeOnce, GrantScopeSession}
	for _, s := range scopes {
		if !s.Valid() {
			t.Fatalf("expected grant scope %s to be valid", s)
		}
		text, err := s.MarshalText()
		if err != nil {
			t.Fatalf("MarshalText() error = %v", err)
		}
		var decoded GrantScope
		if err := decoded.UnmarshalText(text); err != nil {
			t.Fatalf("UnmarshalText() error = %v", err)
		}
		if decoded != s {
			t.Fatalf("round-trip failed: got %s, want %s", decoded, s)
		}
	}
	if GrantScopeUnknown.Valid() {
		t.Fatal("GrantScopeUnknown should not be valid")
	}
	if GrantScope(99).Valid() {
		t.Fatal("GrantScope(99) should not be valid")
	}
	var invalid GrantScope
	if err := invalid.UnmarshalText([]byte("invalid")); err == nil {
		t.Fatal("UnmarshalText(invalid) error = nil, want error")
	}
}

func TestToolKindValidationAndParsing(t *testing.T) {
	kinds := []ToolKind{
		ToolAny, ToolRead, ToolEdit, ToolBash,
		ToolGrep, ToolMCP, ToolWeb,
		ToolTask, ToolAgent, ToolCompute,
	}
	for _, k := range kinds {
		if !ValidToolKind(k) {
			t.Fatalf("expected tool kind %q to be valid", k)
		}
		parsed, err := ParseToolKind(string(k))
		if err != nil {
			t.Fatalf("ParseToolKind(%q) error = %v", k, err)
		}
		if parsed != k {
			t.Fatalf("parsed = %q, want %q", parsed, k)
		}
	}
	if ValidToolKind("unsupported") {
		t.Fatal("expected unsupported tool kind to be invalid")
	}
	if _, err := ParseToolKind("unsupported"); err == nil {
		t.Fatal("ParseToolKind(unsupported) error = nil, want error")
	}
}

func TestComputeIsAllowedByDefaultButExplicitRulesWin(t *testing.T) {
	policy, err := NewPolicy(Config{Default: ActionAsk})
	if err != nil {
		t.Fatal(err)
	}
	request := Request{ToolName: "math", ToolKind: ToolCompute, Detail: "sqrt(144)"}
	if got := policy.Evaluate(request); got.Action != ActionAllow {
		t.Fatalf("default compute action = %v, want allow", got.Action)
	}
	denied, err := NewPolicy(Config{Default: ActionAsk, Rules: []Rule{{Action: ActionDeny, Tool: ToolCompute, Pattern: "*"}}})
	if err != nil {
		t.Fatal(err)
	}
	if got := denied.Evaluate(request); got.Action != ActionDeny {
		t.Fatalf("explicit compute deny = %v, want deny", got.Action)
	}
}

func TestMCPToolPatternMatching(t *testing.T) {
	policy, err := NewPolicy(Config{
		Default: ActionAsk,
		Rules: []Rule{
			{Action: ActionAllow, Tool: ToolMCP, Pattern: "mcp.github.*"},
			{Action: ActionDeny, Tool: ToolMCP, Pattern: "filesystem.*"},
			{Action: ActionAllow, Tool: ToolMCP, Pattern: "*secret*"},
		},
	})
	if err != nil {
		t.Fatalf("NewPolicy() error = %v", err)
	}

	tests := []struct {
		name     string
		toolName string
		detail   string
		want     Action
	}{
		{name: "allow full prefix", toolName: "mcp.github.get_repo", detail: "{}", want: ActionAllow},
		{name: "deny stripped prefix", toolName: "mcp.filesystem.read", detail: "{}", want: ActionDeny},
		{name: "allow argument match", toolName: "mcp.custom.query", detail: "has secret data", want: ActionAllow},
		{name: "default ask unmatched", toolName: "mcp.slack.post", detail: "{}", want: ActionAsk},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decision := policy.Evaluate(Request{
				ToolName: tt.toolName,
				ToolKind: ToolMCP,
				Detail:   tt.detail,
			})
			if decision.Action != tt.want {
				t.Fatalf("Evaluate() = %v, want %v", decision.Action, tt.want)
			}
		})
	}
}

func TestMCPRulePatternsAreCanonicalized(t *testing.T) {
	policy, err := NewPolicy(Config{Default: ActionAsk, Rules: []Rule{
		{Action: ActionAllow, Tool: ToolMCP, Pattern: "github.search"},
		{Action: ActionDeny, Tool: ToolMCP, Pattern: "mcp.github.delete"},
		{Action: ActionAllow, Tool: ToolMCP, Pattern: "github.*"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"mcp.github.search", "mcp.github.delete", "mcp.github.*"}
	for i, pattern := range want {
		if policy.rules[i].Pattern != pattern {
			t.Fatalf("rule %d pattern = %q, want %q", i, policy.rules[i].Pattern, pattern)
		}
	}
	cases := []struct {
		name string
		want Action
	}{
		{"mcp.github.search", ActionAllow},
		{"mcp.github.delete", ActionDeny},
		{"mcp.github.issues", ActionAllow},
	}
	for _, tc := range cases {
		decision := policy.Evaluate(Request{ToolName: tc.name, ToolKind: ToolMCP})
		if decision.Action != tc.want {
			t.Fatalf("%s action = %v, want %v", tc.name, decision.Action, tc.want)
		}
	}
}

func TestRuleFromRequestAndPersistentEligibility(t *testing.T) {
	tests := []struct {
		name         string
		req          Request
		wantEligible bool
		wantRuleOK   bool
		wantRuleTool ToolKind
		wantRulePat  string
		wantRuleMode PatternMode
	}{
		{
			name: "read-only bash is eligible",
			req: Request{
				ToolName: "bash",
				ToolKind: ToolBash,
				Detail:   "git status --short",
				Effect:   tool.CommandEffectReadOnly,
				Risk:     tool.CommandRiskNormal,
			},
			wantEligible: true,
			wantRuleOK:   true,
			wantRuleTool: ToolBash,
			wantRulePat:  "git status --short",
			wantRuleMode: PatternModeGlob,
		},
		{
			name: "mutating bash is ineligible",
			req: Request{
				ToolName: "bash",
				ToolKind: ToolBash,
				Detail:   "rm -rf tmp",
				Effect:   tool.CommandEffectMutating,
				Risk:     tool.CommandRiskNormal,
			},
			wantEligible: false,
			wantRuleOK:   true,
		},
		{
			name: "destructive bash is ineligible",
			req: Request{
				ToolName: "bash",
				ToolKind: ToolBash,
				Detail:   "drop database",
				Effect:   tool.CommandEffectReadOnly,
				Risk:     tool.CommandRiskDestructive,
			},
			wantEligible: false,
			wantRuleOK:   true,
		},
		{
			name: "read is eligible",
			req: Request{
				ToolName: "read",
				ToolKind: ToolRead,
				Detail:   "internal/tui/theme.go",
				Effect:   tool.CommandEffectReadOnly,
				Risk:     tool.CommandRiskNormal,
			},
			wantEligible: true,
			wantRuleOK:   true,
			wantRuleTool: ToolRead,
			wantRulePat:  "internal/tui/theme.go",
			wantRuleMode: PatternModeGlob,
		},
		{
			name: "grep is eligible",
			req: Request{
				ToolName: "grep",
				ToolKind: ToolGrep,
				Detail:   "src",
				Effect:   tool.CommandEffectReadOnly,
				Risk:     tool.CommandRiskNormal,
			},
			wantEligible: true,
			wantRuleOK:   true,
			wantRuleTool: ToolGrep,
			wantRulePat:  "src",
			wantRuleMode: PatternModeGlob,
		},
		{
			name: "web normalizes domain",
			req: Request{
				ToolName: "web",
				ToolKind: ToolWeb,
				Detail:   "https://API.GitHub.COM/repos/owner/repo",
				Effect:   tool.CommandEffectReadOnly,
				Risk:     tool.CommandRiskNormal,
			},
			wantEligible: true,
			wantRuleOK:   true,
			wantRuleTool: ToolWeb,
			wantRulePat:  "api.github.com",
			wantRuleMode: PatternModeDomain,
		},
		{
			name: "mcp tool is eligible",
			req: Request{
				ToolName: "mcp.github.get_repo",
				ToolKind: ToolMCP,
				Detail:   `{"owner":"foo"}`,
				Effect:   tool.CommandEffectReadOnly,
				Risk:     tool.CommandRiskNormal,
			},
			wantEligible: true,
			wantRuleOK:   true,
			wantRuleTool: ToolMCP,
			wantRulePat:  "mcp.github.get_repo",
			wantRuleMode: PatternModeGlob,
		},
		{
			name: "empty detail is ineligible",
			req: Request{
				ToolName: "bash",
				ToolKind: ToolBash,
				Detail:   "",
				Effect:   tool.CommandEffectReadOnly,
				Risk:     tool.CommandRiskNormal,
			},
			wantEligible: false,
			wantRuleOK:   false,
		},
		{
			name: "multiline command is rejected for rule",
			req: Request{
				ToolName: "bash",
				ToolKind: ToolBash,
				Detail:   "echo hello\necho world",
				Effect:   tool.CommandEffectReadOnly,
				Risk:     tool.CommandRiskNormal,
			},
			wantEligible: false,
			wantRuleOK:   false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := PersistentRuleEligible(tc.req); got != tc.wantEligible {
				t.Fatalf("PersistentRuleEligible() = %v, want %v", got, tc.wantEligible)
			}
			rule, ok := RuleFromRequest(tc.req)
			if ok != tc.wantRuleOK {
				t.Fatalf("RuleFromRequest() ok = %v, want %v", ok, tc.wantRuleOK)
			}
			if ok && tc.wantRuleOK {
				if rule.Action != ActionAllow {
					t.Fatalf("rule.Action = %v, want %v", rule.Action, ActionAllow)
				}
				if tc.wantRuleTool != "" && rule.Tool != tc.wantRuleTool {
					t.Fatalf("rule.Tool = %v, want %v", rule.Tool, tc.wantRuleTool)
				}
				if tc.wantRulePat != "" && rule.Pattern != tc.wantRulePat {
					t.Fatalf("rule.Pattern = %v, want %v", rule.Pattern, tc.wantRulePat)
				}
				if tc.wantRuleMode != PatternModeUnknown && rule.PatternMode != tc.wantRuleMode {
					t.Fatalf("rule.PatternMode = %v, want %v", rule.PatternMode, tc.wantRuleMode)
				}
			}
		})
	}
}

func TestPolicyAddRuleThreadSafe(t *testing.T) {
	policy, err := NewPolicy(Config{
		Default: ActionAsk,
		Rules:   []Rule{{Action: ActionDeny, Tool: ToolBash, Pattern: "rm *"}},
	})
	if err != nil {
		t.Fatal(err)
	}

	req := Request{
		ToolName: "bash",
		ToolKind: ToolBash,
		Detail:   "git status",
	}

	// Initially asks
	if d := policy.Evaluate(req); d.Action != ActionAsk {
		t.Fatalf("initial action = %v, want ask", d.Action)
	}

	// Add allow rule
	err = policy.AddRule(Rule{
		Action:  ActionAllow,
		Tool:    ToolBash,
		Pattern: "git status",
	})
	if err != nil {
		t.Fatalf("AddRule() error = %v", err)
	}

	// Now allows
	if d := policy.Evaluate(req); d.Action != ActionAllow {
		t.Fatalf("action after AddRule = %v, want allow", d.Action)
	}

	// Adding duplicate is no-op
	if err := policy.AddRule(Rule{Action: ActionAllow, Tool: ToolBash, Pattern: "git status"}); err != nil {
		t.Fatalf("duplicate AddRule() error = %v", err)
	}
	if len(policy.Rules()) != 2 {
		t.Fatalf("rules count = %d, want 2", len(policy.Rules()))
	}

	// Deny rule still beats added allow rule
	denyReq := Request{
		ToolName: "bash",
		ToolKind: ToolBash,
		Detail:   "rm foo",
	}
	if d := policy.Evaluate(denyReq); d.Action != ActionDeny {
		t.Fatalf("denyReq action = %v, want deny", d.Action)
	}
}

func TestNormalizePatternAndWildcardAll(t *testing.T) {
	tests := []struct {
		kind    ToolKind
		mode    PatternMode
		pattern string
		want    string
	}{
		{ToolBash, PatternModeGlob, "all", "*"},
		{ToolBash, PatternModeGlob, "ALL", "*"},
		{ToolBash, PatternModeGlob, "*", "*"},
		{ToolBash, PatternModeGlob, "", "*"},
		{ToolBash, PatternModeGlob, "git status", "git status"},
		{ToolWeb, PatternModeDomain, "all", "*"},
		{ToolWeb, PatternModeDomain, "API.GitHub.COM", "api.github.com"},
		{ToolMCP, PatternModeGlob, "all", "*"},
		{ToolMCP, PatternModeGlob, "github.search", "mcp.github.search"},
	}
	for _, tc := range tests {
		if got := NormalizePattern(tc.kind, tc.mode, tc.pattern); got != tc.want {
			t.Errorf("NormalizePattern(%s, %s, %q) = %q, want %q", tc.kind, tc.mode, tc.pattern, got, tc.want)
		}
	}
}

func TestPolicyAllowBashAllMatchesAnyCommand(t *testing.T) {
	policy, err := NewPolicy(Config{
		Default: ActionAsk,
		Rules: []Rule{
			{Action: ActionAllow, Tool: ToolBash, Pattern: "all"},
			{Action: ActionDeny, Tool: ToolBash, Pattern: "sudo *"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Normal commands allowed
	for _, cmd := range []string{"ls -la", "cargo build", "npm test", "git log"} {
		d := policy.Evaluate(Request{ToolName: "bash", ToolKind: ToolBash, Detail: cmd})
		if d.Action != ActionAllow {
			t.Errorf("command %q action = %v, want allow", cmd, d.Action)
		}
	}

	// Specific deny still respected
	d := policy.Evaluate(Request{ToolName: "bash", ToolKind: ToolBash, Detail: "sudo rm -rf /"})
	if d.Action != ActionDeny {
		t.Errorf("sudo command action = %v, want deny", d.Action)
	}
}
