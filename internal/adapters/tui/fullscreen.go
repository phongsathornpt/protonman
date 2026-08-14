package tui

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/projectTHORN/proton/internal/application/toolcall"
	applicationturn "github.com/projectTHORN/proton/internal/application/turn"
	"github.com/projectTHORN/proton/internal/domain/model"
	"github.com/projectTHORN/proton/internal/domain/permission"
	"github.com/projectTHORN/proton/internal/domain/tool"
)

const maxFullScreenScrollback = 1000

// KeyKind identifies one input action from a terminal or test source.
type KeyKind uint8

const (
	// KeyRune inserts Rune at the cursor.
	KeyRune KeyKind = iota + 1
	// KeyEnter submits the current input.
	KeyEnter
	// KeyBackspace removes the rune before the cursor.
	KeyBackspace
	// KeyLeft moves the cursor left.
	KeyLeft
	// KeyRight moves the cursor right.
	KeyRight
	// KeyUp selects an older input history entry.
	KeyUp
	// KeyDown selects a newer input history entry.
	KeyDown
	// KeyEscape clears the current input or rejects a modal.
	KeyEscape
	// KeyCtrlC clears the current input.
	KeyCtrlC
)

// Key is one normalized terminal input action.
type Key struct {
	Kind KeyKind
	Rune rune
}

// KeySource supplies normalized key events to the full-screen loop.
type KeySource interface {
	ReadKey(ctx context.Context) (Key, error)
}

// Screen owns terminal mode and frame rendering for the full-screen adapter.
type Screen interface {
	Enter() error
	Exit() error
	Size() (int, int)
	Render(Frame) error
}

// TodoItem is one entry displayed in the TODO pane.
type TodoItem struct {
	Text string
	Done bool
}

// Modal is the transient permission or status dialog rendered above the prompt.
type Modal struct {
	Title   string
	Body    string
	Choices string
}

// Frame is the complete render snapshot passed to a Screen.
type Frame struct {
	Width      int
	Height     int
	Scrollback []string
	Input      string
	Cursor     int
	Mode       string
	PlanMode   bool
	Todo       []TodoItem
	Modal      *Modal
}

// TurnRunner is the optional model loop used for ordinary prompt input.
type TurnRunner interface {
	Run(context.Context, []model.Message, applicationturn.Sink) (applicationturn.Result, error)
}

// FullScreenOption configures the full-screen UI.
type FullScreenOption func(*FullScreenUI) error

// WithTodoItems sets the entries shown in the TODO pane.
func WithTodoItems(items []TodoItem) FullScreenOption {
	return func(ui *FullScreenUI) error {
		ui.todo = append([]TodoItem{}, items...)
		return nil
	}
}

// WithTurnRunner connects ordinary prompt input to the provider-neutral model loop.
func WithTurnRunner(runner TurnRunner) FullScreenOption {
	return func(ui *FullScreenUI) error {
		ui.runner = runner
		return nil
	}
}

// ParseTODO extracts checkbox items from a Markdown TODO file.
func ParseTODO(markdown string) []TodoItem {
	items := make([]TodoItem, 0)
	for _, line := range strings.Split(markdown, "\n") {
		trimmed := strings.TrimSpace(line)
		done := false
		switch {
		case strings.HasPrefix(trimmed, "- [x] "), strings.HasPrefix(trimmed, "- [X] "):
			done = true
			trimmed = strings.TrimSpace(trimmed[6:])
		case strings.HasPrefix(trimmed, "- [ ] "):
			trimmed = strings.TrimSpace(trimmed[6:])
		default:
			continue
		}
		if trimmed != "" {
			items = append(items, TodoItem{Text: trimmed, Done: done})
		}
	}
	return items
}

// FullScreenUI is an event-driven terminal adapter over Proton services.
type FullScreenUI struct {
	service  *toolcall.Service
	registry tool.Registry
	keys     KeySource
	screen   Screen
	runner   TurnRunner

	scrollback []string
	history    []string
	historyPos int
	input      []rune
	cursor     int
	todo       []TodoItem
	planMode   bool
	nextID     uint64
	modal      *Modal
}

