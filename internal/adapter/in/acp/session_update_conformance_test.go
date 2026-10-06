package acp

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
	"github.com/phongsathornpt/protonman/internal/app"
	"github.com/phongsathornpt/protonman/internal/core/permission"
	domain "github.com/phongsathornpt/protonman/pkg/proton-sdk/domain"
)

type usageStreamingRunner struct {
	usage domain.Usage
}

func (r *usageStreamingRunner) Run(ctx context.Context, _ []model.Message, sink app.Sink) (app.Result, error) {
	if err := sink(ctx, app.Event{
		Kind: app.EventTextDelta,
		Text: "Hello world",
	}); err != nil {
		return app.Result{}, err
	}
	if err := sink(ctx, app.Event{
		Kind:  app.EventUsage,
		Usage: r.usage,
	}); err != nil {
		return app.Result{}, err
	}
	return app.Result{
		Message: model.Message{
			Role:    model.RoleAssistant,
			Content: "Hello world",
		},
	}, nil
}

func TestUsageUpdateAndSessionInfoUpdateConformToSchema(t *testing.T) {
	runner := &usageStreamingRunner{
		usage: domain.Usage{
			InputTokens:       150,
			OutputTokens:      50,
			TotalTokens:       200,
			CachedInputTokens: 25,
		},
	}
	server := newTestServerWithRunner(t, permission.ModeAlwaysApprove, runner)
	h := startServeHarness(t, server)

	// Create session
	h.request(1, "session/new", SessionNewParams{Cwd: t.TempDir()})
	newFrame := h.response(1, 2*time.Second)
	var newRes SessionNewResult
	if err := json.Unmarshal(newFrame["result"], &newRes); err != nil {
		t.Fatalf("decode session/new result: %v", err)
	}

	// Send prompt
	h.request(2, "session/prompt", SessionPromptParams{
		SessionID: newRes.SessionID,
		Prompt: []ContentBlock{
			{Type: BlockTypeText, Text: "What is the status?"},
		},
	})

	// Wait for prompt response
	h.response(2, 3*time.Second)

	// Collect notifications sent to harness
	var observedUsage map[string]json.RawMessage
	var observedInfo map[string]json.RawMessage

	for {
		select {
		case frame := <-h.notifications:
			if method, ok := frame["method"]; ok && string(method) == `"session/update"` {
				assertACPSchema(t, "SessionNotification", frame["params"])
				var params struct {
					SessionID string         `json:"sessionId"`
					Update    map[string]any `json:"update"`
				}
				if err := json.Unmarshal(frame["params"], &params); err == nil {
					switch params.Update["sessionUpdate"] {
					case "usage_update":
						observedUsage = frame
					case "session_info_update":
						observedInfo = frame
					}
				}
			}
		case frame := <-h.frames:
			if method, ok := frame["method"]; ok && string(method) == `"session/update"` {
				assertACPSchema(t, "SessionNotification", frame["params"])
				var params struct {
					SessionID string         `json:"sessionId"`
					Update    map[string]any `json:"update"`
				}
				if err := json.Unmarshal(frame["params"], &params); err == nil {
					switch params.Update["sessionUpdate"] {
					case "usage_update":
						observedUsage = frame
					case "session_info_update":
						observedInfo = frame
					}
				}
			}
		case <-time.After(200 * time.Millisecond):
			goto done
		}
	}
done:
	if observedUsage == nil {
		t.Fatal("expected usage_update notification, but none was observed")
	}
	if observedInfo == nil {
		t.Fatal("expected session_info_update notification, but none was observed")
	}

	var usageParams struct {
		SessionID string `json:"sessionId"`
		Update    struct {
			Used uint64 `json:"used"`
			Size uint64 `json:"size"`
		} `json:"update"`
	}
	if err := json.Unmarshal(observedUsage["params"], &usageParams); err != nil {
		t.Fatalf("decode usage update params: %v", err)
	}
	if usageParams.Update.Used != 200 {
		t.Errorf("usage used = %d, want 200", usageParams.Update.Used)
	}
	if usageParams.Update.Size <= 0 {
		t.Errorf("usage size = %d, want > 0", usageParams.Update.Size)
	}

	var infoParams struct {
		SessionID string `json:"sessionId"`
		Update    struct {
			Title     *string `json:"title"`
			UpdatedAt *string `json:"updatedAt"`
		} `json:"update"`
	}
	if err := json.Unmarshal(observedInfo["params"], &infoParams); err != nil {
		t.Fatalf("decode info update params: %v", err)
	}
	if infoParams.Update.Title == nil || *infoParams.Update.Title == "" {
		t.Errorf("expected non-empty title in session_info_update")
	}
	if infoParams.Update.UpdatedAt == nil || *infoParams.Update.UpdatedAt == "" {
		t.Errorf("expected non-empty updatedAt in session_info_update")
	}
}
