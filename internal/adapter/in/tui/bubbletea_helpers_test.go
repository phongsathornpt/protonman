// Code grouped by TUI behavior boundary; shared fixtures live in bubbletea_helpers_test.go.
package tui

import (
	"context"

	domainmodel "github.com/phongsathornpt/protonman/internal/adapter/out/model"
	"github.com/phongsathornpt/protonman/internal/core/permission"
	"github.com/phongsathornpt/protonman/internal/core/tool"
	"github.com/phongsathornpt/protonman/internal/engine/toolcall"
	applicationturn "github.com/phongsathornpt/protonman/internal/engine/turn"
	"testing"
)

type scriptedRunner struct {
	events []applicationturn.Event
	result applicationturn.Result
	err    error
}

func (r *scriptedRunner) Run(
	ctx context.Context,
	_ []domainmodel.Message,
	sink applicationturn.Sink,
) (applicationturn.Result, error) {
	for _, event := range r.events {
		if err := sink(ctx, event); err != nil {
			return applicationturn.Result{}, err
		}
	}
	return r.result, r.err
}

func newTestBubbleModel(
	t *testing.T,
	mode permission.Mode,
	todo []TodoItem,
) *bubbleModel {
	t.Helper()
	registry, _ := newBubbleTestRegistry()
	service := newBubbleTestService(t, registry, mode, permission.Config{})
	return newBubbleModel(
		context.Background(),
		service,
		registry,
		todo,
		nil,
		newPermissionBridge(),
		"/tmp/proton",
	)
}

func emptyTodoItems() []TodoItem {
	return []TodoItem{}
}

type bubbleTestHandler struct {
	definition tool.Definition
	calls      int
}

func (h *bubbleTestHandler) Definition() tool.Definition {
	return h.definition
}

func (h *bubbleTestHandler) Execute(_ context.Context, call tool.Call) (tool.Result, error) {
	h.calls++
	return tool.Result{
		CallID:   call.ID,
		ToolName: call.Name,
		Output:   "file contents",
	}, nil
}

type bubbleTestRegistry struct {
	handler *bubbleTestHandler
}

func (r *bubbleTestRegistry) Lookup(name string) (tool.Handler, bool) {
	if name != r.handler.definition.Name {
		return nil, false
	}
	return r.handler, true
}

func (r *bubbleTestRegistry) Definitions() []tool.Definition {
	return []tool.Definition{r.handler.definition}
}

func newBubbleTestRegistry() (*bubbleTestRegistry, *bubbleTestHandler) {
	registry := newNamedTestRegistry(tool.Definition{
		Name:                "read_file",
		Description:         "read a file",
		Kind:                tool.KindRead,
		PermissionDetailKey: "path",
	})
	return registry, registry.handler
}

func newNamedTestRegistry(definition tool.Definition) *bubbleTestRegistry {
	handler := &bubbleTestHandler{definition: definition}
	return &bubbleTestRegistry{handler: handler}
}

func newBubbleTestService(
	t *testing.T,
	registry tool.Registry,
	mode permission.Mode,
	config permission.Config,
) *toolcall.Service {
	t.Helper()
	policy, err := permission.NewPolicy(config)
	if err != nil {
		t.Fatalf("NewPolicy() error = %v", err)
	}
	service, err := toolcall.NewService(
		registry,
		policy,
		toolcall.WithMode(mode),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	return service
}
