package tool

import (
	"slices"
	"testing"
)

func TestAnalyzeCommandFailClosedSyntax(t *testing.T) {
	for _, command := range []string{
		"",
		"   ",
		"echo a; echo b",
		"echo $(ls)",
		"echo `ls`",
		"echo $HOME",
		"echo hi & echo bye",
		"(echo hi)",
		"echo {a}",
		"echo hi\nbye",
		"pwd &&",
		"&& pwd",
		`echo "unterminated`,
		"echo hi >",
		"cat < f",
	} {
		got := AnalyzeCommand(command)
		if got.Effect != CommandEffectUnknown {
			t.Fatalf("AnalyzeCommand(%q).Effect = %q, want unknown (reason=%s)", command, got.Effect, got.Reason)
		}
		if got.Confidence != CommandConfidenceUnknown {
			t.Fatalf("AnalyzeCommand(%q).Confidence = %q, want unknown", command, got.Confidence)
		}
	}
}

func TestAnalyzeCommandUnknownAndProvenEffects(t *testing.T) {
	unknown := []string{
		"python3 script.py",
		"npm install",
		"yarn build",
		"cargo build",
		"terraform plan",
		"kubectl get pods",
		"wrangler dev",
		"git frobnicate",
		"git",
		"git -C",
		"find . -exec ls",
		"find . -execdir ls",
		"find . -ok ls",
		"find . -okdir ls",
	}
	for _, command := range unknown {
		got := AnalyzeCommand(command)
		if got.Effect != CommandEffectUnknown {
			t.Fatalf("AnalyzeCommand(%q).Effect = %q, want unknown (reason=%s)", command, got.Effect, got.Reason)
		}
	}
}

func TestAnalyzeCommandGitOptionHandling(t *testing.T) {
	for _, command := range []string{
		"git --no-pager status",
		"git -C /tmp status",
		"git -C /tmp --no-pager status",
	} {
		got := AnalyzeCommand(command)
		if got.Effect != CommandEffectReadOnly {
			t.Fatalf("AnalyzeCommand(%q).Effect = %q, want read_only (reason=%s)", command, got.Effect, got.Reason)
		}
	}
}

func TestAnalyzeCommandCompositionUnknownFailClosed(t *testing.T) {
	tests := []struct {
		command string
		reason  string
	}{
		{"git status && python3 x.py", "command effect is not proven"},
		{"python3 x.py && git status", "command effect is not proven"},
		{"echo hi || unknowncmd", "command effect is not proven"},
		{"cat foo | unknowncmd", "command effect is not proven"},
		{"unknowncmd | cat", "pipeline contains an unknown command"},
	}
	for _, tt := range tests {
		got := AnalyzeCommand(tt.command)
		if got.Effect != CommandEffectUnknown {
			t.Fatalf("AnalyzeCommand(%q).Effect = %q, want unknown (reason=%s)", tt.command, got.Effect, got.Reason)
		}
		if got.Reason != tt.reason {
			t.Fatalf("AnalyzeCommand(%q).Reason = %q, want %q", tt.command, got.Reason, tt.reason)
		}
	}
	for _, command := range []string{
		"pwd || git status",
		"pwd |& cat",
		"git status | cat",
	} {
		got := AnalyzeCommand(command)
		if got.Effect != CommandEffectReadOnly {
			t.Fatalf("AnalyzeCommand(%q).Effect = %q, want read_only (reason=%s)", command, got.Effect, got.Reason)
		}
	}
}

