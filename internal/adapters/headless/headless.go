// Package headless runs Proton without a terminal UI.
package headless

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/projectTHORN/proton/internal/adapters/session"
	"github.com/projectTHORN/proton/internal/application/toolcall"
	applicationturn "github.com/projectTHORN/proton/internal/application/turn"
	"github.com/projectTHORN/proton/internal/domain/model"
	"github.com/projectTHORN/proton/internal/domain/permission"
	"github.com/projectTHORN/proton/internal/domain/tool"
)

// Format is the headless output encoding.
type Format string

const (
	// FormatText writes a human-readable transcript.
	FormatText Format = "text"
	// FormatJSON writes one JSON object per event (NDJSON).
	FormatJSON Format = "json"
)

// ErrInvalidRunner indicates that the headless adapter cannot be constructed.
var ErrInvalidRunner = errors.New("invalid headless runner")

// TurnRunner is the optional model/tool loop used by ordinary prompts.
type TurnRunner interface {
	Run(context.Context, []model.Message, applicationturn.Sink) (applicationturn.Result, error)
}

// Runner is the non-interactive adapter over Proton services.
type Runner struct {
	service  *toolcall.Service
	registry tool.Registry
	runner   TurnRunner
	messages []model.Message
	nextID   uint64
}

// New creates a fail-closed headless runner. Ask-mode calls stay denied
// because no permission prompt is installed.
func New(service *toolcall.Service, registry tool.Registry, runner TurnRunner) (*Runner, error) {
	if service == nil {
		return nil, fmt.Errorf("%w: service is required", ErrInvalidRunner)
	}
	if registry == nil {
		return nil, fmt.Errorf("%w: registry is required", ErrInvalidRunner)
	}
	return &Runner{
		service:  service,
		registry: registry,
		runner:   runner,
		messages: make([]model.Message, 0),
	}, nil
}

// Messages returns a copy of the in-memory transcript.
func (r *Runner) Messages() []model.Message {
	return model.CloneMessages(r.messages)
}

// SetMessages replaces the transcript used for later turns.
func (r *Runner) SetMessages(messages []model.Message) error {
	for _, message := range messages {
		if err := message.Validate(); err != nil {
			return fmt.Errorf("load headless transcript: %w", err)
		}
	}
	r.messages = model.CloneMessages(messages)
	return nil
}

// LoadSession restores a persisted transcript.
func (r *Runner) LoadSession(state session.State) error {
	messages := make([]model.Message, 0, len(state.Messages))
	for _, stored := range state.Messages {
		messages = append(messages, model.Message{
			Role:       model.Role(stored.Role),
			Content:    stored.Content,
			ToolName:   stored.ToolName,
			ToolCallID: stored.ToolCallID,
		})
	}
	return r.SetMessages(messages)
}

// SessionState returns the redacted transcript for persistence.
func (r *Runner) SessionState() []session.Message {
	out := make([]session.Message, 0, len(r.messages))
	for _, message := range r.messages {
		out = append(out, session.Message{
			Role:       string(message.Role),
			Content:    message.Content,
			ToolName:   message.ToolName,
			ToolCallID: message.ToolCallID,
		})
	}
	return out
}

// Run executes one headless prompt through the shared tool-call service.
func (r *Runner) Run(ctx context.Context, prompt string, output io.Writer, format Format) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("start headless run: %w", err)
	}
	if output == nil {
		return fmt.Errorf("%w: output writer is required", ErrInvalidRunner)
	}
	if format != FormatText && format != FormatJSON {
		return fmt.Errorf("%w: unsupported output format %q", ErrInvalidRunner, format)
	}
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return fmt.Errorf("headless prompt is empty")
	}

	if isCommand(prompt) {
		return r.runCommand(ctx, prompt, output, format)
	}
	return r.runTurn(ctx, prompt, output, format)
}

func (r *Runner) runCommand(
	ctx context.Context,
	line string,
	output io.Writer,
	format Format,
) error {
	name, argument, parts := splitCommand(line)
	switch name {
	case "help":
		return writeEvent(output, format, Event{Kind: "text", Text: commandHelp()})
	case "tools":
		var builder strings.Builder
		for _, definition := range r.registry.Definitions() {
			fmt.Fprintf(&builder, "- %s [%s]: %s\n", definition.Name, definition.Kind, definition.Description)
		}
		return writeEvent(output, format, Event{Kind: "text", Text: strings.TrimRight(builder.String(), "\n")})
	case "mode":
		if argument == "" {
			return writeEvent(output, format, Event{
				Kind: "text",
				Text: "permission mode: " + r.service.Mode().String(),
			})
		}
		mode, err := permission.ParseMode(argument)
		if err != nil {
			return err
		}
		if err := r.service.SetMode(mode); err != nil {
			return err
		}
		return writeEvent(output, format, Event{Kind: "text", Text: "permission mode: " + mode.String()})
	case "call":
		return r.runCall(ctx, parts, output, format)
	default:
		return fmt.Errorf("unknown command %q; try /help", name)
	}
}

