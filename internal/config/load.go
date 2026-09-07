package config

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/pelletier/go-toml/v2"

	"github.com/projectTHORN/proton/internal/app/appdirs"
	"github.com/projectTHORN/proton/internal/permission"
	"github.com/projectTHORN/proton/internal/sandbox"
)

func Load(ctx context.Context, options Options) (Snapshot, error) {
	homeDir := options.HomeDir
	if homeDir == "" {
		resolvedHome, err := os.UserHomeDir()
		if err != nil {
			return Snapshot{}, fmt.Errorf("resolve home directory: %w", err)
		}
		homeDir = resolvedHome
	}
	workDir := options.WorkDir
	if workDir == "" {
		resolvedWorkDir, err := os.Getwd()
		if err != nil {
			return Snapshot{}, fmt.Errorf("resolve work directory: %w", err)
		}
		workDir = resolvedWorkDir
	}

	if err := validateDirectory(homeDir, "home"); err != nil {
		return Snapshot{}, err
	}
	if err := validateDirectory(workDir, "work"); err != nil {
		return Snapshot{}, err
	}

	snapshot := Snapshot{
		Permission: permission.Config{
			Rules:   make([]permission.Rule, 0),
			Default: permission.ActionAsk,
		},
		Mode:           permission.ModeAsk,
		ProtectedPaths: make([]string, 0),
		Sandbox:        sandbox.NameOff,
		Providers:      make(map[string]ProviderConfig),
		Agent: AgentConfig{
			MaxToolCalls:         DefaultMaxToolCalls,
			MaxLiveSubagents:     DefaultMaxLiveSubagents,
			MaxRetainedSubagents: DefaultMaxRetainedSubagents,
			SubagentMaxRuntime:   DefaultSubagentMaxRuntime,
			SubagentWaitTimeout:  DefaultSubagentWaitTimeout,
			SubagentQueueTimeout: DefaultSubagentQueueTimeout,
			CompletedResultTTL:   DefaultCompletedResultTTL,
		},
		Runtime: DefaultRuntimeConfig(),
		Provenance: map[string]ValueSource{
			FieldModelDefault:         SourceDefault,
			FieldModelProvider:        SourceDefault,
			FieldAgentProfile:         SourceDefault,
			FieldAgentReasoningEffort: SourceDefault,
			FieldAgentMaxToolCalls:    SourceDefault,
			FieldUIPermissionMode:     SourceDefault,
		},
		Sources:  make([]string, 0, 2),
		Warnings: make([]string, 0),
	}

	dirs, err := appdirs.Resolve(homeDir)
	if err != nil {
		return Snapshot{}, err
	}
	userPath := dirs.Config
	if err := loadFile(ctx, userPath, &snapshot, false); err != nil {
		return Snapshot{}, err
	}

	projectPath := appdirs.ProjectConfig(workDir)
	if !options.ProjectTrusted {
		exists, err := fileExists(projectPath)
		if err != nil {
			return Snapshot{}, fmt.Errorf("inspect project config: %w", err)
		}
		if exists {
			snapshot.Warnings = append(snapshot.Warnings,
				"ignored untrusted project config: "+projectPath,
			)
		}
		return snapshot, nil
	}
	if err := loadFile(ctx, projectPath, &snapshot, true); err != nil {
		return Snapshot{}, err
	}
	return snapshot, nil
}

func loadFile(ctx context.Context, path string, snapshot *Snapshot, project bool) (loadErr error) {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("before loading %s: %w", path, err)
	}
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("open config %s: %w", path, err)
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil && loadErr == nil {
			loadErr = fmt.Errorf("close config %s: %w", path, closeErr)
		}
	}()

	var document fileDocument
	if err := toml.NewDecoder(file).Decode(&document); err != nil {
		return fmt.Errorf("decode config %s: %w", path, err)
	}
	source := SourceUser
	if project {
		source = SourceProject
	}
	if err := mergeDocument(document, snapshot, source); err != nil {
		return fmt.Errorf("apply config %s: %w", path, err)
	}
	snapshot.Sources = append(snapshot.Sources, path)
	if project && document.UI.PermissionMode != "" {
		snapshot.Warnings = append(snapshot.Warnings,
			"project permission mode is active because project config is trusted: "+path,
		)
	}
	return nil
}

func fileExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func validateDirectory(path string, label string) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("%s directory is required", label)
	}
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat %s directory %q: %w", label, path, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s path %q is not a directory", label, path)
	}
	return nil
}
