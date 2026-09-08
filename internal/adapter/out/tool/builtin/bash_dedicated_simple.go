package builtin

import (
	"strconv"
	"strings"
	"unicode"
)

func splitSimpleShellWords(command string) ([]string, bool) {
	var words []string
	var current strings.Builder
	var quote rune
	escaped := false
	flush := func() {
		if current.Len() == 0 {
			return
		}
		words = append(words, current.String())
		current.Reset()
	}
	for _, r := range command {
		if escaped {
			current.WriteRune(r)
			escaped = false
			continue
		}
		if r == '\\' && quote != '\'' {
			escaped = true
			continue
		}
		if quote != 0 {
			if r == quote {
				quote = 0
			} else {
				current.WriteRune(r)
			}
			continue
		}
		switch {
		case r == '\'' || r == '"':
			quote = r
		case unicode.IsSpace(r):
			flush()
		default:
			current.WriteRune(r)
		}
	}
	if escaped || quote != 0 {
		return nil, false
	}
	flush()
	return words, true
}

func dedicatedSimpleShellTool(executable string, fields []string, command string) *dedicatedToolSuggestion {
	if hasShellControl(command) {
		return nil
	}
	switch executable {
	case "cat":
		return dedicatedCatTool(fields[1:])
	case "ls":
		return dedicatedListTool(fields[1:])
	case "grep", "rg":
		return dedicatedSearchTool(executable, fields[1:])
	case "find":
		return dedicatedFindTool(fields[1:])
	default:
		return nil
	}
}

func hasShellControl(command string) bool {
	if strings.Contains(command, "$(") || strings.Contains(command, "${") {
		return true
	}
	return strings.ContainsAny(command, "|;&><\n\r`")
}

func staticPathToken(value string) bool {
	value = strings.TrimSpace(value)
	return value != "" && !strings.ContainsAny(value, "$*?[]{}")
}

func dedicatedCatTool(args []string) *dedicatedToolSuggestion {
	if len(args) == 2 && args[0] == "--" {
		args = args[1:]
	}
	if len(args) != 1 || !staticPathToken(args[0]) {
		return nil
	}
	return &dedicatedToolSuggestion{
		tool: "read", args: map[string]any{"path": args[0]},
		reason: "workspace file inspection is available through read",
	}
}

func dedicatedListTool(args []string) *dedicatedToolSuggestion {
	path := "."
	switch len(args) {
	case 0:
	case 1:
		if args[0] == "-1" {
			break
		}
		if !staticPathToken(args[0]) {
			return nil
		}
		path = args[0]
	case 2:
		if args[0] != "-1" || !staticPathToken(args[1]) {
			return nil
		}
		path = args[1]
	default:
		return nil
	}
	return &dedicatedToolSuggestion{
		tool: "list_dir", args: map[string]any{"path": path},
		reason: "directory inspection is available through list_dir",
	}
}

func dedicatedSearchTool(executable string, args []string) *dedicatedToolSuggestion {
	recursive := executable == "rg"
	positionals := make([]string, 0, 2)
	for _, arg := range args {
		switch arg {
		case "-n", "--line-number", "--":
			continue
		case "-r", "-R", "--recursive":
			if executable != "grep" {
				return nil
			}
			recursive = true
		default:
			if strings.HasPrefix(arg, "-") {
				return nil
			}
			positionals = append(positionals, arg)
		}
	}
	if len(positionals) == 0 || len(positionals) > 2 {
		return nil
	}
	pattern := positionals[0]
	path := "."
	if len(positionals) == 2 {
		path = positionals[1]
	}
	if !staticPathToken(path) || executable == "grep" && !recursive {
		return nil
	}
	return &dedicatedToolSuggestion{
		tool: "grep", args: map[string]any{"pattern": pattern, "path": path},
		reason: "repository content search is available through grep",
	}
}

func dedicatedFindTool(args []string) *dedicatedToolSuggestion {
	path := "."
	pattern := "*"
	kind := "any"
	maxDepth := 0
	index := 0
	if index < len(args) && !strings.HasPrefix(args[index], "-") {
		if !staticPathToken(args[index]) {
			return nil
		}
		path = args[index]
		index++
	}
	for index < len(args) {
		switch args[index] {
		case "-name":
			if index+1 >= len(args) {
				return nil
			}
			pattern = args[index+1]
			index += 2
		case "-type":
			if index+1 >= len(args) {
				return nil
			}
			switch args[index+1] {
			case "f":
				kind = "file"
			case "d":
				kind = "dir"
			default:
				return nil
			}
			index += 2
		case "-maxdepth":
			if index+1 >= len(args) {
				return nil
			}
			value, err := strconv.Atoi(args[index+1])
			if err != nil || value < 0 {
				return nil
			}
			maxDepth = value
			index += 2
		default:
			return nil
		}
	}
	result := map[string]any{"path": path, "pattern": pattern, "type": kind}
	if maxDepth > 0 {
		result["max_depth"] = maxDepth
	}
	return &dedicatedToolSuggestion{
		tool: "find_files", args: result,
		reason: "workspace path discovery is available through find_files",
	}
}
