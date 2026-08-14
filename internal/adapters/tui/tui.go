// Package tui provides the initial terminal UI adapter for Proton.
//
// This bootstrap UI is line-oriented so it works in plain terminals and CI.
// The application boundary is intentionally ready for the full-screen event
// loop planned in TODO.md.
package tui

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/projectTHORN/proton/internal/application/toolcall"
	"github.com/projectTHORN/proton/internal/domain/permission"
	"github.com/projectTHORN/proton/internal/domain/tool"
)

// UI is a small interactive terminal shell around the application service.
type UI struct {
	service  *toolcall.Service
	registry tool.Registry
	reader   *bufio.Reader
	writer   io.Writer
	nextID   uint64
}

// New creates a terminal UI over an existing tool-call service.
func New(
	service *toolcall.Service,
	registry tool.Registry,
	reader *bufio.Reader,
	writer io.Writer,
) (*UI, error) {
	if service == nil {
		return nil, fmt.Errorf("terminal UI service is required")
	}
	if registry == nil {
		return nil, fmt.Errorf("terminal UI registry is required")
	}
	if reader == nil {
		return nil, fmt.Errorf("terminal UI reader is required")
	}
	if writer == nil {
		return nil, fmt.Errorf("terminal UI writer is required")
	}
	return &UI{
		service:  service,
		registry: registry,
		reader:   reader,
		writer:   writer,
	}, nil
}

// NewPermissionPrompt creates the interactive prompt used by ask mode.
func NewPermissionPrompt(reader *bufio.Reader, writer io.Writer) toolcall.PermissionPrompt {
	return func(ctx context.Context, request permission.Request) (permission.Resolution, error) {
		if reader == nil || writer == nil {
			return permission.Resolution{}, fmt.Errorf("permission prompt reader and writer are required")
		}
		if err := ctx.Err(); err != nil {
			return permission.Resolution{}, fmt.Errorf("permission prompt canceled: %w", err)
		}
		if _, err := fmt.Fprintf(
			writer,
			"\nPermission required: %s (%s)\nTarget: %s\nAllow once [y], allow for session [s], deny [n]: ",
			request.ToolName,
			request.ToolKind,
			request.Detail,
		); err != nil {
			return permission.Resolution{}, fmt.Errorf("write permission prompt: %w", err)
		}

		line, err := readLine(reader)
		if err != nil {
			return permission.Resolution{}, fmt.Errorf("read permission response: %w", err)
		}
		switch strings.ToLower(strings.TrimSpace(line)) {
		case "y", "yes", "allow":
			return permission.Resolution{
				Action: permission.ActionAllow,
				Reason: "user allowed one call",
			}, nil
		case "s", "session":
			return permission.Resolution{
				Action: permission.ActionAllow,
				Scope:  permission.GrantScopeSession,
				Reason: "user allowed this exact request for the session",
			}, nil
		case "n", "no", "deny":
			return permission.Resolution{
				Action: permission.ActionDeny,
				Reason: "user denied one call",
			}, nil
		default:
			return permission.Resolution{
				Action: permission.ActionDeny,
				Reason: "unrecognized permission response",
			}, nil
		}
	}
}

// Run starts the terminal UI until the user exits or the context is canceled.
func (ui *UI) Run(ctx context.Context) error {
	if _, err := fmt.Fprintln(ui.writer, "Proton — Go coding-agent port"); err != nil {
		return fmt.Errorf("write welcome: %w", err)
	}
	if _, err := fmt.Fprintln(ui.writer, "Type :help for commands. Permission mode: "+ui.service.Mode().String()); err != nil {
		return fmt.Errorf("write help hint: %w", err)
	}

	for {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("terminal UI canceled: %w", err)
		}
		if _, err := fmt.Fprint(ui.writer, "\nproton> "); err != nil {
			return fmt.Errorf("write prompt: %w", err)
		}
		line, err := readLine(ui.reader)
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read terminal input: %w", err)
		}
		shouldExit, err := ui.handleLine(ctx, line)
		if err != nil {
			if _, writeErr := fmt.Fprintf(ui.writer, "error: %v\n", err); writeErr != nil {
				return fmt.Errorf("write command error: %w", writeErr)
			}
		}
		if shouldExit {
			return nil
		}
	}
}

func (ui *UI) handleLine(ctx context.Context, line string) (bool, error) {
	line = strings.TrimSpace(line)
	if line == "" {
		return false, nil
	}
	if !strings.HasPrefix(line, ":") {
		return false, fmt.Errorf("commands start with ':', try :help")
	}

	parts := strings.SplitN(line[1:], " ", 3)
	switch parts[0] {
	case "help":
		return false, ui.help()
	case "tools":
		return false, ui.listTools()
	case "mode":
		return false, ui.changeMode(parts[1:])
	case "call":
		return false, ui.call(ctx, parts[1:])
	case "quit", "exit":
		_, err := fmt.Fprintln(ui.writer, "goodbye")
		return true, err
	default:
		return false, fmt.Errorf("unknown command %q; try :help", parts[0])
	}
}

func (ui *UI) help() error {
	_, err := fmt.Fprintln(
		ui.writer,
		":help | :tools | :call <tool> <json> | :mode <ask|auto|always-approve|deny> | :quit",
	)
	return err
}

func (ui *UI) listTools() error {
	if _, err := fmt.Fprintln(ui.writer, "Registered tools:"); err != nil {
		return err
	}
	for _, definition := range ui.registry.Definitions() {
		if _, err := fmt.Fprintf(ui.writer, "- %s [%s]: %s\n", definition.Name, definition.Kind, definition.Description); err != nil {
			return err
		}
	}
	return nil
}

func (ui *UI) changeMode(arguments []string) error {
	if len(arguments) == 0 {
		_, err := fmt.Fprintln(ui.writer, "permission mode: "+ui.service.Mode().String())
		return err
	}
	mode, err := permission.ParseMode(arguments[0])
	if err != nil {
		return err
	}
	if err := ui.service.SetMode(mode); err != nil {
		return err
	}
	_, err = fmt.Fprintln(ui.writer, "permission mode: "+mode.String())
	return err
}

func (ui *UI) call(ctx context.Context, arguments []string) error {
	if len(arguments) < 1 {
		return fmt.Errorf("usage: :call <tool> <json>")
	}
	name := arguments[0]
	jsonArguments := "{}"
	if len(arguments) == 2 && strings.TrimSpace(arguments[1]) != "" {
		jsonArguments = arguments[1]
	}
	ui.nextID++
	call, err := tool.NewCall(fmt.Sprintf("tui-%d", ui.nextID), name, []byte(jsonArguments))
	if err != nil {
		return err
	}
	result, err := ui.service.Call(ctx, call)
	if result.Output != "" {
		if _, writeErr := fmt.Fprintf(ui.writer, "output:\n%s\n", result.Output); writeErr != nil {
			return writeErr
		}
	}
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(ui.writer, "tool completed")
	return err
}

func readLine(reader *bufio.Reader) (string, error) {
	line, err := reader.ReadString('\n')
	if err != nil && !(errors.Is(err, io.EOF) && line != "") {
		return "", err
	}
	return strings.TrimSuffix(line, "\n"), nil
}
