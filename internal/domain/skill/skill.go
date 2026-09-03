// Package skill defines the domain entities and contracts for Agent Skills.
package skill

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
)

// Scope defines where a skill was discovered.
type Scope string

const (
	// ScopeUser indicates a skill installed globally in the user's home directory.
	ScopeUser Scope = "user"
	// ScopeProject indicates a skill defined inside the current project workspace.
	ScopeProject Scope = "project"
)

var (
	// ErrInvalidName indicates a skill name violates specification constraints.
	ErrInvalidName = errors.New("invalid skill name")
	// ErrInvalidDescription indicates a skill description violates specification constraints.
	ErrInvalidDescription = errors.New("invalid skill description")
	// ErrInvalidCompatibility indicates compatibility requirements exceed limits.
	ErrInvalidCompatibility = errors.New("invalid skill compatibility")
)

// Skill represents an agent skill conforming to the Agent Skills specification.
type Skill struct {
	Name          string            `json:"name"`
	Description   string            `json:"description"`
	Location      string            `json:"location"`
	BaseDir       string            `json:"base_dir"`
	Scope         Scope             `json:"scope"`
	License       string            `json:"license,omitempty"`
	Compatibility string            `json:"compatibility,omitempty"`
	Metadata      map[string]string `json:"metadata,omitempty"`
	AllowedTools  []string          `json:"allowed_tools,omitempty"`
	Instructions  string            `json:"instructions"`
	Resources     []string          `json:"resources,omitempty"`
}

// Validate checks that the skill satisfies specification constraints.
func (s Skill) Validate() error {
	if err := ValidateName(s.Name); err != nil {
		return err
	}
	if err := ValidateDescription(s.Description); err != nil {
		return err
	}
	if len(s.Compatibility) > 500 {
		return fmt.Errorf("%w: compatibility must not exceed 500 characters", ErrInvalidCompatibility)
	}
	return nil
}

// ValidateName ensures the skill name adheres to agentskills.io spec:
// - 1-64 characters.
// - Lowercase alphanumeric characters ('a'-'z', '0'-'9') and hyphens ('-').
// - Must not start or end with a hyphen.
// - Must not contain consecutive hyphens ('--').
func ValidateName(name string) error {
	trimmed := strings.TrimSpace(name)
	if len(trimmed) == 0 {
		return fmt.Errorf("%w: name is required", ErrInvalidName)
	}
	if len(trimmed) > 64 {
		return fmt.Errorf("%w: name %q exceeds 64 characters", ErrInvalidName, name)
	}
	if strings.HasPrefix(trimmed, "-") || strings.HasSuffix(trimmed, "-") {
		return fmt.Errorf("%w: name %q must not start or end with a hyphen", ErrInvalidName, name)
	}
	if strings.Contains(trimmed, "--") {
		return fmt.Errorf("%w: name %q must not contain consecutive hyphens", ErrInvalidName, name)
	}
	for _, r := range trimmed {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			continue
		}
		if unicode.IsUpper(r) {
			return fmt.Errorf("%w: name %q must be lowercase", ErrInvalidName, name)
		}
		return fmt.Errorf("%w: name %q contains invalid character %q", ErrInvalidName, name, r)
	}
	return nil
}

// ValidateDescription ensures the description is non-empty and bounded to 1024 characters.
func ValidateDescription(desc string) error {
	trimmed := strings.TrimSpace(desc)
	if len(trimmed) == 0 {
		return fmt.Errorf("%w: description is required", ErrInvalidDescription)
	}
	if len(trimmed) > 1024 {
		return fmt.Errorf("%w: description exceeds 1024 characters", ErrInvalidDescription)
	}
	return nil
}

// CatalogItem represents a concise summary of an available skill for Tier 1 progressive disclosure.
type CatalogItem struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Location    string `json:"location"`
	Scope       Scope  `json:"scope"`
}

// ToCatalogItem extracts a CatalogItem from the Skill.
func (s Skill) ToCatalogItem() CatalogItem {
	return CatalogItem{
		Name:        s.Name,
		Description: s.Description,
		Location:    s.Location,
		Scope:       s.Scope,
	}
}

// FormatCatalogXML renders the list of available skills into the standard progressive disclosure format.
func FormatCatalogXML(items []CatalogItem) string {
	if len(items) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("<available_skills>\n")
	for _, item := range items {
		b.WriteString("  <skill>\n")
		b.WriteString(fmt.Sprintf("    <name>%s</name>\n", escapeXML(item.Name)))
		b.WriteString(fmt.Sprintf("    <description>%s</description>\n", escapeXML(item.Description)))
		b.WriteString(fmt.Sprintf("    <location>%s</location>\n", escapeXML(item.Location)))
		b.WriteString("  </skill>\n")
	}
	b.WriteString("</available_skills>")
	return b.String()
}

// SystemPromptSection renders both the behavioral instructions and the catalog block.
func SystemPromptSection(items []CatalogItem) string {
	if len(items) == 0 {
		return ""
	}
	catalog := FormatCatalogXML(items)
	return fmt.Sprintf(`The following skills provide specialized instructions for specific tasks.
When a task matches a skill's description, call the activate_skill tool with the skill's name to load its full instructions.
When a skill references relative paths, resolve them against the skill's directory and use absolute paths in tool calls.

%s`, catalog)
}

func escapeXML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	s = strings.ReplaceAll(s, "'", "&apos;")
	return s
}
