package runtime

import (
	tea "charm.land/bubbletea/v2"
	"context"
	"fmt"
	"github.com/charmbracelet/x/ansi"
	turnmsg "github.com/phongsathornpt/protonman/internal/adapter/in/tui/runtime/turn"
	"github.com/phongsathornpt/protonman/internal/adapter/out/config"
	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
	"github.com/phongsathornpt/protonman/internal/app"
	"github.com/phongsathornpt/protonman/internal/core/permission"
	"github.com/phongsathornpt/protonman/internal/core/tool"
	"github.com/phongsathornpt/protonman/internal/feature/agent"
	"github.com/phongsathornpt/protonman/internal/feature/skill"
	sdk "github.com/phongsathornpt/protonman/proton-sdk"
	"strings"
	"testing"
)

func TestSlashDropdownFiltersAndTabAccepts(t *testing.T) {
	model := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	model.resize(80, 24)
	model.panes.bottom.prompt().SetValue("/he")
	if !model.slashOpen() {
		t.Fatal("slash dropdown did not open for /he")
	}
	matches := model.slashMatches()
	if len(matches) != 1 || matches[0].Name != "help" {
		t.Fatalf("slash matches = %#v, want help", matches)
	}
	applied, command := model.acceptSlash(false)
	if !applied || command != nil {
		t.Fatalf("tab accept applied=%v command=%v", applied, command)
	}
	if got := model.panes.bottom.prompt().Value(); got != "/help" {
		t.Fatalf("tab accept value = %q, want /help", got)
	}
}

func TestColonAliasDispatchesHelp(t *testing.T) {
	model := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	model.panes.bottom.prompt().SetValue(":help")
	if command := model.submit(); command != nil {
		t.Fatalf("colon help command = %v, want nil", command)
	}
	if !strings.Contains(plainTranscript(model), "/call") {
		t.Fatalf("colon alias did not render help: %q", plainTranscript(model))
	}
}

func TestSlashAgent(t *testing.T) {
	registry := behaviorRegistry{handler: &countingHandler{definition: tool.Definition{Name: "read", Kind: tool.KindRead, Description: "read"}}}
	service := newBehaviorService(t, registry, permission.ModeAsk)
	bModel := newBubbleModel(context.Background(), service, registry, nil, nil, newPermissionBridge(), "")
	t.Run("default agent display", func(t *testing.T) {
		bModel.executeCommand("/agent")
		content := bModel.viewport.View()
		if !strings.Contains(content, "Active agent profile") {
			t.Fatalf("expected 'Active agent profile', got: %s", content)
		}
		if !strings.Contains(content, "universal") || !strings.Contains(content, "strength") || !strings.Contains(content, "agility") || !strings.Contains(content, "intelligence") {
			t.Fatalf("expected Dota attribute profiles in list, got: %s", content)
		}
	})
	t.Run("switch to dex profile", func(t *testing.T) {
		bModel.executeCommand("/agent intelligence")
		if got, want := bModel.agentProfile, "intelligence"; got != want {
			t.Fatalf("bModel.agentProfile = %q, want %q", got, want)
		}
		content := bModel.viewport.View()
		if !strings.Contains(content, "Agent profile switched to intelligence") {
			t.Fatalf("expected switch confirmation, got: %s", content)
		}
		if len(bModel.messages) != 0 {
			t.Fatalf("profile switch mutated transcript: %+v", bModel.messages)
		}
	})
	t.Run("switch to strength profile", func(t *testing.T) {
		bModel.executeCommand("/agent strength")
		if got, want := bModel.agentProfile, "strength"; got != want {
			t.Fatalf("bModel.agentProfile = %q, want %q", got, want)
		}
		if len(bModel.messages) != 0 {
			t.Fatalf("profile switch mutated transcript: %+v", bModel.messages)
		}
	})
	t.Run("reject invalid profile", func(t *testing.T) {
		bModel.executeCommand("/agent invalid_profile")
		content := bModel.viewport.View()
		if !strings.Contains(content, "unknown agent profile") {
			t.Fatalf("expected unknown agent profile error, got: %s", content)
		}
	})
}

func TestSlashEscapePreservesComposerDraft(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.panes.bottom.prompt().SetValue("/he")
	m.syncSlashView()
	if !m.slashOpen() {
		t.Fatal("slash view did not open")
	}
	updated, command := m.Update(testKey(tea.KeyEsc))
	m = updated.(*bubbleModel)
	if command != nil {
		t.Fatalf("escape command = %v, want nil", command)
	}
	if got := m.panes.bottom.prompt().Value(); got != "/he" {
		t.Fatalf("draft = %q, want /he", got)
	}
	if m.panes.bottom.has(slashViewID) {
		t.Fatal("slash view remained on stack after escape")
	}
}

func TestSlashReasoningUsesActiveModelProfile(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.activeProvider = "protonman"
	m.activeModel = "gemini-3.8-flash"
	m.executeCommand("/reasoning high")
	if got := m.reasoningEffort; got != sdk.ReasoningHigh {
		t.Fatalf("reasoningEffort = %q, want high", got)
	}
	m.executeCommand("/reasoning xhigh")
	if got := m.reasoningEffort; got != sdk.ReasoningHigh {
		t.Fatalf("unsupported xhigh changed reasoningEffort to %q", got)
	}
	if got := plainTranscript(m); !strings.Contains(got, "does not support reasoning effort") || !strings.Contains(got, "low, medium, high") {
		t.Fatalf("transcript missing model-profile rejection: %q", got)
	}
}

