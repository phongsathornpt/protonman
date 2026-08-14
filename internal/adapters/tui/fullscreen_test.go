package tui

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/projectTHORN/proton/internal/application/toolcall"
	"github.com/projectTHORN/proton/internal/domain/permission"
	"github.com/projectTHORN/proton/internal/domain/tool"
)

func TestFullScreenRunsCommandsAndRendersTodoPane(t *testing.T) {
	registry, handler := newTUITestRegistry()
	service := newTUITestService(t, registry, permission.ModeAlwaysApprove)
	keys := keySequence(
		":plan on",
		":todo",
		`:call read_file {"path":"README.md"}`,
		":quit",
	)
	screen := &recordingScreen{width: 80, height: 12}
	ui, err := NewFullScreen(
		service,
		registry,
		keys,
		screen,
		WithTodoItems([]TodoItem{{Text: "ship TUI", Done: false}}),
	)
	if err != nil {
		t.Fatalf("NewFullScreen() error = %v", err)
	}
	if err := ui.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !screen.entered || !screen.exited {
		t.Fatalf("screen entered=%v exited=%v, want both true", screen.entered, screen.exited)
	}
	if got, want := handler.calls, 1; got != want {
		t.Fatalf("handler calls = %d, want %d", got, want)
	}
	if !containsFrameText(screen.frames, "plan mode: on") {
		t.Fatalf("frames do not contain plan mode output")
	}
	if !containsFrameText(screen.frames, "[ ] ship TUI") {
		t.Fatalf("frames do not contain TODO output")
	}
	if !containsFrameText(screen.frames, "output: file contents") {
		t.Fatalf("frames do not contain tool output")
	}
	last := screen.frames[len(screen.frames)-1]
	if last.PlanMode != true {
		t.Fatalf("last frame PlanMode = false, want true")
	}
}

func TestFullScreenPermissionPromptIsModalAndDelegatesToService(t *testing.T) {
	registry, handler := newTUITestRegistry()
	service := newTUITestService(t, registry, permission.ModeAsk)
	keys := keySequence(`:call read_file {"path":"README.md"}`)
	keys.keys = append(keys.keys, Key{Kind: KeyRune, Rune: 'y'})
	keys.keys = append(keys.keys, keySequence(":quit").keys...)
	screen := &recordingScreen{width: 80, height: 12}
	ui, err := NewFullScreen(service, registry, keys, screen)
	if err != nil {
		t.Fatalf("NewFullScreen() error = %v", err)
	}
	service.SetPrompt(ui.PermissionPrompt)
	if err := ui.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got, want := handler.calls, 1; got != want {
		t.Fatalf("handler calls = %d, want %d", got, want)
	}
	if !containsFrameText(screen.frames, "Permission required") {
		t.Fatal("frames do not contain permission modal")
	}
}

func TestRuneKeySourceParsesEditingKeys(t *testing.T) {
	source, err := NewRuneKeySource(strings.NewReader("a\x1b[D\b\r"))
	if err != nil {
		t.Fatalf("NewRuneKeySource() error = %v", err)
	}
	want := []Key{
		{Kind: KeyRune, Rune: 'a'},
		{Kind: KeyLeft},
		{Kind: KeyBackspace},
		{Kind: KeyEnter},
	}
	for index, expected := range want {
		got, err := source.ReadKey(context.Background())
		if err != nil {
			t.Fatalf("ReadKey(%d) error = %v", index, err)
		}
		if got != expected {
			t.Fatalf("ReadKey(%d) = %#v, want %#v", index, got, expected)
		}
	}
}

