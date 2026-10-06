package acp

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	questiontool "github.com/phongsathornpt/protonman/internal/adapter/out/tool/question"
	"github.com/phongsathornpt/protonman/internal/core/permission"
)

func TestElicitationSchemaConformance(t *testing.T) {
	req := buildElicitationRequest("session-123", questiontool.Request{
		Question:    "Which database engine would you prefer?",
		Options:     []string{"PostgreSQL", "SQLite", "MySQL"},
		Multiple:    false,
		Recommended: "SQLite",
	})
	assertACPSchema(t, "CreateElicitationRequest", req)

	// Multi-select with batch questions
	batchReq := buildElicitationRequest("session-123", questiontool.Request{
		Questions: []questiontool.QuestionItem{
			{
				Question: "Select features to enable:",
				Options:  []string{"Auth", "Metrics", "Tracing"},
				Multiple: true,
			},
			{
				Question: "Database name:",
			},
		},
	})
	assertACPSchema(t, "CreateElicitationRequest", batchReq)

	// Validate CreateElicitationResponse schema
	acceptResp := CreateElicitationResponse{
		Action:  "accept",
		Content: map[string]any{"q1": "SQLite"},
	}
	assertACPSchema(t, "CreateElicitationResponse", acceptResp)

	declineResp := CreateElicitationResponse{
		Action: "decline",
	}
	assertACPSchema(t, "CreateElicitationResponse", declineResp)

	cancelResp := CreateElicitationResponse{
		Action: "cancel",
	}
	assertACPSchema(t, "CreateElicitationResponse", cancelResp)
}

func TestElicitationNegotiationAndFallback(t *testing.T) {
	t.Run("client without elicitation capability gets session/request_question", func(t *testing.T) {
		server := newTestServer(t, permission.ModeAsk)
		h := startServeHarness(t, server)

		// Initialize without elicitation
		h.request(1, "initialize", map[string]any{
			"protocolVersion":    ProtocolVersion,
			"clientCapabilities": map[string]any{},
		})
		initResp := h.response(1, 2*time.Second)
		if initResp["error"] != nil {
			t.Fatalf("initialize failed: %s", initResp["error"])
		}

		// Prompt question
		done := make(chan questiontool.Response, 1)
		go func() {
			resp, err := server.questions.prompter("session-1").PromptQuestion(context.Background(), questiontool.Request{
				Question: "Do you want to proceed?",
				Options:  []string{"yes", "no"},
			})
			if err != nil {
				t.Errorf("PromptQuestion error = %v", err)
			}
			done <- resp
		}()

		// Server should emit session/request_question
		frame := h.nextFrame(2 * time.Second)
		var method string
		_ = json.Unmarshal(frame["method"], &method)
		if method != requestQuestionMethod {
			t.Fatalf("expected %s, got %s", requestQuestionMethod, method)
		}
		idVal := frame["id"]

		// Client responds with legacy format
		h.respond(idVal, map[string]any{
			"status": "answered",
			"answer": "yes",
		})

		select {
		case resp := <-done:
			if resp.Status != questiontool.StatusAnswered || resp.Answer != "yes" {
				t.Fatalf("unexpected response: %+v", resp)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("timed out waiting for question response")
		}
	})

	t.Run("client with elicitation.form gets elicitation/create and handles accept", func(t *testing.T) {
		server := newTestServer(t, permission.ModeAsk)
		h := startServeHarness(t, server)

		// Initialize with elicitation.form: {}
		h.request(1, "initialize", map[string]any{
			"protocolVersion": ProtocolVersion,
			"clientCapabilities": map[string]any{
				"elicitation": map[string]any{
					"form": map[string]any{},
				},
			},
		})
		initResp := h.response(1, 2*time.Second)
		if initResp["error"] != nil {
			t.Fatalf("initialize failed: %s", initResp["error"])
		}

		// Prompt question
		done := make(chan questiontool.Response, 1)
		go func() {
			resp, err := server.questions.prompter("session-2").PromptQuestion(context.Background(), questiontool.Request{
				Question:    "Pick database:",
				Options:     []string{"PostgreSQL", "SQLite"},
				Recommended: "SQLite",
			})
			if err != nil {
				t.Errorf("PromptQuestion error = %v", err)
			}
			done <- resp
		}()

		// Server should emit elicitation/create
		frame := h.nextFrame(2 * time.Second)
		var method string
		_ = json.Unmarshal(frame["method"], &method)
		if method != methodElicitationCreate {
			t.Fatalf("expected %s, got %s", methodElicitationCreate, method)
		}

		// Validate wire frame against pinned schema
		var wireReq CreateElicitationRequest
		if err := json.Unmarshal(frame["params"], &wireReq); err != nil {
			t.Fatalf("unmarshal CreateElicitationRequest: %v", err)
		}
		assertACPSchema(t, "CreateElicitationRequest", wireReq)

		idVal := frame["id"]

		// Client responds with action: accept
		h.respond(idVal, map[string]any{
			"action": "accept",
			"content": map[string]any{
				"q1": "SQLite",
			},
		})

		select {
		case resp := <-done:
			if resp.Status != questiontool.StatusAnswered {
				t.Fatalf("expected status answered, got %s", resp.Status)
			}
			if resp.Answer != "SQLite" {
				t.Fatalf("expected answer SQLite, got %s", resp.Answer)
			}
			if len(resp.SelectedOptions) != 1 || resp.SelectedOptions[0] != "SQLite" {
				t.Fatalf("expected selected options [SQLite], got %v", resp.SelectedOptions)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("timed out waiting for elicitation response")
		}
	})

	t.Run("client with elicitation handles decline", func(t *testing.T) {
		server := newTestServer(t, permission.ModeAsk)
		h := startServeHarness(t, server)

		h.request(1, "initialize", map[string]any{
			"protocolVersion": ProtocolVersion,
			"clientCapabilities": map[string]any{
				"elicitation": map[string]any{
					"form": map[string]any{},
				},
			},
		})
		_ = h.response(1, 2*time.Second)

		done := make(chan questiontool.Response, 1)
		go func() {
			resp, _ := server.questions.prompter("session-3").PromptQuestion(context.Background(), questiontool.Request{
				Question: "Confirm deployment?",
			})
			done <- resp
		}()

		frame := h.nextFrame(2 * time.Second)
		idVal := frame["id"]
		h.respond(idVal, map[string]any{
			"action": "decline",
		})

		select {
		case resp := <-done:
			if resp.Status != questiontool.StatusDeclined {
				t.Fatalf("expected status declined, got %s", resp.Status)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("timed out waiting for elicitation decline")
		}
	})

	t.Run("client with elicitation handles cancel", func(t *testing.T) {
		server := newTestServer(t, permission.ModeAsk)
		h := startServeHarness(t, server)

		h.request(1, "initialize", map[string]any{
			"protocolVersion": ProtocolVersion,
			"clientCapabilities": map[string]any{
				"elicitation": map[string]any{
					"form": map[string]any{},
				},
			},
		})
		_ = h.response(1, 2*time.Second)

		done := make(chan questiontool.Response, 1)
		go func() {
			resp, _ := server.questions.prompter("session-4").PromptQuestion(context.Background(), questiontool.Request{
				Question: "Confirm reset?",
			})
			done <- resp
		}()

		frame := h.nextFrame(2 * time.Second)
		idVal := frame["id"]
		h.respond(idVal, map[string]any{
			"action": "cancel",
		})

		select {
		case resp := <-done:
			if resp.Status != questiontool.StatusDeclined {
				t.Fatalf("expected status declined, got %s", resp.Status)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("timed out waiting for elicitation cancel")
		}
	})
}