func TestSlashReasoningAutoResetsSessionOverride(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.activeModel = "gemini-3.8-flash"
	m.reasoningEffort = sdk.ReasoningHigh
	m.executeCommand("/reasoning auto")
	if got := m.reasoningEffort; got != sdk.ReasoningDefault {
		t.Fatalf("reasoningEffort = %q, want auto/default", got)
	}
	if got := plainTranscript(m); !strings.Contains(got, "Thinking level set to auto") {
		t.Fatalf("transcript = %q", got)
	}
}

func TestSlashReasoningOpensUnifiedModelSetup(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.activeProvider = "protonman"
	m.activeModel = "gemini-3.8-flash"
	m.providers = map[string]config.ProviderConfig{"protonman": {Name: "protonman", Type: "openai", BaseURL: "https://protonman.dev/api/v1"}}
	m.modelCatalogs.Set("protonman", []model.RemoteModel{{ID: "gemini-3.8-flash", Name: "Gemini 3.8 Flash"}})
	m.executeCommand("/reasoning")
	if !m.panes.bottom.has(modelSetupViewID) {
		t.Fatal("/reasoning did not open unified model setup")
	}
	got := m.panes.bottom.renderTop(m)
	for _, want := range []string{"Switch Model", "Gemini 3.8 Flash", "Thinking", "low", "medium", "high"} {
		if !strings.Contains(got, want) {
			t.Fatalf("model setup missing %q: %q", want, got)
		}
	}
}

func TestUnifiedModelSetupAdjustsThinkingBeforeApply(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.activeProvider = "protonman"
	m.activeModel = "gemini-3.8-flash"
	m.providers = map[string]config.ProviderConfig{"protonman": {Name: "protonman", Type: "openai", BaseURL: "https://protonman.dev/api/v1"}}
	m.modelCatalogs.Set("protonman", []model.RemoteModel{{ID: "gemini-3.8-flash", Name: "Gemini 3.8 Flash"}})
	m.executeCommand("/model")
	view := m.panes.bottom.find(modelSetupViewID).(*modelSetupPaneView)
	if got := view.selectedReasoning(); got != sdk.ReasoningDefault {
		t.Fatalf("initial thinking = %q, want auto", got)
	}
	handled, _ := m.handleModalKey(testKey(tea.KeyRight))
	if !handled || view.selectedReasoning() != sdk.ReasoningLow {
		t.Fatalf("right did not move thinking to low: %q", view.selectedReasoning())
	}
	if m.reasoningEffort != sdk.ReasoningDefault {
		t.Fatalf("pending setup mutated runtime before apply: %q", m.reasoningEffort)
	}
}

func TestUnifiedModelSetupShiftTabCyclesProviderWithoutPermissionLeak(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.providers = map[string]config.ProviderConfig{
		"alpha": {Name: "alpha", Type: "openai", BaseURL: "https://alpha.example/v1", APIKey: "x"},
		"beta":  {Name: "beta", Type: "openai", BaseURL: "https://beta.example/v1", APIKey: "x"},
	}
	m.activeProvider = "alpha"
	m.modelCatalogs.Set("alpha", []model.RemoteModel{{ID: "a"}})
	m.modelCatalogs.Set("beta", []model.RemoteModel{{ID: "b"}})
	m.executeCommand("/model")
	view := m.panes.bottom.find(modelSetupViewID).(*modelSetupPaneView)
	initialProvider := view.activeProviderName()
	initialMode := m.service.Mode()
	handled, _ := m.handleModalKey(testShiftTab())
	if !handled {
		t.Fatal("shift+tab was not handled by model setup")
	}
	if view.activeProviderName() == initialProvider {
		t.Fatal("shift+tab did not cycle provider")
	}
	if m.service.Mode() != initialMode {
		t.Fatalf("permission mode changed from %s to %s", initialMode, m.service.Mode())
	}
}

func TestInfoViewKeepsIdleChromeEmpty(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, nil)
	m.resize(100, 30)
	m.activeModel = "claude-3-7-sonnet"
	m.reasoningEffort = sdk.ReasoningHigh
	if got := m.infoView(); got != "" {
		t.Fatalf("idle infoView = %q, want empty minimal chrome", got)
	}
}

func TestSlashReasoningSyncsCoordinator(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.activeModel = "gemini-3.8-flash"
	coord := agent.NewCoordinator(nil, nil, nil, nil)
	defer func() {
		_ = coord.Close()
	}()
	m.agents = app.NewAgents(coord)
	m.executeCommand("/reasoning low")
	if got := coord.ReasoningEffort(); got != sdk.ReasoningLow {
		t.Fatalf("coordinator reasoning = %q, want low", got)
	}
	m.executeCommand("/reasoning auto")
	if got := coord.ReasoningEffort(); got != sdk.ReasoningDefault {
		t.Fatalf("coordinator reasoning = %q, want auto/default", got)
	}
}