func TestFullScreenEditsInputAndNavigatesHistory(t *testing.T) {
	registry, _ := newTUITestRegistry()
	service := newTUITestService(t, registry, permission.ModeAlwaysApprove)
	keys := &scriptedKeys{}
	screen := &recordingScreen{width: 80, height: 12}
	ui, err := NewFullScreen(service, registry, keys, screen)
	if err != nil {
		t.Fatalf("NewFullScreen() error = %v", err)
	}
	for _, key := range []Key{
		{Kind: KeyRune, Rune: 'a'},
		{Kind: KeyRune, Rune: 'b'},
		{Kind: KeyRune, Rune: 'c'},
		{Kind: KeyLeft},
		{Kind: KeyBackspace},
		{Kind: KeyRune, Rune: 'x'},
	} {
		if _, err := ui.handleKey(context.Background(), key); err != nil {
			t.Fatalf("handleKey(%#v) error = %v", key, err)
		}
	}
	if _, err := ui.handleKey(context.Background(), Key{Kind: KeyEnter}); err == nil {
		t.Fatal("submit error = nil, want missing model client")
	}
	if got, want := ui.scrollback[0], "> axc"; got != want {
		t.Fatalf("submitted line = %q, want %q", got, want)
	}
	if _, err := ui.handleKey(context.Background(), Key{Kind: KeyUp}); err != nil {
		t.Fatalf("history up error = %v", err)
	}
	if got, want := string(ui.input), "axc"; got != want {
		t.Fatalf("history input = %q, want %q", got, want)
	}
}

func TestParseTODO(t *testing.T) {
	items := ParseTODO("# TODO\n- [x] done item\n- [ ] pending item\n- [X] uppercase done\n")
	if got, want := len(items), 3; got != want {
		t.Fatalf("ParseTODO() items = %d, want %d", got, want)
	}
	if !items[0].Done || items[1].Done || !items[2].Done {
		t.Fatalf("ParseTODO() statuses = %#v, want done/pending/done", items)
	}
}

func TestANSIScreenRendersFrameAndRestoresTerminal(t *testing.T) {
	var output bytes.Buffer
	screen, err := NewANSIScreen(&output, 20, 8)
	if err != nil {
		t.Fatalf("NewANSIScreen() error = %v", err)
	}
	if err := screen.Enter(); err != nil {
		t.Fatalf("Enter() error = %v", err)
	}
	if err := screen.Render(Frame{
		Mode:       "ask",
		Scrollback: []string{"hello"},
		Input:      "type here",
		Todo:       []TodoItem{{Text: "one", Done: true}},
	}); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if err := screen.Exit(); err != nil {
		t.Fatalf("Exit() error = %v", err)
	}
	for _, fragment := range []string{"\x1b[?1049h", "hello", "TODO", "type here", "\x1b[?1049l"} {
		if !strings.Contains(output.String(), fragment) {
			t.Fatalf("screen output does not contain %q", fragment)
		}
	}
}

func TestRenderFrameUsesLiveRegionLayout(t *testing.T) {
	lines := renderFrame(Frame{
		Scrollback: []string{"assistant: thinking", "tool running: read_file"},
		Input:      "hello",
		Activity:   "thinking",
		Mode:       "ask",
		PlanMode:   true,
		Todo:       []TodoItem{{Text: "ship TUI", Done: false}},
	}, 48, 12)
	if got, want := len(lines), 12; got != want {
		t.Fatalf("renderFrame() lines = %d, want %d", got, want)
	}
	output := strings.Join(lines, "\n")
	for _, expected := range []string{
		"assistant: thinking",
		"TODO 0/1 complete",
		"· thinking · permission: ask · plan on",
		"❯ hello",
		"proton · plan · :help",
		"history · ←→ move",
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("rendered layout does not contain %q: %s", expected, output)
		}
	}
}

func TestRenderFrameSanitizesTerminalSequences(t *testing.T) {
	lines := renderFrame(Frame{
		Scrollback: []string{"\x1b[31mprivate output\x1b[0m"},
		Input:      "\x1b]0;private title\x07safe",
		Mode:       "ask",
	}, 48, 8)
	output := strings.Join(lines, "\n")
	for _, forbidden := range []string{"\x1b[31m", "\x1b]0;private title\x07"} {
		if strings.Contains(output, forbidden) {
			t.Fatalf("rendered layout contains terminal sequence %q: %q", forbidden, output)
		}
	}
	if !strings.Contains(output, "private output") || !strings.Contains(output, "safe") {
		t.Fatalf("sanitized content missing from output: %q", output)
	}
}

