package todo

import (
	"regexp"

	tododomain "github.com/phongsathornpt/protonman/internal/core/todo"
)

type (
	// Repository is the shared task-state boundary used by tools and adapters (core contract).
	Repository = tododomain.Repository
	// ReloadableRepository can refresh its in-memory projection from durable state.
	ReloadableRepository = tododomain.ReloadableRepository
	// GoalBoundRepository binds durable task state to the active session goal.
	GoalBoundRepository = tododomain.GoalBoundRepository
	// PatchRepository atomically validates and applies a patch against one revision.
	PatchRepository = tododomain.PatchRepository
)

// validIDPattern is the core-owned durable task-id vocabulary, re-declared
// locally so the patch validator can match ids without exporting the regexp.
var validIDPattern = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,128}$`)
