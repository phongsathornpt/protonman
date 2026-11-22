//go:build desktop || desktop_gio

package controller

import (
	"strings"
	"testing"
	"time"
)

// Session context, memory, runtime, and skills projections stay owned by the controller: an agent-scoped refresh must never leak another agent's sessions.
func TestSessionRefreshTrackerThrottlesIndependently(t *testing.T) {
	tracker := newSessionRefreshTracker(750 * time.Millisecond)
	now := time.Unix(10, 0)
	if !tracker.begin("one", false, now) {
		t.Fatal("first refresh was rejected")
	}
	if tracker.begin("one", false, now.Add(time.Second)) {
		t.Fatal("in-flight refresh was not rejected")
	}
	tracker.finish("one", now)
	if tracker.begin("one", false, now.Add(500*time.Millisecond)) {
		t.Fatal("refresh was not throttled")
	}
	if !tracker.begin("one", false, now.Add(750*time.Millisecond)) {
		t.Fatal("refresh was not admitted after the interval")
	}
	if !tracker.begin("two", false, now) {
		t.Fatal("second session was incorrectly throttled")
	}
	tracker.reset()
	if !tracker.begin("one", false, now) {
		t.Fatal("reset did not admit a new refresh")
	}
}

func TestProjectSessionInspectorPayloads(t *testing.T) {
	contextResult := sessionContextResult{SessionID: " session-1 ", Goal: " ship "}
	contextResult.Todo.Revision = 4
	contextResult.Todo.Items = append(contextResult.Todo.Items, struct {
		ID     string `json:"id"`
		Text   string `json:"text"`
		Status string `json:"status"`
	}{ID: " item-1 ", Text: " verify ", Status: " in_progress "})
	contextState := projectSessionContext(contextResult)
	if contextState.Goal != "ship" || contextState.Todo.Revision != 4 || len(contextState.Todo.Items) != 1 {
		t.Fatalf("context projection = %+v", contextState)
	}
	if contextState.Todo.Items[0].ID != "item-1" || contextState.Todo.Items[0].Text != "verify" || contextState.Todo.Items[0].Status != "in_progress" {
		t.Fatalf("context item projection = %+v", contextState.Todo.Items[0])
	}

	memoryResult := sessionMemoryResult{SessionID: "session-1", WorkspaceKey: " workspace-1 "}
	memoryResult.Workspace = append(memoryResult.Workspace, struct {
		ID         string  `json:"id"`
		Scope      string  `json:"scope"`
		Kind       string  `json:"kind"`
		Key        string  `json:"key"`
		Value      string  `json:"value"`
		Confidence float64 `json:"confidence"`
		UsageCount uint64  `json:"usageCount"`
	}{ID: " m1 ", Scope: " workspace ", Kind: " repo_fact ", Key: " test ", Value: " go test ", Confidence: .9, UsageCount: 3})
	memory := projectSessionMemory(memoryResult)
	if memory.WorkspaceKey != "workspace-1" || len(memory.Workspace) != 1 {
		t.Fatalf("memory projection = %+v", memory)
	}
	if memory.Workspace[0].ID != "m1" || memory.Workspace[0].Value != "go test" || memory.Workspace[0].UsageCount != 3 {
		t.Fatalf("memory entry projection = %+v", memory.Workspace[0])
	}

	runtime := projectSessionRuntime(sessionRuntimeResult{
		SessionID: "session-1", Provider: " openai ", Model: " gpt ", Reasoning: " high ", LowConcurrency: " on ",
	})
	if runtime.Provider != "openai" || runtime.Model != "gpt" || runtime.Reasoning != "high" || runtime.LowConcurrency != "on" {
		t.Fatalf("runtime projection = %+v", runtime)
	}

	skillsResult := sessionSkillsResult{SessionID: "session-1"}
	skillsResult.Skills = append(skillsResult.Skills, struct {
		Name        string   `json:"name"`
		Description string   `json:"description"`
		Scope       string   `json:"scope"`
		Active      bool     `json:"active"`
		Locked      bool     `json:"locked"`
		LockStatus  string   `json:"lockStatus"`
		Resources   []string `json:"resources"`
	}{Name: " pdf ", Description: " parse pdfs ", Scope: " project ", Active: true, Locked: true, LockStatus: " verified ", Resources: []string{"doc.pdf"}})
	skills := projectSessionSkills(skillsResult)
	if len(skills) != 1 || skills[0].Name != "pdf" || !skills[0].Active || !skills[0].Locked || skills[0].LockStatus != "verified" || len(skills[0].Resources) != 1 {
		t.Fatalf("skills projection = %+v", skills)
	}
}

// MCP payload assembly and configuration normalization are controller-owned: a component must never build a session payload itself.

// Permission detail parsing bounds the text handed to the controller and the UI.
func TestParsePermissionDetails(t *testing.T) {
	tests := []struct {
		name         string
		toolCall     map[string]any
		wantTool     string
		wantCmd      string
		wantRisk     string
		wantJSONPart string
	}{
		{
			name: "bash command with destructive indicator",
			toolCall: map[string]any{
				"name": "bash",
				"rawInput": map[string]any{
					"command": "rm -rf ./build",
				},
			},
			wantTool:     "bash",
			wantCmd:      "rm -rf ./build",
			wantRisk:     "destructive",
			wantJSONPart: "rm -rf",
		},
		{
			name: "edit file path with arguments",
			toolCall: map[string]any{
				"kind": "edit",
				"arguments": map[string]any{
					"filePath": "/workspace/main.go",
				},
			},
			wantTool:     "edit",
			wantCmd:      "/workspace/main.go",
			wantRisk:     "",
			wantJSONPart: "main.go",
		},
		{
			name: "explicit elevated risk from runtime",
			toolCall: map[string]any{
				"name": "git",
				"risk": "elevated",
				"rawInput": map[string]any{
					"command": "git push origin main",
				},
			},
			wantTool:     "git",
			wantCmd:      "git push origin main",
			wantRisk:     "elevated",
			wantJSONPart: "origin main",
		},
		{
			name:     "empty toolCall",
			toolCall: nil,
			wantTool: "",
			wantCmd:  "",
			wantRisk: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			toolName, cmd, risk, rawJSON := parsePermissionDetails(tc.toolCall)
			if toolName != tc.wantTool {
				t.Errorf("toolName = %q, want %q", toolName, tc.wantTool)
			}
			if cmd != tc.wantCmd {
				t.Errorf("cmd = %q, want %q", cmd, tc.wantCmd)
			}
			if risk != tc.wantRisk {
				t.Errorf("risk = %q, want %q", risk, tc.wantRisk)
			}
			if tc.wantJSONPart != "" && !strings.Contains(rawJSON, tc.wantJSONPart) {
				t.Errorf("rawJSON %q missing part %q", rawJSON, tc.wantJSONPart)
			}
		})
	}
}
