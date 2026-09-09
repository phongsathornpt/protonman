package skill

import (
	"errors"
	"testing"
)

func TestRegistry(t *testing.T) {
	s1 := Skill{
		Name:        "pdf-tool",
		Description: "Process PDFs",
		Location:    "/path/to/pdf/SKILL.md",
		Scope:       ScopeUser,
	}
	s2 := Skill{
		Name:        "csv-tool",
		Description: "Process CSVs",
		Location:    "/path/to/csv/SKILL.md",
		Scope:       ScopeProject,
	}

	reg := NewRegistry(s1)

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
	reg.Activate("pdf-tool")
	if !reg.IsActivated("pdf-tool") {
		t.Errorf("skill should be marked activated")
	}
	activated := reg.ActivatedList()
	if len(activated) != 1 || activated[0] != "pdf-tool" {
		t.Errorf("expected [pdf-tool], got %v", activated)
	}

	// Test Deactivate
	reg.Deactivate("pdf-tool")
	if reg.IsActivated("pdf-tool") {
		t.Errorf("skill should be deactivated")
	}

	// Test Toggle
	active, err := reg.Toggle("csv-tool")
	if err != nil || !active {
		t.Fatalf("expected csv-tool to be toggled active, err = %v", err)
	}
	active, err = reg.Toggle("csv-tool")
	if err != nil || active {
		t.Fatalf("expected csv-tool to be toggled inactive, err = %v", err)
	}
	// Test Case-insensitive Lookup
	if _, ok := reg.Lookup("PDF-TOOL"); !ok {
		t.Errorf("expected case-insensitive lookup for 'PDF-TOOL'")
	}
	if _, ok := reg.Lookup("Csv-Tool"); !ok {
		t.Errorf("expected case-insensitive lookup for 'Csv-Tool'")
	}

	// Test ActiveSkills and ResetActivated
	reg.Activate("PDF-TOOL")
	if !reg.IsActivated("pdf-tool") {
		t.Errorf("expected skill to be active via case-insensitive check")
	}
	activeSkills := reg.ActiveSkills()
	if len(activeSkills) != 1 || activeSkills[0].Name != "pdf-tool" {
		t.Errorf("expected 1 active skill [pdf-tool], got %v", activeSkills)
	}

	reg.ResetActivated()
	if reg.IsActivated("pdf-tool") {
		t.Errorf("expected all skills to be inactive after ResetActivated()")
	}
	if len(reg.ActivatedList()) != 0 || len(reg.ActiveSkills()) != 0 {
		t.Errorf("expected empty active lists after ResetActivated()")
	}
}

func TestRegistryForkIsolatesActivationState(t *testing.T) {
	parent := NewRegistry(Skill{
		Name: "go-review", Description: "Review Go code", Scope: ScopeUser,
		Instructions: "Run focused Go checks.",
	})
	parent.Activate("go-review")

	child := parent.Fork()
	if child == nil {
		t.Fatal("Fork() = nil")
	}
	if child.IsActivated("go-review") {
		t.Fatal("child inherited parent activation state")
	}
	if _, ok := child.Lookup("go-review"); !ok {
		t.Fatal("child lost shared skill catalog")
	}

	child.Activate("go-review")
	child.Deactivate("go-review")
	if !parent.IsActivated("go-review") {
		t.Fatal("child activation changes leaked to parent")
	}
}

func TestRegistryActivationLimits(t *testing.T) {
	registry := NewRegistry(
		Skill{Name: "one", Description: "First skill", Scope: ScopeUser, Instructions: "12345"},
		Skill{Name: "two", Description: "Second skill", Scope: ScopeUser, Instructions: "67890"},
		Skill{Name: "three", Description: "Third skill", Scope: ScopeUser, Instructions: "abcdef"},
	)
	registry.SetActivationLimits(ActivationLimits{MaxSkills: 2, MaxInstructionBytes: 10})
	if err := registry.Activate("one"); err != nil {
		t.Fatalf("Activate(one) error = %v", err)
	}
	if err := registry.Activate("two"); err != nil {
		t.Fatalf("Activate(two) error = %v", err)
	}
	if err := registry.Activate("three"); !errors.Is(err, ErrActivationLimit) {
		t.Fatalf("Activate(three) error = %v, want ErrActivationLimit", err)
	}
	if registry.IsActivated("three") {
		t.Fatal("skill exceeding activation limits became active")
	}
}
