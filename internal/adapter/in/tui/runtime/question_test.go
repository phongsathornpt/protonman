package runtime

import (
	"context"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/runtime/questionbridge"
	turnmsg "github.com/phongsathornpt/protonman/internal/adapter/in/tui/runtime/turn"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/state/runtimeui"
	questiontool "github.com/phongsathornpt/protonman/internal/adapter/out/tool/question"
	"github.com/phongsathornpt/protonman/internal/core/permission"
)

func newTestQuestionModel(t *testing.T) *bubbleModel {
	t.Helper()
	registry, _ := newBubbleTestRegistry()
	service := newBubbleTestService(t, registry, permission.ModeAsk, permission.Config{})
	m := newBubbleModel(context.Background(), service, registry, emptyTodoItems(), nil, newPermissionBridge(), "")
	m.questionBridge = questionbridge.New()
	attachTestApplication(t, m)
	return m
}

func TestBubbleModelQuestionModalOptionSelect(t *testing.T) {
	m := newTestQuestionModel(t)
	defer m.questionBridge.Close()

	responseCh := make(chan questionbridge.Response, 1)
	req := questionbridge.Request{
		Request: questiontool.Request{
			Question: "Which database should we use?",
			Options:  []string{"PostgreSQL", "SQLite", "MySQL"},
			Multiple: false,
		},
		Response: responseCh,
	}

	m.openQuestion(req)
	if m.questionView() == nil {
		t.Fatal("question view not open after openQuestion")
	}

	// Press Down to select second option (SQLite)
	updated, _ := m.Update(testKey(tea.KeyDown))
	m = updated.(*bubbleModel)

	// Press Enter to confirm
	updated, _ = m.Update(testKey(tea.KeyEnter))
	m = updated.(*bubbleModel)

	if m.questionView() != nil {
		t.Fatal("question view remains open after Enter")
	}

	select {
	case res := <-responseCh:
		if res.Response.Status != questiontool.StatusAnswered {
			t.Fatalf("status = %v, want answered", res.Response.Status)
		}
		if res.Response.Answer != "SQLite" {
			t.Fatalf("answer = %q, want SQLite", res.Response.Answer)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for response")
	}
}

func TestBubbleModelQuestionModalNumberShortcut(t *testing.T) {
	m := newTestQuestionModel(t)
	defer m.questionBridge.Close()

	responseCh := make(chan questionbridge.Response, 1)
	req := questionbridge.Request{
		Request: questiontool.Request{
			Question: "Select backend",
			Options:  []string{"Go", "Rust", "TypeScript"},
			Multiple: false,
		},
		Response: responseCh,
	}

	m.openQuestion(req)

	// Press '2' to pick Rust
	updated, _ := m.Update(testText("2"))
	m = updated.(*bubbleModel)

	if m.questionView() != nil {
		t.Fatal("question view remains open after number shortcut")
	}

	select {
	case res := <-responseCh:
		if res.Response.Status != questiontool.StatusAnswered {
			t.Fatalf("status = %v, want answered", res.Response.Status)
		}
		if res.Response.Answer != "Rust" {
			t.Fatalf("answer = %q, want Rust", res.Response.Answer)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for response")
	}
}

func TestBubbleModelQuestionModalMultiSelect(t *testing.T) {
	m := newTestQuestionModel(t)
	defer m.questionBridge.Close()

	responseCh := make(chan questionbridge.Response, 1)
	req := questionbridge.Request{
		Request: questiontool.Request{
			Question: "Select packages to install",
			Options:  []string{"A", "B", "C"},
			Multiple: true,
		},
		Response: responseCh,
	}

	m.openQuestion(req)

	// Press Space on item 0 (A)
	updated, _ := m.Update(testText(" "))
	m = updated.(*bubbleModel)

	// Press Down to item 1
	updated, _ = m.Update(testKey(tea.KeyDown))
	m = updated.(*bubbleModel)

	// Press Down to item 2 (C)
	updated, _ = m.Update(testKey(tea.KeyDown))
	m = updated.(*bubbleModel)

	// Press Space on item 2 (C)
	updated, _ = m.Update(testText(" "))
	m = updated.(*bubbleModel)

	// Press Enter to confirm
	updated, _ = m.Update(testKey(tea.KeyEnter))
	m = updated.(*bubbleModel)

	if m.questionView() != nil {
		t.Fatal("question view remains open after Enter")
	}

	select {
	case res := <-responseCh:
		if res.Response.Status != questiontool.StatusAnswered {
			t.Fatalf("status = %v, want answered", res.Response.Status)
		}
		if len(res.Response.SelectedOptions) != 2 {
			t.Fatalf("got %d selected options, want 2: %+v", len(res.Response.SelectedOptions), res.Response.SelectedOptions)
		}
		if res.Response.SelectedOptions[0] != "A" || res.Response.SelectedOptions[1] != "C" {
			t.Fatalf("unexpected selections: %+v", res.Response.SelectedOptions)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for response")
	}
}

func TestBubbleModelQuestionModalDecline(t *testing.T) {
	m := newTestQuestionModel(t)
	defer m.questionBridge.Close()

	responseCh := make(chan questionbridge.Response, 1)
	req := questionbridge.Request{
		Request: questiontool.Request{
			Question: "May I delete this file?",
			Options:  []string{"Yes", "No"},
		},
		Response: responseCh,
	}

	m.openQuestion(req)

	// Press Esc to decline
	updated, _ := m.Update(testKey(tea.KeyEsc))
	m = updated.(*bubbleModel)

	if m.questionView() != nil {
		t.Fatal("question view remains open after Escape")
	}

	select {
	case res := <-responseCh:
		if res.Response.Status != questiontool.StatusDeclined {
			t.Fatalf("status = %v, want declined", res.Response.Status)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for response")
	}
}

func TestBubbleModelQuestionModalWriteInCustom(t *testing.T) {
	m := newTestQuestionModel(t)
	defer m.questionBridge.Close()

	responseCh := make(chan questionbridge.Response, 1)
	req := questionbridge.Request{
		Request: questiontool.Request{
			Question: "Which port?",
			Options:  []string{"8080", "3000"},
		},
		Response: responseCh,
	}

	m.openQuestion(req)

	// Press 'w' to enter write-in mode
	updated, _ := m.Update(testText("w"))
	m = updated.(*bubbleModel)

	// Type "9000"
	for _, ch := range "9000" {
		updated, _ = m.Update(testText(string(ch)))
		m = updated.(*bubbleModel)
	}

	// Press Enter
	updated, _ = m.Update(testKey(tea.KeyEnter))
	m = updated.(*bubbleModel)

	if m.questionView() != nil {
		t.Fatal("question view remains open after submitting custom text")
	}

	select {
	case res := <-responseCh:
		if res.Response.Status != questiontool.StatusAnswered {
			t.Fatalf("status = %v, want answered", res.Response.Status)
		}
		if res.Response.Answer != "9000" {
			t.Fatalf("answer = %q, want 9000", res.Response.Answer)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for response")
	}
}

func TestBubbleModelQuestionModalDirectTextInputWhenNoOptions(t *testing.T) {
	m := newTestQuestionModel(t)
	defer m.questionBridge.Close()

	responseCh := make(chan questionbridge.Response, 1)
	req := questionbridge.Request{
		Request: questiontool.Request{
			Question: "Please specify target environment:",
			Options:  nil,
		},
		Response: responseCh,
	}

	m.openQuestion(req)

	// Directly type "production"
	for _, ch := range "production" {
		updated, _ := m.Update(testText(string(ch)))
		m = updated.(*bubbleModel)
	}

	// Press Enter
	updated, _ := m.Update(testKey(tea.KeyEnter))
	m = updated.(*bubbleModel)

	if m.questionView() != nil {
		t.Fatal("question view remains open after submitting text")
	}

	select {
	case res := <-responseCh:
		if res.Response.Status != questiontool.StatusAnswered {
			t.Fatalf("status = %v, want answered", res.Response.Status)
		}
		if res.Response.Answer != "production" {
			t.Fatalf("answer = %q, want production", res.Response.Answer)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for response")
	}
}

func TestBubbleModelQuestionModalWriteInWithSpace(t *testing.T) {
	m := newTestQuestionModel(t)
	defer m.questionBridge.Close()

	responseCh := make(chan questionbridge.Response, 1)
	req := questionbridge.Request{
		Request: questiontool.Request{
			Question: "Full name?",
			Options:  nil,
		},
		Response: responseCh,
	}

	m.openQuestion(req)

	for _, ch := range "John Doe" {
		updated, _ := m.Update(tea.KeyPressMsg{Code: ch, Text: string(ch)})
		m = updated.(*bubbleModel)
	}

	updated, _ := m.Update(testKey(tea.KeyEnter))
	m = updated.(*bubbleModel)

	if m.questionView() != nil {
		t.Fatal("question view remains open after submitting text")
	}

	select {
	case res := <-responseCh:
		if res.Response.Answer != "John Doe" {
			t.Fatalf("answer = %q, want 'John Doe'", res.Response.Answer)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for response")
	}
}

func TestBubbleModelQuestionModalDanglingOnTurnDone(t *testing.T) {
	m := newTestQuestionModel(t)
	defer m.questionBridge.Close()

	responseCh := make(chan questionbridge.Response, 1)
	req := questionbridge.Request{
		Request: questiontool.Request{
			Question: "May I proceed?",
		},
		Response: responseCh,
	}

	m.openQuestion(req)
	if m.questionView() == nil {
		t.Fatal("question view not open")
	}

	// Simulate turn ending with an error (e.g. timeout / canceled)
	m.updateTurnDone(turnmsg.Done{Err: context.Canceled})

	if m.questionView() != nil {
		t.Fatal("question view remained open after turn completed with error")
	}
	if m.activity != runtimeui.ActivityReady {
		t.Fatalf("activity = %q, want %q", m.activity, runtimeui.ActivityReady)
	}
}

func TestBubbleModelQuestionModalMultiQuestionWizard(t *testing.T) {
	m := newTestQuestionModel(t)
	defer m.questionBridge.Close()

	responseCh := make(chan questionbridge.Response, 1)
	req := questionbridge.Request{
		Request: questiontool.Request{
			Questions: []questiontool.QuestionItem{
				{
					Question:    "Database choice?",
					Options:     []string{"PostgreSQL", "SQLite"},
					Recommended: "PostgreSQL",
				},
				{
					Question: "Cache layer?",
					Options:  []string{"Redis", "Memory"},
				},
			},
		},
		Response: responseCh,
	}

	m.openQuestion(req)
	view := m.questionView()
	if view == nil {
		t.Fatal("question view not open")
	}
	if view.currentQuestion != 0 {
		t.Fatalf("currentQuestion = %d, want 0", view.currentQuestion)
	}
	if view.index != 0 {
		t.Fatalf("expected recommended index = 0, got %d", view.index)
	}

	// Question 1: Press '2' (pick SQLite)
	updated, _ := m.Update(testText("2"))
	m = updated.(*bubbleModel)

	// Now should be on Question 2 (currentQuestion = 1)
	view = m.questionView()
	if view == nil {
		t.Fatal("question view closed prematurely between questions")
	}
	if view.currentQuestion != 1 {
		t.Fatalf("currentQuestion = %d, want 1", view.currentQuestion)
	}

	// Question 2: Press '1' (pick Redis)
	updated, _ = m.Update(testText("1"))
	m = updated.(*bubbleModel)

	// Now modal should be closed and answered
	if m.questionView() != nil {
		t.Fatal("question view still open after completing all questions")
	}

	select {
	case res := <-responseCh:
		if res.Response.Status != questiontool.StatusAnswered {
			t.Fatalf("status = %v, want answered", res.Response.Status)
		}
		if len(res.Response.Answers) != 2 {
			t.Fatalf("expected 2 answers, got %d", len(res.Response.Answers))
		}
		if res.Response.Answers[0].Answer != "SQLite" {
			t.Fatalf("answer 0 = %q, want SQLite", res.Response.Answers[0].Answer)
		}
		if res.Response.Answers[1].Answer != "Redis" {
			t.Fatalf("answer 1 = %q, want Redis", res.Response.Answers[1].Answer)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for response")
	}
}

func TestBubbleModelQuestionModalBacktrack(t *testing.T) {
	m := newTestQuestionModel(t)
	defer m.questionBridge.Close()

	responseCh := make(chan questionbridge.Response, 1)
	req := questionbridge.Request{
		Request: questiontool.Request{
			Questions: []questiontool.QuestionItem{
				{
					Question: "Q1",
					Options:  []string{"A", "B"},
				},
				{
					Question: "Q2",
					Options:  []string{"C", "D"},
				},
			},
		},
		Response: responseCh,
	}

	m.openQuestion(req)

	// Answer Q1 with '1' (A)
	updated, _ := m.Update(testText("1"))
	m = updated.(*bubbleModel)

	view := m.questionView()
	if view.currentQuestion != 1 {
		t.Fatalf("currentQuestion = %d, want 1", view.currentQuestion)
	}

	// Press Esc to backtrack to Q1
	updated, _ = m.Update(testKey(tea.KeyEsc))
	m = updated.(*bubbleModel)

	view = m.questionView()
	if view == nil {
		t.Fatal("question view unexpectedly closed on backtrack")
	}
	if view.currentQuestion != 0 {
		t.Fatalf("currentQuestion after backtrack = %d, want 0", view.currentQuestion)
	}

	// Now press Esc on Q0 to decline
	updated, _ = m.Update(testKey(tea.KeyEsc))
	m = updated.(*bubbleModel)

	if m.questionView() != nil {
		t.Fatal("question view remained open after declining at Q0")
	}

	select {
	case res := <-responseCh:
		if res.Response.Status != questiontool.StatusDeclined {
			t.Fatalf("status = %v, want declined", res.Response.Status)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for response")
	}
}
