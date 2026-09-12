package turn

import (
	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
	"github.com/phongsathornpt/protonman/internal/core/tool"
	"github.com/phongsathornpt/protonman/internal/engine/prompt"
	"github.com/phongsathornpt/protonman/internal/feature/skill"
)

func (l *Loop) currentSkillPromptSection() string {
	var catalogItems []skill.CatalogItem
	var activeSkills []skill.Skill
	if l.skillRegistry != nil {
		allSkills := l.skillRegistry.List()
		activeMap := make(map[string]bool)
		for _, name := range l.skillRegistry.ActivatedList() {
			activeMap[name] = true
		}
		for _, candidate := range allSkills {
			if activeMap[candidate.Name] {
				activeSkills = append(activeSkills, candidate)
			} else {
				catalogItems = append(catalogItems, candidate.ToCatalogItem())
			}
		}
	} else if len(l.skills) > 0 {
		catalogItems = append(catalogItems, l.skills...)
	}
	return skill.SystemPromptSection(catalogItems, activeSkills)
}

func (l *Loop) effectivePromptSpec(definitions []tool.Definition, extras []string) prompt.Spec {
	spec := *l.promptSpec
	spec.Capabilities = prompt.ToolCapabilities{}
	spec.Mutations = prompt.MutationCapabilities{}
	spec.AvailableTools = make([]string, 0, len(definitions))
	for _, definition := range definitions {
		spec.AvailableTools = append(spec.AvailableTools, definition.Name)
		switch definition.Kind {
		case tool.KindTask:
			spec.Capabilities.Tasks = true
		case tool.KindAgent:
			spec.Capabilities.Agents = true
		case tool.KindMCP:
			spec.Capabilities.MCP = true
		}
		if tool.EffectiveMutability(definition) == tool.MutabilityMutating {
			switch definition.Safety.MutationDomain {
			case tool.MutationDomainWorkspace:
				spec.Mutations.Source = true
			case tool.MutationDomainWorkspacePolicy:
				spec.Mutations.Context = true
			case tool.MutationDomainTaskState:
				spec.Mutations.Task = true
			case tool.MutationDomainAgentState:
				spec.Mutations.Agent = true
			default:
				if definition.Kind == tool.KindMCP {
					spec.Mutations.External = true
				}
			}
		}
	}
	spec.ExtraInstructions = append(append([]string(nil), l.promptSpec.ExtraInstructions...), extras...)
	return spec
}

func messagesContainImages(messages []model.Message) bool {
	for _, message := range messages {
		for _, part := range message.Parts {
			if part.Type == model.ContentPartImage {
				return true
			}
		}
	}
	return false
}
