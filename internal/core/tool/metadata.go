package tool

import (
	"fmt"
	"strings"
)

// Metadata is the canonical human-facing and classification metadata for a known tool.
// UI adapters may style these semantics, but presentation meaning lives here.
type Metadata struct {
	Name        string
	Kind        Kind
	DisplayName string
}

type callMetadata struct {
	Metadata
	title         func(map[string]any) string
	target        func(map[string]any) string
	affectedPaths func(map[string]any) []string
}

var builtinMetadata = map[string]callMetadata{
	"read":               {Metadata: Metadata{Name: "read", Kind: KindRead, DisplayName: "Read"}, title: titleReadFile, target: targetReadFile, affectedPaths: affectedSinglePath("path", "file_path", "file", "filename", "target")},
	"math":               {Metadata: Metadata{Name: "math", Kind: KindCompute, DisplayName: "Calculate"}, title: titleCalculate, target: targetCalculate},
	"edit":               {Metadata: Metadata{Name: "edit", Kind: KindEdit, DisplayName: "Edit"}, title: titleEdit, target: targetEdit, affectedPaths: affectedEdit},
	"write_file":         {Metadata: Metadata{Name: "write_file", Kind: KindEdit, DisplayName: "Write"}, title: titleWriteFile, target: targetEditPath, affectedPaths: affectedSinglePath("file_path", "path", "file", "filename", "target", "destination", "move_path")},
	"search_replace":     {Metadata: Metadata{Name: "search_replace", Kind: KindEdit, DisplayName: "Edit"}, title: titleSearchReplace, target: targetEditPath, affectedPaths: affectedSinglePath("file_path", "path", "file", "filename", "target", "destination", "move_path")},
	"apply_patch":        {Metadata: Metadata{Name: "apply_patch", Kind: KindEdit, DisplayName: "Patch"}, title: titleApplyPatch, target: targetApplyPatch, affectedPaths: affectedPatch},
	"ls":                 {Metadata: Metadata{Name: "ls", Kind: KindRead, DisplayName: "List"}, title: titleListDir, target: targetListDir},
	"find":               {Metadata: Metadata{Name: "find", Kind: KindRead, DisplayName: "Find files"}, title: titleFindFiles, target: targetFindFiles},
	"grep":               {Metadata: Metadata{Name: "grep", Kind: KindGrep, DisplayName: "Search"}, title: titleGrep, target: targetGrep},
	"bash":               {Metadata: Metadata{Name: "bash", Kind: KindBash, DisplayName: "Run"}, title: titleBash, target: targetBash},
	"web":                {Metadata: Metadata{Name: "web", Kind: KindWebFetch, DisplayName: "Web"}, title: titleWeb, target: targetWeb},
	"git":                {Metadata: Metadata{Name: "git", Kind: KindGit, DisplayName: "Git"}, title: titleGitStatus, target: targetGitStatus},
	"todo":               {Metadata: Metadata{Name: "todo", Kind: KindTask, DisplayName: "Tasks"}, title: titleTodo, target: targetTodo},
	"skill":              {Metadata: Metadata{Name: "skill", Kind: KindRead, DisplayName: "Skill"}, title: titleActivateSkill, target: targetActivateSkill},
	"activate_skill":     {Metadata: Metadata{Name: "activate_skill", Kind: KindRead, DisplayName: "Skill"}, title: titleActivateSkill, target: targetActivateSkill},
	"subagent":           {Metadata: Metadata{Name: "subagent", Kind: KindAgent, DisplayName: "Subagent"}, title: titleSubagent, target: targetSubagent},
	"delegate_task":      {Metadata: Metadata{Name: "delegate_task", Kind: KindAgent, DisplayName: "Delegate"}, title: titleDelegateTask, target: targetDelegateTask},
	"wait_agent":         {Metadata: Metadata{Name: "wait_agent", Kind: KindAgent, DisplayName: "Wait for agents"}, title: titleConstant("Wait for agent activity"), target: targetConstant("agent activity")},
	"get_agent":          {Metadata: Metadata{Name: "get_agent", Kind: KindAgent, DisplayName: "Agent status"}, title: titleAgentID("Get agent status", "Get agent status"), target: targetAgentID},
	"list_agents":        {Metadata: Metadata{Name: "list_agents", Kind: KindAgent, DisplayName: "Subagents"}, title: titleConstant("List subagents"), target: targetConstant("subagents")},
	"cancel_agent":       {Metadata: Metadata{Name: "cancel_agent", Kind: KindAgent, DisplayName: "Cancel agent"}, title: titleAgentID("Cancel agent", "Cancel agent"), target: targetAgentID},
	"resume_agent":       {Metadata: Metadata{Name: "resume_agent", Kind: KindAgent, DisplayName: "Resume agent"}, title: titleAgentID("Resume agent", "Resume agent"), target: targetAgentID},
	"checkpoint_restore": {Metadata: Metadata{Name: "checkpoint_restore", Kind: KindEdit, DisplayName: "Restore"}, title: titleCheckpointRestore, target: targetCheckpointRestore},
}

