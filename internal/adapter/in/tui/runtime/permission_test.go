package runtime

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"context"
	"encoding/json"
	"fmt"
	"github.com/charmbracelet/x/ansi"
	"github.com/phongsathornpt/protonman/internal/core/permission"
	"github.com/phongsathornpt/protonman/internal/core/tool"
	"strings"
	"testing"
	"time"
)

func TestPermissionBridgeRoundTrip(t *testing.T) {
	bridge := newPermissionBridge()
	defer bridge.Close()
	request := permission.Request{CallID: "call-1", ToolName: "bash", ToolKind: permission.ToolBash, Detail: "printf private"}
	resultCh := make(chan permission.Resolution, 1)
	errorCh := make(chan error, 1)
	go func() {
		resolution, err := bridge.Prompt(context.Background(), request)
		resultCh <- resolution
		errorCh <- err
	}()
	messageCh := make(chan tea.Msg, 1)
	go func() {
		messageCh <- bridge.Next()()
	}()
	message := <-messageCh
	pending, ok := message.(permissionRequestMsg)
	if !ok {
		t.Fatalf("bridge message = %T, want permissionRequestMsg", message)
	}
	pending.request.response <- permissionResponse{resolution: permission.Resolution{Action: permission.ActionAllow}}
	if got := <-resultCh; got.Action != permission.ActionAllow {
		t.Fatalf("permission action = %s, want allow", got.Action)
	}
	if err := <-errorCh; err != nil {
		t.Fatalf("permission prompt error = %v, want nil", err)
	}
}

func TestBubbleModelPermissionModalRespondsToSessionGrant(t *testing.T) {
	bridge := newPermissionBridge()
	defer bridge.Close()
	registry, _ := newBubbleTestRegistry()
	service := newBubbleTestService(t, registry, permission.ModeAsk, permission.Config{})
	model := newBubbleModel(context.Background(), service, registry, emptyTodoItems(), nil, bridge, "")
	response := make(chan permissionResponse, 1)
	model.openPermission(permissionRequest{request: permission.Request{ToolName: "bash", ToolKind: permission.ToolBash, Detail: "printf safe", Effect: tool.CommandEffectReadOnly, Risk: tool.CommandRiskNormal}, response: response})
	updated, command := model.Update(testText("s"))
	if command != nil {
		t.Fatalf("permission update command = %v, want nil", command)
	}
	model = updated.(*bubbleModel)
	if model.hasPermissionView() {
		t.Fatal("permission modal remains open after session grant")
	}
	select {
	case result := <-response:
		if result.resolution.Action != permission.ActionAllow {
			t.Fatalf("permission action = %s, want allow", result.resolution.Action)
		}
		if result.resolution.Scope != permission.GrantScopeSession {
			t.Fatalf("permission scope = %v, want session", result.resolution.Scope)
		}
	case <-time.After(time.Second):
		t.Fatal("permission modal did not send a response")
	}
}

func TestPermissionCardOverlaysTranscript(t *testing.T) {
	model := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	model.resize(80, 24)
	model.appendLine("assistant: ready")
	model.refreshViewport()
	model.openPermission(permissionRequest{request: permission.Request{ToolName: "bash", ToolKind: permission.ToolBash, Detail: "rm -rf tmp", Arguments: json.RawMessage(`{"command":"rm -rf tmp"}`)}, response: make(chan permissionResponse, 1)})
	if !strings.Contains(plainTranscript(model), "assistant: ready") {
		t.Fatalf("overlay replaced the transcript: %#v", model.historyState.Cells())
	}
	view := model.View().Content
	for _, expected := range []string{"Permission required — shell modifies state", "bash", "Allow once", "Deny", "↑/↓", "navigate", "esc", "review"} {
		if !strings.Contains(view, expected) {
			t.Fatalf("overlay view does not contain %q: %s", expected, view)
		}
	}
	if strings.Contains(view, "Allow for this request this session") || strings.Contains(view, "s session") {
		t.Fatalf("mutating request exposed session grant: %s", view)
	}
}