func TestRemoteModelReasoningSummaryUsesResolvedProfile(t *testing.T) {
	md := model.RemoteModel{ID: "gemini-3.8-flash"}
	if got := remoteModelReasoningSummary("protonman", md, true); got != "reasoning low/medium/high (default medium)" {
		t.Fatalf("summary = %q", got)
	}
	if got := remoteModelReasoningSummary("custom", model.RemoteModel{ID: "future-model"}, true); got != "" {
		t.Fatalf("unknown summary = %q, want empty", got)
	}
}

func TestSlashReasoningPickerUsesCatalogResolvedProfile(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.activeProvider = "protonman"
	m.activeModel = "gemini-3.8-flash"
	yes := true
	m.modelCatalogs.Set("protonman", []model.RemoteModel{{ID: "gemini-3.8-flash", ToolSupport: &yes}})
	m.executeCommand("/reasoning")
	got := m.panes.bottom.renderTop(m)
	for _, want := range []string{"low", "medium", "high"} {
		if !strings.Contains(got, want) {
			t.Fatalf("picker missing catalog-resolved level %q: %q", want, got)
		}
	}
}

func TestSetReasoningEffortValidatesModelProfile(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.activeProvider = "protonman"
	m.activeModel = "gemini-3.8-flash"
	m.reasoningEffort = sdk.ReasoningMedium
	m.setReasoningEffort(sdk.ReasoningXHigh)
	if got := m.reasoningEffort; got != sdk.ReasoningMedium {
		t.Fatalf("invalid picker-style selection changed effort to %q", got)
	}
}

