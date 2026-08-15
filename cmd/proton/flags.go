package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
)

type cliOptions struct {
	prompt   string
	output   string
	mode     string
	sandbox  string
	yolo     bool
	headless bool
	acp      bool
	help     bool
}

func parseArgs(args []string) (cliOptions, error) {
	var options cliOptions
	flags := flag.NewFlagSet("proton", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&options.prompt, "p", "", "run one headless prompt and exit")
	flags.StringVar(&options.prompt, "prompt", "", "run one headless prompt and exit")
	flags.StringVar(&options.output, "output", "text", "headless output format: text or json")
	flags.StringVar(&options.mode, "permission-mode", "", "override permission mode")
	flags.BoolVar(&options.yolo, "y", false, "set permission mode to always-approve")
	flags.BoolVar(&options.headless, "headless", false, "read the prompt from stdin")
	flags.BoolVar(&options.acp, "acp", false, "serve Agent Client Protocol JSON-RPC on stdio")
	flags.StringVar(&options.sandbox, "sandbox", "", "sandbox profile: off, workspace, read-only, strict")
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
	if options.prompt == "" && flags.NArg() > 0 {
		options.prompt = strings.Join(flags.Args(), " ")
	}
	return options, nil
}

func usage() string {
	return strings.TrimSpace(`
Usage:
  proton                      start the fullscreen TUI
  proton -p "<prompt>"        run one headless prompt
  proton --headless           read the headless prompt from stdin

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
	info, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

func stdoutIsTerminal() bool {
	info, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
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
