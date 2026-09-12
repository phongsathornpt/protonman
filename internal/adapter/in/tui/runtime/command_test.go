package runtime

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/runtime/reasoningpolicy"
	turnmsg "github.com/phongsathornpt/protonman/internal/adapter/in/tui/runtime/turn"
	"github.com/phongsathornpt/protonman/internal/adapter/out/config"
	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
	"github.com/phongsathornpt/protonman/internal/app"
	"github.com/phongsathornpt/protonman/internal/core/permission"
	"github.com/phongsathornpt/protonman/internal/core/tool"
	"github.com/phongsathornpt/protonman/internal/feature/skill"
	sdk "github.com/phongsathornpt/protonman/proton-sdk"
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

func TestTUICommandSurfaceIsCanonical(t *testing.T) {
	model := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	model.executeCommand("/help")
	help := plainTranscript(model)
	for _, keep := range []string{"/help", "/permission", "/low", "/model", "/provider", "/skills", "/agents", "/goal", "/todo", "/clear", "/resume", "/call", "/quit"} {
		if !strings.Contains(help, keep) {
			t.Fatalf("help missing canonical command %q: %q", keep, help)
		}
	}
	removed := []string{"tools", "project", "protonman", "config", "session", "sessions", "new", "agent", "profile", "subagent", "subagents", "reasoning", "thinking", "mode", "ask", "plan", "always-approve", "yolo", "models", "providers", "skill", "history", "transcript", "exit"}
	for _, name := range removed {
		if strings.Contains(help, "/"+name+" ") {
			t.Fatalf("help still advertises removed command /%s: %q", name, help)
		}
		model.executeCommand("/" + name)
		if !strings.Contains(plainTranscript(model), `unknown command "`+name+`"`) {
			t.Fatalf("removed /%s did not resolve as unknown", name)
		}
	}
}

func TestGoalCommandSetsShowsAndClearsFullGoal(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.executeCommand("/goal implement model-aware conversation compaction")
	if got := m.activeGoal; got != "implement model-aware conversation compaction" {
		t.Fatalf("active goal = %q", got)
	}
	m.executeCommand("/goal")
	if got := plainTranscript(m); !strings.Contains(got, "goal · implement model-aware conversation compaction") {
		t.Fatalf("goal transcript = %q", got)
	}
	m.executeCommand("/goal clear")
	if m.activeGoal != "" {
		t.Fatalf("active goal after clear = %q", m.activeGoal)
	}
}

