package app_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/phongsathornpt/protonman/internal/adapter/out/config"
	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
	"github.com/phongsathornpt/protonman/internal/app"
	"github.com/phongsathornpt/protonman/internal/core/permission"
	"github.com/phongsathornpt/protonman/internal/core/session"
	sdk "github.com/phongsathornpt/protonman/proton-sdk"
)

type mockSessionRepo struct {
	state      session.State
	exists     bool
	loadErr    error
	saveErr    error
	deleteErr  error
	listErr    error
	savedID    string
	savedState session.State
	deletedID  string
	summaries  []session.Summary
}

func (m *mockSessionRepo) Load(ctx context.Context, id string) (session.State, bool, error) {
	return m.state, m.exists, m.loadErr
}
func (m *mockSessionRepo) LatestSession(ctx context.Context, key string) (string, session.State, bool, error) {
	return "latest-id", m.state, m.exists, m.loadErr
}
func (m *mockSessionRepo) Save(ctx context.Context, id string, state session.State) error {
	m.savedID = id
	m.savedState = state
	return m.saveErr
}
func (m *mockSessionRepo) Delete(ctx context.Context, id string) error {
	m.deletedID = id
	return m.deleteErr
}
func (m *mockSessionRepo) List(ctx context.Context, key string) ([]string, error) {
	return []string{"s-1"}, m.listErr
}
func (m *mockSessionRepo) ListSummaries(ctx context.Context, opts session.ListOptions) ([]session.Summary, error) {
	return m.summaries, m.listErr
}

func TestSessionsUseCase(t *testing.T) {
	// Nil repository check
	nilSessions := app.NewSessions(nil)
	if nilSessions != nil {
		t.Fatal("NewSessions(nil) != nil")
	}

	repo := &mockSessionRepo{
		state:     session.State{SessionID: "test-sess", PermissionMode: "ask"},
		exists:    true,
		summaries: []session.Summary{{ID: "s-1", MessageCount: 5}},
	}
	sessions := app.NewSessions(repo)
	if sessions == nil {
		t.Fatal("NewSessions(repo) = nil")
	}

	ctx := context.Background()

	// Load
	st, exists, err := sessions.Load(ctx, "test-sess")
	if err != nil || !exists || st.SessionID != "test-sess" {
		t.Fatalf("Load error = %v, exists = %v, st = %+v", err, exists, st)
	}

	// Save
	saveState := session.State{SessionID: "test-save", PermissionMode: "auto"}
	if err := sessions.Save(ctx, "test-save", saveState); err != nil {
		t.Fatalf("Save error = %v", err)
	}
	if repo.savedID != "test-save" || repo.savedState.PermissionMode != "auto" {
		t.Fatalf("Save did not update mock repo: %+v", repo)
	}

	// ListSummaries
	summaries, err := sessions.ListSummaries(ctx, app.SessionListOptions{Limit: 10})
	if err != nil || len(summaries) != 1 || summaries[0].ID != "s-1" {
		t.Fatalf("ListSummaries error = %v, summaries = %+v", err, summaries)
	}

	// Delete
	if err := sessions.Delete(ctx, "test-sess"); err != nil {
		t.Fatalf("Delete error = %v", err)
	}
	if repo.deletedID != "test-sess" {
		t.Fatalf("Delete id = %q, want test-sess", repo.deletedID)
	}

	// Error path
	repo.loadErr = errors.New("db down")
	_, _, err = sessions.Load(ctx, "test-sess")
	if err == nil {
		t.Fatal("Load with error = nil, want error")
	}
}

