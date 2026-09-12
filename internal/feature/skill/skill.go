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

// Valid reports whether the scope is a recognized discovery scope.
func (s Scope) Valid() bool {
	return s == ScopeUser || s == ScopeProject
}

// ParseScope parses a skill scope string.
func ParseScope(value string) (Scope, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "user":
		return ScopeUser, nil
	case "project":
		return ScopeProject, nil
	default:
		return "", fmt.Errorf("%w: %q", ErrInvalidScope, value)
	}
}

var (
	// ErrInvalidScope indicates an unsupported skill discovery scope.
	ErrInvalidScope = errors.New("invalid skill scope")
	// ErrInvalidName indicates a skill name violates specification constraints.
	ErrInvalidName = errors.New("invalid skill name")
	// ErrInvalidDescription indicates a skill description violates specification constraints.
	ErrInvalidDescription = errors.New("invalid skill description")
	// ErrInvalidCompatibility indicates compatibility requirements exceed limits.
	ErrInvalidCompatibility = errors.New("invalid skill compatibility")
)

// Skill represents an agent skill conforming to the Agent Skills specification.
type Skill struct {
	Name          string         `json:"name"`
	Description   string         `json:"description"`
	Location      string         `json:"location"`
	BaseDir       string         `json:"base_dir"`
	Scope         Scope          `json:"scope"`
	License       string         `json:"license,omitempty"`
	Compatibility string         `json:"compatibility,omitempty"`
	Metadata      map[string]any `json:"metadata,omitempty"`
	AllowedTools  []string       `json:"allowed_tools,omitempty"`
	Instructions  string         `json:"instructions"`
	Resources     []string       `json:"resources,omitempty"`
	ComputedHash  string         `json:"computed_hash,omitempty"`
	Locked        bool           `json:"locked,omitempty"`
	LockStatus    string         `json:"lock_status,omitempty"`
}

// Validate checks that the skill satisfies specification constraints.
func (s Skill) Validate() error {
	if err := ValidateName(s.Name); err != nil {
		return err
	}
	if err := ValidateDescription(s.Description); err != nil {
		return err
	}
	if !s.Scope.Valid() {
		return fmt.Errorf("%w: %q", ErrInvalidScope, s.Scope)
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
	Name         string `json:"name"`
	Description  string `json:"description"`
	Location     string `json:"location"`
	Scope        Scope  `json:"scope"`
	ComputedHash string `json:"computed_hash,omitempty"`
	Locked       bool   `json:"locked,omitempty"`
	LockStatus   string `json:"lock_status,omitempty"`
}

// ToCatalogItem extracts a CatalogItem from the Skill.
func (s Skill) ToCatalogItem() CatalogItem {
	return CatalogItem{
		Name:         s.Name,
		Description:  s.Description,
		Location:     s.Location,
		Scope:        s.Scope,
		ComputedHash: s.ComputedHash,
		Locked:       s.Locked,
		LockStatus:   s.LockStatus,
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
		fmt.Fprintf(&b, "    <name>%s</name>\n", escapeXML(item.Name))
		fmt.Fprintf(&b, "    <description>%s</description>\n", escapeXML(item.Description))
		fmt.Fprintf(&b, "    <location>%s</location>\n", escapeXML(item.Location))
		b.WriteString("  </skill>\n")
	}
	b.WriteString("</available_skills>")
	return b.String()
}

// FormatActiveSkillsXML renders the instructions and resources of all currently activated skills.
func FormatActiveSkillsXML(active []Skill) string {
	if len(active) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("<active_skills>\n")
	for _, s := range active {
		fmt.Fprintf(&b, "  <skill name=\"%s\" scope=\"%s\">\n", escapeXML(s.Name), escapeXML(string(s.Scope)))
		fmt.Fprintf(&b, "    <location>%s</location>\n", escapeXML(s.Location))
		fmt.Fprintf(&b, "    <base_dir>%s</base_dir>\n", escapeXML(s.BaseDir))
		if len(s.Resources) > 0 {
			b.WriteString("    <resources>\n")
			for _, r := range s.Resources {
				fmt.Fprintf(&b, "      <file>%s</file>\n", escapeXML(r))
			}
			b.WriteString("    </resources>\n")
		}
		b.WriteString("    <instructions>\n")
		b.WriteString(s.Instructions)
		b.WriteString("\n    </instructions>\n")
		b.WriteString("  </skill>\n")
	}
	b.WriteString("</active_skills>")
	return b.String()
}

// SystemPromptSection renders behavioral instructions, available catalog, and active skills.
func SystemPromptSection(items []CatalogItem, activeSkills ...[]Skill) string {
	var active []Skill
	if len(activeSkills) > 0 {
		active = activeSkills[0]
	}

	if len(items) == 0 && len(active) == 0 {
		return ""
	}

	var parts []string
	if len(items) > 0 {
		catalog := FormatCatalogXML(items)
		parts = append(parts, fmt.Sprintf(`The following skills provide specialized instructions for specific tasks.
When a task matches a skill's description, call the skill tool with the skill's name to load its full instructions.
When a skill references relative paths, resolve them against the skill's directory and use absolute paths in tool calls.

%s`, catalog))
	}

	if len(active) > 0 {
		activeBlock := FormatActiveSkillsXML(active)
		parts = append(parts, fmt.Sprintf(`The following skills are currently ACTIVE in this session. Follow their instructions and apply their guidelines:

%s`, activeBlock))
	}

	return strings.Join(parts, "\n\n")
}

func escapeXML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	s = strings.ReplaceAll(s, "'", "&apos;")
	return s
}