func TestAnalyzeCommandCompositionRiskAndScope(t *testing.T) {
	tests := []struct {
		command string
		risk    CommandRisk
		scope   CommandScope
	}{
		{"rm x && git push --force origin main", CommandRiskRemoteDestructive, CommandScopeRemote},
		{"rm x && git push origin main", CommandRiskDestructive, CommandScopeRemote},
		{"git push origin main && npm publish", CommandRiskNormal, CommandScopePublish},
		{"npm publish && terraform apply", CommandRiskNormal, CommandScopeDeployment},
	}
	for _, tt := range tests {
		got := AnalyzeCommand(tt.command)
		if got.Effect != CommandEffectMutating {
			t.Fatalf("AnalyzeCommand(%q).Effect = %q, want mutating (reason=%s)", tt.command, got.Effect, got.Reason)
		}
		if got.Risk != tt.risk {
			t.Fatalf("AnalyzeCommand(%q).Risk = %q, want %q", tt.command, got.Risk, tt.risk)
		}
		if got.Scope != tt.scope {
			t.Fatalf("AnalyzeCommand(%q).Scope = %q, want %q", tt.command, got.Scope, tt.scope)
		}
	}
}

func TestAnalyzeCommandPathDedupAndHelpers(t *testing.T) {
	got := AnalyzeCommand("rm a.txt a.txt")
	if !slices.Equal(got.AffectedPaths, []string{"a.txt"}) {
		t.Fatalf("AnalyzeCommand(%q).AffectedPaths = %#v", "rm a.txt a.txt", got.AffectedPaths)
	}
	got = AnalyzeCommand("cp a.txt")
	if got.Effect != CommandEffectMutating {
		t.Fatalf("AnalyzeCommand(%q).Effect = %q, want mutating", "cp a.txt", got.Effect)
	}
	if len(got.AffectedPaths) != 0 {
		t.Fatalf("AnalyzeCommand(%q).AffectedPaths = %#v, want empty", "cp a.txt", got.AffectedPaths)
	}
	got = AnalyzeCommand("echo hi > .")
	if got.Effect != CommandEffectMutating {
		t.Fatalf("AnalyzeCommand(%q).Effect = %q, want mutating", "echo hi > .", got.Effect)
	}
	if len(got.AffectedPaths) != 0 {
		t.Fatalf("AnalyzeCommand(%q).AffectedPaths = %#v, want empty", "echo hi > .", got.AffectedPaths)
	}
	if !slices.Equal(appendUniquePaths(nil, "a.txt", "a.txt"), []string{"a.txt"}) {
		t.Fatal("appendUniquePaths did not dedupe")
	}
	if len(appendUniquePaths(nil, ".", " ")) != 0 {
		t.Fatal("appendUniquePaths did not skip empty/dot paths")
	}
	if allDigits("") {
		t.Fatal("allDigits(\"\") = true, want false")
	}
	if !allDigits("12") {
		t.Fatal("allDigits(\"12\") = false, want true")
	}
	if allDigits("1x") {
		t.Fatal("allDigits(\"1x\") = true, want false")
	}
}

func TestVerificationCommandFailClosedSyntax(t *testing.T) {
	for _, command := range []string{
		"",
		"go test ./...; git diff --check",
		"echo hi >",
		"pwd || git status",
	} {
		if label, ok := VerificationCommand(command); ok || label != "" {
			t.Fatalf("VerificationCommand(%q) = (%q,%v), want (\"\",false)", command, label, ok)
		}
	}
}

func TestAnalyzeCommandShellEscapeAndTeeFlags(t *testing.T) {
	got := AnalyzeCommand(`echo a\ b`)
	if got.Effect != CommandEffectReadOnly {
		t.Fatalf("AnalyzeCommand(%q).Effect = %q, want read_only (reason=%s)", `echo a\ b`, got.Effect, got.Reason)
	}
	got = AnalyzeCommand("echo hi | tee -a out.txt")
	if got.Effect != CommandEffectMutating {
		t.Fatalf("AnalyzeCommand(%q).Effect = %q, want mutating (reason=%s)", "echo hi | tee -a out.txt", got.Effect, got.Reason)
	}
	if !slices.Equal(got.AffectedPaths, []string{"out.txt"}) {
		t.Fatalf("AnalyzeCommand(%q).AffectedPaths = %#v, want [out.txt]", "echo hi | tee -a out.txt", got.AffectedPaths)
	}
}

func TestShellWordsUnterminatedQuote(t *testing.T) {
	if _, _, ok := shellWords(`echo "unterminated`); ok {
		t.Fatal("shellWords() = ok, want failure for unterminated quote")
	}
}
