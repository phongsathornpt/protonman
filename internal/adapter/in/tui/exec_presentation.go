package tui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

type execFamily string

const (
	execFamilyGeneric execFamily = "generic"
	execFamilyGit     execFamily = "git"
	execFamilyGo      execFamily = "go"
	execFamilyBun     execFamily = "bun"
	execFamilyNode    execFamily = "node"
	execFamilyPython  execFamily = "python"
	execFamilyVite    execFamily = "vite"
	execFamilyNext    execFamily = "next"
)

type execPresentation struct {
	Family         execFamily
	Action         string
	Title          string
	Summary        string
	SuccessSummary string
	Details        []string
	SuppressRaw    bool
}

func presentExec(command, stdout, stderr string) execPresentation {
	family, action, title := classifyExecCommand(command)
	cleanOut := cleanExecOutput(stdout)
	cleanErr := cleanExecOutput(stderr)
	combined := strings.TrimSpace(strings.TrimSpace(cleanOut) + "\n" + strings.TrimSpace(cleanErr))

	lower := strings.ToLower(combined)
	if strings.Contains(lower, "next.js") {
		family = execFamilyNext
		action = frameworkAction(action, command, "next")
		title = execTitle("Next", action)
	} else if strings.Contains(combined, "VITE v") || strings.Contains(lower, "vite v") {
		family = execFamilyVite
		action = frameworkAction(action, command, "vite")
		title = execTitle("Vite", action)
	}

	p := execPresentation{Family: family, Action: action, Title: title}
	switch family {
	case execFamilyGit:
		summarizeGitExec(&p, combined)
	case execFamilyGo:
		summarizeGoExec(&p, combined)
	case execFamilyBun:
		summarizeBunExec(&p, combined)
	case execFamilyNode:
		summarizeNodeExec(&p, combined)
	case execFamilyPython:
		summarizePythonExec(&p, combined)
	case execFamilyVite:
		summarizeViteExec(&p, combined)
	case execFamilyNext:
		summarizeNextExec(&p, combined)
	}
	return p
}

func classifyExecCommand(command string) (execFamily, string, string) {
	for _, segment := range splitExecSegments(command) {
		words := execWords(segment)
		if len(words) == 0 {
			continue
		}
		name, args := unwrapExec(words)
		if isPythonExecutable(name) {
			action := pythonAction(args)
			return execFamilyPython, action, pythonExecTitle(args, action)
		}
		switch name {
		case "git":
			action := gitAction(args)
			return execFamilyGit, action, execTitle("Git", strings.Join(args, " "))
		case "go":
			action := firstArg(args)
			return execFamilyGo, action, execTitle("Go", strings.Join(args, " "))
		case "bun":
			action := packageRunnerAction(args)
			return execFamilyBun, action, execTitle("Bun", strings.Join(args, " "))
		case "node":
			action := nodeAction(args)
			return execFamilyNode, action, nodeExecTitle(args, action)
		case "npm", "pnpm", "yarn":
			action := packageRunnerAction(args)
			label := strings.ToUpper(name[:1]) + name[1:]
			return execFamilyNode, action, execTitle(label, strings.Join(args, " "))
		case "vite":
			action := frameworkAction(firstArg(args), command, "vite")
			return execFamilyVite, action, execTitle("Vite", action)
		case "next":
			action := frameworkAction(firstArg(args), command, "next")
			return execFamilyNext, action, execTitle("Next", action)
		}
	}
	command = strings.TrimSpace(command)
	if command == "" {
		command = "shell command"
	}
	return execFamilyGeneric, "", "$ " + command
}

func splitExecSegments(command string) []string {
	replacer := strings.NewReplacer("&&", "\n", "||", "\n", ";", "\n")
	return strings.Split(replacer.Replace(command), "\n")
}

func execWords(segment string) []string {
	words := strings.Fields(strings.TrimSpace(segment))
	for len(words) > 0 && (words[0] == "env" || words[0] == "command") {
		words = words[1:]
	}
	for len(words) > 0 && isEnvAssignment(words[0]) {
		words = words[1:]
	}
	return words
}

func isEnvAssignment(word string) bool {
	eq := strings.IndexByte(word, '=')
	if eq <= 0 {
		return false
	}
	for i, r := range word[:eq] {
		if !(r == '_' || r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || i > 0 && r >= '0' && r <= '9') {
			return false
		}
	}
	return true
}

func unwrapExec(words []string) (string, []string) {
	if len(words) == 0 {
		return "", nil
	}
	name := strings.TrimPrefix(words[0], "./")
	args := words[1:]
	switch name {
	case "npx", "bunx":
		if len(args) > 0 {
			return strings.TrimPrefix(args[0], "./"), args[1:]
		}
	case "pnpm":
		if len(args) > 1 && (args[0] == "exec" || args[0] == "dlx") {
			return strings.TrimPrefix(args[1], "./"), args[2:]
		}
	}
	return name, args
}

func firstArg(args []string) string {
	if len(args) == 0 {
		return ""
	}
	return args[0]
}

func execTitle(label, action string) string {
	action = strings.TrimSpace(action)
	if action == "" {
		return label
	}
	return label + " " + action
}

func cleanExecOutput(value string) string {
	return strings.ReplaceAll(ansi.Strip(value), "\r", "")
}
