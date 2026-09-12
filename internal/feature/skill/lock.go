package skill

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/phongsathornpt/protonman/internal/app/appdirs"
)

const (
	// LockFileName is the standard project lock file name for agent skills.
	LockFileName = "skills-lock.json"
	// CurrentLockVersion is the supported schema version for skills-lock.json.
	CurrentLockVersion = 1

	// LockStatusVerified indicates a skill matches its locked content hash.
	LockStatusVerified = "verified"
	// LockStatusDrifted indicates a skill on disk differs from its locked hash.
	LockStatusDrifted = "drifted"
	// LockStatusMissing indicates a skill recorded in the lock file is absent from disk.
	LockStatusMissing = "missing"
	// LockStatusUnlocked indicates a skill on disk is not tracked in the lock file.
	LockStatusUnlocked = "unlocked"
)

var (
	// ErrInvalidLockVersion indicates an unsupported skills-lock.json schema version.
	ErrInvalidLockVersion = errors.New("unsupported skill lock file version")
	// ErrEmptyLock indicates a lock file has no valid content or skills mapping.
	ErrEmptyLock = errors.New("empty skill lock file")
)

// LockEntry describes a single pinned skill in the project lock file.
type LockEntry struct {
	Source       string `json:"source"`
	SourceType   string `json:"sourceType"`
	ComputedHash string `json:"computedHash"`
	Ref          string `json:"ref,omitempty"`
	SkillPath    string `json:"skillPath,omitempty"`
}

// LockFile represents the project-level skills-lock.json structure.
type LockFile struct {
	Version int                  `json:"version"`
	Skills  map[string]LockEntry `json:"skills"`
}

// NewLockFile creates an empty LockFile with CurrentLockVersion.
func NewLockFile() LockFile {
	return LockFile{
		Version: CurrentLockVersion,
		Skills:  make(map[string]LockEntry),
	}
}

// ResolveProjectLockPath locates any existing skills-lock.json for the workspace
// or returns the default root project path where a new lockfile should be written.
func ResolveProjectLockPath(workDir string, projectScope *appdirs.ProjectScope) (string, bool) {
	candidates := make([]string, 0, 3)
	if strings.TrimSpace(workDir) != "" {
		candidates = append(candidates, filepath.Join(workDir, LockFileName))
	}
	if projectScope != nil && projectScope.Available && projectScope.Root != "" {
		candidates = append(candidates, filepath.Join(projectScope.Root, LockFileName))
	}
	if strings.TrimSpace(workDir) != "" {
		candidates = append(candidates, filepath.Join(workDir, ".agents", LockFileName))
	}

	for _, path := range candidates {
		info, err := os.Stat(path)
		if err == nil && !info.IsDir() {
			return path, true
		}
	}

	defaultPath := filepath.Join(workDir, LockFileName)
	return defaultPath, false
}

// ReadLockFile reads and parses a skills-lock.json file.
func ReadLockFile(filePath string) (LockFile, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return NewLockFile(), fmt.Errorf("read skill lock file %q: %w", filePath, err)
	}

	var lock LockFile
	if err := json.Unmarshal(data, &lock); err != nil {
		return NewLockFile(), fmt.Errorf("parse skill lock file %q: %w", filePath, err)
	}

	if lock.Version < CurrentLockVersion {
		return NewLockFile(), fmt.Errorf("%w: got %d, want >= %d", ErrInvalidLockVersion, lock.Version, CurrentLockVersion)
	}

	if lock.Skills == nil {
		lock.Skills = make(map[string]LockEntry)
	}

	// Normalize keys to lowercase trimmed
	normalized := make(map[string]LockEntry, len(lock.Skills))
	for k, v := range lock.Skills {
		cleanKey := strings.ToLower(strings.TrimSpace(k))
		v.ComputedHash = strings.ToLower(strings.TrimSpace(v.ComputedHash))
		normalized[cleanKey] = v
	}
	lock.Skills = normalized

	return lock, nil
}

