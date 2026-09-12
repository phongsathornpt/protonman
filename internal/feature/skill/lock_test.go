package skill

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/phongsathornpt/protonman/internal/app/appdirs"
)

func TestComputeSkillFolderHash(t *testing.T) {
	t.Run("deterministic and hex format", func(t *testing.T) {
		dir := t.TempDir()
		skillDir := filepath.Join(dir, "my-skill")
		if err := os.MkdirAll(skillDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: my-skill\ndescription: A test skill\n---\n# Instructions\n"), 0o644); err != nil {
			t.Fatal(err)
		}

		h1, err := ComputeSkillFolderHash(skillDir)
		if err != nil {
			t.Fatalf("ComputeSkillFolderHash failed: %v", err)
		}
		h2, err := ComputeSkillFolderHash(skillDir)
		if err != nil {
			t.Fatalf("ComputeSkillFolderHash 2 failed: %v", err)
		}
		if h1 != h2 {
			t.Fatalf("hashes differ: %q != %q", h1, h2)
		}
		if len(h1) != 64 {
			t.Fatalf("hash length = %d, want 64 hex chars", len(h1))
		}
	})

	t.Run("changes on content change", func(t *testing.T) {
		dir := t.TempDir()
		skillDir := filepath.Join(dir, "my-skill")
		if err := os.MkdirAll(skillDir, 0o755); err != nil {
			t.Fatal(err)
		}
		skillFile := filepath.Join(skillDir, "SKILL.md")
		if err := os.WriteFile(skillFile, []byte("version 1"), 0o644); err != nil {
			t.Fatal(err)
		}

		h1, err := ComputeSkillFolderHash(skillDir)
		if err != nil {
			t.Fatal(err)
		}

		if err := os.WriteFile(skillFile, []byte("version 2"), 0o644); err != nil {
			t.Fatal(err)
		}

		h2, err := ComputeSkillFolderHash(skillDir)
		if err != nil {
			t.Fatal(err)
		}
		if h1 == h2 {
			t.Fatalf("hash should change on content edit, got identical %q", h1)
		}
	})

	t.Run("changes when file added or renamed", func(t *testing.T) {
		dir := t.TempDir()
		skillDir := filepath.Join(dir, "my-skill")
		if err := os.MkdirAll(skillDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("content"), 0o644); err != nil {
			t.Fatal(err)
		}

		h1, err := ComputeSkillFolderHash(skillDir)
		if err != nil {
			t.Fatal(err)
		}

		extraFile := filepath.Join(skillDir, "extra.txt")
		if err := os.WriteFile(extraFile, []byte("extra file"), 0o644); err != nil {
			t.Fatal(err)
		}

		h2, err := ComputeSkillFolderHash(skillDir)
		if err != nil {
			t.Fatal(err)
		}
		if h1 == h2 {
			t.Fatalf("hash should change when file is added")
		}

		// Renaming extra.txt to other.txt
		if err := os.Rename(extraFile, filepath.Join(skillDir, "other.txt")); err != nil {
			t.Fatal(err)
		}
		h3, err := ComputeSkillFolderHash(skillDir)
		if err != nil {
			t.Fatal(err)
		}
		if h2 == h3 {
			t.Fatalf("hash should change when file is renamed")
		}
	})

	t.Run("ignores .git and node_modules", func(t *testing.T) {
		dir := t.TempDir()
		skillDir := filepath.Join(dir, "my-skill")
		if err := os.MkdirAll(skillDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("content"), 0o644); err != nil {
			t.Fatal(err)
		}

		h1, err := ComputeSkillFolderHash(skillDir)
		if err != nil {
			t.Fatal(err)
		}

		// Create .git and node_modules inside skill
		gitDir := filepath.Join(skillDir, ".git")
		_ = os.MkdirAll(gitDir, 0o755)
		_ = os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("ref: refs/heads/main"), 0o644)

		nmDir := filepath.Join(skillDir, "node_modules", "pkg")
		_ = os.MkdirAll(nmDir, 0o755)
		_ = os.WriteFile(filepath.Join(nmDir, "index.js"), []byte("module.exports = {}"), 0o644)

		h2, err := ComputeSkillFolderHash(skillDir)
		if err != nil {
			t.Fatal(err)
		}
		if h1 != h2 {
			t.Fatalf(".git and node_modules should be ignored, got %q != %q", h1, h2)
		}
	})
}

