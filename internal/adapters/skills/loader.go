package skills

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/projectTHORN/proton/internal/domain/skill"
)

// Options specifies directories and trust flags for skill discovery.
type Options struct {
	HomeDir        string
	WorkDir        string
	ProjectTrusted bool
}

// DiscoveryResult contains discovered skills and any non-fatal diagnostic warnings.
type DiscoveryResult struct {
	Skills   []skill.Skill
	Warnings []string
}

// Discover scans user-level and project-level directories for Agent Skills.
func Discover(ctx context.Context, opts Options) (DiscoveryResult, error) {
	homeDir := opts.HomeDir
	if homeDir == "" {
		resolvedHome, err := os.UserHomeDir()
		if err != nil {
			return DiscoveryResult{}, fmt.Errorf("resolve home directory for skills: %w", err)
		}
		homeDir = resolvedHome
	}

	workDir := opts.WorkDir
	if workDir == "" {
		resolvedWork, err := os.Getwd()
		if err != nil {
			return DiscoveryResult{}, fmt.Errorf("resolve work directory for skills: %w", err)
		}
		workDir = resolvedWork
	}

	result := DiscoveryResult{
		Skills:   make([]skill.Skill, 0),
		Warnings: make([]string, 0),
	}

	// Map of skill name -> skill, for deduplication.
	// Project skills override user skills.
	userSkills := make(map[string]skill.Skill)
	projectSkills := make(map[string]skill.Skill)

	// 1. User-level scopes
	userPaths := []string{
		filepath.Join(homeDir, ".proton", "skills"),
		filepath.Join(homeDir, ".agents", "skills"),
	}
	for _, dir := range userPaths {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		scanDirectory(dir, skill.ScopeUser, userSkills, &result.Warnings)
	}

	// 2. Project-level scopes
	projectPaths := []string{
		filepath.Join(workDir, ".proton", "skills"),
		filepath.Join(workDir, ".agents", "skills"),
	}

	for _, dir := range projectPaths {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		info, err := os.Stat(dir)
		if err != nil || !info.IsDir() {
			continue
		}

		hasSkills := containsAnySkill(dir)
		if !hasSkills {
			continue
		}

		if !opts.ProjectTrusted {
			result.Warnings = append(result.Warnings, fmt.Sprintf(
				"skipping project skills in %q: project is not trusted (set PROTON_TRUST_PROJECT=1)",
				dir,
			))
			continue
		}

		scanDirectory(dir, skill.ScopeProject, projectSkills, &result.Warnings)
	}

	// Merge: user skills first, then project skills override
	effective := make(map[string]skill.Skill, len(userSkills)+len(projectSkills))
	for name, s := range userSkills {
		effective[name] = s
	}
	for name, s := range projectSkills {
		if _, exists := userSkills[name]; exists {
			result.Warnings = append(result.Warnings, fmt.Sprintf(
				"project skill %q overrides user skill from %q",
				name, userSkills[name].Location,
			))
		}
		effective[name] = s
	}

	for _, s := range effective {
		result.Skills = append(result.Skills, s)
	}

	sort.Slice(result.Skills, func(i, j int) bool {
		return result.Skills[i].Name < result.Skills[j].Name
	})

	return result, nil
}

func scanDirectory(
	rootDir string,
	scope skill.Scope,
	target map[string]skill.Skill,
	warnings *[]string,
) {
	entries, err := os.ReadDir(rootDir)
	if err != nil {
		return // Missing directory is acceptable
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		skillDirName := entry.Name()
		if strings.HasPrefix(skillDirName, ".") || skillDirName == "node_modules" {
			continue
		}

		skillFile := filepath.Join(rootDir, skillDirName, "SKILL.md")
		info, err := os.Stat(skillFile)
		if err != nil || info.IsDir() {
			continue
		}

		parsed, err := ParseSkillFile(skillFile, scope)
		if err != nil {
			*warnings = append(*warnings, fmt.Sprintf("failed to load skill %q: %v", skillFile, err))
			continue
		}

		if existing, exists := target[parsed.Name]; exists {
			*warnings = append(*warnings, fmt.Sprintf(
				"duplicate skill %q in %s scope; keeping %q, ignoring %q",
				parsed.Name, scope, existing.Location, parsed.Location,
			))
			continue
		}

		target[parsed.Name] = parsed
	}
}

func containsAnySkill(rootDir string) bool {
	entries, err := os.ReadDir(rootDir)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		skillFile := filepath.Join(rootDir, entry.Name(), "SKILL.md")
		if info, err := os.Stat(skillFile); err == nil && !info.IsDir() {
			return true
		}
	}
	return false
}