// WriteLockFile writes a skills-lock.json file deterministically (sorted keys, 2-space indentation, trailing newline).
func WriteLockFile(filePath string, lock LockFile) error {
	if lock.Version == 0 {
		lock.Version = CurrentLockVersion
	}
	if lock.Skills == nil {
		lock.Skills = make(map[string]LockEntry)
	}

	// json.Marshal on map[string]T in Go sorts keys lexicographically, guaranteeing determinism.
	data, err := json.MarshalIndent(lock, "", "  ")
	if err != nil {
		return fmt.Errorf("serialize skill lock file: %w", err)
	}
	data = append(data, '\n')

	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create skill lock directory %q: %w", dir, err)
	}

	// Atomic write via temporary file
	tmpFile, err := os.CreateTemp(dir, "skills-lock-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary skill lock file: %w", err)
	}
	tmpName := tmpFile.Name()
	defer func() { _ = os.Remove(tmpName) }()

	if _, err := tmpFile.Write(data); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("write temporary skill lock file: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("close temporary skill lock file: %w", err)
	}

	if err := os.Rename(tmpName, filePath); err != nil {
		return fmt.Errorf("replace skill lock file %q: %w", filePath, err)
	}

	return nil
}

type fileHashEntry struct {
	relPath string
	content []byte
}

