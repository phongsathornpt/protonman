package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/projectTHORN/proton/internal/adapter/sessionfs"
	"github.com/projectTHORN/proton/internal/app/appdirs"
	"github.com/projectTHORN/proton/internal/session"
)

func runSessionCommand(ctx context.Context, args []string) (bool, error) {
	if len(args) == 0 || args[0] != "session" {
		return false, nil
	}
	if len(args) < 2 {
		return true, errors.New("session command requires one of: list, resume")
	}
	switch args[1] {
	case "list":
		return true, runSessionList(ctx, args[2:], os.Stdout)
	case "resume":
		resumeArgs, err := sessionResumeArgs(args[2:])
		if err != nil {
			return true, err
		}
		return true, run(ctx, resumeArgs)
	default:
		return true, fmt.Errorf("unknown session command %q", args[1])
	}
}

func sessionResumeArgs(args []string) ([]string, error) {
	if len(args) > 1 {
		return nil, errors.New("usage: proton session resume [session-id]")
	}
	out := []string{"--resume"}
	if len(args) == 1 {
		id := strings.TrimSpace(args[0])
		if id == "" {
			return nil, errors.New("session id cannot be empty")
		}
		out = append(out, "--session", id)
	}
	return out, nil
}

func runSessionList(ctx context.Context, args []string, out io.Writer) error {
	flags := flag.NewFlagSet("session list", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	all := flags.Bool("all", false, "list sessions from all workspaces")
	limit := flags.Int("limit", 50, "maximum sessions to list")
	jsonOutput := flags.Bool("json", false, "emit JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("usage: proton session list [--all] [--limit N] [--json]")
	}
	if *limit < 1 || *limit > 1000 {
		return errors.New("session list --limit must be between 1 and 1000")
	}
	dirs, err := appdirs.Resolve("")
	if err != nil {
		return fmt.Errorf("resolve session store: %w", err)
	}
	store, err := sessionfs.NewFileStore(dirs.Sessions)
	if err != nil {
		return fmt.Errorf("create session store: %w", err)
	}
	options := session.ListOptions{Limit: *limit}
	if !*all {
		workDir, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("resolve current workspace: %w", err)
		}
		options.WorkspaceKey = workspaceKey(workDir)
	}
	summaries, err := store.ListSummaries(ctx, options)
	if err != nil {
		return fmt.Errorf("list sessions: %w", err)
	}
	if *jsonOutput {
		encoder := json.NewEncoder(out)
		encoder.SetIndent("", "  ")
		return encoder.Encode(summaries)
	}
	if len(summaries) == 0 {
		_, err = fmt.Fprintln(out, "No sessions found.")
		return err
	}
	fmt.Fprintln(out, "SESSION ID\tUPDATED\tPROFILE\tPREVIEW")
	for _, summary := range summaries {
		profile := summary.AgentProfile
		if profile == "" {
			profile = "-"
		}
		updated := summary.UpdatedAt.Local().Format("2006-01-02 15:04")
		fmt.Fprintf(out, "%s\t%s\t%s\t%s\n", summary.ID, updated, profile, summary.Preview)
	}
	return nil
}
