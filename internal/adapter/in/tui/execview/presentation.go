package execview

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// Family identifies the command ecosystem used for execution presentation.
type Family string

const (
	FamilyGeneric   Family = "generic"
	FamilyGit       Family = "git"
	FamilyGo        Family = "go"
	FamilyBun       Family = "bun"
	FamilyNode      Family = "node"
	FamilyPython    Family = "python"
	FamilyRust      Family = "rust"
	FamilyMake      Family = "make"
	FamilyDocker    Family = "docker"
	FamilyJVM       Family = "jvm"
	FamilyPHP       Family = "php"
	FamilyRuby      Family = "ruby"
	FamilyDotnet    Family = "dotnet"
	FamilyTerraform Family = "terraform"
	FamilyKubectl   Family = "kubectl"
	FamilyVite      Family = "vite"
	FamilyNext      Family = "next"
)

// Presentation is the semantic rendering metadata for one shell execution.
type Presentation struct {
	Family         Family
	Action         string
	Title          string
	Summary        string
	SuccessSummary string
	Details        []string
	SuppressRaw    bool
}

// Present classifies a command and summarizes its output for the TUI.
func Present(command, stdout, stderr string) Presentation {
	family, action, title := classifyExecCommand(command)
	cleanOut := cleanExecOutput(stdout)
	cleanErr := cleanExecOutput(stderr)
	combined := strings.TrimSpace(strings.TrimSpace(cleanOut) + "\n" + strings.TrimSpace(cleanErr))

	lower := strings.ToLower(combined)
	if strings.Contains(lower, "next.js") {
		family = FamilyNext
		action = frameworkAction(action, command, "next")
		title = execTitle("Next", action)
	} else if strings.Contains(combined, "VITE v") || strings.Contains(lower, "vite v") {
		family = FamilyVite
		action = frameworkAction(action, command, "vite")
		title = execTitle("Vite", action)
	}

	p := Presentation{Family: family, Action: action, Title: title}
	if profile := execProfileForFamily(family); profile != nil && profile.Summarize != nil {
		profile.Summarize(&p, combined)
	}
	return p
}

func classifyExecCommand(command string) (Family, string, string) {
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
	return FamilyGeneric, "", "$ " + command
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
