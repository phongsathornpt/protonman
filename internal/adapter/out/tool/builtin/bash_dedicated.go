package builtin

import (
	"encoding/json"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/phongsathornpt/protonman/internal/core/tool"
)

type dedicatedToolSuggestion struct {
	tool   string
	args   map[string]any
	reason string
}

var (
	pythonImagePathPattern = regexp.MustCompile(`(?i)(?:Image\.open|cv2\.imread|imageio\.imread)\(\s*["']([^"']+)["']`)
	pythonFilePathPattern  = regexp.MustCompile(`(?i)(?:open|Path)\(\s*["']([^"']+)["']`)
	nodeFilePathPattern    = regexp.MustCompile(`(?i)(?:readFileSync|readFile)\(\s*["']([^"']+)["']`)
)

func (h bashHandler) dedicatedToolError(command, cwd string) *tool.ToolError {
	suggestion := dedicatedToolForCommand(command)
	if suggestion == nil {
		return nil
	}
	pathValue, _ := suggestion.args["path"].(string)
	if pathValue != "" {
		resolved, ok := h.dedicatedWorkspacePath(cwd, pathValue)
		if !ok {
			return nil
		}
		suggestion.args["path"] = resolved
	}
	arguments, err := json.Marshal(suggestion.args)
	if err != nil {
		return nil
	}
	message := "bash command duplicates a dedicated workspace tool"
	if suggestion.reason != "" {
		message += ": " + suggestion.reason
	}
	return tool.NewToolError(tool.ErrorCodeInvalidArguments, message).WithRecovery(tool.Recovery{
		Action:    tool.RecoveryUseDedicatedTool,
		Tool:      suggestion.tool,
		Arguments: arguments,
	})
}

func (h bashHandler) dedicatedWorkspacePath(cwd, path string) (string, bool) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", false
	}
	if filepath.IsAbs(path) {
		rel, err := filepath.Rel(h.workspace.Root(), filepath.Clean(path))
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return "", false
		}
		return filepath.ToSlash(rel), true
	}
	base := cwd
	if strings.TrimSpace(base) == "" {
		base = h.workspace.Root()
	}
	absolute := filepath.Join(base, path)
	rel, err := filepath.Rel(h.workspace.Root(), filepath.Clean(absolute))
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	return filepath.ToSlash(rel), true
}

func dedicatedToolForCommand(command string) *dedicatedToolSuggestion {
	fields, ok := splitSimpleShellWords(strings.TrimSpace(command))
	if !ok || len(fields) == 0 {
		return nil
	}
	executable := strings.ToLower(filepath.Base(fields[0]))
	switch {
	case executable == "py" || strings.HasPrefix(executable, "python"):
		return dedicatedPythonTool(command)
	case executable == "node" || executable == "nodejs":
		return dedicatedNodeTool(command)
	case executable == "cat", executable == "ls", executable == "grep", executable == "rg", executable == "find":
		return dedicatedSimpleShellTool(executable, fields, command)
	default:
		return nil
	}
}
func dedicatedPythonTool(command string) *dedicatedToolSuggestion {
	lower := strings.ToLower(command)
	if strings.Contains(lower, "subprocess") || strings.Contains(lower, "os.system(") {
		return nil
	}
	if strings.Contains(lower, "image.open(") || strings.Contains(lower, "cv2.imread(") || strings.Contains(lower, "imageio.imread(") {
		if path := firstPatternGroup(pythonImagePathPattern, command); path != "" {
			return &dedicatedToolSuggestion{
				tool: "read", args: map[string]any{"path": path, "view": "image"},
				reason: "image inspection is available through read",
			}
		}
	}
	readsFile := strings.Contains(lower, ".read(") || strings.Contains(lower, ".read_text(") || strings.Contains(lower, ".read_bytes(")
	if readsFile {
		if path := firstPatternGroup(pythonFilePathPattern, command); path != "" {
			return &dedicatedToolSuggestion{
				tool: "read", args: map[string]any{"path": path},
				reason: "workspace file inspection is available through read",
			}
		}
	}
	return dedicatedPythonDiscoveryTool(command, lower)
}

func dedicatedNodeTool(command string) *dedicatedToolSuggestion {
	lower := strings.ToLower(command)
	if strings.Contains(lower, "readfilesync(") || strings.Contains(lower, "readfile(") {
		if path := firstPatternGroup(nodeFilePathPattern, command); path != "" {
			return &dedicatedToolSuggestion{
				tool: "read", args: map[string]any{"path": path},
				reason: "workspace file inspection is available through read",
			}
		}
	}
	if strings.Contains(lower, "readdirsync(") || strings.Contains(lower, "readdir(") {
		if path := firstPatternGroup(nodeReadDirPattern, command); path != "" {
			return &dedicatedToolSuggestion{
				tool: "list_dir", args: map[string]any{"path": path},
				reason: "directory inspection is available through list_dir",
			}
		}
	}
	return nil
}

func firstPatternGroup(pattern *regexp.Regexp, value string) string {
	match := pattern.FindStringSubmatch(value)
	if len(match) < 2 {
		return ""
	}
	return strings.TrimSpace(match[1])
}
