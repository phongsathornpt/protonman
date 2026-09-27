package questionbridge

import (
	"context"
	"testing"
	"time"

	questiontool "github.com/phongsathornpt/protonman/internal/adapter/out/tool/question"
)

func TestQuestionBridgePromptAndRespond(t *testing.T) {
	bridge := New()
	defer bridge.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	done := make(chan struct{})
	go func() {
		req := questiontool.Request{
			Question: "Which database?",
			Options:  []string{"PostgreSQL", "SQLite"},
		}
		res, err := bridge.PromptQuestion(ctx, req)
		if err != nil {
			t.Errorf("PromptQuestion error: %v", err)
		}
		if res.Status != questiontool.StatusAnswered {
			t.Errorf("Status = %v, want answered", res.Status)
		}
		if res.Answer != "PostgreSQL" {
			t.Errorf("Answer = %q, want PostgreSQL", res.Answer)
		}
		close(done)
	}()

	msg := bridge.Next()()
	reqMsg, ok := msg.(RequestMsg)
	if !ok {
		t.Fatalf("expected RequestMsg, got %T", msg)
	}
	if reqMsg.Request.Request.Question != "Which database?" {
		t.Fatalf("question = %q, want 'Which database?'", reqMsg.Request.Request.Question)
	}
	reqMsg.Request.Respond(questiontool.Response{
		Status:          questiontool.StatusAnswered,
		Answer:          "PostgreSQL",
		SelectedOptions: []string{"PostgreSQL"},
	})

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for response")
	}
}

func TestQuestionBridgeClose(t *testing.T) {
	bridge := New()
	bridge.Close()

	msg := bridge.Next()()
	if _, ok := msg.(ClosedMsg); !ok {
		t.Fatalf("expected ClosedMsg, got %T", msg)
	}

	_, err := bridge.PromptQuestion(context.Background(), questiontool.Request{Question: "test"})
	if err == nil {
		t.Fatal("expected error from prompt after close")
	}
}

func TestQuestionBridgeContextCanceled(t *testing.T) {
	bridge := New()
	defer bridge.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := bridge.PromptQuestion(ctx, questiontool.Request{Question: "test"})
	if err == nil {
		t.Fatal("expected error from canceled context")
	}
}
