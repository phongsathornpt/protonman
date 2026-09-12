package skill

import (
	"context"
	"fmt"
	"github.com/phongsathornpt/protonman/internal/app/appdirs"
	"github.com/phongsathornpt/protonman/internal/base/envconfig"
	"maps"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Options specifies directories and trust flags for skill discovery.
type Options struct {
	HomeDir        string
	WorkDir        string
	ProjectTrusted bool
	ProjectScope   *appdirs.ProjectScope
}

// DiscoveryResult contains discovered skills and any non-fatal diagnostic warnings.
type DiscoveryResult struct {
	Skills     []Skill
	Warnings   []string
	LockPath   string
	LockReport *ProjectLockReport
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

	userDirs, err := appdirs.Resolve(homeDir)
	if err != nil {
		return DiscoveryResult{}, err
	}

	result := DiscoveryResult{
		Skills:   make([]Skill, 0),
		Warnings: make([]string, 0),
	}

	// Map of skill name -> skill, for deduplication.
	// Project skills override user skills.
	userSkills := make(map[string]Skill)
	projectSkills := make(map[string]Skill)

	// 1. User-level scopes
	userPaths := []string{
		userDirs.Skills,
		filepath.Join(homeDir, ".agents", "skills"),
	}
	for _, dir := range userPaths {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		scanDirectory(dir, ScopeUser, userSkills, &result.Warnings)
	}

	// 2. Project-level scopes. When the workspace is the user home, these
	// paths alias user-global roots and must not be treated as project input.
	var projectScope appdirs.ProjectScope
	if opts.ProjectScope != nil {
		projectScope = *opts.ProjectScope
	} else {
		projectScope, err = appdirs.ResolveProjectScope(homeDir, workDir)
		if err != nil {
			return result, fmt.Errorf("resolve project skill scope: %w", err)
		}
	}
	projectPaths := make([]string, 0, 2)
	if projectScope.Available {
		projectPaths = append(projectPaths,
			projectScope.Skills,
			filepath.Join(workDir, ".agents", "skills"),
		)
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
				"skipping project skills in %q: project is not trusted (set %s=1)",
				dir, envconfig.TrustProject,
			))
			continue
		}

		scanDirectory(dir, ScopeProject, projectSkills, &result.Warnings)
	}

	// 3. Project skill lock verification (skills-lock.json)
	if projectScope.Available && opts.ProjectTrusted {
		if targetPath, migrated, migErr := MigrateProjectLockLocation(workDir, &projectScope); migErr == nil && migrated {
			result.LockPath = targetPath
		}
		lockPath, hasLock := ResolveProjectLockPath(workDir, &projectScope)
		result.LockPath = lockPath
		if hasLock {
			lock, err := ReadLockFile(lockPath)
			if err != nil {
				if migLock, migrated, migErr := MigrateLockFile(lockPath); migErr == nil && migrated {
					lock = migLock
					err = nil
				}
			}
			if err != nil {
				result.Warnings = append(result.Warnings, fmt.Sprintf(
					"failed to read project skill lock %q: %v", lockPath, err,
				))
			} else {
				projSkillsList := make([]Skill, 0, len(projectSkills))
				for _, s := range projectSkills {
					projSkillsList = append(projSkillsList, s)
				}
				report := VerifyProjectSkills(lock, projSkillsList)
				report.LockPath = lockPath
				result.LockReport = &report

				for _, res := range report.Results {
					if s, ok := projectSkills[res.Name]; ok {
						s.ComputedHash = res.ComputedHash
						s.Locked = (res.Status == LockStatusVerified || res.Status == LockStatusDrifted)
						s.LockStatus = res.Status
						projectSkills[res.Name] = s
					}
					if res.Status == LockStatusDrifted {
						result.Warnings = append(result.Warnings, fmt.Sprintf(
							"warning: project skill %q integrity mismatch: computed hash %s != locked hash %s (possible drift or tampering)",
							res.Name, res.ComputedHash, res.ExpectedHash,
						))
					} else if res.Status == LockStatusMissing {
						result.Warnings = append(result.Warnings, fmt.Sprintf(
							"warning: locked project skill %q is missing from project skills directory",
							res.Name,
						))
					}
				}
			}
		} else {
			for name, s := range projectSkills {
				if s.ComputedHash == "" && s.BaseDir != "" {
					h, err := ComputeSkillFolderHash(s.BaseDir)
					if err == nil {
						s.ComputedHash = h
					}
				}
				s.Locked = false
				s.LockStatus = LockStatusUnlocked
				projectSkills[name] = s
			}
		}
	}

	// Merge: user skills first, then project skills override
	effective := make(map[string]Skill, len(userSkills)+len(projectSkills))
	maps.Copy(effective, userSkills)
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
	scope Scope,
	target map[string]Skill,
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
