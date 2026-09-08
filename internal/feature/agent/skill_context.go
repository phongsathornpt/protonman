package agent

import (
	"sort"
	"strings"
	"unicode"

	"github.com/phongsathornpt/protonman/internal/feature/skill"
)

const (
	subagentSkillCatalogMaxItems = 8
	subagentSkillCatalogMaxBytes = 4 * 1024
)

type subagentSkillBudget struct {
	maxActive           int
	maxInstructionBytes int
}

func skillBudgetForProfile(profile Profile) subagentSkillBudget {
	switch profile {
	case ProfileStrength:
		return subagentSkillBudget{maxActive: 2, maxInstructionBytes: 8 * 1024}
	case ProfileAgility:
		return subagentSkillBudget{maxActive: 3, maxInstructionBytes: 12 * 1024}
	case ProfileIntelligence:
		return subagentSkillBudget{maxActive: 3, maxInstructionBytes: 16 * 1024}
	default:
		return subagentSkillBudget{maxActive: 2, maxInstructionBytes: 8 * 1024}
	}
}

type rankedSkill struct {
	skill skill.Skill
	score int
}

func selectSubagentSkills(catalog *skill.Registry, req Request) *skill.Registry {
	if catalog == nil {
		return nil
	}
	query := strings.TrimSpace(req.Task + "\n" + req.Context)
	queryTokens := skillQueryTokens(query)
	available := catalog.List()
	candidates := make([]rankedSkill, 0, len(available))
	for _, candidate := range available {
		compatible, affinity := skillProfileAffinity(candidate, req.Profile)
		if !compatible {
			continue
		}
		score := affinity + skillRelevanceScore(candidate, query, queryTokens)
		if score <= 0 {
			continue
		}
		if candidate.Scope == skill.ScopeProject {
			score++
		}
		candidates = append(candidates, rankedSkill{skill: candidate, score: score})
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].score != candidates[j].score {
			return candidates[i].score > candidates[j].score
		}
		return candidates[i].skill.Name < candidates[j].skill.Name
	})
	selected := make([]skill.Skill, 0, min(len(candidates), subagentSkillCatalogMaxItems))
	usedBytes := 0
	for _, candidate := range candidates {
		if len(selected) >= subagentSkillCatalogMaxItems {
			break
		}
		size := skillCatalogItemBytes(candidate.skill)
		if usedBytes+size > subagentSkillCatalogMaxBytes {
			continue
		}
		selected = append(selected, candidate.skill)
		usedBytes += size
	}
	registry := skill.NewRegistry(selected...)
	budget := skillBudgetForProfile(req.Profile)
	registry.SetActivationLimits(skill.ActivationLimits{
		MaxSkills:           budget.maxActive,
		MaxInstructionBytes: budget.maxInstructionBytes,
	})
	return registry
}

func skillRelevanceScore(candidate skill.Skill, query string, queryTokens map[string]struct{}) int {
	name := strings.ToLower(candidate.Name)
	description := strings.ToLower(candidate.Description)
	score := 0
	if query != "" && strings.Contains(strings.ToLower(query), name) {
		score += 20
	}
	for token := range queryTokens {
		if len(token) < 2 {
			continue
		}
		if strings.Contains(name, token) {
			score += 8
		}
		if strings.Contains(description, token) {
			score += 3
		}
	}
	return score
}

func skillProfileAffinity(candidate skill.Skill, profile Profile) (bool, int) {
	profiles, declared := skillMetadataProfiles(candidate.Metadata)
	if declared {
		for _, name := range profiles {
			declaredProfile, err := ParseProfile(name)
			if err == nil && declaredProfile == profile {
				return true, 12
			}
		}
		return false, 0
	}
	return true, 0
}

func skillMetadataProfiles(metadata map[string]any) ([]string, bool) {
	if len(metadata) == 0 {
		return nil, false
	}
	proton, ok := metadata["proton"].(map[string]any)
	if !ok {
		return nil, false
	}
	raw, ok := proton["profiles"]
	if !ok {
		return nil, false
	}
	switch values := raw.(type) {
	case []string:
		return append([]string(nil), values...), true
	case []any:
		out := make([]string, 0, len(values))
		for _, value := range values {
			if text, ok := value.(string); ok {
				out = append(out, text)
			}
		}
		return out, true
	default:
		return nil, true
	}
}

func skillQueryTokens(query string) map[string]struct{} {
	query = strings.ToLower(query)
	tokens := make(map[string]struct{})
	var word strings.Builder
	flush := func() {
		if word.Len() == 0 {
			return
		}
		tokens[word.String()] = struct{}{}
		word.Reset()
	}
	for _, r := range query {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' {
			word.WriteRune(r)
			continue
		}
		flush()
	}
	flush()
	for extension, aliases := range map[string][]string{
		".go": {"go", "golang"}, ".rs": {"rust"}, ".ts": {"typescript"},
		".tsx": {"typescript", "react"}, ".py": {"python"}, ".php": {"php"},
	} {
		if strings.Contains(query, extension) {
			for _, alias := range aliases {
				tokens[alias] = struct{}{}
			}
		}
	}
	return tokens
}

func skillCatalogItemBytes(candidate skill.Skill) int {
	return len(candidate.Name) + len(candidate.Description) + len(candidate.Location) + 64
}