// MetadataForName returns canonical metadata for a known built-in tool.
func metadataForName(name string) (callMetadata, bool) {
	if spec, ok := builtinMetadata[name]; ok {
		return spec, true
	}
	spec, ok := builtinMetadata[CanonicalName(name)]
	return spec, ok
}

func MetadataForName(name string) (Metadata, bool) {
	spec, ok := metadataForName(name)
	if !ok {
		return Metadata{}, false
	}
	return spec.Metadata, true
}

func titleConstant(value string) func(map[string]any) string {
	return func(map[string]any) string { return value }
}
func targetConstant(value string) func(map[string]any) string {
	return func(map[string]any) string { return value }
}

func titleSubagent(args map[string]any) string {
	switch strings.ToLower(ExtractString(args, "action")) {
	case "spawn":
		return titleDelegateTask(args)
	case "wait":
		return "Wait for agent activity"
	case "get":
		return titleAgentID("Get agent status", "Get agent status")(args)
	case "list":
		return "List subagents"
	case "cancel":
		return titleAgentID("Cancel agent", "Cancel agent")(args)
	case "resume":
		return titleAgentID("Resume agent", "Resume agent")(args)
	default:
		return "Subagent"
	}
}

func targetSubagent(args map[string]any) string {
	if strings.EqualFold(ExtractString(args, "action"), "spawn") {
		return targetDelegateTask(args)
	}
	if id := targetAgentID(args); id != "" {
		return id
	}
	return "subagents"
}

func titleTodo(args map[string]any) string {
	switch strings.ToLower(ExtractString(args, "action")) {
	case "get":
		return "Check task list"
	case "update":
		return titleUpdateTodo(args)
	default:
		return "Tasks"
	}
}

func targetTodo(args map[string]any) string {
	if strings.EqualFold(ExtractString(args, "action"), "update") {
		return targetUpdateTodo(args)
	}
	return "task plan"
}

func titleCalculate(args map[string]any) string {
	if expression := ExtractString(args, "expression"); expression != "" {
		return "Calculate " + expression
	}
	return "Calculate"
}
func targetCalculate(args map[string]any) string { return ExtractString(args, "expression") }

func titleReadFile(args map[string]any) string {
	if path := ExtractString(args, "path", "file_path", "file"); path != "" {
		return "Read " + path
	}
	return "Read file"
}
func targetReadFile(args map[string]any) string {
	return ExtractString(args, "path", "file_path", "file")
}

func titleEdit(args map[string]any) string {
	switch strings.ToLower(ExtractString(args, "action")) {
	case "write":
		return titleWriteFile(args)
	case "replace":
		return titleSearchReplace(args)
	case "patch":
		return titleApplyPatch(args)
	case "restore":
		return titleCheckpointRestore(args)
	default:
		return "Edit workspace"
	}
}

func targetEdit(args map[string]any) string {
	switch strings.ToLower(ExtractString(args, "action")) {
	case "write", "replace":
		return targetEditPath(args)
	case "patch":
		return targetApplyPatch(args)
	case "restore":
		return targetCheckpointRestore(args)
	default:
		return targetEditPath(args)
	}
}

