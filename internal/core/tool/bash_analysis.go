package tool

import (
	"path/filepath"
	"strings"
	"unicode"
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

func splitSimpleShell(command string) ([]string, []string, bool) {
	var segments, operators []string
	var b strings.Builder
	var quote rune
	for i := 0; i < len(command); i++ {
		c := rune(command[i])
		if quote != 0 {
			if quote == '"' && (c == '`' || (c == '$' && i+1 < len(command) && command[i+1] == '(')) {
				return nil, nil, false
			}
			b.WriteByte(command[i])
			if c == quote {
				quote = 0
			}
			continue
		}
		if c == '\'' || c == '"' {
			quote = c
			b.WriteByte(command[i])
			continue
		}
		if c == '`' || c == '$' || c == '(' || c == ')' || c == '{' || c == '}' || c == '\n' {
			return nil, nil, false
		}
		if c == ';' {
			return nil, nil, false
		}
		if c == '&' {
			if i > 0 && command[i-1] == '>' {
				b.WriteByte(command[i])
				continue
			}
			if i+1 < len(command) && command[i+1] == '>' {
				b.WriteByte(command[i])
				continue
			}
			if i+1 >= len(command) || command[i+1] != '&' {
				return nil, nil, false
			}
			segments = append(segments, strings.TrimSpace(b.String()))
			b.Reset()
			operators = append(operators, "&&")
			i++
			continue
		}
		if c == '|' {
			if i > 0 && command[i-1] == '>' {
				b.WriteByte(command[i])
				continue
			}
			if i+1 < len(command) && command[i+1] == '|' {
				segments = append(segments, strings.TrimSpace(b.String()))
				b.Reset()
				operators = append(operators, "||")
				i++
				continue
			}
			if i+1 < len(command) && command[i+1] == '&' {
				segments = append(segments, strings.TrimSpace(b.String()))
				b.Reset()
				operators = append(operators, "|&")
				i++
				continue
			}
			segments = append(segments, strings.TrimSpace(b.String()))
			b.Reset()
			operators = append(operators, "|")
			continue
		}
		b.WriteByte(command[i])
	}
	if quote != 0 {
		return nil, nil, false
	}
	segments = append(segments, strings.TrimSpace(b.String()))
	for _, s := range segments {
		if s == "" {
			return nil, nil, false
		}
	}
	return segments, operators, true
}

func shellWords(segment string) ([]string, []string, bool) {
	var words, redirects []string
	var b strings.Builder
	var quote rune
	flush := func() {
		if b.Len() > 0 {
			words = append(words, b.String())
			b.Reset()
		}
	}
	for i := 0; i < len(segment); i++ {
		c := rune(segment[i])
		if quote != 0 {
			if c == quote {
				quote = 0
			} else {
				b.WriteByte(segment[i])
			}
			continue
		}
		if c == '\'' || c == '"' {
			quote = c
			continue
		}
		if unicode.IsSpace(c) {
			flush()
			continue
		}
		if c == '&' && i+1 < len(segment) && segment[i+1] == '>' {
			continue
		}
		if c == '>' {
			if b.Len() > 0 && allDigits(b.String()) {
				b.Reset()
			} else {
				flush()
			}
			if i+1 < len(segment) && (segment[i+1] == '>' || segment[i+1] == '|') {
				i++
			}
			for i+1 < len(segment) && unicode.IsSpace(rune(segment[i+1])) {
				i++
			}
			j := i + 1
			for j < len(segment) && !unicode.IsSpace(rune(segment[j])) {
				j++
			}
			if j == i+1 {
				return nil, nil, false
			}
			redirects = append(redirects, strings.Trim(segment[i+1:j], "'\""))
			i = j - 1
			continue
		}
		if c == '<' {
			return nil, nil, false
		}
		if c == '\\' && i+1 < len(segment) {
			i++
			b.WriteByte(segment[i])
			continue
		}
		b.WriteByte(segment[i])
	}
	if quote != 0 {
		return nil, nil, false
	}
	flush()
	return words, redirects, true
}

func literalMutationPaths(name string, args []string) []string {
	paths := make([]string, 0, len(args))
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") || !isLiteralWorkspacePath(arg) {
			continue
		}
		paths = appendUniquePaths(paths, arg)
	}
	return paths
}

func copyMovePaths(args []string, includeSource bool) []string {
	pos := make([]string, 0, len(args))
	for _, arg := range args {
		if !strings.HasPrefix(arg, "-") && isLiteralWorkspacePath(arg) {
			pos = append(pos, arg)
		}
	}
	if len(pos) < 2 {
		return nil
	}
	if includeSource {
		return appendUniquePaths(nil, pos...)
	}
	return appendUniquePaths(nil, pos[len(pos)-1])
}

func isStreamOnlyRedirect(target string) bool {
	target = strings.TrimSpace(target)
	if strings.HasPrefix(target, "&") {
		if len(target) > 1 && allDigits(target[1:]) {
			return true
		}
	}
	switch target {
	case "/dev/null", "/dev/stdout", "/dev/stderr":
		return true
	default:
		return false
	}
}

func allDigits(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func teePaths(args []string) ([]string, bool) {
	paths := make([]string, 0, len(args))
	hasTarget := false
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") {
			continue
		}
		hasTarget = true
		if isLiteralWorkspacePath(arg) {
			paths = appendUniquePaths(paths, arg)
		}
	}
	return paths, hasTarget
}

func isLiteralWorkspacePath(path string) bool {
	path = strings.TrimSpace(path)
	if path == "" || filepath.IsAbs(path) || strings.ContainsAny(path, "$*?[]{}~`)\n\r") {
		return false
	}
	clean := filepath.Clean(path)
	return clean != "." && clean != ".." && !strings.HasPrefix(clean, ".."+string(filepath.Separator))
}

func appendUniquePaths(dst []string, paths ...string) []string {
	for _, path := range paths {
		path = filepath.Clean(strings.TrimSpace(path))
		if path == "" || path == "." {
			continue
		}
		seen := false
		for _, existing := range dst {
			if existing == path {
				seen = true
				break
			}
		}
		if !seen {
			dst = append(dst, path)
		}
	}
	return dst
}