// NewFullScreen creates a full-screen UI over existing application boundaries.
func NewFullScreen(
	service *toolcall.Service,
	registry tool.Registry,
	keys KeySource,
	screen Screen,
	options ...FullScreenOption,
) (*FullScreenUI, error) {
	if service == nil {
		return nil, fmt.Errorf("full-screen UI service is required")
	}
	if registry == nil {
		return nil, fmt.Errorf("full-screen UI registry is required")
	}
	if keys == nil {
		return nil, fmt.Errorf("full-screen UI key source is required")
	}
	if screen == nil {
		return nil, fmt.Errorf("full-screen UI screen is required")
	}
	ui := &FullScreenUI{
		service:    service,
		registry:   registry,
		keys:       keys,
		screen:     screen,
		historyPos: 0,
		scrollback: make([]string, 0),
		todo:       make([]TodoItem, 0),
	}
	for _, option := range options {
		if option == nil {
			continue
		}
		if err := option(ui); err != nil {
			return nil, err
		}
	}
	return ui, nil
}

// Run enters the alternate screen and processes keys until quit or cancellation.
func (ui *FullScreenUI) Run(ctx context.Context) error {
	if err := ui.screen.Enter(); err != nil {
		return fmt.Errorf("enter full-screen terminal: %w", err)
	}
	runErr := ui.loop(ctx)
	exitErr := ui.screen.Exit()
	if runErr != nil && exitErr != nil {
		return fmt.Errorf("run full-screen UI: %v; exit terminal: %w", runErr, exitErr)
	}
	if runErr != nil {
		return runErr
	}
	if exitErr != nil {
		return fmt.Errorf("exit full-screen terminal: %w", exitErr)
	}
	return nil
}

// PermissionPrompt returns a modal resolver for toolcall.WithPrompt.
func (ui *FullScreenUI) PermissionPrompt(
	ctx context.Context,
	request permission.Request,
) (permission.Resolution, error) {
	ui.modal = &Modal{
		Title:   "Permission required",
		Body:    fmt.Sprintf("%s (%s)\nTarget: %s", request.ToolName, request.ToolKind, request.Detail),
		Choices: "[y] allow once  [s] allow for session  [n] deny",
	}
	defer func() { ui.modal = nil }()
	for {
		if err := ui.render(); err != nil {
			return permission.Resolution{}, err
		}
		key, err := ui.keys.ReadKey(ctx)
		if err != nil {
			return permission.Resolution{}, fmt.Errorf("read permission key: %w", err)
		}
		switch strings.ToLower(string(key.Rune)) {
		case "y":
			return permission.Resolution{
				Action: permission.ActionAllow,
				Reason: "user allowed one call",
			}, nil
		case "s":
			return permission.Resolution{
				Action: permission.ActionAllow,
				Scope:  permission.GrantScopeSession,
				Reason: "user allowed this exact request for the session",
			}, nil
		case "n":
			return permission.Resolution{
				Action: permission.ActionDeny,
				Reason: "user denied one call",
			}, nil
		case "":
			if key.Kind == KeyEscape || key.Kind == KeyCtrlC {
				return permission.Resolution{
					Action: permission.ActionDeny,
					Reason: "permission modal canceled",
				}, nil
			}
		default:
			ui.modal.Body = "Press y, s, n, Escape, or Ctrl-C.\n" +
				fmt.Sprintf("%s (%s)\nTarget: %s", request.ToolName, request.ToolKind, request.Detail)
		}
	}
}

func (ui *FullScreenUI) loop(ctx context.Context) error {
	for {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("full-screen UI canceled: %w", err)
		}
		if err := ui.render(); err != nil {
			return err
		}
		key, err := ui.keys.ReadKey(ctx)
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read full-screen key: %w", err)
		}
		shouldExit, handleErr := ui.handleKey(ctx, key)
		if handleErr != nil {
			ui.appendLine("error: " + handleErr.Error())
		}
		if shouldExit {
			return nil
		}
	}
}

func (ui *FullScreenUI) render() error {
	width, height := ui.screen.Size()
	frame := Frame{
		Width:      width,
		Height:     height,
		Scrollback: append([]string{}, ui.scrollback...),
		Input:      string(ui.input),
		Cursor:     ui.cursor,
		Mode:       ui.service.Mode().String(),
		PlanMode:   ui.planMode,
		Todo:       append([]TodoItem{}, ui.todo...),
		Modal:      cloneModal(ui.modal),
	}
	if err := ui.screen.Render(frame); err != nil {
		return fmt.Errorf("render full-screen frame: %w", err)
	}
	return nil
}