func TestSlashSkills(t *testing.T) {
	registry := behaviorRegistry{handler: &countingHandler{definition: tool.Definition{Name: "read", Kind: tool.KindRead, Description: "read"}}}
	service := newBehaviorService(t, registry, permission.ModeAsk)
	model := newBubbleModel(context.Background(), service, registry, nil, nil, newPermissionBridge(), "")
	t.Run("no skills registered", func(t *testing.T) {
		model.executeCommand("/skills")
		content := model.viewport.View()
		if !strings.Contains(content, "No agent skills discovered") {
			t.Fatalf("expected 'No agent skills discovered', got: %s", content)
		}
	})
	t.Run("skills registered with checkbox", func(t *testing.T) {
		s := skill.Skill{Name: "pdf-processing", Description: "Extract PDF text", Scope: skill.ScopeUser, Location: "/home/user/.agents/skills/pdf-processing/SKILL.md", BaseDir: "/home/user/.agents/skills/pdf-processing", Instructions: "# PDF Processing Guide\nExtracting text.", Resources: []string{"scripts/extract.py"}}
		model.skills = skill.NewRegistry(s)
		model.executeCommand("/skills")
		content := model.viewport.View()
		if strings.Contains(content, "Agent Skills") || strings.Contains(content, "[ ] pdf-processing") {
			t.Fatalf("bare /skills duplicated picker content into transcript: %s", content)
		}
		pickerRender := model.panes.bottom.renderTop(model)
		if !strings.Contains(pickerRender, "pdf-processing") {
			t.Fatalf("expected picker to contain pdf-processing, got: %s", pickerRender)
		}
		if strings.Contains(pickerRender, "Extract PDF text") || strings.Contains(pickerRender, "[user]") {
			t.Fatalf("expected bottom pane picker to show skill name only without scope or description, got: %s", pickerRender)
		}
	})
	t.Run("skill activation", func(t *testing.T) {
		model.panes.bottom.remove(skillsViewID)
		model.executeCommand("/skill")
		if !model.panes.bottom.has(skillsViewID) {
			t.Fatalf("expected /skill without args to open skills picker")
		}
		model.panes.bottom.remove(skillsViewID)
		model.executeCommand("/skill nonexistent")
		if !strings.Contains(model.viewport.View(), "skill \"nonexistent\" not found") {
			t.Fatalf("expected not found error")
		}
		model.executeCommand("/skill pdf-processing")
		content := model.viewport.View()
		if !strings.Contains(content, "[x] Activated skill pdf-processing [user]:") {
			t.Fatalf("expected activation message in viewport, got: %s", content)
		}
		if !strings.Contains(content, "scripts/extract.py") {
			t.Fatalf("expected bundled resource in viewport, got: %s", content)
		}
		if !model.skills.IsActivated("pdf-processing") {
			t.Fatalf("expected skill to be marked activated")
		}
		model.executeCommand("/skills")
		if rendered := model.panes.bottom.renderTop(model); !strings.Contains(rendered, "Skills · 1/1 active") || !strings.Contains(rendered, "[x] pdf-processing") {
			t.Fatalf("expected active skill state in picker, got: %s", rendered)
		}
		model.panes.bottom.remove(skillsViewID)
		model.executeCommand("/skills active")
		content = model.viewport.View()
		if !strings.Contains(content, "Active Agent Skills (1):") || !strings.Contains(content, "[x] pdf-processing") {
			t.Fatalf("expected active skills list, got: %s", content)
		}
	})
	t.Run("skill toggle", func(t *testing.T) {
		model.executeCommand("/skill toggle pdf-processing")
		content := model.viewport.View()
		if !strings.Contains(content, "[ ] Skill \"pdf-processing\" deactivated.") {
			t.Fatalf("expected deactivated message, got: %s", content)
		}
		if model.skills.IsActivated("pdf-processing") {
			t.Fatalf("expected skill to be deactivated")
		}
		model.executeCommand("/skills active")
		content = model.viewport.View()
		if !strings.Contains(content, "No active agent skills in this session.") {
			t.Fatalf("expected no active skills message, got: %s", content)
		}
		model.executeCommand("/skill toggle pdf-processing")
		content = model.viewport.View()
		if !strings.Contains(content, "[x] Skill \"pdf-processing\" activated.") {
			t.Fatalf("expected activated message, got: %s", content)
		}
		if !model.skills.IsActivated("pdf-processing") {
			t.Fatalf("expected skill to be activated again")
		}
	})
	t.Run("status bar omits active skills", func(t *testing.T) {
		info := model.infoView()
		if strings.Contains(info, "pdf-processing") || strings.Contains(info, "skills active") {
			t.Fatalf("minimal infoView leaked skill state: %s", info)
		}
	})
	t.Run("unified skill commands and deactivation verbs", func(t *testing.T) {
		model.skills.Activate("pdf-processing")
		model.executeCommand("/skill active")
		if !strings.Contains(model.viewport.View(), "Active Agent Skills (1):") {
			t.Fatalf("expected /skill active to list active skills")
		}
		initialMsgCount := len(model.messages)
		model.executeCommand("/skill pdf-processing")
		if !strings.Contains(model.viewport.View(), "is already active") {
			t.Fatalf("expected already active message on duplicate activation")
		}
		if len(model.messages) != initialMsgCount {
			t.Fatalf("messages count increased on duplicate activation: %d != %d", len(model.messages), initialMsgCount)
		}
		model.executeCommand("/skill deactivate pdf-processing")
		if model.skills.IsActivated("pdf-processing") {
			t.Fatalf("expected skill to be deactivated")
		}
		if !strings.Contains(model.viewport.View(), "Skill \"pdf-processing\" deactivated.") {
			t.Fatalf("expected deactivated message")
		}
		model.executeCommand("/skills toggle pdf-processing")
		if !model.skills.IsActivated("pdf-processing") {
			t.Fatalf("expected skill to be activated via /skills toggle")
		}
		model.executeCommand("/skill disable pdf-processing")
		if model.skills.IsActivated("pdf-processing") {
			t.Fatalf("expected skill to be deactivated via /skill disable")
		}
	})
	t.Run("skill name autocomplete in composer", func(t *testing.T) {
		model.panes.bottom.remove(skillsViewID)
		model.panes.bottom.prompt().SetValue("/skill ")
		if !model.slashOpen() {
			t.Fatal("slash dropdown did not open for /skill ")
		}
		matches := model.slashMatches()
		if len(matches) == 0 {
			t.Fatal("expected matches for /skill ")
		}
		model.panes.bottom.prompt().SetValue("/skill pd")
		matches = model.slashMatches()
		if len(matches) != 1 || matches[0].Name != "pdf-processing" {
			t.Fatalf("expected pdf-processing match, got: %#v", matches)
		}
		applied, cmd := model.acceptSlash(false)
		if !applied || cmd != nil {
			t.Fatalf("acceptSlash failed: applied=%v, cmd=%v", applied, cmd)
		}
		if got := model.panes.bottom.prompt().Value(); got != "/skill pdf-processing" {
			t.Fatalf("prompt value after accept = %q, want /skill pdf-processing", got)
		}
	})
	t.Run("interactive bottom-pane skills picker", func(t *testing.T) {
		model.panes.bottom.remove(skillsViewID)
		model.executeCommand("/skills")
		if !model.panes.bottom.has(skillsViewID) {
			t.Fatal("expected skills picker in bottom pane after /skills")
		}
		rendered := model.panes.bottom.renderTop(model)
		if !strings.Contains(rendered, "Skills") || !strings.Contains(rendered, "pdf-processing") {
			t.Fatalf("unexpected picker render: %s", rendered)
		}
		wasActive := model.skills.IsActivated("pdf-processing")
		updated, _ := model.Update(testText(" "))
		model = updated.(*bubbleModel)
		if model.skills.IsActivated("pdf-processing") == wasActive {
			t.Fatalf("spacebar did not toggle skill active status")
		}
		updated, _ = model.Update(testKey(tea.KeyEsc))
		model = updated.(*bubbleModel)
		if model.panes.bottom.has(skillsViewID) {
			t.Fatal("esc did not close skills picker")
		}
	})
	t.Run("ctrl+s shortcut toggles skills picker", func(t *testing.T) {
		model.panes.bottom.remove(skillsViewID)
		updated, _ := model.Update(testCtrl('s'))
		model = updated.(*bubbleModel)
		if !model.panes.bottom.has(skillsViewID) {
			t.Fatal("ctrl+s did not open skills picker")
		}
		updated, _ = model.Update(testCtrl('s'))
		model = updated.(*bubbleModel)
		if model.panes.bottom.has(skillsViewID) {
			t.Fatal("second ctrl+s did not close skills picker")
		}
	})
	t.Run("new command resets active skills", func(t *testing.T) {
		model.skills.Activate("pdf-processing")
		if !model.skills.IsActivated("pdf-processing") {
			t.Fatal("expected skill to be active")
		}
		model.executeCommand("/new")
		if model.skills.IsActivated("pdf-processing") {
			t.Fatal("expected /new to clear active skills")
		}
	})
	t.Run("skill activation does not append user message and does not flood instructions", func(t *testing.T) {
		model.messages = nil
		model.skills.Deactivate("pdf-processing")
		model.executeCommand("/skill pdf-processing")
		content := model.viewport.View()
		if !strings.Contains(content, "[x] Activated skill pdf-processing [user]: Extract PDF text") {
			t.Fatalf("expected activation message with description, got: %s", content)
		}
		if strings.Contains(content, "# PDF Processing Guide") {
			t.Fatalf("did not expect raw instructions markdown in viewport")
		}
		if len(model.messages) != 0 {
			t.Fatalf("expected 0 messages appended to model.messages, got %d", len(model.messages))
		}
	})
	t.Run("t shortcut toggles skill in bottom-pane picker", func(t *testing.T) {
		model.panes.bottom.remove(skillsViewID)
		model.executeCommand("/skills")
		wasActive := model.skills.IsActivated("pdf-processing")
		updated, _ := model.Update(testText("t"))
		model = updated.(*bubbleModel)
		if model.skills.IsActivated("pdf-processing") == wasActive {
			t.Fatalf("'t' key did not toggle skill active status")
		}
		model.panes.bottom.remove(skillsViewID)
	})
	t.Run("slashCatalog contains single unified skills command with skill alias", func(t *testing.T) {
		var foundSkills *slashCommand
		count := 0
		for i, cmd := range slashCatalog {
			if cmd.Name == "skills" || cmd.Name == "skill" {
				foundSkills = &slashCatalog[i]
				count++
			}
		}
		if count != 1 {
			t.Fatalf("expected exactly 1 catalog entry for skills, found %d", count)
		}
		if foundSkills == nil || foundSkills.Name != "skills" {
			t.Fatalf("expected primary command name to be 'skills', got %v", foundSkills)
		}
		hasAlias := false
		for _, a := range foundSkills.Aliases {
			if a == "skill" {
				hasAlias = true
				break
			}
		}
		if !hasAlias {
			t.Fatalf("expected 'skill' alias in skills command, got %v", foundSkills.Aliases)
		}
	})
}