func affectedEdit(args map[string]any) []string {
	switch strings.ToLower(ExtractString(args, "action")) {
	case "write", "replace":
		return affectedSinglePath("file_path", "path", "file", "filename", "target", "destination", "move_path")(args)
	case "patch":
		return affectedPatch(args)
	default:
		return nil
	}
}
func titleWriteFile(args map[string]any) string {
	if path := ExtractString(args, "file_path", "path", "file"); path != "" {
		return "Write " + path
	}
	return "Write file"
}
func titleSearchReplace(args map[string]any) string {
	if path := ExtractString(args, "file_path", "path", "file"); path != "" {
		return "Edit " + path
	}
	return "Search and replace"
}
func targetEditPath(args map[string]any) string {
	return ExtractString(args, "file_path", "path", "file", "filename", "target")
}

func titleApplyPatch(args map[string]any) string {
	if target := targetApplyPatch(args); target != "" {
		return "Patch " + target
	}
	return "Apply patch"
}
func targetApplyPatch(args map[string]any) string {
	if path := ExtractString(args, "path", "file_path", "file"); path != "" {
		return path
	}
	if paths := patchPathsFromArgs(args); len(paths) == 1 {
		return paths[0]
	} else if len(paths) > 1 {
		return fmt.Sprintf("%s (+%d files)", paths[0], len(paths)-1)
	}
	return ""
}
func affectedPatch(args map[string]any) []string {
	if path := ExtractString(args, "path", "file_path", "file", "filename", "target"); path != "" {
		return []string{path}
	}
	return patchPathsFromArgs(args)
}
func patchPathsFromArgs(args map[string]any) []string {
	for _, key := range []string{"patch", "diff", "input"} {
		if patch := ExtractString(args, key); patch != "" {
			return ParsePatchPaths(patch)
		}
	}
	return nil
}

