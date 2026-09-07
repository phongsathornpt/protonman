package tui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

type execFamily string

const (
	execFamilyGeneric   execFamily = "generic"
	execFamilyGit       execFamily = "git"
	execFamilyGo        execFamily = "go"
	execFamilyBun       execFamily = "bun"
	execFamilyNode      execFamily = "node"
	execFamilyPython    execFamily = "python"
	execFamilyRust      execFamily = "rust"
	execFamilyMake      execFamily = "make"
	execFamilyDocker    execFamily = "docker"
	execFamilyJVM       execFamily = "jvm"
	execFamilyPHP       execFamily = "php"
	execFamilyRuby      execFamily = "ruby"
	execFamilyDotnet    execFamily = "dotnet"
	execFamilyTerraform execFamily = "terraform"
	execFamilyKubectl   execFamily = "kubectl"
	execFamilyVite      execFamily = "vite"
	execFamilyNext      execFamily = "next"
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
	if profile := execProfileForFamily(family); profile != nil && profile.Summarize != nil {
		profile.Summarize(&p, combined)
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
		if profile := execProfileForCommand(name); profile != nil {
			action := profile.Action(args)
			return profile.Family, action, profile.Title(name, args, action)
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
	case "bundle":
		if len(args) > 1 && args[0] == "exec" {
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