func (r *Runner) runCall(
	ctx context.Context,
	parts []string,
	output io.Writer,
	format Format,
) error {
	if len(parts) < 2 || strings.TrimSpace(parts[1]) == "" {
		return fmt.Errorf("usage: /call <tool> <json>")
	}
	arguments := "{}"
	if len(parts) == 3 && strings.TrimSpace(parts[2]) != "" {
		arguments = parts[2]
	}
	r.nextID++
	call, err := tool.NewCall(
		fmt.Sprintf("headless-%d", r.nextID),
		strings.TrimSpace(parts[1]),
		[]byte(arguments),
	)
	if err != nil {
		return err
	}
	if err := writeEvent(output, format, Event{Kind: "tool_call", Tool: call.Name}); err != nil {
		return err
	}
	result, callErr := r.service.Call(ctx, call)
	r.messages = append(r.messages, model.Message{
		Role:       model.RoleTool,
		Content:    result.Output,
		ToolName:   call.Name,
		ToolCallID: call.ID,
	})
	event := Event{Kind: "tool_result", Tool: call.Name, Output: result.Output}
	if callErr != nil {
		if result.Failure != nil {
			event.Error = fmt.Sprintf("[%s]: %s", result.Failure.Code, result.Failure.Message)
		} else {
			event.Error = callErr.Error()
		}
	}
	if err := writeEvent(output, format, event); err != nil {
		return err
	}
	return callErr
}

func (r *Runner) runTurn(
	ctx context.Context,
	prompt string,
	output io.Writer,
	format Format,
) error {
	if r.runner == nil {
		return fmt.Errorf("model client is not configured; use /help or /call")
	}
	r.messages = append(r.messages, model.Message{Role: model.RoleUser, Content: prompt})
	result, err := r.runner.Run(ctx, r.Messages(), func(_ context.Context, event applicationturn.Event) error {
		switch event.Kind {
		case applicationturn.EventTextDelta:
			return writeEvent(output, format, Event{Kind: "text", Text: event.Text})
		case applicationturn.EventToolCall:
			return writeEvent(output, format, Event{Kind: "tool_call", Tool: event.Call.Name})
		case applicationturn.EventToolResult:
			return writeEvent(output, format, Event{
				Kind:   "tool_result",
				Tool:   event.Call.Name,
				Output: event.Result.Output,
			})
		default:
			return nil
		}
	})
	if result.Message.Content != "" {
		r.messages = append(r.messages, result.Message)
	}
	if err != nil {
		_ = writeEvent(output, format, Event{Kind: "failed", Error: err.Error()})
		return err
	}
	return writeEvent(output, format, Event{Kind: "done"})
}

// Event is one headless output record.
type Event struct {
	Kind   string `json:"kind"`
	Text   string `json:"text,omitempty"`
	Tool   string `json:"tool,omitempty"`
	Output string `json:"output,omitempty"`
	Error  string `json:"error,omitempty"`
}

func writeEvent(output io.Writer, format Format, event Event) error {
	switch format {
	case FormatJSON:
		payload, err := json.Marshal(event)
		if err != nil {
			return fmt.Errorf("encode headless event: %w", err)
		}
		_, err = fmt.Fprintf(output, "%s\n", payload)
		return err
	default:
		switch event.Kind {
		case "text":
			_, err := fmt.Fprintln(output, event.Text)
			return err
		case "tool_call":
			_, err := fmt.Fprintf(output, "tool running: %s\n", event.Tool)
			return err
		case "tool_result":
			if event.Output != "" {
				if _, err := fmt.Fprintln(output, event.Output); err != nil {
					return err
				}
			}
			if event.Error != "" {
				_, err := fmt.Fprintf(output, "error: %s\n", event.Error)
				return err
			}
			return nil
		case "failed":
			_, err := fmt.Fprintf(output, "error: %s\n", event.Error)
			return err
		default:
			return nil
		}
	}
}

func isCommand(line string) bool {
	return strings.HasPrefix(line, "/") || strings.HasPrefix(line, ":")
}

func splitCommand(line string) (name string, argument string, parts []string) {
	body := strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(line), "/:"))
	parts = strings.SplitN(body, " ", 3)
	if len(parts) == 0 {
		return "", "", nil
	}
	name = strings.TrimSpace(parts[0])
	if len(parts) > 1 {
		argument = strings.TrimSpace(parts[1])
	}
	return name, argument, parts
}

func commandHelp() string {
	return strings.Join([]string{
		"/call <tool> <json>   run a registered tool",
		"/tools                list tools",
		"/mode [ask|always-approve|deny]",
		"/help                 list commands",
	}, "\n")
}

// ParseFormat parses the --output value.
func ParseFormat(value string) (Format, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "text", "plain":
		return FormatText, nil
	case "json":
		return FormatJSON, nil
	default:
		return "", fmt.Errorf("unsupported output format %q", value)
	}
}
