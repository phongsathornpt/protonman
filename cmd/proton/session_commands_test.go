package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/projectTHORN/proton/internal/adapter/out/sessionfs"
	"github.com/projectTHORN/proton/internal/app/appdirs"
	"github.com/projectTHORN/proton/internal/core/permission"
	"github.com/projectTHORN/proton/internal/core/session"
)

func TestSessionResumeArgs(t *testing.T) {
	got, err := sessionResumeArgs(nil)
	if err != nil || len(got) != 1 || got[0] != "--resume" {
		t.Fatalf("latest args = %v, err %v", got, err)
	}
	got, err = sessionResumeArgs([]string{"abc"})
	if err != nil || strings.Join(got, " ") != "--resume --session abc" {
		t.Fatalf("explicit args = %v, err %v", got, err)
	}
	if _, err := sessionResumeArgs([]string{"a", "b"}); err == nil {
		t.Fatal("multiple ids unexpectedly accepted")
	}
}

func TestRunSessionListFiltersCurrentWorkspaceAndSupportsJSON(t *testing.T) {
	home := t.TempDir()
	work := t.TempDir()
	oldHome := os.Getenv("PROTON_HOME")
	oldWD, _ := os.Getwd()
	defer func() { _ = os.Setenv("PROTON_HOME", oldHome); _ = os.Chdir(oldWD) }()
	_ = os.Setenv("PROTON_HOME", home)
	_ = os.Chdir(work)
	dirs, err := appdirs.Resolve("")
	if err != nil {
		t.Fatal(err)
	}
	store, err := sessionfs.NewFileStore(dirs.Sessions)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	key := workspaceKey(work)
	if err := store.Save(ctx, "current", session.State{PermissionMode: permission.ModeAsk.String(), WorkspaceKey: key, AgentProfile: "dex", Messages: []session.Message{{Role: "user", Content: "current task"}}}); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(ctx, "other", session.State{PermissionMode: permission.ModeAsk.String(), WorkspaceKey: "other", Messages: []session.Message{{Role: "user", Content: "other task"}}}); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := runSessionList(ctx, []string{"--json"}, &out); err != nil {
		t.Fatal(err)
	}
	var got []session.Summary
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "current" {
		t.Fatalf("summaries = %+v", got)
	}
	out.Reset()
	if err := runSessionList(ctx, []string{"--all", "--json"}, &out); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("all summaries = %+v", got)
	}
}
