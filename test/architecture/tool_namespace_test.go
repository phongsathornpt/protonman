package architecture_test

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

func TestLegacyToolIdentifiersDoNotReturn(t *testing.T) {
	root := repositoryRoot(t)
	legacy := []string{
		"read_file", "list_dir", "find_files", "calculate", "activate_skill",
		"web_fetch", "web_search", "git_status",
		"write_file", "search_replace", "apply_patch", "checkpoint_restore",
		"get_todo", "update_todo",
		"delegate_task", "wait_agent", "get_agent", "list_agents", "cancel_agent", "resume_agent",
	}
	patterns := make([]*regexp.Regexp, 0, len(legacy))
	for _, name := range legacy {
		patterns = append(patterns, regexp.MustCompile(`(?:"|`+"`"+`)`+regexp.QuoteMeta(name)+`(?:"|`+"`"+`)`))
	}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if filepath.Ext(path) != ".go" || filepath.Base(path) == "tool_namespace_test.go" {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for i, pattern := range patterns {
			if pattern.Match(body) {
				t.Errorf("%s contains legacy tool identifier %q", path, legacy[i])
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
