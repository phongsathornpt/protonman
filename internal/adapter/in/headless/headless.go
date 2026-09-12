// Package headless runs Protonman without a terminal UI.
package headless

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/phongsathornpt/protonman/internal/app"
	"github.com/phongsathornpt/protonman/internal/app/appdirs"
	"github.com/phongsathornpt/protonman/internal/base/envconfig"
	"github.com/phongsathornpt/protonman/internal/core/conversation"
	"github.com/phongsathornpt/protonman/internal/core/permission"
	"github.com/phongsathornpt/protonman/internal/core/session"
	"github.com/phongsathornpt/protonman/internal/core/tool"
	"github.com/phongsathornpt/protonman/internal/engine/toolcall"
	"github.com/phongsathornpt/protonman/internal/feature/agent"
	"github.com/phongsathornpt/protonman/internal/feature/skill"
	sdk "github.com/phongsathornpt/protonman/proton-sdk"
)

// Option configures the headless runner.
type Option func(*Runner)

// WithSkills configures the skill registry for the headless runner.
func WithSkills(skills *skill.Registry) Option {
	return func(r *Runner) {
		r.skills = skills
	}
}

// WithSessionID binds orchestration spawned by this runner to one session.
func WithSessionID(sessionID string) Option {
	return func(r *Runner) { r.sessionID = strings.TrimSpace(sessionID) }
}

// WithAgents supplies session-scoped subagent lifecycle control.
func WithAgents(agents app.Agents) Option {
	return func(r *Runner) { r.agents = agents }
}

// Format is the headless output encoding.
type Format uint8

const (
	// FormatUnknown is the invalid zero value.
	FormatUnknown Format = iota
	// FormatText writes a human-readable transcript.
	FormatText
	// FormatJSON writes one JSON object per event (NDJSON).
	FormatJSON
)

func (f Format) String() string {
	switch f {
	case FormatText:
		return "text"
	case FormatJSON:
		return "json"
	default:
		return "unknown"
	}
}

// Valid reports whether the format is a supported non-zero format.
func (f Format) Valid() bool {
	return f == FormatText || f == FormatJSON
}

// MarshalText implements encoding.TextMarshaler.
func (f Format) MarshalText() ([]byte, error) {
	return []byte(f.String()), nil
}

// UnmarshalText implements encoding.TextUnmarshaler.
func (f *Format) UnmarshalText(text []byte) error {
	parsed, err := ParseFormat(string(text))
	if err != nil {
		return err
	}
	*f = parsed
	return nil
}

// ErrInvalidRunner indicates that the headless adapter cannot be constructed.
var ErrInvalidRunner = errors.New("invalid headless runner")

// Runner is the non-interactive adapter over Protonman services.
type Runner struct {
	service   *toolcall.Service
	registry  tool.Registry
	skills    *skill.Registry
	runner    app.Conversation
	messages  []sdk.Message
	retention conversation.RetentionPolicy
	nextID    uint64
	turnSeq   uint64
	sessionID string
	agents    app.Agents
}

// New creates a fail-closed headless runner. Ask-mode calls stay denied
// because no permission prompt is installed.
func New(service *toolcall.Service, registry tool.Registry, runner app.Conversation, options ...Option) (*Runner, error) {
	if service == nil {
		return nil, fmt.Errorf("%w: service is required", ErrInvalidRunner)
	}
	if registry == nil {
		return nil, fmt.Errorf("%w: registry is required", ErrInvalidRunner)
	}
	r := &Runner{
		service:   service,
		registry:  registry,
		runner:    runner,
		messages:  make([]sdk.Message, 0),
		retention: conversation.DefaultRetentionPolicy(),
	}
	for _, opt := range options {
		if opt != nil {
			opt(r)
		}
	}
	return r, nil
}

// Messages returns a copy of the in-memory transcript.
func (r *Runner) Messages() []sdk.Message {
	return sdk.CloneMessages(r.messages)
}

// SetMessages replaces the transcript used for later turns.
func (r *Runner) SetMessages(messages []sdk.Message) error {
	for _, message := range messages {
		if err := message.Validate(); err != nil {
			return fmt.Errorf("load headless transcript: %w", err)
		}
	}
	r.messages = conversation.Retain(sdk.CloneMessages(messages), r.retention)
	return nil
}

// LoadSession restores a persisted transcript.
func (r *Runner) LoadSession(state session.State) error {
	return r.SetMessages(session.ToModelMessages(state.Messages))
}

