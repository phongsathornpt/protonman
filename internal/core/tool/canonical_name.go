package tool

import "strings"

var legacyToolNames = map[string]string{
	"read_file": "read",
}

// CanonicalName maps legacy public tool names to the current model-facing
// capability name. Unknown and external tool names pass through unchanged.
func CanonicalName(name string) string {
	name = strings.TrimSpace(name)
	if canonical, ok := legacyToolNames[name]; ok {
		return canonical
	}
	return name
}