// ComputeSkillFolderHash computes a deterministic SHA-256 hash over the contents of a skill directory.
// It skips .git and node_modules directories, sorts file paths lexicographically using forward slashes,
// and feeds the normalized relative path followed by file content bytes into the digest.
func ComputeSkillFolderHash(skillDir string) (string, error) {
	info, err := os.Stat(skillDir)
	if err != nil {
		return "", fmt.Errorf("stat skill directory %q: %w", skillDir, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("skill path %q is not a directory", skillDir)
	}

	var files []fileHashEntry
	err = filepath.WalkDir(skillDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			name := d.Name()
			if name == ".git" || name == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(skillDir, path)
		if err != nil {
			return fmt.Errorf("relative path for %q: %w", path, err)
		}
		relSlash := filepath.ToSlash(rel)

		content, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read file %q for hashing: %w", path, err)
		}

		files = append(files, fileHashEntry{
			relPath: relSlash,
			content: content,
		})
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("scan skill folder %q: %w", skillDir, err)
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].relPath < files[j].relPath
	})

	h := sha256.New()
	for _, f := range files {
		h.Write([]byte(f.relPath))
		h.Write(f.content)
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

// SkillCheckResult reports the lock status and hash comparison of a skill.
type SkillCheckResult struct {
	Name         string `json:"name"`
	Status       string `json:"status"` // verified, drifted, missing, unlocked
	ExpectedHash string `json:"expected_hash,omitempty"`
	ComputedHash string `json:"computed_hash,omitempty"`
	Source       string `json:"source,omitempty"`
	SourceType   string `json:"source_type,omitempty"`
	Location     string `json:"location,omitempty"`
	Message      string `json:"message,omitempty"`
}

// ProjectLockReport aggregates the status of all project skills against the lock file.
type ProjectLockReport struct {
	LockPath string             `json:"lock_path"`
	HasLock  bool               `json:"has_lock"`
	Version  int                `json:"version"`
	Results  []SkillCheckResult `json:"results"`
}

// IsClean reports whether all locked skills match and there are no drifted or missing skills.
func (r ProjectLockReport) IsClean() bool {
	if !r.HasLock {
		return true
	}
	for _, res := range r.Results {
		if res.Status == LockStatusDrifted || res.Status == LockStatusMissing {
			return false
		}
	}
	return true
}

// Summary returns count breakdown: verified, drifted, missing, unlocked.
func (r ProjectLockReport) Summary() (verified, drifted, missing, unlocked int) {
	for _, res := range r.Results {
		switch res.Status {
		case LockStatusVerified:
			verified++
		case LockStatusDrifted:
			drifted++
		case LockStatusMissing:
			missing++
		case LockStatusUnlocked:
			unlocked++
		}
	}
	return
}

// VerifyProjectSkills checks project-scoped skills against a LockFile.
func VerifyProjectSkills(lock LockFile, projectSkills []Skill) ProjectLockReport {
	report := ProjectLockReport{
		HasLock: len(lock.Skills) > 0,
		Version: lock.Version,
		Results: make([]SkillCheckResult, 0),
	}

	matchedLocked := make(map[string]bool)

	for _, s := range projectSkills {
		cleanName := strings.ToLower(strings.TrimSpace(s.Name))
		entry, isLocked := lock.Skills[cleanName]
		computed := s.ComputedHash
		if s.BaseDir != "" {
			freshHash, err := ComputeSkillFolderHash(s.BaseDir)
			if err != nil {
				report.Results = append(report.Results, SkillCheckResult{
					Name:     cleanName,
					Status:   LockStatusDrifted,
					Location: s.Location,
					Message:  fmt.Sprintf("failed to hash skill: %v", err),
				})
				continue
			}
			computed = freshHash
		}

		if !isLocked {
			report.Results = append(report.Results, SkillCheckResult{
				Name:         cleanName,
				Status:       LockStatusUnlocked,
				ComputedHash: computed,
				Location:     s.Location,
				Message:      "skill is not recorded in project lock file",
			})
			continue
		}

		matchedLocked[cleanName] = true
		if computed == entry.ComputedHash {
			report.Results = append(report.Results, SkillCheckResult{
				Name:         cleanName,
				Status:       LockStatusVerified,
				ExpectedHash: entry.ComputedHash,
				ComputedHash: computed,
				Source:       entry.Source,
				SourceType:   entry.SourceType,
				Location:     s.Location,
			})
		} else {
			report.Results = append(report.Results, SkillCheckResult{
				Name:         cleanName,
				Status:       LockStatusDrifted,
				ExpectedHash: entry.ComputedHash,
				ComputedHash: computed,
				Source:       entry.Source,
				SourceType:   entry.SourceType,
				Location:     s.Location,
				Message:      fmt.Sprintf("content drift: expected %s, got %s", entry.ComputedHash, computed),
			})
		}
	}

	// Identify missing skills: in lock but not in project skills
	for name, entry := range lock.Skills {
		if !matchedLocked[name] {
			report.Results = append(report.Results, SkillCheckResult{
				Name:         name,
				Status:       LockStatusMissing,
				ExpectedHash: entry.ComputedHash,
				Source:       entry.Source,
				SourceType:   entry.SourceType,
				Message:      "skill in lockfile but not found on disk",
			})
		}
	}

	sort.Slice(report.Results, func(i, j int) bool {
		return report.Results[i].Name < report.Results[j].Name
	})

	return report
}

// GenerateProjectLock creates a LockFile for the given project skills, preserving
// any existing metadata (source, sourceType, ref, skillPath) for skills already in existingLock.
func GenerateProjectLock(projectSkills []Skill, existingLock *LockFile) (LockFile, error) {
	lock := NewLockFile()

	for _, s := range projectSkills {
		cleanName := strings.ToLower(strings.TrimSpace(s.Name))
		if cleanName == "" {
			continue
		}

		hash := s.ComputedHash
		if hash == "" && s.BaseDir != "" {
			var err error
			hash, err = ComputeSkillFolderHash(s.BaseDir)
			if err != nil {
				return lock, fmt.Errorf("compute hash for skill %q: %w", s.Name, err)
			}
		}

		entry := LockEntry{
			Source:       "./" + filepath.Base(s.BaseDir),
			SourceType:   "local",
			ComputedHash: hash,
		}

		if existingLock != nil {
			if old, ok := existingLock.Skills[cleanName]; ok {
				if old.Source != "" {
					entry.Source = old.Source
				}
				if old.SourceType != "" {
					entry.SourceType = old.SourceType
				}
				entry.Ref = old.Ref
				entry.SkillPath = old.SkillPath
			}
		}

		lock.Skills[cleanName] = entry
	}

	return lock, nil
}