func TestReadWriteLockFile(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, "skills-lock.json")

	t.Run("read non-existent file returns error", func(t *testing.T) {
		_, err := ReadLockFile(lockPath)
		if err == nil {
			t.Fatal("expected error reading missing lock file")
		}
	})

	t.Run("write and read valid lock file", func(t *testing.T) {
		lock := LockFile{
			Version: 1,
			Skills: map[string]LockEntry{
				"zebra": {
					Source:       "org/zebra",
					SourceType:   "github",
					ComputedHash: "zzzz",
				},
				"alpha": {
					Source:       "org/alpha",
					SourceType:   "github",
					ComputedHash: "aaaa",
				},
			},
		}

		if err := WriteLockFile(lockPath, lock); err != nil {
			t.Fatalf("WriteLockFile failed: %v", err)
		}

		// Verify trailing newline
		raw, err := os.ReadFile(lockPath)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasSuffix(string(raw), "\n") {
			t.Fatal("lock file should end with newline")
		}

		// Verify alphabetical key sorting in raw JSON
		alphaIdx := strings.Index(string(raw), "\"alpha\"")
		zebraIdx := strings.Index(string(raw), "\"zebra\"")
		if alphaIdx == -1 || zebraIdx == -1 || alphaIdx >= zebraIdx {
			t.Fatalf("keys should be sorted alphabetically: alpha=%d, zebra=%d", alphaIdx, zebraIdx)
		}

		read, err := ReadLockFile(lockPath)
		if err != nil {
			t.Fatalf("ReadLockFile failed: %v", err)
		}
		if read.Version != 1 {
			t.Fatalf("version = %d, want 1", read.Version)
		}
		if len(read.Skills) != 2 {
			t.Fatalf("skill count = %d, want 2", len(read.Skills))
		}
		if read.Skills["alpha"].ComputedHash != "aaaa" {
			t.Fatalf("alpha hash = %q, want 'aaaa'", read.Skills["alpha"].ComputedHash)
		}
	})

	t.Run("rejects invalid version", func(t *testing.T) {
		invalidPath := filepath.Join(dir, "invalid-version.json")
		if err := os.WriteFile(invalidPath, []byte(`{"version": 0, "skills": {}}`), 0o644); err != nil {
			t.Fatal(err)
		}
		_, err := ReadLockFile(invalidPath)
		if err == nil {
			t.Fatal("expected error for version 0")
		}
	})
}

func TestVerifyProjectSkills(t *testing.T) {
	dir := t.TempDir()

	// Skill 1: verified
	s1Dir := filepath.Join(dir, "s1")
	_ = os.MkdirAll(s1Dir, 0o755)
	_ = os.WriteFile(filepath.Join(s1Dir, "SKILL.md"), []byte("skill 1"), 0o644)
	h1, _ := ComputeSkillFolderHash(s1Dir)

	// Skill 2: drifted
	s2Dir := filepath.Join(dir, "s2")
	_ = os.MkdirAll(s2Dir, 0o755)
	_ = os.WriteFile(filepath.Join(s2Dir, "SKILL.md"), []byte("skill 2 modified"), 0o644)

	// Skill 3: unlocked (not in lock)
	s3Dir := filepath.Join(dir, "s3")
	_ = os.MkdirAll(s3Dir, 0o755)
	_ = os.WriteFile(filepath.Join(s3Dir, "SKILL.md"), []byte("skill 3"), 0o644)

	skills := []Skill{
		{Name: "s1", Scope: ScopeProject, BaseDir: s1Dir, Location: filepath.Join(s1Dir, "SKILL.md")},
		{Name: "s2", Scope: ScopeProject, BaseDir: s2Dir, Location: filepath.Join(s2Dir, "SKILL.md")},
		{Name: "s3", Scope: ScopeProject, BaseDir: s3Dir, Location: filepath.Join(s3Dir, "SKILL.md")},
	}

	lock := LockFile{
		Version: 1,
		Skills: map[string]LockEntry{
			"s1": {
				Source:       "org/s1",
				SourceType:   "github",
				ComputedHash: h1,
			},
			"s2": {
				Source:       "org/s2",
				SourceType:   "github",
				ComputedHash: "old-expected-hash-that-wont-match",
			},
			"s-missing": {
				Source:       "org/s-missing",
				SourceType:   "github",
				ComputedHash: "hash-missing",
			},
		},
	}

	report := VerifyProjectSkills(lock, skills)
	if report.IsClean() {
		t.Fatal("report should not be clean with drifted and missing skills")
	}

	verified, drifted, missing, unlocked := report.Summary()
	if verified != 1 {
		t.Errorf("verified = %d, want 1", verified)
	}
	if drifted != 1 {
		t.Errorf("drifted = %d, want 1", drifted)
	}
	if missing != 1 {
		t.Errorf("missing = %d, want 1", missing)
	}
	if unlocked != 1 {
		t.Errorf("unlocked = %d, want 1", unlocked)
	}
}

