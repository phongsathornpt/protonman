package acp

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/projectTHORN/proton/internal/model"
	"github.com/projectTHORN/proton/internal/tool"
)

// ToolKindForName returns the ACP ToolKind for a Proton tool.
func ToolKindForName(name string) ToolKind {
	switch name {
	case "read_file", "list_dir":
		return ToolKindRead
	case "write_file", "search_replace", "apply_patch":
		return ToolKindEdit
	case "grep":
		return ToolKindSearch
	case "bash":
		return ToolKindExecute
	case "web_fetch":
		return ToolKindFetch
	default:
		return ToolKindOther
	}
}

// TitleForToolCall produces a human-readable title describing what the tool is doing.
func TitleForToolCall(call tool.Call) string {
	var args map[string]any
	_ = json.Unmarshal(call.Arguments, &args)

	switch call.Name {
	case "read_file":
		if path, ok := args["path"].(string); ok && path != "" {
			return fmt.Sprintf("Read %s", path)
		}
		return "Read file"
	case "write_file":
		if path, ok := args["path"].(string); ok && path != "" {
			return fmt.Sprintf("Write %s", path)
		}
		return "Write file"
	case "search_replace":
		if path, ok := args["path"].(string); ok && path != "" {
			return fmt.Sprintf("Edit %s", path)
		}
		return "Search and replace"
	case "apply_patch":
		if path, ok := args["path"].(string); ok && path != "" {
			return fmt.Sprintf("Patch %s", path)
		}
		return "Apply patch"
	case "list_dir":
		if path, ok := args["path"].(string); ok && path != "" {
			return fmt.Sprintf("List %s", path)
		}
		return "List directory"
	case "grep":
		if query, ok := args["query"].(string); ok && query != "" {
			return fmt.Sprintf("Search %q", query)
		}
		return "Search workspace"
	case "bash":
		if cmd, ok := args["command"].(string); ok && cmd != "" {
			trimmed := strings.TrimSpace(cmd)
			if len(trimmed) > 40 {
				trimmed = trimmed[:37] + "..."
			}
			return fmt.Sprintf("Run: %s", trimmed)
		}
		return "Run shell command"
	case "web_fetch":
		if url, ok := args["url"].(string); ok && url != "" {
			return fmt.Sprintf("Fetch %s", url)
		}
		return "Fetch URL"
	case "activate_skill":
		if name, ok := args["name"].(string); ok && name != "" {
			return fmt.Sprintf("Activate skill %s", name)
		}
		return "Activate skill"
	case "delegate_task":
		if task, ok := args["task"].(string); ok && task != "" {
			trimmed := strings.TrimSpace(task)
			if len(trimmed) > 30 {
				trimmed = trimmed[:27] + "..."
			}
			return fmt.Sprintf("Delegate: %s", trimmed)
		}
		return "Delegate subtask"
	case "checkpoint_restore":
		if id, ok := args["checkpoint_id"].(string); ok && id != "" {
			return fmt.Sprintf("Restore checkpoint %s", id)
		}
		return "Restore checkpoint"
	default:
		return call.Name
	}
}

// LocationsForToolCall returns file paths affected by a tool call for Zed's Follow-the-Agent.
func LocationsForToolCall(call tool.Call) []ToolCallLocation {
	var args map[string]any
	if err := json.Unmarshal(call.Arguments, &args); err != nil {
		return nil
	}

	switch call.Name {
	case "read_file", "write_file", "search_replace", "apply_patch":
		if path, ok := args["path"].(string); ok && path != "" {
			return []ToolCallLocation{{Path: path}}
		}
	}
	return nil
}

// ContentBlocksToModelMessage converts ACP prompt content blocks into a model.Message.
func ContentBlocksToModelMessage(blocks []ContentBlock) model.Message {
	msg := model.Message{
		Role: model.RoleUser,
	}

	var textParts []string
	var parts []model.ContentPart
	hasMultiModal := false

	for _, block := range blocks {
		switch block.Type {
		case BlockTypeText:
			if block.Text != "" {
				textParts = append(textParts, block.Text)
				parts = append(parts, model.ContentPart{
					Type: model.ContentPartText,
					Text: block.Text,
				})
			}
		case BlockTypeImage:
			hasMultiModal = true
			mime := block.MIMEType
			if mime == "" {
				mime = "image/png"
			}
			parts = append(parts, model.ContentPart{
				Type:     model.ContentPartImage,
				MIMEType: mime,
				Data:     block.Data,
			})
			textParts = append(textParts, fmt.Sprintf("[Attached Image: %s]", mime))
		case BlockTypeResource:
			if block.Resource != nil && block.Resource.Text != "" {
				resText := fmt.Sprintf("--- Context from %s ---\n%s\n--- End Context ---", block.Resource.URI, block.Resource.Text)
				textParts = append(textParts, resText)
				parts = append(parts, model.ContentPart{
					Type: model.ContentPartText,
					Text: resText,
				})
			}
		case BlockTypeResourceLink:
			refText := fmt.Sprintf("[Reference to %s: %s]", block.Name, block.URI)
			textParts = append(textParts, refText)
			parts = append(parts, model.ContentPart{
				Type: model.ContentPartText,
				Text: refText,
			})
		}
	}

	msg.Content = strings.Join(textParts, "\n\n")
	if hasMultiModal {
		msg.Parts = parts
	}
	return msg
}

// DefaultAvailableCommands returns slash commands advertised to Zed.
func DefaultAvailableCommands() []AvailableCommand {
	return []AvailableCommand{
		{Name: "tools", Description: "List registered tools and schemas"},
		{Name: "skills", Description: "List discovered Agent Skills"},
		{Name: "skill", Description: "Inspect or activate an Agent Skill", Input: &AvailableCommandInput{Hint: "skill-name"}},
		{Name: "mode", Description: "Switch permission mode", Input: &AvailableCommandInput{Hint: "ask | plan | always-approve"}},
		{Name: "ask", Description: "Switch to interactive permission mode"},
		{Name: "plan", Description: "Switch to read-only plan mode"},
		{Name: "always-approve", Description: "Switch to autonomous approval mode"},
		{Name: "new", Description: "Reset context and start a fresh session"},
		{Name: "model", Description: "Switch or view active model", Input: &AvailableCommandInput{Hint: "model-name"}},
		{Name: "reasoning", Description: "Show or set session reasoning effort", Input: &AvailableCommandInput{Hint: "auto | none | low | medium | high | xhigh | max"}},
	}
}

// DefaultSessionModes returns the standard modes available in Proton.
func DefaultSessionModes(current string) *SessionModeState {
	if current == "" {
		current = "ask"
	}
	return &SessionModeState{
		CurrentModeID: current,
		AvailableModes: []SessionMode{
			{
				ID:          "ask",
				Name:        "Ask",
				Description: "Interactive permission requests before tool execution (default)",
			},
			{
				ID:          "plan",
				Name:        "Plan",
				Description: "Read-only inspection mode. Mutating file tools and commands are blocked",
			},
			{
				ID:          "always-approve",
				Name:        "Always Approve",
				Description: "Autonomous execution without confirmation prompts",
			},
		},
	}
}
