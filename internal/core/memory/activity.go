package memory

import "context"

// ActivityKind identifies a user-visible durable-memory operation.
type ActivityKind string

const (
	ActivityContextIncluded    ActivityKind = "context_included"
	ActivityContextUnavailable ActivityKind = "context_unavailable"
	ActivityEntriesSaved       ActivityKind = "entries_saved"
	ActivityUpdateFailed       ActivityKind = "update_failed"
)

// Activity contains counts only; memory keys and values stay out of status and
// telemetry surfaces.
type Activity struct {
	Kind             ActivityKind
	WorkspaceEntries int
	GlobalEntries    int
}

// ActivityObserver receives successful or failed background memory updates.
type ActivityObserver func(Activity)

// ActivitySink transports turn-scoped memory activity alongside model and tool
// progress. Implementations should not retain the context.
type ActivitySink func(context.Context, Activity) error

type activitySinkKey struct{}

func WithActivitySink(ctx context.Context, sink ActivitySink) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if sink == nil {
		return ctx
	}
	return context.WithValue(ctx, activitySinkKey{}, sink)
}

func ReportActivity(ctx context.Context, activity Activity) error {
	if ctx == nil {
		return nil
	}
	sink, _ := ctx.Value(activitySinkKey{}).(ActivitySink)
	if sink == nil {
		return nil
	}
	return sink(ctx, activity)
}
