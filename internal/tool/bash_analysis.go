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
	Confidence    CommandConfidence
	Reason        string
	AffectedPaths []string
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
	combined := BashAnalysis{Effect: CommandEffectReadOnly, Confidence: CommandConfidenceCertain, Reason: "all command segments are read only"}
	for i, segment := range segments {
		analysis := analyzeSimpleSegment(segment)
		combined.AffectedPaths = appendUniquePaths(combined.AffectedPaths, analysis.AffectedPaths...)
		if analysis.Effect == CommandEffectMutating {
			combined.Effect = CommandEffectMutating
			combined.Confidence = analysis.Confidence
			combined.Reason = analysis.Reason
		} else if analysis.Effect == CommandEffectUnknown && combined.Effect != CommandEffectMutating {
			combined = BashAnalysis{Effect: CommandEffectUnknown, Confidence: CommandConfidenceUnknown, Reason: analysis.Reason, AffectedPaths: combined.AffectedPaths}
		}
		if i < len(operators) && operators[i] == "|" && analysis.Effect != CommandEffectReadOnly && combined.Effect != CommandEffectMutating {
			combined = unknownBashAnalysis("pipeline contains an unknown command")
		}
	}
	return combined
}

// ClassifyCommandEffect preserves the existing API while delegating to the
// richer analyzer.
func ClassifyCommandEffect(command string) CommandEffect { return AnalyzeCommand(command).Effect }

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
		for _, path := range redirects {
			if isLiteralWorkspacePath(path) {
				paths = appendUniquePaths(paths, path)
			}
		}
		return BashAnalysis{Effect: CommandEffectMutating, Confidence: CommandConfidenceCertain, Reason: "shell output redirection modifies state", AffectedPaths: paths}
	}
	name := strings.TrimPrefix(words[0], "./")
	args := words[1:]
	switch name {
	case "pwd", "ls", "cat", "head", "tail", "grep", "rg", "wc", "stat", "file", "realpath", "readlink", "which", "whereis", "env", "printenv":
		return BashAnalysis{Effect: CommandEffectReadOnly, Confidence: CommandConfidenceCertain, Reason: name + " is a known read-only command"}
	case "find":
		for _, arg := range args {
			if arg == "-delete" {
				return BashAnalysis{Effect: CommandEffectMutating, Confidence: CommandConfidenceCertain, Reason: "find -delete modifies filesystem state"}
			}
			if arg == "-exec" || arg == "-execdir" || arg == "-ok" || arg == "-okdir" {
				return unknownBashAnalysis("find executes an arbitrary command")
			}
		}
		return BashAnalysis{Effect: CommandEffectReadOnly, Confidence: CommandConfidenceCertain, Reason: "find without execution/delete is read only"}
	case "git":
		return analyzeGitCommand(args)
	case "rm", "rmdir", "touch", "mkdir", "chmod", "chown", "truncate", "install", "ln":
		return BashAnalysis{Effect: CommandEffectMutating, Confidence: CommandConfidenceCertain, Reason: name + " modifies filesystem state", AffectedPaths: literalMutationPaths(name, args)}
	case "cp":
		return BashAnalysis{Effect: CommandEffectMutating, Confidence: CommandConfidenceCertain, Reason: "cp modifies filesystem state", AffectedPaths: copyMovePaths(args, false)}
	case "mv":
		return BashAnalysis{Effect: CommandEffectMutating, Confidence: CommandConfidenceCertain, Reason: "mv modifies filesystem state", AffectedPaths: copyMovePaths(args, true)}
	default:
		return unknownBashAnalysis("command effect is not proven")
	}
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
		return BashAnalysis{Effect: CommandEffectMutating, Confidence: CommandConfidenceCertain, Reason: "git branch invocation modifies repository state"}
	case "add", "apply", "checkout", "switch", "restore", "reset", "clean", "commit", "merge", "rebase", "cherry-pick", "revert", "stash", "tag", "fetch", "pull", "push":
		return BashAnalysis{Effect: CommandEffectMutating, Confidence: CommandConfidenceCertain, Reason: "git " + args[0] + " modifies repository or remote state"}
	default:
		return unknownBashAnalysis("git subcommand effect is not proven")
	}
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
			if i+1 < len(command) && command[i+1] == '|' {
				segments = append(segments, strings.TrimSpace(b.String()))
				b.Reset()
				operators = append(operators, "||")
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
		if c == '>' {
			flush()
			if i+1 < len(segment) && segment[i+1] == '>' {
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
		return appendUniquePaths(nil, pos[len(pos)-2], pos[len(pos)-1])
	}
	return appendUniquePaths(nil, pos[len(pos)-1])
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