func TestGenerateProjectLock(t *testing.T) {
	dir := t.TempDir()
	sDir := filepath.Join(dir, "review")
	_ = os.MkdirAll(sDir, 0o755)
	_ = os.WriteFile(filepath.Join(sDir, "SKILL.md"), []byte("review instructions"), 0o644)

	skills := []Skill{
		{Name: "review", Scope: ScopeProject, BaseDir: sDir, Location: filepath.Join(sDir, "SKILL.md")},
	}

	existing := &LockFile{
		Version: 1,
		Skills: map[string]LockEntry{
			"review": {
				Source:     "org/review-tool",
				SourceType: "github",
				Ref:        "v1.2.0",
			},
		},
	}

	generated, err := GenerateProjectLock(skills, existing)
	if err != nil {
		t.Fatalf("GenerateProjectLock failed: %v", err)
	}

	entry, ok := generated.Skills["review"]
	if !ok {
		t.Fatal("expected 'review' skill in generated lock")
	}
	if entry.Source != "org/review-tool" {
		t.Errorf("source = %q, want 'org/review-tool'", entry.Source)
	}
	if entry.Ref != "v1.2.0" {
		t.Errorf("ref = %q, want 'v1.2.0'", entry.Ref)
	}
	if len(entry.ComputedHash) != 64 {
		t.Errorf("computedHash length = %d, want 64", len(entry.ComputedHash))
	}
}