func TestSlashSubagentsToggle(t *testing.T) {
	registry := behaviorRegistry{handler: &countingHandler{definition: tool.Definition{Name: "read", Kind: tool.KindRead, Description: "read"}}}
	service := newBehaviorService(t, registry, permission.ModeAsk)
	coord := agent.NewCoordinator(nil, nil, nil, nil)
	defer coord.Close()
	m := newBubbleModel(context.Background(), service, registry, nil, nil, newPermissionBridge(), "")
	m.agents = app.NewAgents(coord)
	m.executeCommand("/subagents off")
	if m.subagentsEnabled || coord.Enabled() {
		t.Fatal("/subagents off did not disable runtime capability")
	}
	if !strings.Contains(m.viewport.View(), "Subagents disabled") {
		t.Fatalf("missing disable confirmation: %q", m.viewport.View())
	}
	m.executeCommand("/subagents on")
	if !m.subagentsEnabled || !coord.Enabled() {
		t.Fatal("/subagents on did not enable runtime capability")
	}
	m.executeCommand("/subagents nope")
	if !strings.Contains(m.viewport.View(), "subagents must be on or off") {
		t.Fatalf("missing invalid toggle error: %q", m.viewport.View())
	}
}

func newTestSkillsModel(t *testing.T, count int) *bubbleModel {
	t.Helper()
	registry := behaviorRegistry{handler: &countingHandler{definition: tool.Definition{Name: "read", Kind: tool.KindRead}}}
	service := newBehaviorService(t, registry, permission.ModeAsk)
	model := newBubbleModel(context.Background(), service, registry, nil, nil, newPermissionBridge(), "")
	model.resize(80, 24)
	skills := make([]skill.Skill, 0, count)
	for i := 1; i <= count; i++ {
		skills = append(skills, skill.Skill{Name: fmt.Sprintf("skill-%02d", i), Description: fmt.Sprintf("Description for skill %02d", i), Scope: skill.ScopeUser})
	}
	model.skills = skill.NewRegistry(skills...)
	return model
}

func TestSkillsPickerWindowingLargeList(t *testing.T) {
	model := newTestSkillsModel(t, 15)
	updated, _ := model.Update(testCtrl('s'))
	model = updated.(*bubbleModel)
	if !model.panes.bottom.has(skillsViewID) {
		t.Fatal("expected skills picker to be open")
	}
	render := model.panes.bottom.renderTop(model)
	view := model.panes.bottom.find(skillsViewID).(*skillsPaneView)
	if got := view.picker.GlobalIndex(); got != 0 {
		t.Fatalf("expected initial selected index 0, got %d", got)
	}
	for range 8 {
		updated, _ = model.Update(testKey(tea.KeyDown))
		model = updated.(*bubbleModel)
	}
	render = model.panes.bottom.renderTop(model)
	if got := view.picker.GlobalIndex(); got != 8 {
		t.Fatalf("expected selected index 8 after navigation, got %d", got)
	}
	if !strings.Contains(render, "skill-09") {
		t.Fatalf("expected skill-09 to be visible in window, got: %s", render)
	}
}