func TestPermissionEscParksForScroll(t *testing.T) {
	response := make(chan permissionResponse, 1)
	model := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	model.resize(80, 24)
	model.openPermission(permissionRequest{request: permission.Request{ToolName: "read", ToolKind: permission.ToolRead, Detail: "README.md"}, response: response})
	updated, command := model.Update(testKey(tea.KeyEsc))
	if command != nil {
		t.Fatalf("esc review command = %v, want nil", command)
	}
	model = updated.(*bubbleModel)
	if !model.hasPermissionView() {
		t.Fatal("esc dismissed the permission card")
	}
	if !model.permissionView().parked {
		t.Fatal("esc did not enter transcript review mode")
	}
	if !strings.Contains(model.View().Content, "tab review") {
		t.Fatalf("review view missing approval hint: %s", model.View().Content)
	}
	updated, command = model.Update(testKey(tea.KeyTab))
	if command != nil {
		t.Fatalf("tab command = %v, want nil", command)
	}
	model = updated.(*bubbleModel)
	if model.permissionView().parked {
		t.Fatal("tab did not return focus to the permission card")
	}
	updated, command = model.Update(testText("n"))
	if command != nil {
		t.Fatalf("deny command = %v, want nil", command)
	}
	model = updated.(*bubbleModel)
	if model.hasPermissionView() {
		t.Fatal("deny left the permission card open")
	}
	select {
	case result := <-response:
		if result.resolution.Action != permission.ActionDeny {
			t.Fatalf("permission action = %s, want deny", result.resolution.Action)
		}
	case <-time.After(time.Second):
		t.Fatal("deny did not send a response")
	}
}

func TestPermissionCtrlCCancelsTurnInsteadOfDenying(t *testing.T) {
	response := make(chan permissionResponse, 1)
	model := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	model.busy = true
	canceled := false
	model.turnCancel = func() {
		canceled = true
	}
	model.openPermission(permissionRequest{request: permission.Request{ToolName: "bash", ToolKind: permission.ToolBash, Detail: "pwd", Arguments: json.RawMessage(`{"command":"pwd"}`)}, response: response})
	updated, command := model.Update(testCtrl('c'))
	model = updated.(*bubbleModel)
	if command != nil {
		t.Fatalf("ctrl+c command = %v, want nil while canceling active turn", command)
	}
	if !canceled {
		t.Fatal("ctrl+c did not cancel active turn")
	}
	select {
	case got := <-response:
		t.Fatalf("ctrl+c resolved permission unexpectedly: %+v", got)
	default:
	}
}

func TestPermissionPaneFitsNarrowResponsiveTerminals(t *testing.T) {
	for _, size := range [][2]int{{24, 8}, {40, 12}, {60, 16}} {
		m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
		m.resize(size[0], size[1])
		command := "rm -rf /workspace/project/a/very/long/path/that/should/not/overflow/the/terminal"
		m.openPermission(permissionRequest{request: permission.Request{ToolName: "bash", ToolKind: permission.ToolBash, Detail: command, Arguments: json.RawMessage(fmt.Sprintf(`{"command":%q}`, command))}, response: make(chan permissionResponse, 1)})
		view := m.View().Content
		if got := lipgloss.Width(view); got > size[0] {
			t.Fatalf("permission frame width=%d exceeds %d at %dx%d", got, size[0], size[0], size[1])
		}
		if got := lipgloss.Height(view); got > size[1] {
			t.Fatalf("permission frame height=%d exceeds %d at %dx%d", got, size[1], size[0], size[1])
		}
		plain := ansi.Strip(view)
		if !strings.Contains(plain, "Permission") || !strings.Contains(plain, "Allow once") {
			t.Fatalf("permission frame lost essential action at %dx%d: %q", size[0], size[1], plain)
		}
	}
}

