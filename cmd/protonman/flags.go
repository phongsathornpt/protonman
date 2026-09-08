package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"

	"github.com/phongsathornpt/protonman/internal/base/envconfig"
	"github.com/phongsathornpt/protonman/internal/feature/agent"
)

type cliOptions struct {
	prompt       string
	output       string
	mode         string
	agentProfile string
	sandbox      string
	yolo         bool
	headless     bool
	acp          bool
	help         bool
	version      bool
	resume       bool
	newSession   bool
	sessionID    string
}

func parseArgs(args []string) (cliOptions, error) {
	var options cliOptions
	flags := flag.NewFlagSet("protonman", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&options.prompt, "p", "", "run one headless prompt and exit")
	flags.StringVar(&options.prompt, "prompt", "", "run one headless prompt and exit")
	flags.StringVar(&options.output, "output", "text", "headless output format: text or json")
	flags.StringVar(&options.mode, "permission-mode", "", "override permission mode")
	profileHelp := "default agent profile: " + agent.ProfileList(", ")
	flags.StringVar(&options.agentProfile, "agent", "", profileHelp)
	flags.StringVar(&options.agentProfile, "profile", "", profileHelp)
	flags.StringVar(&options.agentProfile, "a", "", profileHelp)
	flags.BoolVar(&options.yolo, "y", false, "set permission mode to always-approve")
	flags.BoolVar(&options.headless, "headless", false, "read the prompt from stdin")
	flags.BoolVar(&options.acp, "acp", false, "serve Agent Client Protocol JSON-RPC on stdio")
	flags.StringVar(&options.sandbox, "sandbox", "", "sandbox profile: off, workspace, read-only, strict")
	flags.BoolVar(&options.resume, "r", false, "resume the previous session")
	flags.BoolVar(&options.resume, "resume", false, "resume the previous session")
	flags.BoolVar(&options.resume, "continue", false, "resume the previous session")
	flags.BoolVar(&options.resume, "c", false, "resume the previous session")
	flags.BoolVar(&options.newSession, "n", false, "start a new session (default)")
	flags.BoolVar(&options.newSession, "new-session", false, "start a new session (default)")
	flags.BoolVar(&options.newSession, "new", false, "start a new session (default)")
	flags.StringVar(&options.sessionID, "s", "", "session id to load or create")
	flags.StringVar(&options.sessionID, "session", "", "session id to load or create")
	flags.BoolVar(&options.help, "help", false, "show usage")
	flags.BoolVar(&options.help, "h", false, "show usage")
	flags.BoolVar(&options.version, "version", false, "show version")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return cliOptions{help: true}, nil
		}
		return cliOptions{}, err
	}
	if options.help {
		return options, nil
	}
	if options.resume && options.newSession {
		return cliOptions{}, errors.New("cannot specify both --resume and --new-session")
	}
	if options.prompt == "" && flags.NArg() > 0 {
		options.prompt = strings.Join(flags.Args(), " ")
	}
	return options, nil
}

func usage() string {
	return strings.TrimSpace(`
Usage:
  protonman                   start the fullscreen TUI (new session)
  protonman --resume             resume the previous session
  protonman session list         list sessions for the current workspace
  protonman session resume [id]  resume the latest or a specific session
  protonman -p "<prompt>"        run one headless prompt
  protonman --headless           read the headless prompt from stdin
  protonman --version            print the binary version and exit

Session flags:
  -r, --resume, --continue    resume the previous session
  -s, --session string        session id to load or create
  -n, --new-session, --new    start a new session (default)

Agent flags:
  -a, --agent, --profile string  agent profile: universal | strength | agility | intelligence (legacy aliases: pow | int | dex | worker | explorer | reviewer)

General flags:
  --version                   print the binary version and exit

Headless flags:
  -p, --prompt string         prompt text
  --output text|json          output format (default text)
  --permission-mode string    ask | always-approve | deny
  -y                          always-approve
  --headless                  read prompt from stdin
  --acp                       serve ACP JSON-RPC on stdio
  --sandbox profile           off | workspace | read-only | strict
`) + "\n"
}

func stdinIsTerminal() bool {
	if envconfig.Bool(envconfig.ForceTTY) {
		return true
	}
	return term.IsTerminal(int(os.Stdin.Fd()))
}

func stdoutIsTerminal() bool {
	if envconfig.Bool(envconfig.ForceTTY) {
		return true
	}
	return term.IsTerminal(int(os.Stdout.Fd()))
}

func readStdinPrompt() (string, error) {
	body, err := io.ReadAll(os.Stdin)
	if err != nil {
		return "", fmt.Errorf("read stdin: %w", err)
	}
	prompt := strings.TrimSpace(string(body))
	if prompt == "" {
		return "", errors.New("headless prompt is empty")
	}
	return prompt, nil
}