func TestDiscoverWithProjectLock(t *testing.T) {
	home := t.TempDir()
	work := t.TempDir()

	// Create project skill
	projSkillDir := filepath.Join(work, ".protonman", "skills", "doc-gen")
	createSkill(t, filepath.Join(work, ".protonman", "skills"), "doc-gen", "Generate documentation")

	h, err := ComputeSkillFolderHash(projSkillDir)
	if err != nil {
		t.Fatal(err)
	}

	// Create project lock with matching hash
	lock := LockFile{
		Version: 1,
		Skills: map[string]LockEntry{
			"doc-gen": {
				Source:       "local",
				SourceType:   "local",
				ComputedHash: h,
			},
		},
	}
	lockPath := filepath.Join(work, "skills-lock.json")
	if err := WriteLockFile(lockPath, lock); err != nil {
		t.Fatal(err)
	}

	t.Run("matching lock discovers verified skill", func(t *testing.T) {
		res, err := Discover(context.Background(), Options{
			HomeDir:        home,
			WorkDir:        work,
			ProjectTrusted: true,
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(res.Skills) != 1 {
			t.Fatalf("discovered skills = %d, want 1", len(res.Skills))
		}
		s := res.Skills[0]
		if !s.Locked {
			t.Error("expected skill to be marked locked")
		}
		if s.LockStatus != LockStatusVerified {
			t.Errorf("lock status = %q, want %q", s.LockStatus, LockStatusVerified)
		}
		if res.LockReport == nil || !res.LockReport.IsClean() {
			t.Error("expected clean lock report")
		}
	})

	t.Run("drifted lock reports warning", func(t *testing.T) {
		// Tamper with skill file
		_ = os.WriteFile(filepath.Join(projSkillDir, "SKILL.md"), []byte("---\nname: doc-gen\ndescription: Tampered\n---\n# Tampered"), 0o644)

		res, err := Discover(context.Background(), Options{
			HomeDir:        home,
			WorkDir:        work,
			ProjectTrusted: true,
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(res.Skills) != 1 {
			t.Fatalf("discovered skills = %d, want 1", len(res.Skills))
		}
		s := res.Skills[0]
		if !s.Locked {
			t.Error("expected skill to be marked locked")
		}
		if s.LockStatus != LockStatusDrifted {
			t.Errorf("lock status = %q, want %q", s.LockStatus, LockStatusDrifted)
		}
		hasDriftWarn := false
		for _, w := range res.Warnings {
			if strings.Contains(w, "integrity mismatch") {
				hasDriftWarn = true
				break
			}
		}
		if !hasDriftWarn {
			t.Errorf("expected drift warning, got warnings: %v", res.Warnings)
		}
	})
}

func TestMigrateLockFile(t *testing.T) {
	dir := t.TempDir()

	t.Run("migrates v0 unversioned lock file with legacy keys", func(t *testing.T) {
		lockPath := filepath.Join(dir, "legacy-lock.json")
		legacyContent := `{
			"skills": {
				"My-Skill": {
					"source": "local",
					"source_type": "git",
					"computed_hash": "AABBCCDD",
					"skill_path": "skills/my-skill"
				}
			}
		}`
		if err := os.WriteFile(lockPath, []byte(legacyContent), 0o644); err != nil {
			t.Fatal(err)
		}

		lock, migrated, err := MigrateLockFile(lockPath)
		if err != nil {
			t.Fatalf("MigrateLockFile failed: %v", err)
		}
		if !migrated {
			t.Fatal("expected migrated = true")
		}
		if lock.Version != CurrentLockVersion {
			t.Fatalf("lock version = %d, want %d", lock.Version, CurrentLockVersion)
		}
		entry, ok := lock.Skills["my-skill"]
		if !ok {
			t.Fatal("expected normalized lowercase key 'my-skill'")
		}
		if entry.SourceType != "git" || entry.ComputedHash != "aabbccdd" || entry.SkillPath != "skills/my-skill" {
			t.Fatalf("entry fields not unmarshaled correctly: %+v", entry)
		}

		diskLock, err := ReadLockFile(lockPath)
		if err != nil {
			t.Fatalf("ReadLockFile on migrated file failed: %v", err)
		}
		if diskLock.Version != CurrentLockVersion {
			t.Fatalf("disk lock version = %d, want %d", diskLock.Version, CurrentLockVersion)
		}
	})

	t.Run("leaves current version unchanged", func(t *testing.T) {
		currentPath := filepath.Join(dir, "current-lock.json")
		lock := NewLockFile()
		lock.Skills["test"] = LockEntry{Source: "test", ComputedHash: "1234"}
		if err := WriteLockFile(currentPath, lock); err != nil {
			t.Fatal(err)
		}

		_, migrated, err := MigrateLockFile(currentPath)
		if err != nil {
			t.Fatalf("MigrateLockFile failed: %v", err)
		}
		if migrated {
			t.Fatal("expected migrated = false for current version")
		}
	})
}

func TestMigrateProjectLockLocation(t *testing.T) {
	workDir := t.TempDir()
	scope := appdirs.ProjectScope{
		Root:      filepath.Join(workDir, ".protonman"),
		Available: true,
	}
	if err := os.MkdirAll(scope.Root, 0o755); err != nil {
		t.Fatal(err)
	}

	rootLockPath := filepath.Join(workDir, "skills-lock.json")
	lock := NewLockFile()
	lock.Skills["foo"] = LockEntry{Source: "foo", ComputedHash: "abcd"}
	if err := WriteLockFile(rootLockPath, lock); err != nil {
		t.Fatal(err)
	}

	targetPath, migrated, err := MigrateProjectLockLocation(workDir, &scope)
	if err != nil {
		t.Fatalf("MigrateProjectLockLocation failed: %v", err)
	}
	if !migrated {
		t.Fatal("expected migrated = true")
	}
	if targetPath != filepath.Join(scope.Root, "skills-lock.json") {
		t.Fatalf("targetPath = %q", targetPath)
	}

	targetLock, err := ReadLockFile(targetPath)
	if err != nil {
		t.Fatalf("ReadLockFile failed: %v", err)
	}
	if _, ok := targetLock.Skills["foo"]; !ok {
		t.Fatal("expected skill 'foo' in migrated lockfile")
	}

	if _, err := os.Stat(rootLockPath + ".bak"); err != nil {
		t.Fatalf("expected backup file %s.bak to exist: %v", rootLockPath, err)
	}
	if _, err := os.Stat(rootLockPath); !os.IsNotExist(err) {
		t.Fatalf("expected original %s to be moved: err=%v", rootLockPath, err)
	}
}
