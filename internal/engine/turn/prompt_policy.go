package turn

import (
	"github.com/projectTHORN/proton/internal/engine/prompt"
	"github.com/projectTHORN/proton/internal/adapter/out/model"
	"github.com/projectTHORN/proton/internal/feature/skill"
	"github.com/projectTHORN/proton/internal/core/tool"
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
	spec.Provider = l.languageModel.Provider()
	spec.ModelID = l.languageModel.ModelID()
	if profile, ok := model.ResolvedModelProfile(l.languageModel); ok {
		spec.ModelProfile = profile.ProfileName
		spec.ModelProfileMatch = string(profile.ProfileMatch)
		spec.ModelCatalogOverride = profile.CatalogOverride
		spec.ModelPromptHints = append([]string(nil), profile.AgentPolicy.PromptHints...)
	}
	spec.ToolNames = make([]string, 0, len(definitions))
	spec.TaskPlanEnabled = false
	spec.DelegationEnabled = false
	spec.MutationEnabled = false
	for _, definition := range definitions {
		spec.ToolNames = append(spec.ToolNames, definition.Name)
		switch definition.Name {
		case "get_todo", "update_todo":
			spec.TaskPlanEnabled = true
		case "delegate_task":
			spec.DelegationEnabled = true
		}
		if tool.EffectiveMutability(definition) == tool.MutabilityMutating {
			spec.MutationEnabled = true
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