// SessionState returns the redacted transcript for persistence.
func (r *Runner) SessionState() []session.Message {
	return session.FromModelMessages(r.messages)
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
		return fmt.Errorf("%w: unsupported output format %s", ErrInvalidRunner, format)
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
	case "skills", "skill":
		return r.handleSkillsCommand(argument, parts, output, format)
	default:
		return fmt.Errorf("unknown command %q; try /help", name)
	}
}

func (r *Runner) handleSkillsCommand(argument string, parts []string, output io.Writer, format Format) error {
	trimmedArg := strings.TrimSpace(argument)

	if r.skills == nil || len(r.skills.List()) == 0 {
		return writeEvent(output, format, Event{
			Kind: EventKindText,
			Text: fmt.Sprintf("No agent skills discovered.\nPlace skills in %s or .protonman/skills/ (with %s=1).", appdirs.UserSkillsDisplay(), envconfig.TrustProject),
		})
	}

	if trimmedArg == "" {
		skillsList := r.skills.List()
		activeCount := len(r.skills.ActivatedList())
		var builder strings.Builder
		fmt.Fprintf(&builder, "Agent Skills (%d/%d active):", activeCount, len(skillsList))
		for _, s := range skillsList {
			box := "[ ]"
			if r.skills.IsActivated(s.Name) {
				box = "[x]"
			}
			lockTag := ""
			if s.Locked {
				if s.LockStatus == skill.LockStatusVerified {
					lockTag = " [locked]"
				} else if s.LockStatus == skill.LockStatusDrifted {
					lockTag = " [drift]"
				}
			}
			fmt.Fprintf(&builder, "\n  %s %s [%s]%s: %s", box, s.Name, s.Scope, lockTag, s.Description)
		}
		return writeEvent(output, format, Event{Kind: EventKindText, Text: builder.String()})
	}

	if trimmedArg == "check" || trimmedArg == "verify" {
		lockPath := r.skills.ProjectLockPath()
		if lockPath == "" {
			lockPath = skill.LockFileName
		}
		lock, err := skill.ReadLockFile(lockPath)
		if err != nil || len(lock.Skills) == 0 {
			return writeEvent(output, format, Event{
				Kind: EventKindText,
				Text: fmt.Sprintf("No project skill lock found (%s).\nUse /skills lock to generate a lockfile for project skills.", lockPath),
			})
		}

		projectSkills := make([]skill.Skill, 0)
		for _, s := range r.skills.List() {
			if s.Scope == skill.ScopeProject {
				projectSkills = append(projectSkills, s)
			}
		}
		report := skill.VerifyProjectSkills(lock, projectSkills)
		report.LockPath = lockPath
		r.skills.SetProjectLock(lockPath, &report)

		var builder strings.Builder
		fmt.Fprintf(&builder, "Project Skill Lock (%s):", report.LockPath)
		verified, drifted, missing, unlocked := report.Summary()
		fmt.Fprintf(&builder, "\n  Summary: %d verified, %d drifted, %d missing, %d unlocked", verified, drifted, missing, unlocked)
		for _, res := range report.Results {
			switch res.Status {
			case skill.LockStatusVerified:
				fmt.Fprintf(&builder, "\n  [verified] %s (%s)", res.Name, shortHash(res.ComputedHash))
			case skill.LockStatusDrifted:
				fmt.Fprintf(&builder, "\n  [drifted]  %s: expected %s, got %s", res.Name, shortHash(res.ExpectedHash), shortHash(res.ComputedHash))
			case skill.LockStatusMissing:
				fmt.Fprintf(&builder, "\n  [missing]  %s: expected %s (not on disk)", res.Name, shortHash(res.ExpectedHash))
			case skill.LockStatusUnlocked:
				fmt.Fprintf(&builder, "\n  [unlocked] %s: on disk but not locked", res.Name)
			}
		}
		if report.IsClean() {
			builder.WriteString("\nAll locked skills verified cleanly.")
		}
		return writeEvent(output, format, Event{Kind: EventKindText, Text: builder.String()})
	}

	if trimmedArg == "lock" {
		projectSkills := make([]skill.Skill, 0)
		for _, s := range r.skills.List() {
			if s.Scope == skill.ScopeProject {
				projectSkills = append(projectSkills, s)
			}
		}
		if len(projectSkills) == 0 {
			return writeEvent(output, format, Event{
				Kind:  EventKindFailed,
				Error: "No project skills found to lock. Only project-scoped skills can be locked.",
			})
		}

		lockPath := r.skills.ProjectLockPath()
		if lockPath == "" {
			lockPath = skill.LockFileName
		}

		existingLock, _ := skill.ReadLockFile(lockPath)
		newLock, err := skill.GenerateProjectLock(projectSkills, &existingLock)
		if err != nil {
			return err
		}

		if err := skill.WriteLockFile(lockPath, newLock); err != nil {
			return err
		}

		report := skill.VerifyProjectSkills(newLock, projectSkills)
		report.LockPath = lockPath
		r.skills.SetProjectLock(lockPath, &report)

		return writeEvent(output, format, Event{
			Kind: EventKindText,
			Text: fmt.Sprintf("Locked %d project skill(s) to %s.", len(newLock.Skills), lockPath),
		})
	}

	if trimmedArg == "active" {
		active := r.skills.ActivatedList()
		if len(active) == 0 {
			return writeEvent(output, format, Event{
				Kind: EventKindText,
				Text: "No active agent skills in this session.\nActivate skills using /skill <name> or the skill tool.",
			})
		}
		var builder strings.Builder
		fmt.Fprintf(&builder, "Active Agent Skills (%d):", len(active))
		for _, name := range active {
			if s, ok := r.skills.Lookup(name); ok {
				fmt.Fprintf(&builder, "\n  [x] %s [%s]: %s", s.Name, s.Scope, s.Description)
			}
		}
		return writeEvent(output, format, Event{Kind: EventKindText, Text: builder.String()})
	}

	if trimmedArg == "toggle" {
		if len(parts) < 3 || strings.TrimSpace(parts[2]) == "" {
			return fmt.Errorf("usage: /skill toggle <name>")
		}
		target := strings.TrimSpace(parts[2])
		active, err := r.skills.Toggle(target)
		if err != nil {
			return err
		}
		state := "deactivated"
		box := "[ ]"
		if active {
			state = "activated"
			box = "[x]"
		}
		return writeEvent(output, format, Event{
			Kind: EventKindText,
			Text: fmt.Sprintf("%s Skill %q %s.", box, target, state),
		})
	}

	if trimmedArg == "deactivate" || trimmedArg == "disable" || trimmedArg == "remove" || trimmedArg == "off" {
		if len(parts) < 3 || strings.TrimSpace(parts[2]) == "" {
			return fmt.Errorf("usage: /skill %s <name>", trimmedArg)
		}
		target := strings.TrimSpace(parts[2])
		if _, ok := r.skills.Lookup(target); !ok {
			return fmt.Errorf("skill %q not found; try /skills to list available skills", target)
		}
		if !r.skills.IsActivated(target) {
			return writeEvent(output, format, Event{
				Kind: EventKindText,
				Text: fmt.Sprintf("[ ] Skill %q is not active.", target),
			})
		}
		r.skills.Deactivate(target)
		return writeEvent(output, format, Event{
			Kind: EventKindText,
			Text: fmt.Sprintf("[ ] Skill %q deactivated.", target),
		})
	}

	target := trimmedArg
	if (trimmedArg == "activate" || trimmedArg == "enable" || trimmedArg == "on") && len(parts) >= 3 {
		target = strings.TrimSpace(parts[2])
	}

	s, ok := r.skills.Lookup(target)
	if !ok {
		return fmt.Errorf("skill %q not found; try /skills to list available skills", target)
	}
	if r.skills.IsActivated(s.Name) {
		return writeEvent(output, format, Event{
			Kind: EventKindText,
			Text: fmt.Sprintf("[x] Skill %q is already active. Use /skills toggle %s to deactivate.", s.Name, s.Name),
		})
	}

	if err := r.skills.Activate(s.Name); err != nil {
		return err
	}
	var builder strings.Builder
	fmt.Fprintf(&builder, "[x] Activated skill %s [%s]: %s", s.Name, s.Scope, s.Description)
	if len(s.Resources) > 0 {
		builder.WriteString("\nBundled resources:")
		for _, res := range s.Resources {
			fmt.Fprintf(&builder, "\n  - %s", res)
		}
	}
	return writeEvent(output, format, Event{
		Kind: EventKindText,
		Text: builder.String(),
	})
}

