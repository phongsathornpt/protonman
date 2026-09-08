package builtin

import (
	"regexp"
	"strings"
)

var (
	pythonWalkPathPattern = regexp.MustCompile(`(?i)os\.walk\(\s*["']([^"']+)["']`)
	pythonPathGlobPattern = regexp.MustCompile(`(?i)Path\(\s*["']([^"']+)["']\s*\)\.(r?glob)\(\s*["']([^"']+)["']`)
	pythonIterdirPattern  = regexp.MustCompile(`(?i)Path\(\s*["']([^"']+)["']\s*\)\.iterdir\(`)
	nodeReadDirPattern    = regexp.MustCompile(`(?i)(?:readdirSync|readdir)\(\s*["']([^"']+)["']`)
)

func dedicatedPythonDiscoveryTool(command, lower string) *dedicatedToolSuggestion {
	if pythonHasMutationSignals(lower) {
		return nil
	}
	if match := pythonPathGlobPattern.FindStringSubmatch(command); len(match) == 4 {
		args := map[string]any{"path": strings.TrimSpace(match[1]), "pattern": strings.TrimSpace(match[3]), "type": "file"}
		if strings.EqualFold(match[2], "glob") {
			args["max_depth"] = 1
		}
		return &dedicatedToolSuggestion{
			tool: "find", args: args,
			reason: "workspace path discovery is available through find",
		}
	}
	if path := firstPatternGroup(pythonWalkPathPattern, command); path != "" {
		return &dedicatedToolSuggestion{
			tool: "find", args: map[string]any{"path": path, "pattern": "*", "type": "any"},
			reason: "workspace tree discovery is available through find",
		}
	}
	if path := firstPatternGroup(pythonIterdirPattern, command); path != "" {
		return &dedicatedToolSuggestion{
			tool: "ls", args: map[string]any{"path": path},
			reason: "directory inspection is available through ls",
		}
	}
	return nil
}

func pythonHasMutationSignals(lower string) bool {
	for _, signal := range []string{
		".write(", ".write_text(", ".write_bytes(", ".unlink(", ".rename(",
		"os.remove(", "os.unlink(", "os.rename(", "os.replace(", "os.mkdir(",
		"os.makedirs(", "os.rmdir(", "shutil.",
	} {
		if strings.Contains(lower, signal) {
			return true
		}
	}
	return false
}
