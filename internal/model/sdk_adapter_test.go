package model

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	sdk "github.com/projectTHORN/proton/proton-sdk"
)

func TestOpenCodeSDKAdapterAlwaysSendsClientIdentity(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("x-opencode-client"); got != "proton-test" {
			t.Fatalf("x-opencode-client = %q", got)
		}
		if got := r.Header.Get("x-opencode-session"); got != "" {
			t.Fatalf("x-opencode-session = %q, want empty", got)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n")
	}))
	defer server.Close()
	model := newSDKOpenAILanguageModel(DefaultOpenCodeName, server.URL, "", "test", WithClientName("proton-test"))
	if model.Provider() != DefaultOpenCodeName {
		t.Fatalf("provider = %q", model.Provider())
	}
	stream, err := model.Stream(context.Background(), sdk.Request{Messages: []sdk.Message{{Role: sdk.RoleUser, Content: "hi"}}})
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
}

func TestOpenCodeSDKAdapterSendsSessionIdentityWhenPresent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("x-opencode-client"); got != "proton" {
			t.Fatalf("x-opencode-client = %q", got)
		}
		if got := r.Header.Get("x-opencode-session"); got != "session-1" {
			t.Fatalf("x-opencode-session = %q", got)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n")
	}))
	defer server.Close()
	model := newSDKOpenAILanguageModel(DefaultOpenCodeName, server.URL, "", "test", WithSessionID("session-1"))
	stream, err := model.Stream(context.Background(), sdk.Request{Messages: []sdk.Message{{Role: sdk.RoleUser, Content: "hi"}}})
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
}