func (r *Runner) retainMessages() {
	if r == nil {
		return
	}
	r.messages = conversation.Retain(r.messages, r.retention)
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
	callCtx := agent.WithTurnRef(ctx, agent.TurnRef{SessionID: r.sessionID, TurnID: fmt.Sprintf("headless-call-%d", r.nextID)})
	result, callErr := r.service.Call(callCtx, call)
	resultContent, marshalErr := json.Marshal(result.ModelPayload())
	if marshalErr != nil {
		marshalErr = fmt.Errorf("encode headless tool result: %w", marshalErr)
	}
	r.messages = append(r.messages, sdk.Message{
		ID:      sdk.NewMessageID(),
		Role:    sdk.RoleUser,
		Content: fmt.Sprintf("/call %s", call.Name),
	}, sdk.Message{
		ID:   sdk.NewMessageID(),
		Role: sdk.RoleAssistant,
		ToolCalls: []sdk.ToolCall{{
			ID:        call.ID,
			Name:      call.Name,
			Arguments: append(json.RawMessage(nil), call.Arguments...),
		}},
	}, sdk.Message{
		ID:         sdk.NewMessageID(),
		Role:       sdk.RoleTool,
		Content:    string(resultContent),
		ToolName:   call.Name,
		ToolCallID: call.ID,
	})
	r.retainMessages()
	event := Event{Kind: "tool_result", Tool: call.Name, Output: result.Output}
	if callErr != nil {
		if result.Failure != nil {
			event.Error = fmt.Sprintf("[%s]: %s", result.Failure.Code, result.Failure.Message)
		} else {
			event.Error = callErr.Error()
		}
	}
	if err := writeEvent(output, format, event); err != nil {
		if callErr != nil {
			return errors.Join(callErr, err)
		}
		return err
	}
	if marshalErr != nil {
		if callErr != nil {
			return errors.Join(callErr, marshalErr)
		}
		return marshalErr
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
	r.messages = append(r.messages, sdk.Message{ID: sdk.NewMessageID(), Role: sdk.RoleUser, Content: prompt})
	r.retainMessages()
	r.turnSeq++
	turnID := fmt.Sprintf("headless-turn-%d", r.turnSeq)
	turnCtx := agent.WithTurnRef(ctx, agent.TurnRef{SessionID: r.sessionID, TurnID: turnID})
	result, err := r.runner.Run(turnCtx, append([]sdk.Message(nil), r.messages...), func(_ context.Context, event app.Event) error {
		switch event.Kind {
		case app.EventTextDelta:
			return writeEvent(output, format, Event{Kind: EventKindText, Text: event.Text})
		case app.EventToolCall:
			return writeEvent(output, format, Event{Kind: EventKindToolCall, Tool: event.Call.Name})
		case app.EventToolResult:
			return writeEvent(output, format, Event{
				Kind:   EventKindToolResult,
				Tool:   event.Call.Name,
				Output: event.Result.Output,
			})
		default:
			return nil
		}
	})
	if turnCtx.Err() != nil {
		r.agents.CancelTurn(turnID, agent.CancelTurnAndChildren)
	}
	if len(result.Messages) > 0 && (err == nil || result.ReplaySafe) {
		r.messages = append(r.messages, result.Messages...)
	} else if err == nil && result.Message.Content != "" {
		r.messages = append(r.messages, result.Message)
	}
	if err != nil {
		if (!result.ReplaySafe || len(result.Messages) == 0) && len(r.messages) > 0 && r.messages[len(r.messages)-1].Role == sdk.RoleUser && r.messages[len(r.messages)-1].Content == prompt {
			last := len(r.messages) - 1
			r.messages[last] = sdk.Message{}
			r.messages = r.messages[:last]
		}
		r.retainMessages()
		if writeErr := writeEvent(output, format, Event{Kind: EventKindFailed, Error: err.Error()}); writeErr != nil {
			return errors.Join(err, writeErr)
		}
		return err
	}
	r.retainMessages()
	return writeEvent(output, format, Event{Kind: EventKindDone})
}

func shortHash(h string) string {
	if len(h) <= 12 {
		return h
	}
	return h[:12]
}

// EventKind identifies one headless output record.
type EventKind string

const (
	// EventKindText is a stream text record.
	EventKindText EventKind = "text"
	// EventKindToolCall reports a running tool call.
	EventKindToolCall EventKind = "tool_call"
	// EventKindToolResult reports a completed tool call.
	EventKindToolResult EventKind = "tool_result"
	// EventKindFailed reports a failed execution.
	EventKindFailed EventKind = "failed"
	// EventKindDone marks the completion of a run.
	EventKindDone EventKind = "done"
)

// Event is one headless output record.
type Event struct {
	Kind   EventKind `json:"kind"`
	Text   string    `json:"text,omitempty"`
	Tool   string    `json:"tool,omitempty"`
	Output string    `json:"output,omitempty"`
	Error  string    `json:"error,omitempty"`
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
		case EventKindText:
			_, err := fmt.Fprintln(output, event.Text)
			return err
		case EventKindToolCall:
			_, err := fmt.Fprintf(output, "tool running: %s\n", event.Tool)
			return err
		case EventKindToolResult:
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
		case EventKindFailed:
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
		"/skills [name]        browse, activate, or toggle skills (alias: /skill)",
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
		return FormatUnknown, fmt.Errorf("unsupported output format %q", value)
	}
}
