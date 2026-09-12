package tool

import (
	"strings"
)

// CommandConfidence describes how certain BashAnalysis is about a command effect.
type CommandConfidence string

const (
	CommandConfidenceUnknown CommandConfidence = "unknown"
	CommandConfidenceCertain CommandConfidence = "certain"
)

// BashAnalysis is a conservative, shell-aware summary used by permissions,
// execution metadata, and UI. Unknown constructs always fail closed.
type BashAnalysis struct {
	Effect        CommandEffect
	Risk          CommandRisk
	Scope         CommandScope
	Confidence    CommandConfidence
	Reason        string
	AffectedPaths []string
	ConflictProne bool
}

// AnalyzeCommand classifies simple shell commands without pretending to be a
// complete shell parser. Unsupported expansion/control syntax remains unknown.
func AnalyzeCommand(command string) BashAnalysis {
	command = strings.TrimSpace(command)
	if command == "" {
		return unknownBashAnalysis("empty command")
	}
	segments, operators, ok := splitSimpleShell(command)
	if !ok || len(segments) == 0 {
		return unknownBashAnalysis("unsupported shell syntax")
	}
	if len(segments) == 1 {
		return normalizeCommandScope(analyzeSimpleSegment(segments[0]))
	}
	combined := BashAnalysis{Effect: CommandEffectReadOnly, Confidence: CommandConfidenceCertain, Reason: "all command segments are read only"}
	for i, segment := range segments {
		analysis := normalizeCommandScope(analyzeSimpleSegment(segment))
		combined.AffectedPaths = appendUniquePaths(combined.AffectedPaths, analysis.AffectedPaths...)
		combined.ConflictProne = combined.ConflictProne || analysis.ConflictProne
		if commandRiskRank(analysis.Risk) > commandRiskRank(combined.Risk) {
			combined.Risk = analysis.Risk
		}
		if commandScopeRank(analysis.Scope) > commandScopeRank(combined.Scope) {
			combined.Scope = analysis.Scope
		}
		if analysis.Effect == CommandEffectMutating {
			combined.Effect = CommandEffectMutating
			combined.Confidence = analysis.Confidence
			combined.Reason = analysis.Reason
		} else if analysis.Effect == CommandEffectUnknown && combined.Effect != CommandEffectMutating {
			combined = BashAnalysis{Effect: CommandEffectUnknown, Confidence: CommandConfidenceUnknown, Reason: analysis.Reason, AffectedPaths: combined.AffectedPaths, ConflictProne: combined.ConflictProne}
		}
		if i < len(operators) && operators[i] == "|" && analysis.Effect != CommandEffectReadOnly && combined.Effect != CommandEffectMutating {
			conflictProne := combined.ConflictProne
			combined = unknownBashAnalysis("pipeline contains an unknown command")
			combined.ConflictProne = conflictProne
		}
	}
	return combined
}

// VerificationCommand reports whether a shell command is a conservative empirical
// verifier for source changes. It intentionally recognizes only well-known test,
// build, lint, typecheck, and diff-check invocations.
func VerificationCommand(command string) (string, bool) {
	segments, _, ok := splitSimpleShell(strings.TrimSpace(command))
	if !ok || len(segments) == 0 {
		return "", false
	}
	labels := make([]string, 0, len(segments))
	for _, segment := range segments {
		words, redirects, ok := shellWords(segment)
		if !ok || len(words) == 0 || len(redirects) > 0 {
			return "", false
		}
		label, ok := verificationWords(words)
		if !ok {
			return "", false
		}
		labels = append(labels, label)
	}
	return strings.Join(labels, " + "), true
}

func verificationWords(words []string) (string, bool) {
	name := strings.TrimPrefix(words[0], "./")
	args := words[1:]
	switch name {
	case "go":
		if len(args) > 0 && (args[0] == "test" || args[0] == "vet") {
			return "go " + args[0], true
		}
	case "cargo":
		if len(args) > 0 && (args[0] == "test" || args[0] == "check" || args[0] == "clippy") {
			return "cargo " + args[0], true
		}
	case "pytest", "py.test":
		return "pytest", true
	case "npm", "pnpm", "yarn", "bun":
		if len(args) > 0 {
			script := args[0]
			if script == "run" && len(args) > 1 {
				script = args[1]
			}
			if script == "test" || script == "lint" || script == "typecheck" || script == "check" || script == "build" {
				return name + " " + script, true
			}
		}
	case "git":
		if len(args) >= 2 && args[0] == "diff" && args[1] == "--check" {
			return "git diff --check", true
		}
	}
	return "", false
}

// ClassifyCommandEffect preserves the existing API while delegating to the
// richer analyzer.
func ClassifyCommandEffect(command string) CommandEffect { return AnalyzeCommand(command).Effect }

func commandRiskRank(risk CommandRisk) int {
	switch risk {
	case CommandRiskRemoteDestructive:
		return 2
	case CommandRiskDestructive:
		return 1
	default:
		return 0
	}
}

func normalizeCommandScope(analysis BashAnalysis) BashAnalysis {
	if analysis.Effect == CommandEffectMutating && analysis.Scope == CommandScopeUnknown {
		analysis.Scope = CommandScopeLocal
	}
	return analysis
}

func commandScopeRank(scope CommandScope) int {
	switch scope {
	case CommandScopeDeployment:
		return 4
	case CommandScopePublish:
		return 3
	case CommandScopeRemote:
		return 2
	case CommandScopeLocal:
		return 1
	default:
		return 0
	}
}

func unknownBashAnalysis(reason string) BashAnalysis {
	return BashAnalysis{Effect: CommandEffectUnknown, Confidence: CommandConfidenceUnknown, Reason: reason}
}

