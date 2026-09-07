// Package skill manages application-level skill registration and activation.
package skill

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
)

// ErrDuplicateSkill indicates a skill with the same name is already registered.
var ErrDuplicateSkill = errors.New("duplicate skill")

// Registry manages discovered and activated skills.
type Registry struct {
	mu        sync.RWMutex
	skills    map[string]Skill
	order     []string
	activated map[string]bool
}

// NewRegistry creates a registry populated with the provided skills.
func NewRegistry(skills ...Skill) *Registry {
	r := &Registry{
		skills:    make(map[string]Skill),
		order:     make([]string, 0, len(skills)),
		activated: make(map[string]bool),
	}
	for _, s := range skills {
		_ = r.Register(s)
	}
	return r
}

// Fork creates an isolated activation session over the same immutable skill catalog.
// Skill definitions are copied so child activation state never leaks to the parent.
func (r *Registry) Fork() *Registry {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	fork := &Registry{
		skills:    make(map[string]Skill, len(r.skills)),
		order:     append([]string(nil), r.order...),
		activated: make(map[string]bool),
	}
	for name, item := range r.skills {
		fork.skills[name] = item
	}
	return fork
}

// Register adds a skill to the registry.
func (r *Registry) Register(s Skill) error {
	if err := s.Validate(); err != nil {
		return fmt.Errorf("register skill: %w", err)
	}
	name := strings.ToLower(strings.TrimSpace(s.Name))

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.skills[name]; exists {
		return fmt.Errorf("%w: %s", ErrDuplicateSkill, name)
	}
	r.skills[name] = s
	r.order = append(r.order, name)
	return nil
}

// Lookup finds a skill by name (case-insensitive).
func (r *Registry) Lookup(name string) (Skill, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	s, ok := r.skills[strings.ToLower(strings.TrimSpace(name))]
	return s, ok
}

// List returns all registered skills sorted by name.
func (r *Registry) List() []Skill {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]Skill, 0, len(r.skills))
	for _, s := range r.skills {
		result = append(result, s)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})
	return result
}

// Catalog returns catalog items for Tier 1 progressive disclosure.
func (r *Registry) Catalog() []CatalogItem {
	skills := r.List()
	items := make([]CatalogItem, 0, len(skills))
	for _, s := range skills {
		items = append(items, s.ToCatalogItem())
	}
	return items
}

// MarkActivated records that a skill was loaded in the current session.
func (r *Registry) MarkActivated(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.activated[strings.ToLower(strings.TrimSpace(name))] = true
}

// IsActivated checks if a skill has been loaded in the current session (case-insensitive).
func (r *Registry) IsActivated(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.activated[strings.ToLower(strings.TrimSpace(name))]
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

// ActiveSkills returns all currently activated skills sorted by name.
func (r *Registry) ActiveSkills() []Skill {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]Skill, 0, len(r.activated))
	for name, active := range r.activated {
		if active {
			if s, ok := r.skills[name]; ok {
				result = append(result, s)
			}
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})
	return result
}

// Deactivate unmarks a skill as active in the current session (case-insensitive).
func (r *Registry) Deactivate(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.activated, strings.ToLower(strings.TrimSpace(name)))
}

// ResetActivated clears all active skills in the registry.
func (r *Registry) ResetActivated() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.activated = make(map[string]bool)
}

// Toggle flips a skill's active status. Returns new active state or error if skill not found.
func (r *Registry) Toggle(name string) (bool, error) {
	cleanName := strings.ToLower(strings.TrimSpace(name))
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.skills[cleanName]; !exists {
		return false, fmt.Errorf("skill %q not found", cleanName)
	}

	active := !r.activated[cleanName]
	if active {
		r.activated[cleanName] = true
	} else {
		delete(r.activated, cleanName)
	}
	return active, nil
}
