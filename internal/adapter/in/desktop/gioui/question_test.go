//go:build desktop || desktop_gio

package gioui

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/phongsathornpt/protonman/internal/adapter/out/acpclient"
	desktopstate "github.com/phongsathornpt/protonman/internal/feature/desktop"
)

func TestQuestionRequestRoundTrip(t *testing.T) {
	controller := newTestController()
	controller.state.Sessions = []desktopstate.SessionState{{ID: "session-1", Status: desktopstate.TaskRunning}}
	controller.state.ActiveSessionID = "session-1"

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	params := questionParams{
		SessionID: "session-1",
		Question:  "Which framework should we use?",
		Options:   []string{"Gio", "Bubble Tea"},
	}
	encodedParams, _ := json.Marshal(params)
	request := acpclient.Request{
		ID:     json.RawMessage(`"question-1"`),
		Method: requestQuestionMethod,
		Params: encodedParams,
	}

	result := make(chan any, 1)
	errs := make(chan error, 1)
	go func() {
		value, err := controller.handleQuestionRequestFromAgent("", nil, ctx, request)
		result <- value
		errs <- err
	}()

	deadline := time.Now().Add(time.Second)
	for {
		controller.mu.RLock()
		ready := len(controller.state.QuestionInbox) == 1
		controller.mu.RUnlock()
		if ready {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("question request was not projected into inbox")
		}
		time.Sleep(time.Millisecond)
	}

	controller.mu.RLock()
	if controller.state.Sessions[0].Status != desktopstate.TaskWaitingUser {
		t.Fatalf("session status = %q, want waiting_user", controller.state.Sessions[0].Status)
	}
	if controller.state.QuestionInbox[0].Questions[0].Question != "Which framework should we use?" {
		t.Fatalf("question text = %q", controller.state.QuestionInbox[0].Questions[0].Question)
	}
	controller.mu.RUnlock()

	controller.resolveQuestion(string(request.ID), desktopstate.QuestionResponse{
		Status:          "answered",
		Answer:          "Gio",
		SelectedOptions: []string{"Gio"},
	})

	select {
	case err := <-errs:
		if err != nil {
			t.Fatalf("question handler error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("question handler did not return")
	}

	value := <-result
	resp, ok := value.(desktopstate.QuestionResponse)
	if !ok || resp.Status != "answered" || resp.Answer != "Gio" {
		t.Fatalf("unexpected question response: %#v", value)
	}

	if len(controller.state.QuestionInbox) != 0 {
		t.Fatalf("question inbox was not cleared: %#v", controller.state.QuestionInbox)
	}
	if got := controller.state.Sessions[0].Status; got != desktopstate.TaskRunning {
		t.Fatalf("session status = %q, want running", got)
	}
}

func TestQuestionRequestCancellation(t *testing.T) {
	controller := newTestController()
	controller.state.Sessions = []desktopstate.SessionState{{ID: "session-2", Status: desktopstate.TaskRunning}}
	controller.state.ActiveSessionID = "session-2"

	ctx, cancel := context.WithCancel(context.Background())

	params := questionParams{
		SessionID: "session-2",
		Question:  "Should we proceed?",
		Options:   []string{"Yes", "No"},
	}
	encodedParams, _ := json.Marshal(params)
	request := acpclient.Request{
		ID:     json.RawMessage(`"question-2"`),
		Method: requestQuestionMethod,
		Params: encodedParams,
	}

	result := make(chan any, 1)
	errs := make(chan error, 1)
	go func() {
		value, err := controller.handleQuestionRequestFromAgent("", nil, ctx, request)
		result <- value
		errs <- err
	}()

	deadline := time.Now().Add(time.Second)
	for {
		controller.mu.RLock()
		ready := len(controller.state.QuestionInbox) == 1
		controller.mu.RUnlock()
		if ready {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("question request was not projected into inbox")
		}
		time.Sleep(time.Millisecond)
	}

	// Cancel context
	cancel()

	select {
	case err := <-errs:
		if err != nil {
			t.Fatalf("question handler returned error on cancel: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("question handler did not unblock on context cancellation")
	}

	value := <-result
	resp, ok := value.(desktopstate.QuestionResponse)
	if !ok || resp.Status != "declined" {
		t.Fatalf("expected declined status on cancellation, got: %#v", value)
	}
}