func analyzeSimpleSegment(segment string) BashAnalysis {
	words, redirects, ok := shellWords(segment)
	if !ok || len(words) == 0 {
		return unknownBashAnalysis("unsupported shell expansion or quoting")
	}
	if len(redirects) > 0 {
		paths := make([]string, 0, len(redirects))
		mutatesFile := false
		for _, path := range redirects {
			if isStreamOnlyRedirect(path) {
				continue
			}
			mutatesFile = true
			if isLiteralWorkspacePath(path) {
				paths = appendUniquePaths(paths, path)
			}
		}
		if mutatesFile {
			return BashAnalysis{Effect: CommandEffectMutating, Confidence: CommandConfidenceCertain, Reason: "shell output redirection modifies state", AffectedPaths: paths}
		}
	}
	name := strings.TrimPrefix(words[0], "./")
	args := words[1:]
	switch name {
	case "pwd", "ls", "cat", "head", "tail", "grep", "rg", "wc", "stat", "file", "realpath", "readlink", "which", "whereis", "env", "printenv", "echo", "printf", "true", "false", "test", "[":
		return BashAnalysis{Effect: CommandEffectReadOnly, Confidence: CommandConfidenceCertain, Reason: name + " is a known read-only command"}
	case "find":
		for _, arg := range args {
			if arg == "-delete" {
				return BashAnalysis{Effect: CommandEffectMutating, Risk: CommandRiskDestructive, Confidence: CommandConfidenceCertain, Reason: "find -delete modifies filesystem state"}
			}
			if arg == "-exec" || arg == "-execdir" || arg == "-ok" || arg == "-okdir" {
				return unknownBashAnalysis("find executes an arbitrary command")
			}
		}
		return BashAnalysis{Effect: CommandEffectReadOnly, Confidence: CommandConfidenceCertain, Reason: "find without execution/delete is read only"}
	case "git":
		return analyzeGitCommand(args)
	case "rm", "rmdir", "truncate":
		return BashAnalysis{Effect: CommandEffectMutating, Risk: CommandRiskDestructive, Confidence: CommandConfidenceCertain, Reason: name + " destructively modifies filesystem state", AffectedPaths: literalMutationPaths(name, args)}
	case "touch", "mkdir", "chmod", "chown", "install", "ln":
		return BashAnalysis{Effect: CommandEffectMutating, Confidence: CommandConfidenceCertain, Reason: name + " modifies filesystem state", AffectedPaths: literalMutationPaths(name, args)}
	case "cp":
		return BashAnalysis{Effect: CommandEffectMutating, Confidence: CommandConfidenceCertain, Reason: "cp modifies filesystem state", AffectedPaths: copyMovePaths(args, false)}
	case "mv":
		return BashAnalysis{Effect: CommandEffectMutating, Confidence: CommandConfidenceCertain, Reason: "mv modifies filesystem state", AffectedPaths: copyMovePaths(args, true)}
	case "tee":
		paths, hasTarget := teePaths(args)
		if !hasTarget {
			return BashAnalysis{Effect: CommandEffectReadOnly, Confidence: CommandConfidenceCertain, Reason: "tee without file operands only writes stdout"}
		}
		return BashAnalysis{Effect: CommandEffectMutating, Confidence: CommandConfidenceCertain, Reason: "tee writes file operands", AffectedPaths: paths}
	case "npm", "pnpm":
		if len(args) > 0 && args[0] == "publish" {
			return BashAnalysis{Effect: CommandEffectMutating, Scope: CommandScopePublish, Confidence: CommandConfidenceCertain, Reason: name + " publish publishes a package"}
		}
	case "yarn":
		if len(args) > 0 && args[0] == "publish" || len(args) > 1 && args[0] == "npm" && args[1] == "publish" {
			return BashAnalysis{Effect: CommandEffectMutating, Scope: CommandScopePublish, Confidence: CommandConfidenceCertain, Reason: "yarn publishes a package"}
		}
	case "cargo":
		if len(args) > 0 && args[0] == "publish" {
			return BashAnalysis{Effect: CommandEffectMutating, Scope: CommandScopePublish, Confidence: CommandConfidenceCertain, Reason: "cargo publish publishes a crate"}
		}
	case "wrangler":
		if len(args) > 0 && args[0] == "deploy" {
			return BashAnalysis{Effect: CommandEffectMutating, Scope: CommandScopeDeployment, Confidence: CommandConfidenceCertain, Reason: "wrangler deploy changes a remote deployment"}
		}
	case "terraform":
		if len(args) > 0 && args[0] == "apply" {
			return BashAnalysis{Effect: CommandEffectMutating, Scope: CommandScopeDeployment, Confidence: CommandConfidenceCertain, Reason: "terraform apply changes remote infrastructure"}
		}
		if len(args) > 0 && args[0] == "destroy" {
			return BashAnalysis{Effect: CommandEffectMutating, Risk: CommandRiskRemoteDestructive, Scope: CommandScopeDeployment, Confidence: CommandConfidenceCertain, Reason: "terraform destroy destructively changes remote infrastructure"}
		}
	case "kubectl":
		if len(args) > 0 && args[0] == "apply" {
			return BashAnalysis{Effect: CommandEffectMutating, Scope: CommandScopeDeployment, Confidence: CommandConfidenceCertain, Reason: "kubectl apply changes a remote deployment"}
		}
		if len(args) > 0 && args[0] == "delete" {
			return BashAnalysis{Effect: CommandEffectMutating, Risk: CommandRiskRemoteDestructive, Scope: CommandScopeDeployment, Confidence: CommandConfidenceCertain, Reason: "kubectl delete destructively changes a remote deployment"}
		}
	default:
		return unknownBashAnalysis("command effect is not proven")
	}
	return unknownBashAnalysis(name + " subcommand effect is not proven")
}
