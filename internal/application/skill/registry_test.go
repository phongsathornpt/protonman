package skill_test

import (
	"testing"

	applicationskill "github.com/projectTHORN/proton/internal/application/skill"
	domainskill "github.com/projectTHORN/proton/internal/domain/skill"
)

func TestRegistry(t *testing.T) {
	s1 := domainskill.Skill{
		Name:        "pdf-tool",
		Description: "Process PDFs",
		Location:    "/path/to/pdf/SKILL.md",
		Scope:       domainskill.ScopeUser,
	}
	s2 := domainskill.Skill{
		Name:        "csv-tool",
		Description: "Process CSVs",
		Location:    "/path/to/csv/SKILL.md",
		Scope:       domainskill.ScopeProject,
	}

	reg := applicationskill.NewRegistry(s1)

	// Test Lookup
	if found, ok := reg.Lookup("pdf-tool"); !ok || found.Name != "pdf-tool" {
		t.Fatalf("expected to find pdf-tool")
	}
	if _, ok := reg.Lookup("unknown"); ok {
		t.Fatalf("expected unknown skill to not be found")
	}

	// Test Register
	if err := reg.Register(s2); err != nil {
		t.Fatalf("failed to register csv-tool: %v", err)
	}

	// Test duplicate error
	if err := reg.Register(s2); err == nil {
		t.Fatalf("expected error on duplicate skill registration")
	}

	// Test List and Catalog
	list := reg.List()
	if len(list) != 2 {
		t.Fatalf("expected 2 skills, got %d", len(list))
	}
	// Sorted by name: csv-tool before pdf-tool
	if list[0].Name != "csv-tool" || list[1].Name != "pdf-tool" {
		t.Errorf("expected sorted list, got %s, %s", list[0].Name, list[1].Name)
	}

	catalog := reg.Catalog()
	if len(catalog) != 2 || catalog[0].Name != "csv-tool" {
		t.Errorf("unexpected catalog: %v", catalog)
	}

	// Test Activation tracking
	if reg.IsActivated("pdf-tool") {
		t.Errorf("skill should not be activated initially")
	}
	reg.MarkActivated("pdf-tool")
	if !reg.IsActivated("pdf-tool") {
		t.Errorf("skill should be marked activated")
	}
	activated := reg.ActivatedList()
	if len(activated) != 1 || activated[0] != "pdf-tool" {
		t.Errorf("expected [pdf-tool], got %v", activated)
	}
}
