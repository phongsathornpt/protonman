// Package skill manages application-level skill registration and activation.
package skill

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/projectTHORN/proton/internal/domain/skill"
)

// ErrDuplicateSkill indicates a skill with the same name is already registered.
var ErrDuplicateSkill = errors.New("duplicate skill")

// Registry manages discovered and activated skills.
type Registry struct {
	mu        sync.RWMutex
	skills    map[string]skill.Skill
	order     []string
	activated map[string]bool
}

// NewRegistry creates a registry populated with the provided skills.
func NewRegistry(skills ...skill.Skill) *Registry {
	r := &Registry{
		skills:    make(map[string]skill.Skill),
		order:     make([]string, 0, len(skills)),
		activated: make(map[string]bool),
	}
	for _, s := range skills {
		_ = r.Register(s)
	}
	return r
}

// Register adds a skill to the registry.
func (r *Registry) Register(s skill.Skill) error {
	if err := s.Validate(); err != nil {
		return fmt.Errorf("register skill: %w", err)
	}
	name := strings.TrimSpace(s.Name)

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.skills[name]; exists {
		return fmt.Errorf("%w: %s", ErrDuplicateSkill, name)
	}
	r.skills[name] = s
	r.order = append(r.order, name)
	return nil
}

// Lookup finds a skill by name.
func (r *Registry) Lookup(name string) (skill.Skill, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	s, ok := r.skills[strings.TrimSpace(name)]
	return s, ok
}

// List returns all registered skills sorted by name.
func (r *Registry) List() []skill.Skill {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]skill.Skill, 0, len(r.skills))
	for _, s := range r.skills {
		result = append(result, s)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})
	return result
}

// Catalog returns catalog items for Tier 1 progressive disclosure.
func (r *Registry) Catalog() []skill.CatalogItem {
	skills := r.List()
	items := make([]skill.CatalogItem, 0, len(skills))
	for _, s := range skills {
		items = append(items, s.ToCatalogItem())
	}
	return items
}

// MarkActivated records that a skill was loaded in the current session.
func (r *Registry) MarkActivated(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.activated[strings.TrimSpace(name)] = true
}

// IsActivated checks if a skill has been loaded in the current session.
func (r *Registry) IsActivated(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.activated[strings.TrimSpace(name)]
}

// ActivatedList returns the names of all skills activated in the current session.
func (r *Registry) ActivatedList() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]string, 0, len(r.activated))
	for name, active := range r.activated {
		if active {
			result = append(result, name)
		}
	}
	sort.Strings(result)
	return result
}
