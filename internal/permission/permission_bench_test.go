package permission

import (
	"testing"

	"github.com/projectTHORN/proton/internal/glob"
)

func BenchmarkGlobMatch_Wildcard(b *testing.B) {
	pattern := "*"
	value := "some/long/nested/path/to/a/workspace/file.go"
	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		_ = glob.Match(pattern, value)
	}
}

func BenchmarkGlobMatch_Prefix(b *testing.B) {
	pattern := "rm *"
	value := "rm -rf /tmp/test-dir/created-file.txt"
	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		_ = glob.Match(pattern, value)
	}
}

func BenchmarkGlobMatch_Suffix(b *testing.B) {
	pattern := "*.go"
	value := "internal/permission/permission.go"
	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		_ = glob.Match(pattern, value)
	}
}

func BenchmarkGlobMatch_Complex(b *testing.B) {
	pattern := "*perm*.*go"
	value := "internal/permission/permission.go"
	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		_ = glob.Match(pattern, value)
	}
}

func BenchmarkPolicyEvaluate(b *testing.B) {
	config := Config{
		Default: ActionAsk,
		Rules: []Rule{
			{Action: ActionDeny, Tool: ToolBash, Pattern: "rm *"},
			{Action: ActionDeny, Tool: ToolBash, Pattern: "sudo *"},
			{Action: ActionDeny, Tool: ToolEdit, Pattern: "*.pem"},
			{Action: ActionAllow, Tool: ToolRead, Pattern: "*.md"},
			{Action: ActionAllow, Tool: ToolRead, Pattern: "*.go"},
			{Action: ActionAllow, Tool: ToolGrep, Pattern: "*"},
			{Action: ActionAllow, Tool: ToolWebFetch, Pattern: "github.com", PatternMode: PatternModeDomain},
		},
	}
	policy, err := NewPolicy(config)
	if err != nil {
		b.Fatalf("NewPolicy: %v", err)
	}
	req := Request{
		ToolName: "read_file",
		ToolKind: ToolRead,
		Detail:   "README.md",
	}

	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		_ = policy.Evaluate(req)
	}
}
