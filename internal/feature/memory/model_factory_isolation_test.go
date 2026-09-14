package memory

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/phongsathornpt/protonman/internal/base/runtimepolicy"
	corememory "github.com/phongsathornpt/protonman/internal/core/memory"
	"github.com/phongsathornpt/protonman/internal/core/modelclient"
	"github.com/phongsathornpt/protonman/internal/core/session"
	sdk "github.com/phongsathornpt/protonman/proton-sdk"
)

// TestSessionBoundFactoryExposesUndecoratedBaseForSubagents locks the invariant
// that root-session memory decoration is separable from the provider-neutral
// base factory. Subagent admission builds from BaseFactory so child turns never
// receive root durable memory.
func TestSessionBoundFactoryExposesUndecoratedBaseForSubagents(t *testing.T) {
	repo := &fakeRepository{workspace: []corememory.Entry{{
		ID: "mem-1", Scope: corememory.ScopeWorkspace, Kind: corememory.KindProcedure,
		Key: "verification tests", Value: "Run go test ./...", WorkspaceKey: "ws", Confidence: 1, UpdatedAt: time.Now().UTC(),
	}}}
	base := &captureModel{}
	factory := NewSessionBoundModelFactory(captureFactory{model: base}, repo, nil, runtimepolicy.DurableMemory())
	carrier, ok := factory.(interface {
		BaseFactory() modelclient.Factory
	})
	if !ok {
		t.Fatal("session-bound factory must expose BaseFactory for subagent isolation")
	}
	binder, ok := factory.(interface {
		BindSession(sessionID, workspaceKey string)
	})
	if !ok {
		t.Fatal("session-bound factory must expose BindSession")
	}
	binder.BindSession("session-root", "ws")

	// The root model decorates the current user turn.
	rootModel := factory.Build(modelclient.Request{ModelID: "m"})
	stream, err := rootModel.Stream(context.Background(), sdk.Request{Messages: []sdk.Message{
		{ID: "u", Role: sdk.RoleUser, Content: "please run test verification"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	_ = stream.Close()
	if len(base.requests) != 1 || !strings.Contains(base.requests[0].Messages[0].Content, "<proton-memory-context>") {
		t.Fatalf("root model did not decorate: %+v", base.requests)
	}

	// A model built from the exposed base factory must be undecorated: the same
	// user turn must reach the provider without any memory context.
	childFactory := carrier.BaseFactory()
	if childFactory == nil {
		t.Fatal("BaseFactory() returned nil")
	}
	childCapture := &captureModel{}
	childModel := captureFactory{model: childCapture}.Build(modelclient.Request{ModelID: "m"})
	if _, decorated := childModel.(*memoryLanguageModel); decorated {
		t.Fatal("BaseFactory must not return a memory-decorated model")
	}
	childStream, err := childModel.Stream(context.Background(), sdk.Request{Messages: []sdk.Message{
		{ID: "u", Role: sdk.RoleUser, Content: "please run test verification"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	_ = childStream.Close()
	if len(childCapture.requests) != 1 || strings.Contains(childCapture.requests[0].Messages[0].Content, "<proton-memory-context>") {
		t.Fatalf("child turn received memory context: %+v", childCapture.requests)
	}
}

// TestSessionBoundFactoryWithoutBindingDoesNotRetrieve verifies that an unbound
// root factory never injects memory. This is the guard against a root session
// reading a workspace it was never bound to.
func TestSessionBoundFactoryWithoutBindingDoesNotRetrieve(t *testing.T) {
	repo := &fakeRepository{workspace: []corememory.Entry{{
		ID: "mem-1", Scope: corememory.ScopeWorkspace, Kind: corememory.KindProcedure,
		Key: "verification tests", Value: "Run go test ./...", WorkspaceKey: "ws", Confidence: 1, UpdatedAt: time.Now().UTC(),
	}}}
	base := &captureModel{}
	model := NewSessionBoundModelFactory(captureFactory{model: base}, repo, nil, runtimepolicy.DurableMemory()).Build(modelclient.Request{ModelID: "m"})
	stream, err := model.Stream(context.Background(), sdk.Request{Messages: []sdk.Message{
		{ID: "u", Role: sdk.RoleUser, Content: "please run test verification"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	_ = stream.Close()
	if len(base.requests) != 1 {
		t.Fatalf("requests = %d, want 1", len(base.requests))
	}
	if strings.Contains(base.requests[0].Messages[0].Content, "<proton-memory-context>") {
		t.Fatalf("unbound factory injected memory: %q", base.requests[0].Messages[0].Content)
	}
	if len(repo.used) != 0 {
		t.Fatalf("unbound factory recorded usage: %+v", repo.used)
	}
}

// workspaceAwareRepository returns workspace entries only for the requested
// workspace key, so a rebind to an unrelated workspace observes no memory.
type workspaceAwareRepository struct {
	fakeRepository
	byWorkspace map[string][]corememory.Entry
}

func (r *workspaceAwareRepository) Load(_ context.Context, scope corememory.Scope, workspaceKey string) ([]corememory.Entry, error) {
	if scope == corememory.ScopeWorkspace {
		return append([]corememory.Entry(nil), r.byWorkspace[workspaceKey]...), nil
	}
	return append([]corememory.Entry(nil), r.global...), nil
}

// TestSessionBoundFactoryRebindInvalidatesRetrievalCache verifies that switching
// the active session cannot reuse a previously retrieved context, which would
// otherwise leak one session's memory into another.
func TestSessionBoundFactoryRebindInvalidatesRetrievalCache(t *testing.T) {
	now := time.Now().UTC()
	repo := &workspaceAwareRepository{
		fakeRepository: fakeRepository{processed: map[string]uint64{}},
		byWorkspace: map[string][]corememory.Entry{
			"ws": {{
				ID: "mem-1", Scope: corememory.ScopeWorkspace, Kind: corememory.KindProcedure,
				Key: "verification tests", Value: "Run go test ./...", WorkspaceKey: "ws", Confidence: 1, UpdatedAt: now,
			}},
		},
	}
	base := &captureModel{}
	factory := NewSessionBoundModelFactory(captureFactory{model: base}, repo, nil, runtimepolicy.DurableMemory())
	binder := factory.(interface {
		BindSession(sessionID, workspaceKey string)
	})
	binder.BindSession("session-a", "ws")
	model := factory.Build(modelclient.Request{ModelID: "m"})
	request := sdk.Request{Messages: []sdk.Message{{ID: "u", Role: sdk.RoleUser, Content: "please run test verification"}}}
	for range 2 {
		stream, err := model.Stream(context.Background(), request)
		if err != nil {
			t.Fatal(err)
		}
		_ = stream.Close()
	}
	if len(repo.used) != 1 {
		t.Fatalf("usage accounting = %d, want one retrieval for repeated round", len(repo.used))
	}
	// Rebind to an unrelated workspace; the cached context must not survive.
	binder.BindSession("session-b", "other-workspace")
	stream, err := model.Stream(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	_ = stream.Close()
	if got := base.requests[len(base.requests)-1].Messages[0].Content; strings.Contains(got, "<proton-memory-context>") {
		t.Fatalf("rebind reused stale retrieval context: %q", got)
	}
}

// TestSessionBindingExtractsOncePerActiveSession verifies extraction is claimed
// per active session rather than once per process, so a newly active session is
// still extracted after a switch.
func TestSessionBindingExtractsOncePerActiveSession(t *testing.T) {
	var binding sessionBinding
	binding.bind("session-a", "ws")
	if !binding.claimExtraction("session-a") {
		t.Fatal("first claim for session-a must succeed")
	}
	if binding.claimExtraction("session-a") {
		t.Fatal("second claim for session-a must be suppressed")
	}
	if !binding.claimExtraction("session-b") {
		t.Fatal("claim for a newly active session must succeed")
	}
}

// TestExtractorSkipsBoundActiveSession verifies the extractor is constructed with
// the bound active session, so that session is the one skipped as historical.
func TestExtractorSkipsBoundActiveSession(t *testing.T) {
	now := time.Now().UTC()
	repo := &fakeRepository{processed: map[string]uint64{}}
	sessions := &extractionSessionRepo{
		states: map[string]session.State{
			"session-active":   {SessionID: "session-active", Revision: 1, WorkspaceKey: "ws", UpdatedAt: now.Add(-time.Hour)},
			"session-historic": {SessionID: "session-historic", Revision: 1, WorkspaceKey: "ws", UpdatedAt: now.Add(-time.Hour)},
		},
		summaries: []session.Summary{
			{ID: "session-active", WorkspaceKey: "ws", UpdatedAt: now.Add(-time.Hour)},
			{ID: "session-historic", WorkspaceKey: "ws", UpdatedAt: now.Add(-time.Hour)},
		},
	}
	model := &extractionModel{output: `{"memories":[]}`}
	extractor := NewExtractor(sessions, repo, model, "session-active", "ws", runtimepolicy.DurableMemory())
	extractor.now = func() time.Time { return now }
	if err := extractor.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, ok := repo.processed["session-historic"]; !ok {
		t.Fatalf("historic session was not processed: %+v", repo.processed)
	}
	if _, ok := repo.processed["session-active"]; ok {
		t.Fatalf("active session must not be extracted: %+v", repo.processed)
	}
}