type scriptedKeys struct {
	keys  []Key
	index int
}

func (s *scriptedKeys) ReadKey(ctx context.Context) (Key, error) {
	if err := ctx.Err(); err != nil {
		return Key{}, err
	}
	if s.index >= len(s.keys) {
		return Key{}, io.EOF
	}
	key := s.keys[s.index]
	s.index++
	return key, nil
}

func keySequence(lines ...string) *scriptedKeys {
	keys := make([]Key, 0)
	for _, line := range lines {
		for _, value := range line {
			keys = append(keys, Key{Kind: KeyRune, Rune: value})
		}
		keys = append(keys, Key{Kind: KeyEnter})
	}
	return &scriptedKeys{keys: keys}
}

type recordingScreen struct {
	width   int
	height  int
	entered bool
	exited  bool
	frames  []Frame
}

func (s *recordingScreen) Enter() error {
	s.entered = true
	return nil
}

func (s *recordingScreen) Exit() error {
	s.exited = true
	return nil
}

func (s *recordingScreen) Size() (int, int) {
	return s.width, s.height
}

func (s *recordingScreen) Render(frame Frame) error {
	clone := frame
	clone.Scrollback = append([]string{}, frame.Scrollback...)
	clone.Todo = append([]TodoItem{}, frame.Todo...)
	clone.Modal = cloneModal(frame.Modal)
	s.frames = append(s.frames, clone)
	return nil
}

type tuiTestHandler struct {
	definition tool.Definition
	calls      int
}

func (h *tuiTestHandler) Definition() tool.Definition {
	return h.definition
}

func (h *tuiTestHandler) Execute(_ context.Context, call tool.Call) (tool.Result, error) {
	h.calls++
	return tool.Result{
		CallID:   call.ID,
		ToolName: call.Name,
		Output:   "file contents",
	}, nil
}

type tuiTestRegistry struct {
	handler *tuiTestHandler
}

func (r *tuiTestRegistry) Lookup(name string) (tool.Handler, bool) {
	if name != r.handler.definition.Name {
		return nil, false
	}
	return r.handler, true
}

func (r *tuiTestRegistry) Definitions() []tool.Definition {
	return []tool.Definition{r.handler.definition}
}

func newTUITestRegistry() (*tuiTestRegistry, *tuiTestHandler) {
	handler := &tuiTestHandler{
		definition: tool.Definition{
			Name:                "read_file",
			Description:         "read a file",
			Kind:                tool.KindRead,
			PermissionDetailKey: "path",
		},
	}
	return &tuiTestRegistry{handler: handler}, handler
}

func newTUITestService(
	t *testing.T,
	registry tool.Registry,
	mode permission.Mode,
) *toolcall.Service {
	t.Helper()
	policy, err := permission.NewPolicy(permission.Config{
		Rules: []permission.Rule{{
			Action: permission.ActionAllow,
			Tool:   permission.ToolRead,
		}},
	})
	if mode == permission.ModeAsk {
		policy, err = permission.NewPolicy(permission.Config{})
	}
	if err != nil {
		t.Fatalf("NewPolicy() error = %v", err)
	}
	service, err := toolcall.NewService(registry, policy, toolcall.WithMode(mode))
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	return service
}

func containsFrameText(frames []Frame, target string) bool {
	for _, frame := range frames {
		for _, line := range frame.Scrollback {
			if strings.Contains(line, target) {
				return true
			}
		}
		if frame.Modal != nil && strings.Contains(frame.Modal.Title, target) {
			return true
		}
	}
	return false
}
