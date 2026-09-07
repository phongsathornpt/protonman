package session

import "context"

// Repository owns persisted session state independently of the storage medium.
// Inbound and application layers depend on this contract rather than FileStore.
type Repository interface {
	Load(context.Context, string) (State, bool, error)
	LatestSession(context.Context, string) (string, State, bool, error)
	Save(context.Context, string, State) error
	Delete(context.Context, string) error
	List(context.Context, string) ([]string, error)
	ListSummaries(context.Context, ListOptions) ([]Summary, error)
}