func TestPermissionBashRiskPresentationUsesCommandEffect(t *testing.T) {
	model := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	model.resize(80, 24)
	tests := []struct {
		name    string
		command string
		want    string
	}{{name: "read only", command: "pwd", want: "Permission request — shell read only"}, {name: "mutating", command: "rm -rf tmp", want: "Permission required — shell modifies state"}, {name: "unknown", command: "make test", want: "Permission required — shell effects unknown"}, {name: "composed mutation", command: "pwd && rm tmp", want: "Permission required — shell modifies state"}, {name: "remote", command: "git push origin main", want: "Permission required — modifies remote"}, {name: "publish", command: "npm publish", want: "Permission required — publishes package"}, {name: "deployment", command: "wrangler deploy", want: "Permission required — changes deployment"}, {name: "destructive deployment", command: "terraform destroy", want: "Permission required — destructive deployment change"}}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			model.panes.bottom.remove(permissionViewID)
			model.openPermission(permissionRequest{request: permission.Request{ToolName: "bash", ToolKind: permission.ToolBash, Detail: tc.command, Arguments: json.RawMessage(fmt.Sprintf(`{"command":%q}`, tc.command))}, response: make(chan permissionResponse, 1)})
			view := model.View().Content
			if !strings.Contains(view, tc.want) {
				t.Fatalf("view missing %q:\n%s", tc.want, view)
			}
		})
	}
}

func TestPermissionOptionListEnterAndNumbers(t *testing.T) {
	response := make(chan permissionResponse, 1)
	model := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	model.openPermission(permissionRequest{request: permission.Request{ToolName: "bash", ToolKind: permission.ToolBash, Detail: "ls", Effect: tool.CommandEffectReadOnly, Risk: tool.CommandRiskNormal}, response: response})
	updated, _ := model.Update(testKey(tea.KeyDown))
	model = updated.(*bubbleModel)
	if model.permissionView().index != 1 {
		t.Fatalf("permission index after down = %d, want 1", model.permissionView().index)
	}
	updated, _ = model.Update(testKey(tea.KeyEnter))
	model = updated.(*bubbleModel)
	if model.hasPermissionView() {
		t.Fatal("enter left permission modal open")
	}
	select {
	case result := <-response:
		if result.resolution.Scope != permission.GrantScopeSession {
			t.Fatalf("enter on session row scope = %v", result.resolution.Scope)
		}
	case <-time.After(time.Second):
		t.Fatal("enter did not resolve permission")
	}
	response = make(chan permissionResponse, 1)
	model.openPermission(permissionRequest{request: permission.Request{ToolName: "bash", ToolKind: permission.ToolBash, Effect: tool.CommandEffectReadOnly, Risk: tool.CommandRiskNormal}, response: response})
	updated, _ = model.Update(testText("3"))
	model = updated.(*bubbleModel)
	select {
	case result := <-response:
		if result.resolution.Action != permission.ActionDeny {
			t.Fatalf("number 3 action = %s, want deny", result.resolution.Action)
		}
	case <-time.After(time.Second):
		t.Fatal("number shortcut did not resolve")
	}
}

func TestShiftTabCyclesAskPlanAlwaysApprove(t *testing.T) {
	model := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	updated, _ := model.Update(testShiftTab())
	model = updated.(*bubbleModel)
	if !model.planMode {
		t.Fatal("first shift+tab did not enter plan")
	}
	if model.service.Mode() != permission.ModeAsk {
		t.Fatalf("plan cycle changed mode = %s", model.service.Mode())
	}
	updated, _ = model.Update(testShiftTab())
	model = updated.(*bubbleModel)
	if model.planMode {
		t.Fatal("second shift+tab left plan on")
	}
	if model.service.Mode() != permission.ModeAlwaysApprove {
		t.Fatalf("second shift+tab mode = %s, want always-approve", model.service.Mode())
	}
	updated, _ = model.Update(testShiftTab())
	model = updated.(*bubbleModel)
	if model.planMode || model.service.Mode() != permission.ModeAsk {
		t.Fatalf("third shift+tab = plan=%v mode=%s", model.planMode, model.service.Mode())
	}
}

