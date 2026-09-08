//go:build linux

package sandbox

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
)

const sandboxBootstrapEnv = "PROTON_INTERNAL_SANDBOX_BOOTSTRAP"

type bootstrapPolicy struct {
	Workspace string `json:"workspace"`
	Cwd       string `json:"cwd"`
	Command   string `json:"command"`
	ReadOnly  bool   `json:"read_only"`
}

func init() {
	encoded := os.Getenv(sandboxBootstrapEnv)
	if encoded == "" {
		return
	}
	if err := runSandboxBootstrap(encoded); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "protonman sandbox bootstrap:", err)
		os.Exit(126)
	}
	os.Exit(0)
}

func runSandboxBootstrap(encoded string) error {
	body, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return fmt.Errorf("decode policy: %w", err)
	}
	var policy bootstrapPolicy
	if err := json.Unmarshal(body, &policy); err != nil {
		return fmt.Errorf("parse policy: %w", err)
	}
	if policy.Workspace == "" || policy.Cwd == "" || policy.Command == "" {
		return fmt.Errorf("invalid empty sandbox policy")
	}
	if err := applyLandlock(policy.Workspace, policy.ReadOnly); err != nil {
		return err
	}
	return execSandboxShell(policy.Cwd, policy.Command)
}
