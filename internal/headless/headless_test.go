package headless

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/projectTHORN/proton/internal/model"
	"github.com/projectTHORN/proton/internal/permission"
	"github.com/projectTHORN/proton/internal/session"
	"github.com/projectTHORN/proton/internal/skill"
	"github.com/projectTHORN/proton/internal/tool"
	"github.com/projectTHORN/proton/internal/toolcall"
	applicationturn "github.com/projectTHORN/proton/internal/turn"
)

func TestHeadlessCallRunsThroughService(t *testing.T) {
	registry, handler := newTestRegistry()
	service := newTestService(t, registry, permission.ModeAlwaysApprove)
	runner, err := New(service, registry, nil)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	var out bytes.Buffer
	if err := runner.Run(context.Background(), `:call read_file {"path":"README.md"}`, &out, FormatText); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if handler.calls != 1 {
		t.Fatalf("handler calls = %d, want 1", handler.calls)
	}
	if !strings.Contains(out.String(), "file contents") {
		t.Fatalf("output = %q, want file contents", out.String())
	}
}

func TestHeadlessAskModeDeniesWithoutPrompt(t *testing.T) {
	registry, handler := newTestRegistry()
	service := newTestService(t, registry, permission.ModeAsk)
	runner, err := New(service, registry, nil)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	var out bytes.Buffer
	err = runner.Run(context.Background(), `/call read_file {"path":"README.md"}`, &out, FormatText)
	if err == nil {
		t.Fatal("Run() error = nil, want permission denied")
	}
	if handler.calls != 0 {
		t.Fatalf("handler calls = %d, want 0", handler.calls)
	}
	if !strings.Contains(out.String(), "error:") {
		t.Fatalf("denied output missing error: %q", out.String())
	}
}

func TestHeadlessJSONEmitsEvents(t *testing.T) {
	registry, _ := newTestRegistry()
	service := newTestService(t, registry, permission.ModeAlwaysApprove)
	runner, err := New(service, registry, nil)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	var out bytes.Buffer
	if err := runner.Run(context.Background(), `/call read_file {}`, &out, FormatJSON); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !strings.Contains(out.String(), `"kind":"tool_call"`) {
		t.Fatalf("json missing tool_call: %s", out.String())
	}
	if !strings.Contains(out.String(), `"kind":"tool_result"`) {
		t.Fatalf("json missing tool_result: %s", out.String())
	}
}

func TestHeadlessPersistsTranscriptWithoutToolArguments(t *testing.T) {
	registry, _ := newTestRegistry()
	service := newTestService(t, registry, permission.ModeAlwaysApprove)
	runner, err := New(service, registry, nil)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	var out bytes.Buffer
	if err := runner.Run(context.Background(), `/call read_file {"path":"secret"}`, &out, FormatText); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	stored := runner.SessionState()
	if len(stored) != 1 {
		t.Fatalf("session messages = %d, want 1", len(stored))
	}
	if strings.Contains(stored[0].Content, "secret") {
		t.Fatalf("persisted tool arguments: %+v", stored[0])
	}
	if stored[0].ToolName != "read_file" {
		t.Fatalf("tool name = %q", stored[0].ToolName)
	}

	next, err := New(service, registry, nil)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if err := next.LoadSession(session.State{Messages: stored}); err != nil {
		t.Fatalf("LoadSession() error = %v", err)
	}
	if len(next.Messages()) != 1 {
		t.Fatalf("restored messages = %d, want 1", len(next.Messages()))
	}
}

func TestHeadlessTurnRequiresRunner(t *testing.T) {
	registry, _ := newTestRegistry()
	service := newTestService(t, registry, permission.ModeAsk)
	runner, err := New(service, registry, nil)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	err = runner.Run(context.Background(), "hello", ioDiscard(), FormatText)
	if err == nil || !strings.Contains(err.Error(), "model client is not configured") {
		t.Fatalf("Run() error = %v, want model client message", err)
	}
}

func TestHeadlessTurnStreamsEvents(t *testing.T) {
	registry, _ := newTestRegistry()
	service := newTestService(t, registry, permission.ModeAsk)
	loop := &scriptedTurn{
		events: []applicationturn.Event{
			{Kind: applicationturn.EventTextDelta, Text: "hello"},
		},
		result: applicationturn.Result{
			Message: model.Message{Role: model.RoleAssistant, Content: "hello"},
		},
	}
	runner, err := New(service, registry, loop)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	var out bytes.Buffer
	if err := runner.Run(context.Background(), "hi", &out, FormatText); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !strings.Contains(out.String(), "hello") {
		t.Fatalf("turn output = %q", out.String())
	}
	if len(runner.Messages()) != 2 {
		t.Fatalf("messages = %d, want 2", len(runner.Messages()))
	}
}

func ioDiscard() *bytes.Buffer {
	return &bytes.Buffer{}
}

type scriptedTurn struct {
	events []applicationturn.Event
	result applicationturn.Result
}

func (s *scriptedTurn) Run(
	ctx context.Context,
	_ []model.Message,
	sink applicationturn.Sink,
) (applicationturn.Result, error) {
	for _, event := range s.events {
		if err := sink(ctx, event); err != nil {
			return applicationturn.Result{}, err
		}
	}
	return s.result, nil
}

type testHandler struct {
	definition tool.Definition
	calls      int
}

func (h *testHandler) Definition() tool.Definition { return h.definition }

func (h *testHandler) Execute(_ context.Context, call tool.Call) (tool.Result, error) {
	h.calls++
	return tool.Result{CallID: call.ID, ToolName: call.Name, Output: "file contents"}, nil
}

type testRegistry struct {
	handler *testHandler
}

func (r *testRegistry) Lookup(name string) (tool.Handler, bool) {
	if name != r.handler.definition.Name {
		return nil, false
	}
	return r.handler, true
}