func (ui *FullScreenUI) handleKey(ctx context.Context, key Key) (bool, error) {
	switch key.Kind {
	case KeyRune:
		if key.Rune != 0 && utf8.ValidRune(key.Rune) {
			ui.insertRune(key.Rune)
		}
		return false, nil
	case KeyEnter:
		return ui.submit(ctx)
	case KeyBackspace:
		ui.backspace()
	case KeyLeft:
		if ui.cursor > 0 {
			ui.cursor--
		}
	case KeyRight:
		if ui.cursor < len(ui.input) {
			ui.cursor++
		}
	case KeyUp:
		ui.historyPrevious()
	case KeyDown:
		ui.historyNext()
	case KeyEscape, KeyCtrlC:
		ui.input = []rune{}
		ui.cursor = 0
	default:
		return false, fmt.Errorf("unsupported key kind %d", key.Kind)
	}
	return false, nil
}

func (ui *FullScreenUI) submit(ctx context.Context) (bool, error) {
	line := strings.TrimSpace(string(ui.input))
	ui.input = []rune{}
	ui.cursor = 0
	if line == "" {
		return false, nil
	}
	ui.history = append(ui.history, line)
	ui.historyPos = len(ui.history)
	ui.appendLine("> " + line)
	if !strings.HasPrefix(line, ":") {
		return false, ui.runPrompt(ctx, line)
	}
	return ui.executeCommand(ctx, line)
}

func (ui *FullScreenUI) executeCommand(ctx context.Context, line string) (bool, error) {
	parts := strings.SplitN(strings.TrimSpace(strings.TrimPrefix(line, ":")), " ", 3)
	command := strings.TrimSpace(parts[0])
	argument := ""
	if len(parts) > 1 {
		argument = strings.TrimSpace(parts[1])
	}
	switch command {
	case "help":
		ui.appendLine(":call <tool> <json> | :tools | :mode <mode> | :plan [on|off] | :todo | :quit")
	case "tools":
		ui.appendLine("Registered tools:")
		for _, definition := range ui.registry.Definitions() {
			ui.appendLine(fmt.Sprintf("- %s [%s]: %s", definition.Name, definition.Kind, definition.Description))
		}
	case "mode":
		if argument == "" {
			ui.appendLine("permission mode: " + ui.service.Mode().String())
			return false, nil
		}
		mode, err := permission.ParseMode(argument)
		if err != nil {
			return false, err
		}
		if err := ui.service.SetMode(mode); err != nil {
			return false, err
		}
		ui.appendLine("permission mode: " + mode.String())
	case "plan":
		if err := ui.setPlanMode(argument); err != nil {
			return false, err
		}
	case "todo":
		ui.appendTodo()
	case "call":
		return false, ui.call(ctx, parts)
	case "quit", "exit":
		return true, nil
	default:
		return false, fmt.Errorf("unknown command %q; try :help", command)
	}
	return false, nil
}

func (ui *FullScreenUI) call(ctx context.Context, parts []string) error {
	if len(parts) < 2 || strings.TrimSpace(parts[1]) == "" {
		return fmt.Errorf("usage: :call <tool> <json>")
	}
	arguments := "{}"
	if len(parts) == 3 && strings.TrimSpace(parts[2]) != "" {
		arguments = parts[2]
	}
	ui.nextID++
	call, err := tool.NewCall(
		fmt.Sprintf("fullscreen-%d", ui.nextID),
		strings.TrimSpace(parts[1]),
		[]byte(arguments),
	)
	if err != nil {
		return err
	}
	ui.appendLine("tool running: " + call.Name)
	result, err := ui.service.Call(ctx, call)
	if result.Output != "" {
		ui.appendOutput(result.Output)
	}
	if result.CheckpointID != "" {
		ui.appendLine("checkpoint: " + result.CheckpointID)
	}
	if err != nil {
		if result.Failure != nil {
			return fmt.Errorf("[%s] %s", result.Failure.Code, result.Failure.Message)
		}
		return err
	}
	ui.appendLine("tool completed: " + call.Name)
	return nil
}

func (ui *FullScreenUI) runPrompt(ctx context.Context, prompt string) error {
	if ui.runner == nil {
		return errors.New("model client is not configured")
	}
	_, err := ui.runner.Run(
		ctx,
		[]model.Message{{Role: model.RoleUser, Content: prompt}},
		func(_ context.Context, event applicationturn.Event) error {
			switch event.Kind {
			case applicationturn.EventTextDelta:
				ui.appendLine("assistant: " + event.Text)
			case applicationturn.EventToolCall:
				ui.appendLine("tool requested: " + event.Call.Name)
			case applicationturn.EventToolResult:
				ui.appendLine("tool finished: " + event.Call.Name)
				if event.Result.Output != "" {
					ui.appendOutput(event.Result.Output)
				}
			case applicationturn.EventFailed:
				if event.Err != nil {
					ui.appendLine("turn failed: " + event.Err.Error())
				}
			}
			return nil
		},
	)
	return err
}

