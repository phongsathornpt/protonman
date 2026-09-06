package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"

	"github.com/projectTHORN/proton/internal/envconfig"
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
	resume       bool
	newSession   bool
	sessionID    string
}

func parseArgs(args []string) (cliOptions, error) {
	var options cliOptions
	flags := flag.NewFlagSet("proton", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&options.prompt, "p", "", "run one headless prompt and exit")
	flags.StringVar(&options.prompt, "prompt", "", "run one headless prompt and exit")
	flags.StringVar(&options.output, "output", "text", "headless output format: text or json")
	flags.StringVar(&options.mode, "permission-mode", "", "override permission mode")
	flags.StringVar(&options.agentProfile, "agent", "", "default agent profile: pow, dex, int, worker, explorer, reviewer")
	flags.StringVar(&options.agentProfile, "profile", "", "default agent profile: pow, dex, int, worker, explorer, reviewer")
	flags.StringVar(&options.agentProfile, "a", "", "default agent profile: pow, dex, int, worker, explorer, reviewer")
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
  proton                      start the fullscreen TUI (new session)
  proton --resume             resume the previous session
  proton -p "<prompt>"        run one headless prompt
  proton --headless           read the headless prompt from stdin

Session flags:
  -r, --resume, --continue    resume the previous session
  -s, --session string        session id to load or create
  -n, --new-session, --new    start a new session (default)

Agent flags:
  -a, --agent, --profile string  agent profile: pow | dex | int | worker | explorer | reviewer

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
