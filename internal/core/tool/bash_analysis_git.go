package tool

import "strings"

func analyzeGitCommand(args []string) BashAnalysis {
	for len(args) > 0 && strings.HasPrefix(args[0], "-") {
		if args[0] == "-C" || args[0] == "-c" || args[0] == "--git-dir" || args[0] == "--work-tree" {
			if len(args) < 2 {
				return unknownBashAnalysis("incomplete git option")
			}
			args = args[2:]
			continue
		}
		args = args[1:]
	}
	if len(args) == 0 {
		return unknownBashAnalysis("git subcommand is missing")
	}
	switch args[0] {
	case "status", "diff", "log", "show", "rev-parse", "rev-list", "ls-files", "ls-tree", "grep", "describe":
		return BashAnalysis{Effect: CommandEffectReadOnly, Confidence: CommandConfidenceCertain, Reason: "git " + args[0] + " is read only"}
	case "branch":
		if classifyGitBranchCommand(args[1:]) == CommandEffectReadOnly {
			return BashAnalysis{Effect: CommandEffectReadOnly, Confidence: CommandConfidenceCertain, Reason: "git branch invocation is read only"}
		}
		return BashAnalysis{Effect: CommandEffectMutating, Risk: gitCommandRisk(args[0], args[1:]), Confidence: CommandConfidenceCertain, Reason: "git branch invocation modifies repository state"}
	case "add", "apply", "checkout", "switch", "restore", "reset", "clean", "commit", "merge", "rebase", "cherry-pick", "revert", "stash", "tag", "fetch", "pull", "push":
		scope := CommandScopeLocal
		if args[0] == "push" {
			scope = CommandScopeRemote
		}
		conflictProne := args[0] == "apply" || args[0] == "merge" || args[0] == "rebase" || args[0] == "cherry-pick"
		return BashAnalysis{Effect: CommandEffectMutating, Risk: gitCommandRisk(args[0], args[1:]), Scope: scope, Confidence: CommandConfidenceCertain, Reason: "git " + args[0] + " modifies repository or remote state", ConflictProne: conflictProne}
	default:
		return unknownBashAnalysis("git subcommand effect is not proven")
	}
}

func gitCommandRisk(subcommand string, args []string) CommandRisk {
	hasShortFlag := func(flag byte) bool {
		for _, arg := range args {
			if len(arg) > 1 && strings.HasPrefix(arg, "-") && !strings.HasPrefix(arg, "--") && strings.ContainsRune(arg[1:], rune(flag)) {
				return true
			}
		}
		return false
	}
	hasArg := func(want string) bool {
		for _, arg := range args {
			if arg == want || strings.HasPrefix(arg, want+"=") {
				return true
			}
		}
		return false
	}
	switch subcommand {
	case "reset":
		if hasArg("--hard") {
			return CommandRiskDestructive
		}
	case "clean":
		if hasArg("--force") || hasShortFlag('f') {
			return CommandRiskDestructive
		}
	case "checkout", "switch":
		if hasArg("--force") || hasShortFlag('f') || hasArg("--") {
			return CommandRiskDestructive
		}
	case "restore":
		return CommandRiskDestructive
	case "branch":
		if hasArg("--delete") || hasShortFlag('d') || hasShortFlag('D') {
			return CommandRiskDestructive
		}
	case "stash":
		for _, arg := range args {
			if arg == "drop" || arg == "clear" {
				return CommandRiskDestructive
			}
		}
	case "push":
		if hasArg("--force") || hasArg("--force-with-lease") || hasShortFlag('f') {
			return CommandRiskRemoteDestructive
		}
	}
	return CommandRiskNormal
}

func classifyGitBranchCommand(args []string) CommandEffect {
	if len(args) == 0 {
		return CommandEffectReadOnly
	}
	for _, arg := range args {
		switch {
		case arg == "--list", arg == "-l", arg == "--show-current", arg == "--contains", arg == "--no-contains", arg == "--merged", arg == "--no-merged", strings.HasPrefix(arg, "--format="):
			continue
		default:
			return CommandEffectMutating
		}
	}
	return CommandEffectReadOnly
}
