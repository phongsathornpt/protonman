package memory

import (
	"html"
	"strconv"
	"strings"

	"github.com/phongsathornpt/protonman/internal/base/runtimepolicy"
	corememory "github.com/phongsathornpt/protonman/internal/core/memory"
)

// RenderContext serializes selected memories as bounded evidence data. The
// managed system prompt owns instruction precedence; this payload never does.
func RenderContext(entries []corememory.Entry, policy runtimepolicy.MemoryPolicy) string {
	contextText, _, _ := renderContext(entries, policy)
	return contextText
}

func renderContext(entries []corememory.Entry, policy runtimepolicy.MemoryPolicy) (string, int, int) {
	if len(entries) == 0 {
		return "", 0, 0
	}
	if policy.MaxContextBytes <= 0 {
		policy = runtimepolicy.DurableMemory()
	}
	const open = "<memory-context>\n"
	const close = "</memory-context>"
	var body strings.Builder
	workspaceCount, globalCount := 0, 0
	for _, entry := range entries {
		line := "  <memory scope=" + strconv.Quote(string(entry.Scope)) +
			" kind=" + strconv.Quote(string(entry.Kind)) +
			" key=" + strconv.Quote(html.EscapeString(entry.Key)) + ">" +
			html.EscapeString(entry.Value) + "</memory>\n"
		if len(open)+body.Len()+len(line)+len(close) > policy.MaxContextBytes {
			continue
		}
		body.WriteString(line)
		if entry.Scope == corememory.ScopeWorkspace {
			workspaceCount++
		} else if entry.Scope == corememory.ScopeGlobal {
			globalCount++
		}
	}
	if body.Len() == 0 {
		return "", 0, 0
	}
	return open + body.String() + close, workspaceCount, globalCount
}