func TestSkillsPickerWrapAround(t *testing.T) {
	model := newTestSkillsModel(t, 5)
	updated, _ := model.Update(testCtrl('s'))
	model = updated.(*bubbleModel)
	updated, _ = model.Update(testKey(tea.KeyUp))
	model = updated.(*bubbleModel)
	view := model.panes.bottom.find(skillsViewID).(*skillsPaneView)
	if got := view.picker.GlobalIndex(); got != 4 {
		t.Fatalf("expected wrap-around index 4, got %d", got)
	}
	updated, _ = model.Update(testKey(tea.KeyDown))
	model = updated.(*bubbleModel)
	if got := view.picker.GlobalIndex(); got != 0 {
		t.Fatalf("expected wrap-around index 0, got %d", got)
	}
}

func TestSkillsPickerFastNavigation(t *testing.T) {
	model := newTestSkillsModel(t, 12)
	updated, _ := model.Update(testCtrl('s'))
	model = updated.(*bubbleModel)
	updated, _ = model.Update(testKey(tea.KeyPgDown))
	model = updated.(*bubbleModel)
	render := model.panes.bottom.renderTop(model)
	view := model.panes.bottom.find(skillsViewID).(*skillsPaneView)
	if got := view.picker.GlobalIndex(); got <= 0 {
		t.Fatalf("expected pgdown to advance selection, got index %d", got)
	}
	updated, _ = model.Update(testText("G"))
	model = updated.(*bubbleModel)
	if got := view.picker.GlobalIndex(); got != 11 {
		t.Fatalf("expected index 11 after G, got %d", got)
	}
	updated, _ = model.Update(testText("g"))
	model = updated.(*bubbleModel)
	if got := view.picker.GlobalIndex(); got != 0 {
		t.Fatalf("expected index 0 after g, got %d", got)
	}
	updated, _ = model.Update(testText("3"))
	model = updated.(*bubbleModel)
	if got := view.picker.GlobalIndex(); got != 2 {
		t.Fatalf("expected item 3 after number 3, got: %s", render)
	}
}

func TestComposerDraftPreservedOnHistoryNavigation(t *testing.T) {
	model := newTestSkillsModel(t, 2)
	model.panes.bottom.recordHistory("git status")
	model.panes.bottom.recordHistory("docker ps")
	draftText := "my half-written complex query"
	model.panes.bottom.prompt().SetValue(draftText)
	updated, _ := model.Update(testKey(tea.KeyUp))
	model = updated.(*bubbleModel)
	if model.panes.bottom.prompt().Value() != "docker ps" {
		t.Fatalf("expected 'docker ps' from history, got: %q", model.panes.bottom.prompt().Value())
	}
	updated, _ = model.Update(testKey(tea.KeyUp))
	model = updated.(*bubbleModel)
	if model.panes.bottom.prompt().Value() != "git status" {
		t.Fatalf("expected 'git status' from history, got: %q", model.panes.bottom.prompt().Value())
	}
	updated, _ = model.Update(testKey(tea.KeyDown))
	model = updated.(*bubbleModel)
	if model.panes.bottom.prompt().Value() != "docker ps" {
		t.Fatalf("expected 'docker ps', got: %q", model.panes.bottom.prompt().Value())
	}
	updated, _ = model.Update(testKey(tea.KeyDown))
	model = updated.(*bubbleModel)
	if model.panes.bottom.prompt().Value() != draftText {
		t.Fatalf("expected restored draft %q, got: %q", draftText, model.panes.bottom.prompt().Value())
	}
}

func TestSlashAutocompleteWrapAround(t *testing.T) {
	model := newTestSkillsModel(t, 8)
	model.panes.bottom.prompt().SetValue("/skill ")
	if !model.slashOpen() {
		t.Fatal("expected slash open for /skill ")
	}
	model.syncSlashView()
	state := model.slashState()
	if state == nil {
		t.Fatal("expected slash pane state")
	}
	_ = state.HandlePaneKey(newPaneRenderContext(model), testKey(tea.KeyUp))
	matches := model.slashMatches()
	if state.picker.Index() != len(matches)-1 {
		t.Fatalf("expected wrapped index %d, got %d", len(matches)-1, state.picker.Index())
	}
	_ = state.HandlePaneKey(newPaneRenderContext(model), testKey(tea.KeyDown))
	if state.picker.Index() != 0 {
		t.Fatalf("expected wrapped index 0, got %d", state.picker.Index())
	}
}

func TestSlashAutocompleteUsesBubblesListPresentation(t *testing.T) {
	model := newTestSkillsModel(t, 5)
	model.panes.bottom.prompt().SetValue("/skill ")
	if !model.slashOpen() {
		t.Fatal("expected slash open for /skill ")
	}
	rendered := model.renderSlash(0)
	for _, want := range []string{"[ ] skill-01", "[ ] skill-02", "Description for skill 01", "user"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("bubbles slash list missing %q:\n%s", want, rendered)
		}
	}
}

func TestSkillsCommandUsesPickerAsOnlyListSurface(t *testing.T) {
	model := newTestSkillsModel(t, 25)
	model.executeCommand("/skills")
	view := model.viewport.View()
	if strings.Contains(view, "skill-01") || strings.Contains(view, "more skills") || strings.Contains(view, "Agent Skills") {
		t.Fatalf("bare /skills should not dump list state into transcript, got:\n%s", view)
	}
	picker := model.panes.bottom.renderTop(model)
	if !strings.Contains(picker, "Skills · 0/25 active") || !strings.Contains(picker, "skill-01") {
		t.Fatalf("skills picker should own list presentation, got:\n%s", picker)
	}
}

