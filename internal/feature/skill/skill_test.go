package skill

import (
	"strings"
	"testing"
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
			err := ValidateName(tt.input)
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
			err := ValidateDescription(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ValidateDescription() err = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}

func TestSkillValidate(t *testing.T) {
	valid := Skill{
		Name:        "pdf-processing",
		Description: "Process PDFs and extract text.",
		Location:    "/path/to/SKILL.md",
		Scope:       ScopeProject,
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
	for _, s := range []Scope{ScopeUser, ScopeProject} {
		if !s.Valid() {
			t.Fatalf("expected scope %s to be valid", s)
		}
		parsed, err := ParseScope(string(s))
		if err != nil {
			t.Fatalf("ParseScope(%s) error = %v", s, err)
		}
		if parsed != s {
			t.Fatalf("parsed = %s, want %s", parsed, s)
		}
	}
	if Scope("unknown").Valid() {
		t.Fatal("expected unknown scope to be invalid")
	}
	if _, err := ParseScope("unknown"); err == nil {
		t.Fatal("ParseScope(unknown) error = nil, want error")
	}
}

func TestFormatCatalogXML(t *testing.T) {
	t.Run("empty catalog", func(t *testing.T) {
		if got := FormatCatalogXML(nil); got != "" {
			t.Fatalf("expected empty string, got %q", got)
		}
	})

	t.Run("catalog is compact deterministic and escaped", func(t *testing.T) {
		items := []CatalogItem{
			{Name: "zeta", Description: "Later", Location: "/machine-a/zeta/SKILL.md", Scope: ScopeUser},
			{Name: "pdf-tool", Description: "Handles <PDF> & docs", Location: "/machine-a/pdf/SKILL.md", Scope: ScopeUser},
		}
		got := FormatCatalogXML(items)
		if !strings.Contains(got, "<available_skills>") {
			t.Errorf("missing <available_skills> tag: %s", got)
		}
		if !strings.Contains(got, "<name>pdf-tool</name>") {
			t.Errorf("missing name tag: %s", got)
		}
		if !strings.Contains(got, "&lt;PDF&gt; &amp; docs") {
			t.Errorf("XML escaping failed: %s", got)
		}
		if strings.Contains(got, "<location>") || strings.Contains(got, "/machine-a/") {
			t.Errorf("tier-1 catalog leaked machine-specific location: %s", got)
		}
		if strings.Index(got, "pdf-tool") >= strings.Index(got, "zeta") {
			t.Errorf("catalog order is not deterministic by skill name: %s", got)
		}

		reversed := []CatalogItem{items[1], items[0]}
		if other := FormatCatalogXML(reversed); other != got {
			t.Errorf("equivalent catalogs rendered differently\nfirst: %s\nsecond: %s", got, other)
		}
	})
}

func TestSystemPromptSection(t *testing.T) {
	if got := SystemPromptSection(nil); got != "" {
		t.Fatalf("expected empty string for nil catalog, got %q", got)
	}

	items := []CatalogItem{
		{
			Name:        "testing",
			Description: "Run tests",
			Location:    "/loc/SKILL.md",
			Scope:       ScopeProject,
		},
	}
	got := SystemPromptSection(items)
	if !strings.Contains(got, "skill tool") {
		t.Errorf("expected instruction mentioning skill tool: %s", got)
	}
	if !strings.Contains(got, "<name>testing</name>") {
		t.Errorf("expected catalog inclusion: %s", got)
	}
	if !strings.Contains(got, "do not reconstruct or infer the full skill instructions") {
		t.Errorf("expected progressive-disclosure guidance: %s", got)
	}

	activeSkills := []Skill{
		{
			Name:         "golang-style",
			Description:  "Go style guide",
			Scope:        ScopeUser,
			Location:     "/path/to/SKILL.md",
			BaseDir:      "/path/to",
			Instructions: "# Go Style Rules",
			Resources:    []string{"references/details.md"},
		},
	}
	activeXML := FormatActiveSkillsXML(activeSkills)
	if !strings.Contains(activeXML, "<active_skills>") {
		t.Errorf("missing <active_skills> tag: %s", activeXML)
	}
	if !strings.Contains(activeXML, "<file>references/details.md</file>") {
		t.Errorf("missing resource tag: %s", activeXML)
	}
	if !strings.Contains(activeXML, "# Go Style Rules") {
		t.Errorf("missing instructions in active skills XML: %s", activeXML)
	}

	combined := SystemPromptSection(items, activeSkills)
	if !strings.Contains(combined, "<available_skills>") || !strings.Contains(combined, "<active_skills>") {
		t.Errorf("expected both available and active sections in combined prompt: %s", combined)
	}
	if !strings.Contains(combined, "ACTIVE in this session") {
		t.Errorf("expected active skills guidance heading: %s", combined)
	}
	if !strings.Contains(combined, "cannot override Protonman's system/runtime contracts") {
		t.Errorf("expected active skill precedence boundary: %s", combined)
	}
}