func TestPermissionBashPresentationShowsCwdAndEffectReason(t *testing.T) {
	model := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	model.resize(100, 30)
	model.openPermission(permissionRequest{request: permission.Request{ToolName: "bash", ToolKind: permission.ToolBash, Detail: "git status --short", Arguments: json.RawMessage(`{"command":"git status --short","cwd":"internal/agent"}`)}, response: make(chan permissionResponse, 1)})
	view := model.View().Content
	for _, want := range []string{"Cwd: internal/agent", "Effect: read_only", "git status is read only"} {
		if !strings.Contains(view, want) {
			t.Fatalf("view missing %q:\n%s", want, view)
		}
	}
}

func TestPermissionRequestLivesInBottomPane(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.openPermission(permissionRequest{request: permission.Request{ToolName: "bash", ToolKind: permission.ToolBash, Detail: "pwd"}, response: make(chan permissionResponse, 1)})
	if top := m.panes.bottom.top(); top == nil || top.ID() != permissionViewID {
		t.Fatalf("top view = %#v, want permission", top)
	}
	if m.panes.bottom.composerVisible() {
		t.Fatal("composer remained visible while approval view owns input")
	}
}

func TestStalePermissionRequestAfterTurnEndIsDenied(t *testing.T) {
	model := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	response := make(chan permissionResponse, 1)
	updated, _ := model.Update(permissionRequestMsg{request: permissionRequest{request: permission.Request{ToolName: "bash", ToolKind: permission.ToolBash}, response: response}})
	model = updated.(*bubbleModel)
	if model.hasPermissionView() {
		t.Fatal("stale permission request opened a modal after the turn ended")
	}
	select {
	case result := <-response:
		if result.resolution.Action != permission.ActionDeny {
			t.Fatalf("stale permission action = %s, want deny", result.resolution.Action)
		}
	case <-time.After(time.Second):
		t.Fatal("stale permission request was not resolved")
	}
}

func TestBubbleModelPermissionModalAllowsAndSavesProjectRule(t *testing.T) {
	bridge := newPermissionBridge()
	defer bridge.Close()
	registry, _ := newBubbleTestRegistry()
	service := newBubbleTestService(t, registry, permission.ModeAsk, permission.Config{})
	workDir := t.TempDir()
	model := newBubbleModel(context.Background(), service, registry, emptyTodoItems(), nil, bridge, workDir)
	model.projectTrusted = true

	response := make(chan permissionResponse, 1)
	model.openPermission(permissionRequest{
		request: permission.Request{
			ToolName: "bash",
			ToolKind: permission.ToolBash,
			Detail:   "git status --short",
			Effect:   tool.CommandEffectReadOnly,
			Risk:     tool.CommandRiskNormal,
		},
		response: response,
	})

	// Press 'p' to allow and save to project
	updated, saveCmd := model.Update(testText("p"))
	model = updated.(*bubbleModel)
	if model.hasPermissionView() {
		t.Fatal("permission modal remains open after project save grant")
	}
	if saveCmd == nil {
		t.Fatal("expected saveCmd from project save")
	}

	// Verify response was sent as allow once
	select {
	case res := <-response:
		if res.resolution.Action != permission.ActionAllow {
			t.Fatalf("permission action = %s, want allow", res.resolution.Action)
		}
	case <-time.After(time.Second):
		t.Fatal("no response received")
	}

	// Verify live policy was updated
	d := service.Policy().Evaluate(permission.Request{
		ToolName: "bash",
		ToolKind: permission.ToolBash,
		Detail:   "git status --short",
	})
	if d.Action != permission.ActionAllow {
		t.Fatalf("live policy decision = %v, want allow", d.Action)
	}

	// Execute saveCmd and verify project setting saved msg
	msg := saveCmd()
	savedMsg, ok := msg.(permissionRuleSavedMsg)
	if !ok {
		t.Fatalf("msg type = %T, want permissionRuleSavedMsg", msg)
	}
	if savedMsg.err != nil {
		t.Fatalf("save rule error: %v", savedMsg.err)
	}
	if savedMsg.scope != "project" {
		t.Fatalf("scope = %q, want project", savedMsg.scope)
	}

	// Process message in model
	updated, _ = model.Update(savedMsg)
	model = updated.(*bubbleModel)
	if !strings.Contains(plainTranscript(model), "Saved allow rule to project config") {
		t.Fatalf("transcript missing save notice: %s", plainTranscript(model))
	}
}

