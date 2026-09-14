package acp

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/phongsathornpt/protonman/internal/core/permission"
	"github.com/phongsathornpt/protonman/internal/core/tool"
)

func TestNormalizeSessionDirectories(t *testing.T) {
	cwd, roots, err := normalizeSessionDirectories("/workspace/../workspace", []string{
		"/shared/lib",
		"/shared/lib",
		"/workspace",
		"/shared/docs",
	})
	if err != nil {
		t.Fatal(err)
	}
	if cwd != "/workspace" {
		t.Fatalf("cwd = %q, want /workspace", cwd)
	}
	if got, want := strings.Join(roots, ","), "/shared/lib,/shared/docs"; got != want {
		t.Fatalf("roots = %q, want %q", got, want)
	}
}

func TestNormalizeSessionDirectoriesRejectsRelativeRoots(t *testing.T) {
	if _, _, err := normalizeSessionDirectories("/workspace", []string{"relative"}); err == nil {
		t.Fatal("relative additional directory accepted")
	}
	if _, _, err := normalizeSessionDirectories("relative", nil); err == nil {
		t.Fatal("relative cwd accepted")
	}
	if _, _, err := normalizeSessionDirectories("", []string{"/shared"}); err == nil {
		t.Fatal("additional directories accepted without cwd")
	}
}

func TestACPSessionNewForwardsWorkspaceRootsToRegistryFactory(t *testing.T) {
	server := newTestServer(t, permission.ModeAsk)
	var gotSessionID, gotCwd string
	var gotRoots []string
	WithSessionRegistryFactory(func(sessionID, cwd string, roots []string) (tool.Registry, error) {
		gotSessionID = sessionID
		gotCwd = cwd
		gotRoots = cloneDirectories(roots)
		return server.registry, nil
	})(server)

	result, _, err := server.dispatch(context.Background(), RPCRequest{
		Method: "session/new",
		Params: json.RawMessage(`{"cwd":"/workspace","additionalDirectories":["/shared/lib","/shared/docs"]}`),
	}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("session/new error = %v", err)
	}
	created := result.(SessionNewResult)
	if gotSessionID != created.SessionID {
		t.Fatalf("factory session id = %q, want %q", gotSessionID, created.SessionID)
	}
	if gotCwd != "/workspace" {
		t.Fatalf("factory cwd = %q, want /workspace", gotCwd)
	}
	if got := strings.Join(gotRoots, ","); got != "/shared/lib,/shared/docs" {
		t.Fatalf("factory roots = %q", got)
	}
}

func TestACPSessionListReportsActiveAdditionalDirectories(t *testing.T) {
	server := newTestServer(t, permission.ModeAsk)
	WithSessionRegistryFactory(func(_ string, _ string, _ []string) (tool.Registry, error) {
		return server.registry, nil
	})(server)
	created, _, err := server.dispatch(context.Background(), RPCRequest{
		Method: "session/new",
		Params: json.RawMessage(`{"cwd":"/workspace","additionalDirectories":["/shared/lib"]}`),
	}, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	id := created.(SessionNewResult).SessionID

	listed, _, err := server.dispatch(context.Background(), RPCRequest{Method: "session/list"}, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	for _, info := range listed.(SessionListResult).Sessions {
		if info.SessionID == id {
			if len(info.AdditionalDirectories) != 1 || info.AdditionalDirectories[0] != "/shared/lib" {
				t.Fatalf("additionalDirectories = %#v", info.AdditionalDirectories)
			}
			return
		}
	}
	t.Fatalf("session %q missing from session/list", id)
}

func TestACPResumeCanReplaceInactiveRootSet(t *testing.T) {
	server := newTestServer(t, permission.ModeAsk)
	calls := make([][]string, 0, 2)
	WithSessionRegistryFactory(func(_ string, _ string, roots []string) (tool.Registry, error) {
		calls = append(calls, cloneDirectories(roots))
		return server.registry, nil
	})(server)
	created, _, err := server.dispatch(context.Background(), RPCRequest{
		Method: "session/new",
		Params: json.RawMessage(`{"cwd":"/workspace","additionalDirectories":["/shared/one"]}`),
	}, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	id := created.(SessionNewResult).SessionID

	_, _, err = server.dispatch(context.Background(), RPCRequest{
		Method: "session/resume",
		Params: json.RawMessage(`{"sessionId":"` + id + `","cwd":"/workspace","additionalDirectories":["/shared/two"]}`),
	}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("session/resume error = %v", err)
	}
	if len(calls) != 2 || len(calls[1]) != 1 || calls[1][0] != "/shared/two" {
		t.Fatalf("factory calls = %#v", calls)
	}
}
