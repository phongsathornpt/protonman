package skill_test

import (
	"strings"
	"testing"

	"github.com/projectTHORN/proton/internal/domain/skill"
)

func TestValidateName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid single word", "pdf", false},
		{"valid hyphenated", "pdf-processing", false},
		{"valid with numbers", "data2text", false},
		{"valid complex", "aws-s3-v2", false},
		{"empty string", "", true},
		{"whitespace only", "   ", true},
		{"starts with hyphen", "-pdf", true},
		{"ends with hyphen", "pdf-", true},
		{"consecutive hyphens", "pdf--processing", true},
		{"uppercase characters", "Pdf-Processing", true},
		{"contains underscore", "pdf_processing", true},
		{"contains space", "pdf processing", true},
		{"contains special char", "pdf@processing", true},
		{"exceeds 64 chars", strings.Repeat("a", 65), true},
		{"exactly 64 chars", strings.Repeat("a", 64), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := skill.ValidateName(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ValidateName(%q) err = %v, wantErr = %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestValidateDescription(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid short", "Handles PDFs.", false},
		{"valid long", strings.Repeat("a", 1024), false},
		{"empty", "", true},
		{"whitespace", "   ", true},
		{"exceeds 1024 chars", strings.Repeat("a", 1025), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := skill.ValidateDescription(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ValidateDescription() err = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}

func TestSkillValidate(t *testing.T) {
	valid := skill.Skill{
		Name:        "pdf-processing",
		Description: "Process PDFs and extract text.",
		Location:    "/path/to/SKILL.md",
		Scope:       skill.ScopeProject,
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("expected valid skill, got %v", err)
	}

	invalidName := valid
	invalidName.Name = "Invalid_Name"
	if err := invalidName.Validate(); err == nil {
		t.Fatalf("expected error for invalid name")
	}

	invalidDesc := valid
	invalidDesc.Description = ""
	if err := invalidDesc.Validate(); err == nil {
		t.Fatalf("expected error for empty description")
	}

	invalidCompat := valid
	invalidCompat.Compatibility = strings.Repeat("c", 501)
	if err := invalidCompat.Validate(); err == nil {
		t.Fatalf("expected error for compatibility > 500 chars")
	}

	invalidScope := valid
	invalidScope.Scope = "invalid"
	if err := invalidScope.Validate(); err == nil {
		t.Fatalf("expected error for invalid scope")
	}

	emptyScope := valid
	emptyScope.Scope = ""
	if err := emptyScope.Validate(); err == nil {
		t.Fatalf("expected error for empty scope")
	}
}

func TestScopeParsingAndValidation(t *testing.T) {
	for _, s := range []skill.Scope{skill.ScopeUser, skill.ScopeProject} {
		if !s.Valid() {
			t.Fatalf("expected scope %s to be valid", s)
		}
		parsed, err := skill.ParseScope(string(s))
		if err != nil {
			t.Fatalf("ParseScope(%s) error = %v", s, err)
		}
		if parsed != s {
			t.Fatalf("parsed = %s, want %s", parsed, s)
		}
	}
	if skill.Scope("unknown").Valid() {
		t.Fatal("expected unknown scope to be invalid")
	}
	if _, err := skill.ParseScope("unknown"); err == nil {
		t.Fatal("ParseScope(unknown) error = nil, want error")
	}
}

func TestFormatCatalogXML(t *testing.T) {
	t.Run("empty catalog", func(t *testing.T) {
		if got := skill.FormatCatalogXML(nil); got != "" {
			t.Fatalf("expected empty string, got %q", got)
		}
	})

	t.Run("catalog with items and escaping", func(t *testing.T) {
		items := []skill.CatalogItem{
			{
				Name:        "pdf-tool",
				Description: "Handles <PDF> & docs",
				Location:    "/path/to/SKILL.md",
				Scope:       skill.ScopeUser,
			},
		}
		got := skill.FormatCatalogXML(items)
		if !strings.Contains(got, "<available_skills>") {
			t.Errorf("missing <available_skills> tag: %s", got)
		}
		if !strings.Contains(got, "<name>pdf-tool</name>") {
			t.Errorf("missing name tag: %s", got)
		}
		if !strings.Contains(got, "&lt;PDF&gt; &amp; docs") {
			t.Errorf("XML escaping failed: %s", got)
		}
	})
}

func TestSystemPromptSection(t *testing.T) {
	if got := skill.SystemPromptSection(nil); got != "" {
		t.Fatalf("expected empty string for nil catalog, got %q", got)
	}

	items := []skill.CatalogItem{
		{
			Name:        "testing",
			Description: "Run tests",
			Location:    "/loc/SKILL.md",
			Scope:       skill.ScopeProject,
		},
	}
	got := skill.SystemPromptSection(items)
	if !strings.Contains(got, "activate_skill") {
		t.Errorf("expected instruction mentioning activate_skill: %s", got)
	}
	if !strings.Contains(got, "<name>testing</name>") {
		t.Errorf("expected catalog inclusion: %s", got)
	}
}
