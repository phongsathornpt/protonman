package tool

// Canonical public tool names. Runtime, prompts, permissions, telemetry, and
// tests should use this vocabulary rather than historical aliases.
const (
	NameRead     = "read"
	NameLS       = "ls"
	NameFind     = "find"
	NameMath     = "math"
	NameGit      = "git"
	NameEdit     = "edit"
	NameWeb      = "web"
	NameTodo     = "todo"
	NameSkill    = "skill"
	NameSubagent = "subagent"
	NameBash     = "bash"
)

// Canonical facade actions shared by the public tool capabilities.
const (
	ActionGet    = "get"
	ActionUpdate = "update"
	ActionSpawn  = "spawn"
	ActionWait   = "wait"
	ActionList   = "list"
	ActionCancel = "cancel"
	ActionResume = "resume"
)

const (
	ActionStatus  = "status"
	ActionDiff    = "diff"
	ActionLog     = "log"
	ActionFetch   = "fetch"
	ActionSearch  = "search"
	ActionWrite   = "write"
	ActionReplace = "replace"
	ActionPatch   = "patch"
	ActionRestore = "restore"
)