func TestBubbleModelPermissionModalAllowsAndSavesGlobalRule(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("PROTONMAN_HOME", homeDir)

	bridge := newPermissionBridge()
	defer bridge.Close()
	registry, _ := newBubbleTestRegistry()
	service := newBubbleTestService(t, registry, permission.ModeAsk, permission.Config{})
	model := newBubbleModel(context.Background(), service, registry, emptyTodoItems(), nil, bridge, t.TempDir())

	response := make(chan permissionResponse, 1)
	model.openPermission(permissionRequest{
		request: permission.Request{
			ToolName: "read",
			ToolKind: permission.ToolRead,
			Detail:   "README.md",
			Effect:   tool.CommandEffectReadOnly,
			Risk:     tool.CommandRiskNormal,
		},
		response: response,
	})

	// Press 'g' to allow and save globally
	updated, saveCmd := model.Update(testText("g"))
	model = updated.(*bubbleModel)
	if model.hasPermissionView() {
		t.Fatal("permission modal remains open after global save grant")
	}
	if saveCmd == nil {
		t.Fatal("expected saveCmd from global save")
	}

	// Verify response was sent as allow once
	select {
	case res := <-response:
		if res.resolution.Action != permission.ActionAllow {
			t.Fatalf("permission action = %s, want allow", res.resolution.Action)
		}
	case <-time.After(time.Second):
		t.Fatal("no response received")
	}

	// Verify live policy was updated
	d := service.Policy().Evaluate(permission.Request{
		ToolName: "read",
		ToolKind: permission.ToolRead,
		Detail:   "README.md",
	})
	if d.Action != permission.ActionAllow {
		t.Fatalf("live policy decision = %v, want allow", d.Action)
	}

	// Execute saveCmd and verify global setting saved msg
	msg := saveCmd()
	savedMsg, ok := msg.(permissionRuleSavedMsg)
	if !ok {
		t.Fatalf("msg type = %T, want permissionRuleSavedMsg", msg)
	}
	if savedMsg.err != nil {
		t.Fatalf("save rule error: %v", savedMsg.err)
	}
	if savedMsg.scope != "global" {
		t.Fatalf("scope = %q, want global", savedMsg.scope)
	}
}

func TestBubbleModelPermissionModalProjectOptionHiddenWhenUntrusted(t *testing.T) {
	model := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	model.projectTrusted = false
	model.resize(100, 30)

	model.openPermission(permissionRequest{
		request: permission.Request{
			ToolName: "bash",
			ToolKind: permission.ToolBash,
			Detail:   "git status",
			Effect:   tool.CommandEffectReadOnly,
			Risk:     tool.CommandRiskNormal,
		},
		response: make(chan permissionResponse, 1),
	})

	view := model.View().Content
	if strings.Contains(view, "Allow and save to project") || strings.Contains(view, "p project") {
		t.Fatalf("untrusted workspace exposed project rule option:\n%s", view)
	}
	if !strings.Contains(view, "Allow and save globally") {
		t.Fatalf("view missing global rule option:\n%s", view)
	}

	// 'p' shortcut should do nothing when project option is hidden
	updated, cmd := model.Update(testText("p"))
	model = updated.(*bubbleModel)
	if cmd != nil {
		t.Fatalf("unexpected cmd on 'p' when untrusted: %v", cmd)
	}
	if !model.hasPermissionView() {
		t.Fatal("permission modal should remain open after invalid shortcut 'p'")
	}
}