func TestClearCommandResetsConversationButPreservesSessionControls(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.activeGoal = "finish compaction"
	m.activeProvider = "opencode"
	m.activeModel = "model-x"
	m.reasoningEffort = sdk.ReasoningHigh
	m.conversation.SetMessages([]model.Message{{Role: model.RoleUser, Content: "old context"}})
	m.conversation.Enqueue("queued prompt")
	m.conversationViewport.tailOnly = true
	m.conversationViewport.staleTail = true
	m.conversationViewport.lineAnchors = []ScrollAnchor{{}}
	m.panes.showTranscript = true
	m.appendUser("old context")

	m.executeCommand("/clear")
	if len(m.conversation.Messages()) != 0 || m.conversation.QueueLen() != 0 {
		t.Fatalf("conversation state not cleared: messages=%d queue=%d", len(m.conversation.Messages()), m.conversation.QueueLen())
	}
	if m.activeGoal != "finish compaction" || m.activeProvider != "opencode" || m.activeModel != "model-x" || m.reasoningEffort != sdk.ReasoningHigh {
		t.Fatalf("session controls changed: goal=%q provider=%q model=%q reasoning=%q", m.activeGoal, m.activeProvider, m.activeModel, m.reasoningEffort)
	}
	if m.conversationViewport.tailOnly || m.conversationViewport.staleTail || len(m.conversationViewport.lineAnchors) != 0 || !m.conversationViewport.following() {
		t.Fatalf("derived viewport state survived clear: %+v", m.conversationViewport)
	}
	if m.panes.showTranscript {
		t.Fatal("transcript overlay remained open after clear")
	}
	if got := plainTranscript(m); strings.Contains(got, "old context") || !strings.Contains(got, "conversation cleared") {
		t.Fatalf("transcript after clear = %q", got)
	}
	if m.service.Mode() != permission.ModeAsk {
		t.Fatalf("permission changed to %q", m.service.Mode())
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
	handled, _ := m.handlePaneKey(testKey(tea.KeyRight))
	if !handled || view.selectedReasoning() != sdk.ReasoningLow {
		t.Fatalf("right did not move thinking to low: %q", view.selectedReasoning())
	}
	if m.reasoningEffort != sdk.ReasoningDefault {
		t.Fatalf("pending setup mutated runtime before apply: %q", m.reasoningEffort)
	}
}

func TestUnifiedModelSetupShiftTabCyclesPermissionWithoutProviderLeak(t *testing.T) {
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
	updated, _ := m.Update(testShiftTab())
	m = updated.(*bubbleModel)
	if !m.planMode {
		t.Fatal("shift+tab did not cycle permission into plan mode")
	}
	if view.activeProviderName() != initialProvider {
		t.Fatalf("shift+tab changed provider from %q to %q", initialProvider, view.activeProviderName())
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

func TestRemoteModelReasoningSummaryUsesResolvedProfile(t *testing.T) {
	md := model.RemoteModel{ID: "gemini-3.8-flash"}
	if got := reasoningpolicy.Summary("protonman", md, true); got != "reasoning low/medium/high (default medium)" {
		t.Fatalf("summary = %q", got)
	}
	if got := reasoningpolicy.Summary("custom", model.RemoteModel{ID: "future-model"}, true); got != "" {
		t.Fatalf("unknown summary = %q, want empty", got)
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
		model.executeCommand("/skills")
		if !model.panes.bottom.has(skillsViewID) {
			t.Fatalf("expected /skills without args to open skills picker")
		}
		model.panes.bottom.remove(skillsViewID)
		model.executeCommand("/skills nonexistent")
		if !strings.Contains(model.viewport.View(), "skill \"nonexistent\" not found") {
			t.Fatalf("expected not found error")
		}
		model.executeCommand("/skills pdf-processing")
		content := plainTranscript(model)
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
		if rendered := model.panes.bottom.renderTop(model); !strings.Contains(rendered, "Skills") || !strings.Contains(rendered, "1/1 active") || !strings.Contains(rendered, "[x] pdf-processing") {
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
		model.executeCommand("/skills toggle pdf-processing")
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
		model.executeCommand("/skills toggle pdf-processing")
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
		model.executeCommand("/skills active")
		if !strings.Contains(model.viewport.View(), "Active Agent Skills (1):") {
			t.Fatalf("expected /skills active to list active skills")
		}
		initialMsgCount := len(model.conversation.Messages())
		model.executeCommand("/skills pdf-processing")
		if !strings.Contains(model.viewport.View(), "is already active") {
			t.Fatalf("expected already active message on duplicate activation")
		}
		if len(model.conversation.Messages()) != initialMsgCount {
			t.Fatalf("messages count increased on duplicate activation: %d != %d", len(model.conversation.Messages()), initialMsgCount)
		}
		model.executeCommand("/skills deactivate pdf-processing")
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
		model.executeCommand("/skills disable pdf-processing")
		if model.skills.IsActivated("pdf-processing") {
			t.Fatalf("expected skill to be deactivated via /skills disable")
		}
	})
	t.Run("skills lock and check in tui", func(t *testing.T) {
		origSkills := model.skills
		origWorkDir := model.workDir
		defer func() {
			model.skills = origSkills
			model.workDir = origWorkDir
		}()

		tempDir := t.TempDir()
		skillDir := filepath.Join(tempDir, "git-helper")
		if err := os.MkdirAll(skillDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("instructions"), 0o644); err != nil {
			t.Fatal(err)
		}
		projSkill := skill.Skill{
			Name:        "git-helper",
			Description: "Git helper tools",
			Scope:       skill.ScopeProject,
			BaseDir:     skillDir,
			Location:    filepath.Join(skillDir, "SKILL.md"),
		}
		model.skills = skill.NewRegistry(projSkill)
		model.workDir = tempDir

		model.executeCommand("/skills check")
		view := model.viewport.View()
		if !strings.Contains(view, "No project skill lock found") {
			t.Fatalf("expected no lock message, got: %s", view)
		}

		model.executeCommand("/skills lock")
		view = model.viewport.View()
		if !strings.Contains(view, "Locked 1 project skill(s)") {
			t.Fatalf("expected locked message, got: %s", view)
		}

		model.executeCommand("/skills check")
		view = model.viewport.View()
		if !strings.Contains(view, "[verified] git-helper") || !strings.Contains(view, "All locked skills verified cleanly") {
			t.Fatalf("expected verified message, got: %s", view)
		}

		if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("modified"), 0o644); err != nil {
			t.Fatal(err)
		}
		model.executeCommand("/skills check")
		view = model.viewport.View()
		if !strings.Contains(view, "[drifted]  git-helper") {
			t.Fatalf("expected drifted message, got: %s", view)
		}
	})
	t.Run("skill name autocomplete in composer", func(t *testing.T) {
		model.panes.bottom.remove(skillsViewID)
		model.panes.bottom.prompt().SetValue("/skills ")
		if !model.slashOpen() {
			t.Fatal("slash dropdown did not open for /skills ")
		}
		matches := model.slashMatches()
		if len(matches) == 0 {
			t.Fatal("expected matches for /skills ")
		}
		model.panes.bottom.prompt().SetValue("/skills pd")
		matches = model.slashMatches()
		if len(matches) != 1 || matches[0].Name != "pdf-processing" {
			t.Fatalf("expected pdf-processing match, got: %#v", matches)
		}
		applied, cmd := model.acceptSlash(false)
		if !applied || cmd != nil {
			t.Fatalf("acceptSlash failed: applied=%v, cmd=%v", applied, cmd)
		}
		if got := model.panes.bottom.prompt().Value(); got != "/skills pdf-processing" {
			t.Fatalf("prompt value after accept = %q, want /skills pdf-processing", got)
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
	t.Run("skill activation does not append user message and does not flood instructions", func(t *testing.T) {
		model.conversation.SetMessages(nil)
		model.skills.Deactivate("pdf-processing")
		model.executeCommand("/skills pdf-processing")
		content := model.viewport.View()
		if !strings.Contains(content, "[x] Activated skill pdf-processing [user]: Extract PDF text") {
			t.Fatalf("expected activation message with description, got: %s", content)
		}
		if strings.Contains(content, "# PDF Processing Guide") {
			t.Fatalf("did not expect raw instructions markdown in viewport")
		}
		if len(model.conversation.Messages()) != 0 {
			t.Fatalf("expected 0 messages appended to model.messages, got %d", len(model.conversation.Messages()))
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
	t.Run("skill selection persists to project if .protonman exists else global", func(t *testing.T) {
		homeDir := t.TempDir()
		t.Setenv("PROTONMAN_HOME", homeDir)

		// 1. Without .protonman in project: saves to global
		noProjectDir := t.TempDir()
		m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
		m.workDir = noProjectDir
		s1 := skill.Skill{Name: "global-skill", Description: "Global", Scope: skill.ScopeUser}
		m.skills = skill.NewRegistry(s1)
		attachTestApplication(t, m)

		m.executeCommand("/skills global-skill")
		if !m.skills.IsActivated("global-skill") {
			t.Fatal("expected global-skill to be activated")
		}

		snap, err := config.Load(context.Background(), config.Options{HomeDir: homeDir, WorkDir: noProjectDir})
		if err != nil {
			t.Fatal(err)
		}
		if len(snap.Skills.Active) != 1 || snap.Skills.Active[0] != "global-skill" {
			t.Fatalf("expected global skills to have global-skill, got: %v", snap.Skills.Active)
		}
		if snap.Provenance[config.FieldSkillsActive] != config.SourceUser {
			t.Fatalf("expected provenance user, got: %s", snap.Provenance[config.FieldSkillsActive])
		}

		// 2. With .protonman in project: saves to project
		projectDir := t.TempDir()
		if err := os.MkdirAll(filepath.Join(projectDir, ".protonman"), 0o755); err != nil {
			t.Fatal(err)
		}
		m2 := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
		m2.workDir = projectDir
		s2 := skill.Skill{Name: "project-skill", Description: "Project", Scope: skill.ScopeProject}
		m2.skills = skill.NewRegistry(s2)
		attachTestApplication(t, m2)

		m2.executeCommand("/skills project-skill")
		if !m2.skills.IsActivated("project-skill") {
			t.Fatal("expected project-skill to be activated")
		}

		snap2, err := config.Load(context.Background(), config.Options{HomeDir: homeDir, WorkDir: projectDir, ProjectTrusted: true})
		if err != nil {
			t.Fatal(err)
		}
		if len(snap2.Skills.Active) != 1 || snap2.Skills.Active[0] != "project-skill" {
			t.Fatalf("expected project skills to have project-skill, got: %v", snap2.Skills.Active)
		}
		if snap2.Provenance[config.FieldSkillsActive] != config.SourceProject {
			t.Fatalf("expected provenance project, got: %s", snap2.Provenance[config.FieldSkillsActive])
		}

		// 3. Toggle via picker pane in project
		m2.applyPaneAction(paneAction{kind: paneActionToggleSkill, skillName: "project-skill"})
		if m2.skills.IsActivated("project-skill") {
			t.Fatal("expected project-skill to be deactivated after toggle")
		}
		snap3, err := config.Load(context.Background(), config.Options{HomeDir: homeDir, WorkDir: projectDir, ProjectTrusted: true})
		if err != nil {
			t.Fatal(err)
		}
		if len(snap3.Skills.Active) != 0 {
			t.Fatalf("expected project skills to be empty after deactivating, got: %v", snap3.Skills.Active)
		}
	})
}

func newTestSkillsModel(t *testing.T, count int) *bubbleModel {
	t.Helper()
	registry := behaviorRegistry{handler: &countingHandler{definition: tool.Definition{Name: "read", Kind: tool.KindRead}}}
	service := newBehaviorService(t, registry, permission.ModeAsk)
	model := newBubbleModel(context.Background(), service, registry, nil, nil, newPermissionBridge(), "")
	attachTestApplication(t, model)
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
	model.panes.bottom.prompt().SetValue("/skills ")
	if !model.slashOpen() {
		t.Fatal("expected slash open for /skills ")
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
	model.panes.bottom.prompt().SetValue("/skills ")
	if !model.slashOpen() {
		t.Fatal("expected slash open for /skills ")
	}
	view := &slashPaneView{}
	view.sync(newPaneRenderContext(model))
	rendered := view.Render(newPaneRenderContext(model))
	for _, want := range []string{"[ ] skill-01", "[ ] skill-02", "Description for skill 01", "user"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("bubbles slash list missing %q:\n%s", want, rendered)
		}
	}
}

func TestSlashPaneRenderIsSideEffectFree(t *testing.T) {
	model := newTestSkillsModel(t, 5)
	model.panes.bottom.prompt().SetValue("/skills ")
	if !model.slashOpen() {
		t.Fatal("expected slash open for /skills ")
	}
	model.syncSlashView()
	view := model.slashState()
	if view == nil {
		t.Fatal("expected slash pane state")
	}
	first := view.Render(newPaneRenderContext(model))
	matchesBefore := append([]slashCommand(nil), view.matches...)
	indexBefore := view.picker.Index()
	second := view.Render(newPaneRenderContext(model))
	if first != second {
		t.Fatalf("repeated Render diverged:\n%s\n---\n%s", first, second)
	}
	if len(view.matches) != len(matchesBefore) || view.picker.Index() != indexBefore {
		t.Fatal("Render mutated slash pane state")
	}
	unsynced := &slashPaneView{}
	if got := unsynced.Render(newPaneRenderContext(model)); got != "" {
		t.Fatalf("unsynced slash Render = %q, want empty", got)
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
	if !strings.Contains(picker, "Skills") || !strings.Contains(picker, "0/25 active") || !strings.Contains(picker, "skill-01") {
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
	if !strings.Contains(view, draft) || !strings.Contains(view, "Skills") || !strings.Contains(view, "0/5 active") {
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
	bModel.conversation.AppendMessages(model.Message{Role: model.RoleUser, Content: "do something that will be cancelled"})
	updated, _ := bModel.Update(turnmsg.Done{Err: context.Canceled})
	bModel = updated.(*bubbleModel)
	if len(bModel.conversation.Messages()) != 0 {
		t.Fatalf("expected orphan user message to be rolled back on cancellation, got len=%d: %#v", len(bModel.conversation.Messages()), bModel.conversation.Messages())
	}
}

func TestQueueClearedOnTurnCancel(t *testing.T) {
	model := newTestSkillsModel(t, 1)
	model.busy = true
	cancelled := false
	model.turnCancel = func() {
		cancelled = true
	}
	model.conversation.Enqueue("next queued command 1")
	model.conversation.Enqueue("next queued command 2")
	updated, _ := model.Update(testCtrl('c'))
	model = updated.(*bubbleModel)
	if !cancelled {
		t.Fatal("expected turnCancel to be called")
	}
	if model.conversation.QueueLen() != 0 {
		t.Fatalf("expected queue to be cleared on cancel, got: %v", model.conversation.Queue())
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

func TestLowConcurrencyPickerMouseWheelNavigation(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.executeCommand("/low")
	view := m.panes.bottom.find(lowConcurrencyViewID).(*lowConcurrencyPaneView)
	updated, _ := m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	m = updated.(*bubbleModel)
	if got := view.index; got != 1 {
		t.Fatalf("expected index 1 after wheel down, got %d", got)
	}
	updated, _ = m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelUp})
	m = updated.(*bubbleModel)
	if got := view.index; got != 0 {
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

func TestShortcutMatrixBlockingPanesOwnGlobalKeys(t *testing.T) {
	t.Run("permission blocks transcript shortcut", func(t *testing.T) {
		m := newTestBubbleModel(t, permission.ModeAsk, nil)
		m.panes.bottom.push(&permissionPaneView{})
		updated, _ := m.Update(testCtrl('t'))
		m = updated.(*bubbleModel)
		if m.panes.showTranscript {
			t.Fatal("ctrl+t escaped pending permission pane")
		}
		if !m.panes.bottom.has(permissionViewID) {
			t.Fatal("transcript shortcut removed pending permission pane")
		}
	})
	t.Run("provider blocks transcript shortcut", func(t *testing.T) {
		m := newTestBubbleModel(t, permission.ModeAsk, nil)
		m.panes.bottom.push(newProviderPaneView())
		updated, _ := m.Update(testCtrl('t'))
		m = updated.(*bubbleModel)
		if m.panes.showTranscript {
			t.Fatal("ctrl+t escaped provider editor")
		}
		if !m.panes.bottom.has(providerViewID) {
			t.Fatal("transcript shortcut unexpectedly closed provider pane")
		}
	})
	t.Run("provider reserves shift-tab for permission", func(t *testing.T) {
		m := newTestBubbleModel(t, permission.ModeAsk, nil)
		view := newProviderPaneView()
		view.focusIndex = 1
		view.syncInputFocus()
		m.panes.bottom.push(view)
		updated, _ := m.Update(testShiftTab())
		m = updated.(*bubbleModel)
		if view.focusIndex != 1 {
			t.Fatalf("shift+tab changed provider focus to %d", view.focusIndex)
		}
		if !m.planMode {
			t.Fatal("shift+tab did not cycle global permission")
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
	for _, want := range []string{"/help", "/permission", "↑/↓", "navigate", "tab", "complete", fmt.Sprintf("1/%d", len(slashCatalog))} {
		if !strings.Contains(plain, want) {
			t.Fatalf("slash completion missing %q: %q", want, plain)
		}
	}
}

func TestSlashCompletionTypingPreservesPrintableKeys(t *testing.T) {
	// The slash pane renders below the composer and shares its draft, so every
	// printable key the user types must reach the textarea. Regression: vim-style
	// navigation bindings ("j", "k", "g", "G") and the "q" close key used to be
	// claimed by the picker, so typing "/goal" produced "/oal".
	for _, want := range []string{"/goal", "/todo", "/quit", "/agents", "/skills", "/help"} {
		m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
		m.resize(80, 24)
		for _, ch := range want {
			updated, _ := m.Update(testText(string(ch)))
			m = updated.(*bubbleModel)
		}
		if got := m.panes.bottom.prompt().Value(); got != want {
			t.Fatalf("typing %q produced composer value %q", want, got)
		}
		if !m.slashOpen() {
			t.Fatalf("slash completion closed while typing %q", want)
		}
	}
}

func TestSlashCompletionKeepsNavigationAndCloseWorking(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.resize(80, 24)
	m.panes.bottom.prompt().SetValue("/")
	m.syncSlashView()
	if !m.slashOpen() {
		t.Fatal("slash completion did not open")
	}
	updated, _ := m.Update(testKey(tea.KeyDown))
	m = updated.(*bubbleModel)
	if got := m.slashState().picker.Index(); got != 1 {
		t.Fatalf("down did not move slash selection: index=%d", got)
	}
	updated, _ = m.Update(testKey(tea.KeyUp))
	m = updated.(*bubbleModel)
	if got := m.slashState().picker.Index(); got != 0 {
		t.Fatalf("up did not move slash selection: index=%d", got)
	}
	updated, _ = m.Update(testKey(tea.KeyEnter))
	m = updated.(*bubbleModel)
	if m.panes.bottom.has(slashViewID) {
		t.Fatal("enter did not accept and close slash completion")
	}
}

func TestSlashPickerRendersBelowComposerLikeModelPicker(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.resize(100, 30)
	m.panes.bottom.prompt().SetValue("/")
	m.syncSlashView()
	m.requestRelayout()
	m.reconcileLayout()
	plain := ansi.Strip(m.View().Content)
	composer := strings.Index(plain, "> /")
	commands := strings.Index(plain, "/help")
	if composer < 0 || commands < 0 || composer >= commands {
		t.Fatalf("slash picker should render below composer: composer=%d commands=%d\n%s", composer, commands, plain)
	}
	for _, want := range []string{"↑/↓", "navigate", "enter", "select", "tab", "complete", "esc", "go back", fmt.Sprintf("1/%d", len(slashCatalog))} {
		if !strings.Contains(plain, want) {
			t.Fatalf("slash picker missing reference element %q:\n%s", want, plain)
		}
	}
}

func TestControlCommandsDoNotEchoAsUserConversation(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	for _, line := range []string{"/goal compact safely", "/goal", "/permission", "/model"} {
		m.dispatch(line)
	}
	for _, cell := range m.ensureHistoryState().Cells() {
		if user, ok := cell.(*UserCell); ok && strings.HasPrefix(strings.TrimSpace(user.Text), "/") {
			t.Fatalf("control command leaked into user transcript: %q", user.Text)
		}
	}
}

func TestSlashPickerHelpAndPagingShareOneLine(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.resize(80, 24)
	m.panes.bottom.prompt().SetValue("/")
	m.syncSlashView()
	view := m.panes.bottom.find(slashViewID)
	if view == nil {
		t.Fatal("slash completion did not open")
	}
	plain := ansi.Strip(view.Render(newPaneRenderContext(m)))
	lines := strings.Split(plain, "\n")
	for i, line := range lines {
		if !strings.Contains(line, "↑/↓") {
			continue
		}
		if !strings.Contains(line, fmt.Sprintf("1/%d", len(slashCatalog))) {
			t.Fatalf("slash help and paging split across lines: %q", plain)
		}
		if i > 0 && strings.TrimSpace(lines[i-1]) == "" {
			t.Fatalf("blank row before slash help: %q", plain)
		}
		return
	}
	t.Fatalf("slash help line missing: %q", plain)
}

func TestLowCommandControlsSessionLowConcurrencyMode(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.activeProvider = model.DefaultOpenCodeName
	m.activeModel = "nemotron-3.5-lightning-free"

	if got := m.lowConcurrencyMode; got != model.LowConcurrencyAuto {
		t.Fatalf("initial low concurrency = %s, want auto", got)
	}
	before := plainTranscript(m)
	m.executeCommand("/low")
	if got := m.lowConcurrencyMode; got != model.LowConcurrencyAuto {
		t.Fatalf("/low inspect mutated mode to %s", got)
	}
	if !m.panes.bottom.has(lowConcurrencyViewID) {
		t.Fatal("/low did not open low concurrency picker")
	}
	if got := plainTranscript(m); got != before {
		t.Fatalf("/low picker polluted transcript: before=%q after=%q", before, got)
	}
	m.panes.bottom.remove(lowConcurrencyViewID)
	m.executeCommand("/low on")
	if got := m.lowConcurrencyMode; got != model.LowConcurrencyOn {
		t.Fatalf("/low on = %s, want on", got)
	}
	m.executeCommand("/low off")
	if got := m.lowConcurrencyMode; got != model.LowConcurrencyOff {
		t.Fatalf("/low off = %s, want off", got)
	}
	m.executeCommand("/low auto")
	if got := m.lowConcurrencyMode; got != model.LowConcurrencyAuto {
		t.Fatalf("/low auto = %s, want auto", got)
	}
	if got := plainTranscript(m); got != before {
		t.Fatalf("low setting feedback polluted transcript: %q", got)
	}
	if !strings.Contains(m.transientNotice, "low concurrency · auto · effective on") {
		t.Fatalf("low setting transient notice = %q", m.transientNotice)
	}
}

func TestLowCommandRejectsChangeDuringActiveTurn(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.busy = true
	m.executeCommand("/low on")
	if m.lowConcurrencyMode != model.LowConcurrencyAuto {
		t.Fatalf("busy /low changed mode to %s", m.lowConcurrencyMode)
	}
	if got := plainTranscript(m); !strings.Contains(got, "cannot change low concurrency mode") {
		t.Fatalf("busy /low error missing: %q", got)
	}
}

func TestLowConcurrencyStateSurvivesBubbleModelRestartCapture(t *testing.T) {
	ui := &BubbleTeaUI{lowConcurrencyMode: model.LowConcurrencyOn}
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.lowConcurrencyMode = ui.lowConcurrencyMode
	if m.lowConcurrencyMode != model.LowConcurrencyOn {
		t.Fatalf("restored low concurrency = %s, want on", m.lowConcurrencyMode)
	}
	ui.lowConcurrencyMode = m.lowConcurrencyMode
	if ui.lowConcurrencyMode != model.LowConcurrencyOn {
		t.Fatalf("captured low concurrency = %s, want on", ui.lowConcurrencyMode)
	}
}

func TestResumeCommandBusyRejection(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.busy = true
	m.executeCommand("/resume")
	if got := plainTranscript(m); !strings.Contains(got, "cannot switch session while a turn is running") {
		t.Fatalf("busy /resume error missing: %q", got)
	}
}

func TestResumeCommandOpensPicker(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	sessService := app.NewMemorySessions()
	ctx := context.Background()
	_ = sessService.SaveCurrent(ctx, app.SessionDetail{
		ID:           "prev-sess-1",
		WorkspaceKey: "ws-test",
		ActiveGoal:   "earlier goal",
	})
	m.sessions = sessService
	m.workspaceKey = "ws-test"
	m.sessionID = "current-sess"

	m.executeCommand("/resume")
	if !m.panes.bottom.has(sessionResumeViewID) {
		t.Fatal("/resume did not open session resume picker pane")
	}
	pane := m.panes.bottom.find(sessionResumeViewID)
	if pane == nil {
		t.Fatal("session resume pane is nil")
	}
	view, ok := pane.(*sessionResumePaneView)
	if !ok || len(view.items) != 1 || view.items[0].summary.ID != "prev-sess-1" {
		t.Fatalf("unexpected session resume pane items: %+v", view)
	}
}

func TestResumeCommandDirectID(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	sessService := app.NewMemorySessions()
	ctx := context.Background()
	_ = sessService.SaveCurrent(ctx, app.SessionDetail{
		ID:           "target-sess",
		WorkspaceKey: "ws-test",
		ActiveGoal:   "resumed objective",
		Messages: []model.Message{
			{Role: model.RoleUser, Content: "earlier user request"},
		},
	})
	m.sessions = sessService
	m.workspaceKey = "ws-test"
	m.sessionID = "current-sess"
	m.activeGoal = "current goal"

	m.executeCommand("/resume target-sess")
	if m.sessionID != "target-sess" {
		t.Fatalf("sessionID = %q, want target-sess", m.sessionID)
	}
	if m.activeGoal != "resumed objective" {
		t.Fatalf("activeGoal = %q, want resumed objective", m.activeGoal)
	}
	transcript := plainTranscript(m)
	if !strings.Contains(transcript, "earlier user request") {
		t.Fatalf("transcript missing resumed message: %q", transcript)
	}
	if !strings.Contains(transcript, "resumed session target-sess") {
		t.Fatalf("transcript missing confirmation: %q", transcript)
	}
}

func TestResumeCommandLatest(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	sessService := app.NewMemorySessions()
	ctx := context.Background()
	_ = sessService.SaveCurrent(ctx, app.SessionDetail{
		ID:           "workspace-ws-test-latest",
		WorkspaceKey: "ws-test",
		ActiveGoal:   "latest goal",
	})
	m.sessions = sessService
	m.workspaceKey = "ws-test"
	m.sessionID = "current-sess"

	m.executeCommand("/resume latest")
	if m.sessionID != "workspace-ws-test-latest" {
		t.Fatalf("sessionID = %q, want workspace-ws-test-latest", m.sessionID)
	}
	if m.activeGoal != "latest goal" {
		t.Fatalf("activeGoal = %q, want latest goal", m.activeGoal)
	}
}

func TestResumeCommandAnySession(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	sessService := app.NewMemorySessions()
	ctx := context.Background()
	_ = sessService.SaveCurrent(ctx, app.SessionDetail{
		ID:           "other-sess",
		WorkspaceKey: "other-workspace",
		ActiveGoal:   "other goal",
	})
	m.sessions = sessService
	m.workspaceKey = "my-workspace"
	m.sessionID = "current-sess"

	m.executeCommand("/resume other-sess")
	if m.sessionID != "other-sess" {
		t.Fatalf("sessionID = %q, want other-sess", m.sessionID)
	}
	if m.activeGoal != "other goal" {
		t.Fatalf("activeGoal = %q, want other goal", m.activeGoal)
	}
}

func TestResumePaneNavigationAndSelect(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	sessService := app.NewMemorySessions()
	ctx := context.Background()
	_ = sessService.SaveCurrent(ctx, app.SessionDetail{
		ID:           "sess-1",
		WorkspaceKey: "ws-test",
		ActiveGoal:   "goal-1",
	})
	_ = sessService.SaveCurrent(ctx, app.SessionDetail{
		ID:           "sess-2",
		WorkspaceKey: "ws-test",
		ActiveGoal:   "goal-2",
	})
	m.sessions = sessService
	m.workspaceKey = "ws-test"
	m.sessionID = "current-sess"

	m.executeCommand("/resume")
	if !m.panes.bottom.has(sessionResumeViewID) {
		t.Fatal("session resume pane not opened")
	}

	pane := m.panes.bottom.find(sessionResumeViewID)
	view, ok := pane.(*sessionResumePaneView)
	if !ok || len(view.items) != 2 {
		t.Fatalf("unexpected items in resume view: %+v", view)
	}

	// Down arrow to second item
	res := view.HandlePaneKey(newPaneRenderContext(m), tea.KeyPressMsg{Code: tea.KeyDown})
	if !res.handled {
		t.Fatal("Down key not handled")
	}
	if view.picker.Index() != 1 {
		t.Fatalf("view.picker.Index() after Down = %d, want 1", view.picker.Index())
	}

	// Confirm (Enter) on second item
	res = view.HandlePaneKey(newPaneRenderContext(m), tea.KeyPressMsg{Code: tea.KeyEnter})
	if !res.handled || res.action.kind != paneActionResumeSession {
		t.Fatalf("unexpected confirm action: %+v", res.action)
	}
	_ = m.applyPaneAction(res.action)

	if m.sessionID != view.items[1].summary.ID {
		t.Fatalf("m.sessionID after resume = %q, want %q", m.sessionID, view.items[1].summary.ID)
	}
	if m.panes.bottom.has(sessionResumeViewID) {
		t.Fatal("session resume pane still open after resume")
	}
}

func TestResumeCurrentSessionClosesPane(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	sessService := app.NewMemorySessions()
	ctx := context.Background()
	_ = sessService.SaveCurrent(ctx, app.SessionDetail{
		ID:           "current-sess",
		WorkspaceKey: "ws-test",
		ActiveGoal:   "my goal",
	})
	m.sessions = sessService
	m.workspaceKey = "ws-test"
	m.sessionID = "current-sess"

	m.executeCommand("/resume")
	if !m.panes.bottom.has(sessionResumeViewID) {
		t.Fatal("session resume pane not opened")
	}

	// Resume current session directly
	m.executeCommand("/resume current-sess")
	if m.panes.bottom.has(sessionResumeViewID) {
		t.Fatal("session resume pane still open after selecting current session")
	}
	if !strings.Contains(plainTranscript(m), "already in session current-sess") {
		t.Fatalf("expected already in session message: %q", plainTranscript(m))
	}
}
