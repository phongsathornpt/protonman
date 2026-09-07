package turn

import (
	"context"
	"log/slog"
	"strings"

	"github.com/projectTHORN/proton/internal/agentprompt"
	"github.com/projectTHORN/proton/internal/model"
	"github.com/projectTHORN/proton/internal/modelprofile"
	"github.com/projectTHORN/proton/internal/tool"
	sdk "github.com/projectTHORN/proton/proton-sdk"
)

type resolvedModelState struct {
	profile modelprofile.Resolved
	has     bool
}

func resolvedModelStateFor(languageModel sdk.LanguageModel) resolvedModelState {
	profile, ok := model.ResolvedModelProfile(languageModel)
	return resolvedModelState{profile: profile, has: ok}
}

func (l *Loop) prepareTurnInput(ctx context.Context, messages []model.Message) ([]model.Message, []string, string) {
	history := model.CloneMessages(messages)
	var promptExtras []string
	if l.promptSpec != nil {
		filtered := make([]model.Message, 0, len(history))
		for _, message := range history {
			if message.Role == model.RoleSystem {
				if text := strings.TrimSpace(message.Content); text != "" && !agentprompt.IsManaged(text) {
					promptExtras = append(promptExtras, text)
				}
				continue
			}
			filtered = append(filtered, message)
		}
		history = filtered
	}
	projectInstructions := ""
	if l.promptSpec != nil {
		loaded, err := agentprompt.LoadProjectInstructions(l.promptSpec.Workspace)
		if err != nil {
			slog.WarnContext(ctx, "project instructions unavailable", "error", err)
		} else {
			projectInstructions = loaded
		}
	}
	if l.promptSpec == nil {
		history = l.injectLegacySkillPrompt(history)
	}
	return history, promptExtras, projectInstructions
}

func (l *Loop) injectLegacySkillPrompt(history []model.Message) []model.Message {
	section := l.currentSkillPromptSection()
	if section == "" {
		return history
	}
	if len(history) > 0 && history[0].Role == model.RoleSystem {
		base := history[0].Content
		if marker := strings.Index(base, skillPromptMarker); marker >= 0 {
			base = base[:marker]
		}
		history[0].Content = strings.TrimSpace(base + "\n\n" + skillPromptMarker + "\n" + section)
		return history
	}
	return append([]model.Message{{Role: model.RoleSystem, Content: skillPromptMarker + "\n" + section}}, history...)
}

func (l *Loop) prepareRoundRequest(
	ctx context.Context,
	history []model.Message,
	definitions []tool.Definition,
	promptExtras []string,
	projectInstructions string,
	reasoning modelprofile.ReasoningResolution,
	grounding groundingState,
	caps sdk.ModelCapabilities,
	toolCallsUsed int,
	forceNoProgressSynthesis bool,
	softToolBudgetWarned bool,
	resolved resolvedModelState,
) (sdk.Request, toolDispatchState, bool, error) {
	var tools []tool.Definition
	reqMessages := model.CloneMessages(history)
	dispatch := toolDispatchState{reason: toolDispatchDisabledNoTools}

	if forceNoProgressSynthesis {
		dispatch.reason = toolDispatchDisabledNoProgress
		reqMessages = append(reqMessages, model.Message{Role: model.RoleSystem, Content: NoProgressPrompt})
	} else {
		tools = l.tools.Definitions()
		if grounding.pending() {
			tools = grounding.filterDefinitions(tools)
		}
		switch {
		case len(tools) > 0 && !caps.Tools:
			tools = nil
			dispatch.reason = toolDispatchDisabledModelTools
		case len(tools) == 0:
			dispatch.reason = toolDispatchDisabledNoTools
		case l.maxToolCalls > 0 && toolCallsUsed >= l.maxToolCalls:
			tools = nil
			dispatch.reason = toolDispatchDisabledMaxCalls
			reqMessages = append(reqMessages, model.Message{Role: model.RoleSystem, Content: MaxToolCallsPrompt})
		default:
			dispatch.reason = toolDispatchEnabled
			if l.maxToolCalls > 0 {
				dispatch.remainingToolCalls = l.maxToolCalls - toolCallsUsed
			}
		}
	}
	if shouldWarnSoftToolBudget(toolCallsUsed, l.maxToolCalls, softToolBudgetWarned) && dispatch.enabled() {
		reqMessages = append(reqMessages, model.Message{Role: model.RoleSystem, Content: SoftToolBudgetPrompt})
		softToolBudgetWarned = true
	}
	if l.promptSpec != nil {
		spec := l.effectivePromptSpec(definitions, promptExtras)
		spec.ReasoningRequested = reasoningRequestedLabel(reasoning)
		spec.ReasoningEffective = reasoningEffectiveLabel(reasoning)
		spec.ReasoningSource = string(reasoning.Source)
		spec.ReasoningClamped = reasoning.Clamped
		spec.GroundingRequired = grounding.pending()
		spec.GroundingEvidence = string(grounding.evidence)
		if projectInstructions != "" {
			if base := strings.TrimSpace(spec.ProjectInstructions); base != "" {
				spec.ProjectInstructions = base + "\n\n" + projectInstructions
			} else {
				spec.ProjectInstructions = projectInstructions
			}
		}
		spec.Skills = l.currentSkillPromptSection()
		systemPrompt := agentprompt.Render(spec)
		reqMessages = append([]model.Message{{Role: model.RoleSystem, Content: systemPrompt}}, reqMessages...)
		slog.DebugContext(ctx, "turn system prompt prepared",
			"prompt_version", agentprompt.Version,
			"prompt_bytes", len(systemPrompt),
			"tool_count", len(tools),
		)
	}

	slog.DebugContext(ctx, "turn tool dispatch state",
		"enabled", dispatch.enabled(),
		"reason", dispatch.reason,
		"published_tools", len(tools),
		"tool_calls_used", toolCallsUsed,
		"remaining_tool_calls", dispatch.remainingToolCalls,
	)

	sdkTools := make([]sdk.Tool, 0, len(tools))
	for _, definition := range tools {
		inputSchema := definition.InputSchema
		if resolved.has {
			inputSchema = modelprofile.PublishInputSchema(resolved.profile, definition.InputSchema)
		}
		sdkTools = append(sdkTools, sdk.Tool{
			Name: definition.Name, Description: definition.Description,
			InputSchema: inputSchema, OutputSchema: definition.OutputSchema,
			Dynamic: definition.Kind == tool.KindMCP,
		})
	}
	request := sdk.Request{Messages: reqMessages, Tools: sdkTools}
	if grounding.pending() && dispatch.enabled() && len(sdkTools) > 0 && resolved.has &&
		resolved.profile.Capabilities.ToolChoiceRequired == modelprofile.SupportYes {
		request.Options.ToolChoice = sdk.ToolChoiceRequired
	}
	request.Options.ReasoningEffort = reasoning.Effective
	if err := request.Validate(); err != nil {
		return sdk.Request{}, dispatch, softToolBudgetWarned, err
	}
	if err := validateContextBudget(l.languageModel, request); err != nil {
		return sdk.Request{}, dispatch, softToolBudgetWarned, err
	}
	return request, dispatch, softToolBudgetWarned, nil
}