func TestSkillsPickerKeepsComposerVisibleAndDraft(t *testing.T) {
	model := newTestSkillsModel(t, 5)
	draft := "keep this draft"
	model.panes.bottom.prompt().SetValue(draft)

	updated, _ := model.Update(testCtrl('s'))
	model = updated.(*bubbleModel)
	if !model.panes.bottom.has(skillsViewID) {
		t.Fatal("expected skills picker open")
	}
	if !model.panes.bottom.composerVisible() {
		t.Fatal("skills picker must not replace the composer")
	}
	view := testPlain(model.View().Content)
	if !strings.Contains(view, draft) || !strings.Contains(view, "Skills · 0/5 active") {
		t.Fatalf("skills picker and composer must render together: %q", view)
	}

	updated, _ = model.Update(testKey(tea.KeyEsc))
	model = updated.(*bubbleModel)
	if model.panes.bottom.has(skillsViewID) {
		t.Fatal("esc did not close skills picker")
	}
	if got := model.panes.bottom.prompt().Value(); got != draft {
		t.Fatalf("composer draft = %q, want %q", got, draft)
	}
}

func TestSkillsPickerCtrlCEscapesModal(t *testing.T) {
	model := newTestSkillsModel(t, 5)
	updated, _ := model.Update(testCtrl('s'))
	model = updated.(*bubbleModel)
	if !model.panes.bottom.has(skillsViewID) {
		t.Fatal("expected skills picker open")
	}
	updated, _ = model.Update(testCtrl('c'))
	model = updated.(*bubbleModel)
	if model.panes.bottom.has(skillsViewID) {
		t.Fatal("expected ctrl+c to close skills picker")
	}
}

func TestTranscriptOverlayQAndCtrlC(t *testing.T) {
	model := newTestSkillsModel(t, 2)
	model.panes.showTranscript = true
	updated, _ := model.Update(testText("q"))
	model = updated.(*bubbleModel)
	if model.panes.showTranscript {
		t.Fatal("expected 'q' to close transcript overlay")
	}
	model.panes.showTranscript = true
	updated, _ = model.Update(testCtrl('c'))
	model = updated.(*bubbleModel)
	if model.panes.showTranscript {
		t.Fatal("expected ctrl+c to close transcript overlay")
	}
}

func TestMessageHistoryIntegrityOnTurnCancel(t *testing.T) {
	bModel := newTestSkillsModel(t, 1)
	bModel.messages = append(bModel.messages, model.Message{Role: model.RoleUser, Content: "do something that will be cancelled"})
	updated, _ := bModel.Update(turnmsg.Done{Err: context.Canceled})
	bModel = updated.(*bubbleModel)
	if len(bModel.messages) != 0 {
		t.Fatalf("expected orphan user message to be rolled back on cancellation, got len=%d: %#v", len(bModel.messages), bModel.messages)
	}
}

func TestQueueClearedOnTurnCancel(t *testing.T) {
	model := newTestSkillsModel(t, 1)
	model.busy = true
	cancelled := false
	model.turnCancel = func() {
		cancelled = true
	}
	model.queue = []string{"next queued command 1", "next queued command 2"}
	updated, _ := model.Update(testCtrl('c'))
	model = updated.(*bubbleModel)
	if !cancelled {
		t.Fatal("expected turnCancel to be called")
	}
	if len(model.queue) != 0 {
		t.Fatalf("expected queue to be cleared on cancel, got: %v", model.queue)
	}
}

func TestMultilineTextareaDynamicExpansion(t *testing.T) {
	model := newTestSkillsModel(t, 1)
	model.resize(80, 24)
	model.panes.bottom.prompt().SetValue("hello")
	model.requestRelayout()
	model.reconcileLayout()
	if model.panes.bottom.prompt().Height() != 1 {
		t.Fatalf("expected height 1 for single line, got %d", model.panes.bottom.prompt().Height())
	}
	model.panes.bottom.prompt().SetValue("line 1\nline 2\nline 3")
	model.requestRelayout()
	model.reconcileLayout()
	if model.panes.bottom.prompt().Height() != 3 {
		t.Fatalf("expected height 3 for 3 lines, got %d", model.panes.bottom.prompt().Height())
	}
	model.panes.bottom.prompt().SetValue("1\n2\n3\n4\n5\n6")
	model.requestRelayout()
	model.reconcileLayout()
	if model.panes.bottom.prompt().Height() != 4 {
		t.Fatalf("expected height 4 for 6 lines, got %d", model.panes.bottom.prompt().Height())
	}
}

func TestSkillsPickerMouseWheelNavigation(t *testing.T) {
	model := newTestSkillsModel(t, 10)
	updated, _ := model.Update(testCtrl('s'))
	model = updated.(*bubbleModel)
	updated, _ = model.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	model = updated.(*bubbleModel)
	view := model.panes.bottom.find(skillsViewID).(*skillsPaneView)
	if got := view.picker.GlobalIndex(); got != 1 {
		t.Fatalf("expected index 1 after wheel down, got %d", got)
	}
	updated, _ = model.Update(tea.MouseWheelMsg{Button: tea.MouseWheelUp})
	model = updated.(*bubbleModel)
	if got := view.picker.GlobalIndex(); got != 0 {
		t.Fatalf("expected index 0 after wheel up, got %d", got)
	}
}