func TestPermissionBridgeConcurrentPrompts(t *testing.T) {
	bridge := newPermissionBridge()
	defer bridge.Close()

	type result struct {
		callID string
		err    error
	}
	results := make(chan result, 2)
	for _, id := range []string{"a", "b"} {
		id := id
		go func() {
			_, err := bridge.Prompt(context.Background(), permission.Request{CallID: id, ToolName: "read", ToolKind: permission.ToolRead})
			results <- result{callID: id, err: err}
		}()
	}

	seen := map[string]bool{}
	for range 2 {
		msg := bridge.Next()()
		pending, ok := msg.(permissionRequestMsg)
		if !ok {
			t.Fatalf("bridge message = %T, want permissionRequestMsg", msg)
		}
		seen[pending.request.request.CallID] = true
		pending.request.response <- permissionResponse{resolution: permission.Resolution{Action: permission.ActionAllow}}
	}
	if !seen["a"] || !seen["b"] {
		t.Fatalf("concurrent prompts lost request: %#v", seen)
	}
	for range 2 {
		if got := <-results; got.err != nil {
			t.Fatalf("prompt %s failed: %v", got.callID, got.err)
		}
	}
}

func TestPermissionBridgeCloseReleasesPendingPrompt(t *testing.T) {
	bridge := newPermissionBridge()
	errCh := make(chan error, 1)
	go func() {
		_, err := bridge.Prompt(context.Background(), permission.Request{CallID: "pending", ToolName: "read", ToolKind: permission.ToolRead})
		errCh <- err
	}()
	msg := bridge.Next()()
	if _, ok := msg.(permissionRequestMsg); !ok {
		t.Fatalf("bridge message = %T, want permissionRequestMsg", msg)
	}
	bridge.Close()
	select {
	case err := <-errCh:
		if err == nil || !strings.Contains(err.Error(), "closed") {
			t.Fatalf("pending prompt error = %v, want closed error", err)
		}
	case <-time.After(time.Second):
		t.Fatal("pending permission prompt did not unblock after Close")
	}
	if msg := bridge.Next()(); msg != (permissionBridgeClosedMsg{}) {
		t.Fatalf("Next after close = %T, want permissionBridgeClosedMsg", msg)
	}
}

func TestPermissionCommandOpensModePickerAndAppliesPlan(t *testing.T) {
	model := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	model.resize(80, 24)
	_ = model.executeCommand("/permission")
	view, ok := model.panes.bottom.find(permissionModeViewID).(*permissionModePaneView)
	if !ok || view == nil {
		t.Fatal("/permission did not open permission mode picker")
	}
	view.index = int(permissionModePlan)
	result := view.HandlePaneKey(newPaneRenderContext(model), testKey(tea.KeyEnter))
	_ = model.applyPaneAction(result.action)
	if !model.planMode || model.service.Mode() != permission.ModeAsk {
		t.Fatalf("plan selection = plan=%v mode=%s, want plan=true mode=ask", model.planMode, model.service.Mode())
	}
}

func TestPermissionModePickerAppliesAlwaysApprove(t *testing.T) {
	model := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	model.resize(80, 24)
	model.openPermissionModePane()
	view := model.panes.bottom.find(permissionModeViewID).(*permissionModePaneView)
	view.index = int(permissionModeAlwaysApprove)
	result := view.HandlePaneKey(newPaneRenderContext(model), testKey(tea.KeyEnter))
	_ = model.applyPaneAction(result.action)
	if model.planMode || model.service.Mode() != permission.ModeAlwaysApprove {
		t.Fatalf("always approve selection = plan=%v mode=%s", model.planMode, model.service.Mode())
	}
}