func titleListDir(args map[string]any) string {
	if path := ExtractString(args, "path", "dir_path", "directory", "dir"); path != "" {
		return "List " + path
	}
	return "List directory"
}
func targetListDir(args map[string]any) string {
	if path := ExtractString(args, "path", "dir_path", "directory", "dir"); path != "" {
		return path
	}
	return "."
}
func titleFindFiles(args map[string]any) string {
	pattern := ExtractString(args, "pattern")
	if pattern == "" {
		pattern = "*"
	}
	return "Find files " + TruncateRunes(pattern, 40)
}
func targetFindFiles(args map[string]any) string {
	path := ExtractString(args, "path")
	if path == "" {
		path = "."
	}
	pattern := ExtractString(args, "pattern")
	if pattern == "" {
		pattern = "*"
	}
	return fmt.Sprintf("%q in %s", pattern, path)
}
func titleGrep(args map[string]any) string {
	pattern := ExtractString(args, "pattern", "query")
	path := ExtractString(args, "path", "dir_path", "directory")
	if pattern == "" {
		return "Search workspace"
	}
	if path != "" && path != "." {
		return fmt.Sprintf("Search %q in %s", TruncateRunes(pattern, 30), path)
	}
	return fmt.Sprintf("Search %q", TruncateRunes(pattern, 40))
}
func targetGrep(args map[string]any) string {
	pattern := ExtractString(args, "pattern", "query")
	path := ExtractString(args, "path", "dir_path", "directory")
	if pattern != "" && path != "" && path != "." {
		return fmt.Sprintf("%q in %s", pattern, path)
	}
	if pattern != "" {
		return fmt.Sprintf("%q", pattern)
	}
	return ""
}
func titleInspectCode(args map[string]any) string {
	query := ExtractString(args, "query", "pattern")
	path := ExtractString(args, "path")
	if query == "" {
		return "Inspect code"
	}
	if path != "" && path != "." {
		return fmt.Sprintf("Inspect %q in %s", TruncateRunes(query, 30), path)
	}
	return fmt.Sprintf("Inspect %q", TruncateRunes(query, 40))
}
func targetInspectCode(args map[string]any) string {
	query := ExtractString(args, "query", "pattern")
	path := ExtractString(args, "path")
	if path == "" {
		path = "."
	}
	if query == "" {
		return path
	}
	return fmt.Sprintf("%q in %s", query, path)
}
func titleBash(args map[string]any) string {
	if cmd := ExtractString(args, "command", "cmd"); cmd != "" {
		return "Run: " + TruncateRunes(cmd, 40)
	}
	return "Run shell command"
}
func targetBash(args map[string]any) string { return ExtractString(args, "command", "cmd") }
func titleWeb(args map[string]any) string {
	switch strings.ToLower(ExtractString(args, "action")) {
	case "search":
		return titleWebSearch(args)
	case "fetch":
		return titleWebFetch(args)
	default:
		return "Web"
	}
}
func targetWeb(args map[string]any) string {
	if strings.EqualFold(ExtractString(args, "action"), "search") {
		return targetWebSearch(args)
	}
	return targetWebFetch(args)
}
func titleWebFetch(args map[string]any) string {
	if value := ExtractString(args, "url"); value != "" {
		return "Fetch " + TruncateRunes(value, 45)
	}
	return "Fetch URL"
}
func targetWebFetch(args map[string]any) string { return ExtractString(args, "url") }
func titleWebSearch(args map[string]any) string {
	if query := ExtractString(args, "query", "pattern"); query != "" {
		return "Search web: " + TruncateRunes(query, 35)
	}
	return "Search web"
}
func targetWebSearch(args map[string]any) string {
	if query := ExtractString(args, "query", "pattern"); query != "" {
		return fmt.Sprintf("%q", query)
	}
	return ""
}
func titleGitStatus(args map[string]any) string {
	if path := ExtractString(args, "path"); path != "" && path != "." {
		return "Git status (" + path + ")"
	}
	return "Check git status"
}
func targetGitStatus(args map[string]any) string { return ExtractString(args, "path") }
func titleUpdateTodo(args map[string]any) string {
	if ops, ok := args["operations"].([]any); ok && len(ops) > 0 {
		return fmt.Sprintf("Update tasks (%d changes)", len(ops))
	}
	return "Update tasks"
}
func targetUpdateTodo(args map[string]any) string {
	if ops, ok := args["operations"].([]any); ok {
		return fmt.Sprintf("%d task operations", len(ops))
	}
	return "task plan"
}
func titleActivateSkill(args map[string]any) string {
	if name := ExtractString(args, "name", "skill"); name != "" {
		return "Activate skill " + name
	}
	return "Activate skill"
}
func targetActivateSkill(args map[string]any) string {
	if name := ExtractString(args, "name", "skill"); name != "" {
		return fmt.Sprintf("%q", name)
	}
	return ""
}
func titleDelegateTask(args map[string]any) string {
	task, profile := ExtractString(args, "task"), ExtractString(args, "profile")
	if profile != "" && task != "" {
		return fmt.Sprintf("Delegate [%s]: %s", profile, TruncateRunes(task, 30))
	}
	if task != "" {
		return "Delegate: " + TruncateRunes(task, 30)
	}
	return "Delegate subtask"
}
func targetDelegateTask(args map[string]any) string {
	task, profile := ExtractString(args, "task"), ExtractString(args, "profile")
	if profile != "" && task != "" {
		return fmt.Sprintf("[%s] %s", profile, TruncateRunes(task, 40))
	}
	if task != "" {
		return TruncateRunes(task, 40)
	}
	return "subagents"
}
func titleAgentID(prefix, fallback string) func(map[string]any) string {
	return func(args map[string]any) string {
		if id := targetAgentID(args); id != "" {
			return prefix + " " + TruncateRunes(id, 20)
		}
		return fallback
	}
}
func targetAgentID(args map[string]any) string { return ExtractString(args, "agent_id", "id") }
func titleCheckpointRestore(args map[string]any) string {
	if id := targetCheckpointRestore(args); id != "" {
		return "Restore checkpoint " + id
	}
	return "Restore checkpoint"
}
func targetCheckpointRestore(args map[string]any) string {
	return ExtractString(args, "checkpoint_id", "id")
}
func affectedSinglePath(keys ...string) func(map[string]any) []string {
	return func(args map[string]any) []string {
		if path := ExtractString(args, keys...); path != "" {
			return []string{path}
		}
		return nil
	}
}