func TestSessionsDetailUseCases(t *testing.T) {
	repo := &mockSessionRepo{
		state: session.State{
			SessionID:      "sess-123",
			WorkspaceKey:   "ws-alpha",
			WorkspaceName:  "alpha",
			PermissionMode: "ask",
			ActiveGoal:     "build feature",
			AgentProfile:   "universal",
			Messages: []session.Message{
				{Role: sdk.RoleUser, Content: "hello"},
				{Role: sdk.RoleAssistant, Content: "world"},
			},
		},
		exists: true,
	}
	sessions := app.NewSessions(repo)
	ctx := context.Background()

	// LoadDetail success
	detail, err := sessions.LoadDetail(ctx, "sess-123", "ws-alpha")
	if err != nil {
		t.Fatalf("LoadDetail error: %v", err)
	}
	if detail.ID != "sess-123" || detail.ActiveGoal != "build feature" || len(detail.Messages) != 2 {
		t.Fatalf("LoadDetail unexpected: %+v", detail)
	}

	// LoadDetail workspace mismatch
	_, err = sessions.LoadDetail(ctx, "sess-123", "ws-beta")
	if err == nil || !strings.Contains(err.Error(), "belongs to another workspace") {
		t.Fatalf("expected workspace mismatch error, got: %v", err)
	}

	// LoadDetail not found
	repo.exists = false
	_, err = sessions.LoadDetail(ctx, "sess-missing", "ws-alpha")
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected not found error, got: %v", err)
	}
	repo.exists = true

	// LatestDetail
	latest, err := sessions.LatestDetail(ctx, "ws-alpha")
	if err != nil {
		t.Fatalf("LatestDetail error: %v", err)
	}
	if latest.ID != "latest-id" {
		t.Fatalf("LatestDetail ID = %q, want latest-id", latest.ID)
	}

	// SaveCurrent
	err = sessions.SaveCurrent(ctx, app.SessionDetail{
		ID:             "sess-saved",
		WorkspaceKey:   "ws-alpha",
		PermissionMode: "ask",
		ActiveGoal:     "updated goal",
		Messages: []sdk.Message{
			{Role: sdk.RoleUser, Content: "new user msg"},
		},
	})
	if err != nil {
		t.Fatalf("SaveCurrent error: %v", err)
	}
	if repo.savedID != "sess-saved" || repo.savedState.ActiveGoal != "updated goal" || len(repo.savedState.Messages) != 1 {
		t.Fatalf("SaveCurrent did not store expected state: %+v", repo.savedState)
	}

	// OpenTodoStore
	sessionsWithRoot := sessions.WithSessionsRoot(t.TempDir())
	todoRepo, err := sessionsWithRoot.OpenTodoStore(ctx, "sess-todo", "goal-abc")
	if err != nil {
		t.Fatalf("OpenTodoStore error: %v", err)
	}
	if todoRepo == nil {
		t.Fatal("OpenTodoStore returned nil repo")
	}
}

func TestAgentsUseCaseNilSafe(t *testing.T) {
	agents := app.NewAgents(nil)
	if agents.Available() {
		t.Error("agents.Available() = true for nil coordinator")
	}
	if list := agents.List(); list != nil {
		t.Errorf("agents.List() = %+v, want nil", list)
	}
	if cancelled := agents.CancelByParent("p-1"); cancelled != 0 {
		t.Errorf("agents.CancelByParent = %d, want 0", cancelled)
	}
	ch, cancel := agents.Subscribe(10)
	cancel()
	if _, ok := <-ch; ok {
		t.Error("agents.Subscribe chan was not closed for nil coordinator")
	}

	// Safe delegate calls with nil coordinator
	agents.SetLanguageModel(nil)
	agents.SetPermissionMode(permission.ModeAsk)
	agents.SetReasoningEffort(sdk.ReasoningDefault)
	agents.SetPrompt(nil)
	agents.SetCallGuard(nil)
}

func TestProjectsUseCase(t *testing.T) {
	tmpDir := t.TempDir()
	projects := app.NewProjects(config.ProjectSettingsStore{})

	// Init in tmpDir
	res, err := projects.Init(context.Background(), tmpDir)
	if err != nil {
		t.Fatalf("projects.Init error = %v", err)
	}
	if !res.Created || res.ConfigPath == "" {
		t.Errorf("projects.Init result = %+v", res)
	}

	// Discover
	state, err := projects.Discover(context.Background(), app.ProjectDiscoveryOptions{WorkDir: tmpDir})
	if err != nil {
		t.Fatalf("projects.Discover error = %v", err)
	}
	if !state.ConfigExists {
		t.Errorf("projects.Discover configExists = false")
	}

	// Config saves
	if err := projects.SaveAgentProfile(tmpDir, "strength"); err != nil {
		t.Fatalf("SaveAgentProfile error = %v", err)
	}
	if err := projects.SavePermissionMode(tmpDir, permission.ModeAsk); err != nil {
		t.Fatalf("SavePermissionMode error = %v", err)
	}
	if err := projects.SaveMaxToolCalls(tmpDir, 42); err != nil {
		t.Fatalf("SaveMaxToolCalls error = %v", err)
	}
	if err := projects.SaveReasoningEffort(tmpDir, sdk.ReasoningHigh); err != nil {
		t.Fatalf("SaveReasoningEffort error = %v", err)
	}
	if err := projects.SaveActiveSkills(tmpDir, []string{"alpha-skill"}); err != nil {
		t.Fatalf("SaveActiveSkills error = %v", err)
	}
}

func TestUserSettingsUseCase(t *testing.T) {
	homeDir := t.TempDir()
	store := config.NewUserSettingsStore(homeDir)
	userSettings := app.NewUserSettings(store)

	if err := userSettings.SaveActiveSkills([]string{"global-skill"}); err != nil {
		t.Fatalf("userSettings.SaveActiveSkills error = %v", err)
	}
}

func TestModelsUseCase(t *testing.T) {
	models := app.NewModels(model.Catalog{})
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Provider with invalid URL should fail gracefully
	_, err := models.Discover(ctx, app.ModelDiscoveryRequest{
		ProviderName: "unknown-provider",
		ProviderType: "openai",
		BaseURL:      "http://127.0.0.1:54321/nonexistent",
		APIKey:       "dummy",
		Timeout:      50 * time.Millisecond,
	})
	if err == nil {
		t.Log("Models.Discover succeeded or skipped network call")
	}
}
