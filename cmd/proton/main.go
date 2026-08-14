// Command proton starts the Proton coding-agent bootstrap UI.
package main

import (
	"bufio"
	"context"
	"fmt"
	"os"

	"github.com/projectTHORN/proton/internal/adapters/tools"
	"github.com/projectTHORN/proton/internal/adapters/tui"
	"github.com/projectTHORN/proton/internal/application/toolcall"
	"github.com/projectTHORN/proton/internal/domain/permission"
)

func main() {
	if err := run(context.Background()); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	registry, err := tools.NewDefaultRegistry()
	if err != nil {
		return fmt.Errorf("create tool registry: %w", err)
	}

	policy, err := permission.NewPolicy(permission.Config{
		Default: permission.ActionAsk,
	})
	if err != nil {
		return fmt.Errorf("create permission policy: %w", err)
	}

	reader := bufio.NewReader(os.Stdin)
	service, err := toolcall.NewService(
		registry,
		policy,
		toolcall.WithPrompt(tui.NewPermissionPrompt(reader, os.Stdout)),
	)
	if err != nil {
		return fmt.Errorf("create tool-call service: %w", err)
	}

	ui, err := tui.New(service, registry, reader, os.Stdout)
	if err != nil {
		return fmt.Errorf("create terminal UI: %w", err)
	}

	return ui.Run(ctx)
}