func TestHistoryStateTrimO1(t *testing.T) {
	state := NewHistoryState(10)
	for i := 1; i <= 15; i++ {
		state.Append(&UserCell{Text: fmt.Sprintf("msg %d", i)})
	}
	if state.LineCount() > 10 {
		t.Fatalf("expected state.LineCount() <= 10, got %d", state.LineCount())
	}
	if len(state.Committed()) > 10 {
		t.Fatalf("expected committed <= 10, got %d", len(state.Committed()))
	}
	total := 0
	for _, c := range state.Committed() {
		total += c.LineCount()
	}
	if state.LineCount() != total {
		t.Fatalf("cached line count %d != mathd %d", state.LineCount(), total)
	}
}

func TestStatusBarNeverWrapsOn80Columns(t *testing.T) {
	model := newTestSkillsModel(t, 5)
	model.resize(80, 24)
	model.skills.Activate("skill-with-a-very-long-descriptive-name")
	info := model.infoView()
	lines := strings.Split(info, "\n")
	if len(lines) > 1 {
		t.Fatalf("expected infoView to be strictly a single line, got %d lines: %s", len(lines), info)
	}
	visualWidth := ansi.StringWidth(info)
	if visualWidth > 80 {
		t.Fatalf("expected infoView width <= 80, got %d: %s", visualWidth, info)
	}
}

func TestShortcutMatrixGlobalKeysSurviveModalRouting(t *testing.T) {
	t.Run("permission lets transcript shortcut bubble", func(t *testing.T) {
		m := newTestBubbleModel(t, permission.ModeAsk, nil)
		m.panes.bottom.push(&permissionPaneView{})
		updated, _ := m.Update(testCtrl('t'))
		m = updated.(*bubbleModel)
		if !m.panes.showTranscript {
			t.Fatal("ctrl+t did not open transcript above permission pane")
		}
		if !m.panes.bottom.has(permissionViewID) {
			t.Fatal("transcript shortcut removed pending permission pane")
		}
	})
	t.Run("provider lets transcript shortcut bubble", func(t *testing.T) {
		m := newTestBubbleModel(t, permission.ModeAsk, nil)
		m.panes.bottom.push(newProviderPaneView())
		updated, _ := m.Update(testCtrl('t'))
		m = updated.(*bubbleModel)
		if !m.panes.showTranscript {
			t.Fatal("ctrl+t did not open transcript above provider pane")
		}
		if !m.panes.bottom.has(providerViewID) {
			t.Fatal("transcript shortcut unexpectedly closed provider pane")
		}
	})
	t.Run("provider keeps shift-tab local", func(t *testing.T) {
		m := newTestBubbleModel(t, permission.ModeAsk, nil)
		view := newProviderPaneView()
		view.focusIndex = 1
		view.syncInputFocus()
		m.panes.bottom.push(view)
		updated, _ := m.Update(testShiftTab())
		m = updated.(*bubbleModel)
		if view.focusIndex != 0 {
			t.Fatalf("shift+tab focus = %d, want previous provider field", view.focusIndex)
		}
		if m.planMode {
			t.Fatal("provider-local shift+tab leaked into global mode cycling")
		}
	})
}

func TestShortcutMatrixInterruptAndToggleSemantics(t *testing.T) {
	t.Run("ctrl-c closes modal before quitting", func(t *testing.T) {
		m := newTestBubbleModel(t, permission.ModeAsk, nil)
		m.panes.bottom.push(&skillsPaneView{})
		updated, cmd := m.Update(testCtrl('c'))
		m = updated.(*bubbleModel)
		if cmd != nil {
			t.Fatal("ctrl+c on modal returned quit command")
		}
		if m.panes.bottom.has(skillsViewID) {
			t.Fatal("ctrl+c did not close skills pane")
		}
	})
	t.Run("ctrl-p closes model setup through binding", func(t *testing.T) {
		m := newTestBubbleModel(t, permission.ModeAsk, nil)
		m.panes.bottom.push(&modelSetupPaneView{})
		updated, _ := m.Update(testCtrl('p'))
		m = updated.(*bubbleModel)
		if m.panes.bottom.has(modelSetupViewID) {
			t.Fatal("ctrl+p did not close model setup")
		}
	})
	t.Run("ctrl-c closes transcript overlay", func(t *testing.T) {
		m := newTestBubbleModel(t, permission.ModeAsk, nil)
		m.panes.showTranscript = true
		updated, cmd := m.Update(testCtrl('c'))
		m = updated.(*bubbleModel)
		if cmd != nil {
			t.Fatal("ctrl+c on transcript returned quit command")
		}
		if m.panes.showTranscript {
			t.Fatal("ctrl+c did not close transcript overlay")
		}
	})
}

func TestSlashCompletionUsesInlineCommandGrammar(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.resize(80, 24)
	m.panes.bottom.prompt().SetValue("/")
	m.syncSlashView()
	view := m.panes.bottom.find(slashViewID)
	if view == nil {
		t.Fatal("slash completion did not open")
	}
	plain := ansi.Strip(view.Render(newPaneRenderContext(m)))
	for _, want := range []string{"/help", "more", "↑↓ navigate", "tab complete", "esc close"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("slash completion missing %q: %q", want, plain)
		}
	}
}
