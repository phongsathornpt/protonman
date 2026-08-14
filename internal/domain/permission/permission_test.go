package permission

import (
	"testing"
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
			request:    Request{ToolName: "read_file", ToolKind: ToolRead, Detail: ".env"},
			wantAction: ActionAsk,
		},
		{
			name: "allow applies when no stronger rule matches",
			rules: []Rule{
				{Action: ActionAllow, Tool: ToolRead, Pattern: "*.md"},
			},
			request:    Request{ToolName: "read_file", ToolKind: ToolRead, Detail: "README.md"},
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
				Tool:        ToolWebFetch,
				Pattern:     "*.example.com",
				PatternMode: PatternModeDomain,
			},
		},
	})
	if err != nil {
		t.Fatalf("NewPolicy() error = %v", err)
	}

	decision := policy.Evaluate(Request{
		ToolName: "web_fetch",
		ToolKind: ToolWebFetch,
		Detail:   "https://API.Example.COM/v1/status",
	})
	if decision.Action != ActionAllow {
		t.Fatalf("Evaluate().Action = %s, want allow", decision.Action)
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
