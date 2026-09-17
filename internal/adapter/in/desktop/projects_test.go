//go:build desktop

package desktop

import (
	"path/filepath"
	"strings"
	"testing"

	desktopstate "github.com/phongsathornpt/protonman/internal/feature/desktop"
)

type projectPreferences struct{ values map[string]string }

func (p *projectPreferences) String(key string) string    { return p.values[key] }
func (p *projectPreferences) SetString(key, value string) { p.values[key] = value }

func TestNormalizeProjectsRetainsMissingAbsoluteFolders(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "checkout")
	projects := normalizeProjects([]persistedProject{{
		ID: "project-1", Name: "Example",
		Folders: []desktopstate.ProjectFolder{{Path: missing, Primary: true}},
	}})
	if len(projects) != 1 || len(projects[0].Folders) != 1 {
		t.Fatalf("project folders = %#v", projects)
	}
	if projects[0].Folders[0].Path != missing || !projects[0].Folders[0].Primary {
		t.Fatalf("folder = %#v", projects[0].Folders[0])
	}
}

func TestProjectAdditionalFoldersExcludesPrimaryAndUnavailableFolders(t *testing.T) {
	primary := t.TempDir()
	additional := t.TempDir()
	missing := filepath.Join(t.TempDir(), "missing")
	project := desktopstate.ProjectState{Folders: []desktopstate.ProjectFolder{
		{Path: primary, Primary: true}, {Path: additional}, {Path: missing},
	}}
	got := projectAdditionalFolders(project, primary)
	if len(got) != 1 || got[0] != additional {
		t.Fatalf("additional folders = %#v", got)
	}
}

func TestFilterProjectsMatchesFolderAndConversation(t *testing.T) {
	projects := []desktopstate.ProjectState{{ID: "p1", Name: "Frontend", Folders: []desktopstate.ProjectFolder{{Path: "/repo/web"}}}, {ID: "p2", Name: "Backend"}}
	sessions := []desktopstate.SessionState{{ProjectID: "p2", Title: "Fix auth flow"}}
	if got := filterProjects(projects, sessions, "web"); len(got) != 1 || got[0].ID != "p1" {
		t.Fatalf("folder search = %#v", got)
	}
	if got := filterProjects(projects, sessions, "auth"); len(got) != 1 || got[0].ID != "p2" {
		t.Fatalf("conversation search = %#v", got)
	}
}

func TestProjectsRoundTripThroughPreferences(t *testing.T) {
	root := t.TempDir()
	prefs := &projectPreferences{values: make(map[string]string)}
	want := []desktopstate.ProjectState{{ID: "p1", Name: "Frontend", Folders: []desktopstate.ProjectFolder{{Path: root, Primary: true}}, AgentIDs: []string{"protonman", "reviewer"}, DefaultAgentID: "reviewer"}}
	if err := persistProjects(prefs, want); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prefs.String(projectsPreferencesKey), "Frontend") {
		t.Fatalf("stored projects = %q", prefs.String(projectsPreferencesKey))
	}
	got := loadProjects(prefs)
	if len(got) != 1 || got[0].ID != "p1" || len(got[0].Folders) != 1 || len(got[0].AgentIDs) != 2 || got[0].DefaultAgentID != "reviewer" {
		t.Fatalf("loaded projects = %#v", got)
	}
}