func (r *testRegistry) Definitions() []tool.Definition {
	return []tool.Definition{r.handler.definition}
}

func newTestRegistry() (*testRegistry, *testHandler) {
	handler := &testHandler{
		definition: tool.Definition{
			Name:                "read_file",
			Description:         "read a file",
			Kind:                tool.KindRead,
			PermissionDetailKey: "path",
		},
	}
	return &testRegistry{handler: handler}, handler
}

func newTestService(t *testing.T, registry tool.Registry, mode permission.Mode) *toolcall.Service {
	t.Helper()
	policy, err := permission.NewPolicy(permission.Config{})
	if err != nil {
		t.Fatalf("NewPolicy() error = %v", err)
	}
	service, err := toolcall.NewService(registry, policy, toolcall.WithMode(mode))
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	return service
}

func TestFormatEnumAndTextMarshaling(t *testing.T) {
	formats := []Format{FormatText, FormatJSON}
	for _, f := range formats {
		if !f.Valid() {
			t.Fatalf("expected format %s to be valid", f)
		}
		text, err := f.MarshalText()
		if err != nil {
			t.Fatalf("MarshalText() error = %v", err)
		}
		var decoded Format
		if err := decoded.UnmarshalText(text); err != nil {
			t.Fatalf("UnmarshalText() error = %v", err)
		}
		if decoded != f {
			t.Fatalf("round-trip failed: got %s, want %s", decoded, f)
		}
	}
	if FormatUnknown.Valid() {
		t.Fatal("FormatUnknown should not be valid")
	}
	if Format(99).Valid() {
		t.Fatal("Format(99) should not be valid")
	}
	var invalid Format
	if err := invalid.UnmarshalText([]byte("invalid")); err == nil {
		t.Fatal("UnmarshalText(invalid) error = nil, want error")
	}
}

func TestHeadlessSkillsCommands(t *testing.T) {
	registry, _ := newTestRegistry()
	service := newTestService(t, registry, permission.ModeAlwaysApprove)

	t.Run("no skills configured", func(t *testing.T) {
		runner, err := New(service, registry, nil)
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}
		var out bytes.Buffer
		if err := runner.Run(context.Background(), "/skills", &out, FormatText); err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		if !strings.Contains(out.String(), "No agent skills discovered.") {
			t.Fatalf("output = %q, want no skills discovered", out.String())
		}
	})

	t.Run("list and activate skills", func(t *testing.T) {
		skills := skill.NewRegistry(
			skill.Skill{
				Name:        "pdf-processing",
				Description: "Extract PDF text",
				Scope:       skill.ScopeUser,
				Resources:   []string{"scripts/extract.py"},
			},
			skill.Skill{
				Name:        "git-helper",
				Description: "Git helper tools",
				Scope:       skill.ScopeProject,
			},
		)
		runner, err := New(service, registry, nil, WithSkills(skills))
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		// Initial list: 0/2 active
		var out bytes.Buffer
		if err := runner.Run(context.Background(), "/skills", &out, FormatText); err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		if !strings.Contains(out.String(), "Agent Skills (0/2 active):") ||
			!strings.Contains(out.String(), "[ ] pdf-processing [user]: Extract PDF text") ||
			!strings.Contains(out.String(), "[ ] git-helper [project]: Git helper tools") {
			t.Fatalf("output = %q, want skills checklist", out.String())
		}

		// List active: none
		out.Reset()
		if err := runner.Run(context.Background(), "/skills active", &out, FormatText); err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		if !strings.Contains(out.String(), "No active agent skills in this session.") {
			t.Fatalf("output = %q, want no active skills", out.String())
		}

		// Activate pdf-processing (case-insensitive)
		out.Reset()
		if err := runner.Run(context.Background(), "/skill PDF-Processing", &out, FormatText); err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		if !strings.Contains(out.String(), "[x] Activated skill pdf-processing [user]: Extract PDF text") ||
			!strings.Contains(out.String(), "scripts/extract.py") {
			t.Fatalf("output = %q, want activated skill notification card", out.String())
		}
		if !skills.IsActivated("pdf-processing") {
			t.Fatal("expected pdf-processing to be activated in registry")
		}

		// Active list now shows 1
		out.Reset()
		if err := runner.Run(context.Background(), "/skills active", &out, FormatText); err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		if !strings.Contains(out.String(), "Active Agent Skills (1):") ||
			!strings.Contains(out.String(), "[x] pdf-processing") {
			t.Fatalf("output = %q, want active list with 1 skill", out.String())
		}

		// Toggle off
		out.Reset()
		if err := runner.Run(context.Background(), "/skill toggle pdf-processing", &out, FormatText); err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		if !strings.Contains(out.String(), "[ ] Skill \"pdf-processing\" deactivated.") {
			t.Fatalf("output = %q, want deactivated message", out.String())
		}
		if skills.IsActivated("pdf-processing") {
			t.Fatal("expected pdf-processing to be deactivated")
		}

		// Deactivate already inactive skill
		out.Reset()
		if err := runner.Run(context.Background(), "/skill deactivate pdf-processing", &out, FormatText); err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		if !strings.Contains(out.String(), "[ ] Skill \"pdf-processing\" is not active.") {
			t.Fatalf("output = %q, want not active message", out.String())
		}
	})

	t.Run("help lists skill commands", func(t *testing.T) {
		runner, err := New(service, registry, nil)
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}
		var out bytes.Buffer
		if err := runner.Run(context.Background(), "/help", &out, FormatText); err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		if !strings.Contains(out.String(), "/skills [active]") ||
			!strings.Contains(out.String(), "/skill <name>") {
			t.Fatalf("help output missing skill commands: %q", out.String())
		}
	})
}

