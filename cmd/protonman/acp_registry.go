package main

import (
	"context"
	"fmt"
	"strings"

	agenttool "github.com/phongsathornpt/protonman/internal/adapter/out/tool/agent"
	"github.com/phongsathornpt/protonman/internal/adapter/out/tool/builtin"
	skilltool "github.com/phongsathornpt/protonman/internal/adapter/out/tool/skill"
	todotool "github.com/phongsathornpt/protonman/internal/adapter/out/tool/todo"
	webtool "github.com/phongsathornpt/protonman/internal/adapter/out/tool/web"
	"github.com/phongsathornpt/protonman/internal/app/appdirs"
	"github.com/phongsathornpt/protonman/internal/core/session"
	"github.com/phongsathornpt/protonman/internal/core/tool"
	"github.com/phongsathornpt/protonman/internal/core/workspace"
	tododomain "github.com/phongsathornpt/protonman/internal/feature/todo"
	"github.com/phongsathornpt/protonman/internal/platform/checkpoint"
	"github.com/phongsathornpt/protonman/internal/platform/sandbox"
)

// registryForACPSession builds a fresh filesystem boundary for one ACP session.
// cwd is always the primary root and remains the base for relative paths;
// additionalDirectories are full read/write roots for that session only.
func (r *appRuntime) registryForACPSession(sessionID, cwd string, additionalDirectories []string) (tool.Registry, error) {
	if r == nil {
		return nil, fmt.Errorf("runtime is unavailable")
	}
	cwd = strings.TrimSpace(cwd)
	if cwd == "" {
		cwd = r.workDir
	}
	if cwd == "" {
		return nil, fmt.Errorf("ACP session cwd is required")
	}

	layout, err := appdirs.ResolveRuntimeLayout("", cwd)
	if err != nil {
		return nil, fmt.Errorf("resolve ACP session layout: %w", err)
	}
	ws, err := workspace.New(cwd, r.config.ProtectedPaths)
	if err != nil {
		return nil, fmt.Errorf("create ACP session workspace: %w", err)
	}
	if err := ws.ReserveInternalPath(layout.User.Root); err != nil {
		return nil, fmt.Errorf("reserve Protonman internal state for ACP session: %w", err)
	}
	for _, root := range additionalDirectories {
		if err := ws.AddRoot(root); err != nil {
			return nil, fmt.Errorf("add ACP session workspace root %q: %w", root, err)
		}
	}

	// Active skill directories remain read-only even when ACP grants other
	// workspace roots full mutation access.
	if r.skills != nil {
		for _, name := range r.skills.ActivatedList() {
			item, ok := r.skills.Lookup(name)
			if !ok || strings.TrimSpace(item.BaseDir) == "" {
				continue
			}
			if err := ws.AddReadRoot(item.BaseDir); err != nil {
				return nil, fmt.Errorf("authorize active skill %q for ACP session: %w", name, err)
			}
		}
	}

	checkpointStore, err := checkpoint.NewWorkspaceFileStore(layout.User.Checkpoints, workspaceKey(cwd), ws)
	if err != nil {
		return nil, fmt.Errorf("create ACP session checkpoint store: %w", err)
	}
	sandboxProfile, err := sandbox.NewProfile(r.config.Sandbox, cwd)
	if err != nil {
		return nil, fmt.Errorf("create ACP session sandbox profile: %w", err)
	}
	launcher := sandbox.NewOSLauncher(sandboxProfile)

	resources, err := session.ResolveResources(r.sessionsRoot, sessionID)
	if err != nil {
		return nil, fmt.Errorf("resolve ACP session resources: %w", err)
	}
	ctx := context.Background()
	todoStore, err := tododomain.OpenMarkdownStore(ctx, resources.Todo)
	if err != nil {
		return nil, fmt.Errorf("open ACP session todo store: %w", err)
	}
	goal := ""
	if r.stateStore != nil {
		state, found, loadErr := r.stateStore.Load(ctx, sessionID)
		if loadErr != nil {
			return nil, loadErr
		}
		if found {
			goal = state.ActiveGoal
		}
	}
	if _, _, err := todoStore.BindGoal(ctx, goal); err != nil {
		return nil, fmt.Errorf("bind ACP session goal: %w", err)
	}

	baseRegistry, err := builtin.NewDefaultRegistry(ws,
		builtin.WithCheckpointStore(checkpointStore),
		builtin.WithSandbox(launcher),
		builtin.WithAdditionalHandlers(
			webtool.NewWebFetch(sandboxProfile.Network, webtool.WithWebFetchTimeout(r.config.Runtime.WebFetchTimeout)),
			todotool.NewTodoForSession(todoStore, sessionID),
			skilltool.NewActivateSkill(r.skills, ws),
			agenttool.NewSubagent(r.coordinator),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("create ACP session tool registry: %w", err)
	}
	return agenttool.NewCapabilityRegistry(baseRegistry, r.coordinator), nil
}
