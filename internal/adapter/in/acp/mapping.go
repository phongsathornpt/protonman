package acp

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
	"github.com/phongsathornpt/protonman/internal/core/tool"
)

// ToolKindForName returns the ACP ToolKind for a Protonman tool name.
func ToolKindForName(name string) ToolKind {
	return ToolKindForCall(tool.Call{Name: name, Arguments: json.RawMessage(`{}`)})
}

// ToolKindForCall classifies canonical capability actions without depending on legacy tool names.
func ToolKindForCall(call tool.Call) ToolKind {
	call = tool.NormalizeLegacyCall(call)
	args := call.ArgumentsMap()
	if call.Name == "web" {
		if strings.EqualFold(tool.ExtractString(args, "action"), "search") {
			return ToolKindSearch
		}
		return ToolKindFetch
	}
	if call.Name == "todo" {
		if strings.EqualFold(tool.ExtractString(args, "action"), "get") {
			return ToolKindRead
		}
		return ToolKindEdit
	}
	switch tool.KindForName(call.Name) {
	case tool.KindRead:
		return ToolKindRead
	case tool.KindEdit:
		return ToolKindEdit
	case tool.KindGrep:
		return ToolKindSearch
	case tool.KindBash, tool.KindAgent, tool.KindGit:
		return ToolKindExecute
	case tool.KindWeb:
		return ToolKindFetch
	case tool.KindTask:
		return ToolKindEdit
	default:
		return ToolKindOther
	}
}

// TitleForToolCall produces a human-readable title describing what the tool is doing.
func TitleForToolCall(call tool.Call) string {
	return call.Title()
}

// LocationsForToolCall returns file paths affected by a tool call for Zed's Follow-the-Agent.
func LocationsForToolCall(call tool.Call) []ToolCallLocation {
	paths := call.AffectedPaths()
	if len(paths) == 0 {
		return nil
	}
	locs := make([]ToolCallLocation, len(paths))
	for i, p := range paths {
		locs[i] = ToolCallLocation{Path: p}
	}
	return locs
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

// DefaultSessionModes returns the standard modes available in Protonman.
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