func (ui *FullScreenUI) setPlanMode(argument string) error {
	switch strings.ToLower(argument) {
	case "":
		ui.planMode = !ui.planMode
	case "on", "true":
		ui.planMode = true
	case "off", "false":
		ui.planMode = false
	default:
		return fmt.Errorf("usage: :plan [on|off]")
	}
	state := "off"
	if ui.planMode {
		state = "on"
	}
	ui.appendLine("plan mode: " + state)
	return nil
}

func (ui *FullScreenUI) appendTodo() {
	if len(ui.todo) == 0 {
		ui.appendLine("TODO pane is empty")
		return
	}
	ui.appendLine("TODO:")
	for _, item := range ui.todo {
		mark := " "
		if item.Done {
			mark = "x"
		}
		ui.appendLine(fmt.Sprintf("[%s] %s", mark, item.Text))
	}
}

func (ui *FullScreenUI) appendOutput(output string) {
	for _, line := range strings.Split(strings.TrimSuffix(output, "\n"), "\n") {
		ui.appendLine("output: " + line)
	}
}

func (ui *FullScreenUI) appendLine(line string) {
	ui.scrollback = append(ui.scrollback, line)
	if len(ui.scrollback) > maxFullScreenScrollback {
		ui.scrollback = ui.scrollback[len(ui.scrollback)-maxFullScreenScrollback:]
	}
}

func (ui *FullScreenUI) insertRune(value rune) {
	ui.input = append(ui.input, 0)
	copy(ui.input[ui.cursor+1:], ui.input[ui.cursor:])
	ui.input[ui.cursor] = value
	ui.cursor++
}

func (ui *FullScreenUI) backspace() {
	if ui.cursor == 0 {
		return
	}
	ui.input = append(ui.input[:ui.cursor-1], ui.input[ui.cursor:]...)
	ui.cursor--
}

func (ui *FullScreenUI) historyPrevious() {
	if len(ui.history) == 0 || ui.historyPos == 0 {
		return
	}
	ui.historyPos--
	ui.input = []rune(ui.history[ui.historyPos])
	ui.cursor = len(ui.input)
}

func (ui *FullScreenUI) historyNext() {
	if ui.historyPos >= len(ui.history) {
		return
	}
	ui.historyPos++
	if ui.historyPos == len(ui.history) {
		ui.input = []rune{}
		ui.cursor = 0
		return
	}
	ui.input = []rune(ui.history[ui.historyPos])
	ui.cursor = len(ui.input)
}

func cloneModal(modal *Modal) *Modal {
	if modal == nil {
		return nil
	}
	clone := *modal
	return &clone
}

// RuneKeySource converts rune and ANSI escape input into normalized keys.
type RuneKeySource struct {
	reader *bufio.Reader
}

// NewRuneKeySource creates a key source from a terminal reader.
func NewRuneKeySource(reader io.Reader) (*RuneKeySource, error) {
	if reader == nil {
		return nil, errors.New("key source reader is required")
	}
	return &RuneKeySource{reader: bufio.NewReader(reader)}, nil
}

// ReadKey reads one key, recognizing common arrow escape sequences.
func (s *RuneKeySource) ReadKey(ctx context.Context) (Key, error) {
	if err := ctx.Err(); err != nil {
		return Key{}, err
	}
	runeValue, _, err := s.reader.ReadRune()
	if err != nil {
		return Key{}, err
	}
	switch runeValue {
	case '\r', '\n':
		return Key{Kind: KeyEnter}, nil
	case '\b', 0x7f:
		return Key{Kind: KeyBackspace}, nil
	case 0x03:
		return Key{Kind: KeyCtrlC}, nil
	case 0x1b:
		return s.readEscape()
	default:
		return Key{Kind: KeyRune, Rune: runeValue}, nil
	}
}

func (s *RuneKeySource) readEscape() (Key, error) {
	sequence, _, err := s.reader.ReadRune()
	if err != nil {
		if errors.Is(err, io.EOF) {
			return Key{Kind: KeyEscape}, nil
		}
		return Key{}, err
	}
	if sequence != '[' {
		return Key{Kind: KeyEscape}, nil
	}
	code, _, err := s.reader.ReadRune()
	if err != nil {
		return Key{}, err
	}
	switch code {
	case 'A':
		return Key{Kind: KeyUp}, nil
	case 'B':
		return Key{Kind: KeyDown}, nil
	case 'C':
		return Key{Kind: KeyRight}, nil
	case 'D':
		return Key{Kind: KeyLeft}, nil
	default:
		return Key{Kind: KeyEscape}, nil
	}
}
