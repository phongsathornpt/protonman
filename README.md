<div align="center">

# protonMAN

**Open-source AI coding agent for the terminal, automation, and editor workflows.**

Built in Go with a security-first runtime, multi-agent orchestration, provider-neutral model support, durable sessions, and a fast TUI.

[![CI](https://github.com/phongsathornpt/protonman/actions/workflows/ci.yml/badge.svg)](https://github.com/phongsathornpt/protonman/actions/workflows/ci.yml)
[![CodeQL](https://github.com/phongsathornpt/protonman/actions/workflows/codeql.yml/badge.svg)](https://github.com/phongsathornpt/protonman/actions/workflows/codeql.yml)
[![Release](https://img.shields.io/github/v/release/phongsathornpt/protonman?display_name=tag)](https://github.com/phongsathornpt/protonman/releases)
[![License](https://img.shields.io/github/license/phongsathornpt/protonman)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.27%2B-00ADD8?logo=go&logoColor=white)](go.mod)

[Website](https://protonman.dev/) ·
[Documentation](docs/) ·
[Releases](https://github.com/phongsathornpt/protonman/releases) ·
[Contributing](CONTRIBUTING.md) ·
[Security](SECURITY.md)

</div>

<p align="center">
  <img src="docs/assets/protonman-readme.webp" alt="protonMAN terminal interface" width="100%">
</p>

## What is protonMAN?

protonMAN is an autonomous coding agent designed to work directly inside real software repositories.

It combines an interactive terminal UI, headless automation, Agent Client Protocol support, multiple model providers, persistent sessions, agent skills, and isolated subagents behind a single Go runtime.

The project is intentionally opinionated about two things: **agents should be useful enough to finish real work, and powerful tools should still have explicit safety boundaries**.

## Highlights

- **Terminal-first workflow** with a compact Bubble Tea TUI and streaming responses.
- **Autonomous agent loop** with progress-aware safety limits instead of a tiny fixed tool-call budget.
- **Multi-agent orchestration** using Universal, Strength, Agility, and Intelligence roles.
- **Provider-neutral models** through the bundled `proton-sdk`, including OpenAI-compatible and Anthropic protocols.
- **Free-model support** through OpenCode, including low-concurrency scheduling and replay-safe stream recovery.
- **Workspace-safe tools** for reading, editing, searching, Git, shell execution, web access, tasks, skills, and subagents.
- **Permission engine** with `ask`, `plan`, and `always-approve` modes.
- **OS sandboxing** with macOS Seatbelt and native Linux Landlock support.
- **Durable sessions and memory** under `~/.protonman`.
- **Agent Skills** support using the open [Agent Skills specification](https://agentskills.io).
- **Headless and ACP modes** for CI, scripts, IDEs, and editor integrations.

## Install

### Linux and macOS

```sh
curl -fsSL https://github.com/phongsathornpt/protonman/releases/latest/download/install.sh | sh
```

Verify the installation:

```sh
protonman --version
```

Update later without reinstalling manually:

```sh
protonman update
```

To install a specific release:

```sh
protonman update v1.2.3
```

> Supported release targets currently include Linux amd64 and macOS arm64.

For installer details, checksums, custom install paths, and private-release authentication, see [docs/install.md](docs/install.md).

## Quick start

Launch the interactive TUI from any repository:

```sh
cd your-project
protonman
```

Run a one-shot task:

```sh
protonman -p "Explain the architecture and identify the riskiest coupling"
```

Run non-interactively with approval enabled:

```sh
protonman -y -p "Run the tests and fix the failures"
```

Read a prompt from stdin and emit JSON:

```sh
cat prompt.txt | protonman --headless --output json
```

Serve Agent Client Protocol over stdio:

```sh
protonman --acp
```

Run with a strict sandbox:

```sh
protonman --sandbox strict -p "Audit this repository"
```

## Agent model

protonMAN uses one primary agent and three specialized subagent profiles.

| Agent | TUI | Purpose |
| --- | :---: | --- |
| **Universal** | `UNI` | Owns the task end-to-end, integrates work, and verifies the result |
| **Strength** | `STR` | Implementation, refactors, migrations, and substantial code changes |
| **Agility** | `AGI` | Fast read-only exploration, tracing, and focused repository investigation |
| **Intelligence** | `INT` | Architecture, difficult debugging, concurrency, performance, and deep reasoning |

Subagents run asynchronously and return evidence-backed results to the parent agent. The Universal agent remains responsible for the final integration and verification.

## Built-in tools

| Tool | Purpose |
| --- | --- |
| `read` | Read source files, structured data, metadata, and supported images |
| `edit` | Write, replace, patch, and restore files with checkpoint protection |
| `grep` | Regex search with bounded pagination |
| `find` | Recursive path discovery with glob and depth filters |
| `ls` | Directory inspection with protected-path filtering |
| `git` | Repository status and version-control inspection |
| `bash` | Bounded shell execution with effect analysis |
| `web` | Search the web or fetch known URLs under network policy |
| `todo` | Durable session task planning with revision control |
| `skill` | Load Agent Skills into the active context |
| `subagent` | Spawn, inspect, wait for, or cancel specialized agents |

Tool calls pass through policy evaluation before execution. Mutating file operations create checkpoints that can be restored later.

## Model providers

protonMAN supports both hosted and local providers.

| Provider | Type | Notes |
| --- | --- | --- |
| **OpenCode** | OpenAI-compatible | Free models available without an API key |
| **Protonman** | OpenAI-compatible | Hosted gateway at `protonman.dev` |
| **Ollama** | OpenAI-compatible | Local model inference |
| **OpenAI** | OpenAI-compatible | Native support through `proton-sdk` |
| **Anthropic** | Anthropic Messages | Native Messages protocol support |

Provider configuration lives in `~/.protonman/config.json` and can also be managed from the TUI with `/provider`.

OpenCode free models can use protonMAN's adaptive low-concurrency scheduler. The scheduler starts conservatively, reacts to provider congestion, respects `Retry-After`, and uses replay-safe retry behavior for streams that fail before visible output is committed.

See [docs/proton-sdk.md](docs/proton-sdk.md) for the provider abstraction and SDK contract.

## Permission and sandbox model

protonMAN does not treat tool execution as an unbounded side effect buffet, because apparently files are worth keeping.

### Permission modes

| Mode | Behavior |
| --- | --- |
| `ask` | Prompt when a tool call is not already allowed by policy |
| `plan` | Read-only workflow; mutating actions are blocked |
| `always-approve` | Automatically approves promptable actions while explicit deny rules still win |

Rule precedence is:

```text
deny > ask > allow
```

### Sandbox profiles

| Profile | Workspace | Host filesystem | Network |
| --- | --- | --- | --- |
| `off` | unrestricted | unrestricted | allowed |
| `workspace` | read/write | blocked except temp | allowed |
| `read-only` | read-only | blocked except temp | blocked |
| `strict` | read/write | blocked | blocked |

The runtime additionally enforces workspace path confinement, protected paths, symlink-escape prevention, execution deadlines, and automatic pre-edit checkpoints.

See [SECURITY.md](SECURITY.md) and [docs/architecture.md](docs/architecture.md) for the full model.

## TUI essentials

Common shortcuts:

| Key | Action |
| --- | --- |
| `Enter` | Send |
| `?` | Open shortcut help |
| `Shift+Tab` | Cycle permission mode |
| `Ctrl+P` | Model setup |
| `Ctrl+S` | Skills |
| `Ctrl+O` | Tasks |
| `Ctrl+T` | Transcript |
| `Ctrl+C` | Cancel or exit |

Useful slash commands include:

```text
/model
/provider
/permission
/low
/goal
/todo
/agents
/skills
/resume
/transcript
/help
```

## Architecture

The runtime keeps UI, orchestration, policy, tools, and external integrations separated behind explicit boundaries.

```text
TUI / Headless / ACP
        │
        ▼
Application turn loop
        │
        ├── Model providers / proton-sdk
        ├── Session + memory
        └── Subagent orchestration
        │
        ▼
Tool-call service + permission policy
        │
        ▼
Workspace-safe tools + OS sandbox
```

For package boundaries, execution flow, and design constraints, read [docs/architecture.md](docs/architecture.md).

## Build from source

Development requires Go 1.27+ and Git.

```sh
git clone https://github.com/phongsathornpt/protonman.git
cd protonman

make build
./bin/protonman --version
```

Useful development commands:

```sh
make tui
make test
make test-race
make test-e2e
make bench
make fmt
make vet
```

## Project documentation

| Document | Description |
| --- | --- |
| [Architecture](docs/architecture.md) | Runtime boundaries and package responsibilities |
| [Install](docs/install.md) | Installation and release binaries |
| [Settings](docs/settings.md) | User and project configuration |
| [Memory](docs/memory.md) | Durable memory design |
| [System prompt](docs/system-prompt.md) | Agent behavioral contract |
| [proton-sdk](docs/proton-sdk.md) | Model/provider SDK architecture |
| [Desktop](docs/desktop.md) | Desktop application architecture |
| [ACP conformance](docs/acp-conformance.md) | Agent Client Protocol behavior |
| [Releasing](docs/releasing.md) | Versioning and release process |

## Contributing

Contributions are welcome.

Before opening a pull request:

```sh
make fmt
make vet
make test
```

For repository conventions, branch naming, and review expectations, read [CONTRIBUTING.md](CONTRIBUTING.md).

Please report security vulnerabilities privately according to [SECURITY.md](SECURITY.md), rather than filing a public issue and creating an exciting day for everyone.

## Community

- Use [GitHub Issues](https://github.com/phongsathornpt/protonman/issues) for bugs and feature requests.
- Use [SUPPORT.md](SUPPORT.md) for support guidance.
- Follow [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md) when participating in the project.

## License

protonMAN is licensed under the [Apache License 2.0](LICENSE).
