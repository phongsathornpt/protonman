---
name: acp-agent-integration
description: Comprehensive operational guide and reference for integrating, configuring, and troubleshooting Agent Client Protocol (ACP) coding agents (Protonman, OpenCode, Cline, Claude Code, Antigravity) with JSON-RPC stdio daemons, multi-agent profiles, and error diagnosis.
---

# Agent Client Protocol (ACP) Integration Guide

This skill provides the contract, commands, and operational troubleshooting playbook for running and integrating coding agents over the Agent Client Protocol (ACP).

---

## 1. Protocol Architecture

ACP (Agent Client Protocol) is an open JSON-RPC 2.0 stdio protocol used by IDEs, editors (such as Zed), and desktop frontends (such as Protonman Desktop) to drive AI coding agents.

```text
Editor / Desktop Frontend (Client)
             |
             | JSON-RPC 2.0 over stdin / stdout
             v
   Coding Agent Process (Daemon / Server)
```

### Key Handshake & Lifecycle Methods
- `initialize`: Client negotiates protocol version (e.g. `protocolVersion: 1`), capabilities, and client metadata.
- `session/new`: Spawns or initializes a new session bound to workspace root `cwd` and optional `additionalDirectories`.
- `session/prompt`: Sends user message/prompt and streams incremental text or tool calls.
- `session/cancel`: Signals cancellation of an active prompt turn.
- Reverse requests: Agent requests client confirmation via `permission/request` or user input via `question/ask`.

---

## 2. Agent Compatibility & Command Matrix

| Agent | ACP Support | Command | Arguments | Configuration Details |
|---|---|---|---|---|
| **Protonman** | **Native** | `protonman` | `["--acp"]` | Native ACP daemon. Supports extensions: `protonman/session/context`, `protonman/session/memory`, `protonman/session/runtime`. |
| **Cline** | **Native** | `cline` | `["--acp"]` | Native `--acp` flag in Cline CLI (`cline --acp`) for editor integration. |
| **OpenCode** | **Native** | `opencode` | `["acp"]` | Native `acp` subcommand (`opencode acp`), as implemented by the official OpenCode Zed extension. |
| **Google Antigravity** | **Archive / Bridge** | `/path/to/agy_acp_server.par` | `[]` | Public CLI `agy` has **NO** `--acp` flag. Requires Google's platform archive (`agy_acp_server.par`) or an ACP-to-NDJSON bridge. |
| **Claude Code** | **Adapter Required** | `npx ...` / bridge | `[]` | Public CLI `claude` has **NO** native `--acp` mode. Requires an ACP adapter (e.g. Zed's Claude adapter or bridge). |

---

## 3. Configuration & Profile Specifications

### Protonman Desktop Configuration
ACP agent profiles are persisted in `~/.protonman/config.json` under `preferences["acp.agents.v1"]`:

```json
{
  "preferences": {
    "acp.agents.v1": [
      {
        "id": "protonman",
        "displayName": "Protonman",
        "command": "protonman",
        "args": ["--acp"]
      },
      {
        "id": "cline",
        "displayName": "Cline",
        "command": "cline",
        "args": ["--acp"]
      },
      {
        "id": "opencode",
        "displayName": "OpenCode",
        "command": "opencode",
        "args": ["acp"]
      }
    ]
  }
}
```

### Environment Variable Override
To launch Protonman Desktop with specific agents from shell or CI:
```bash
PROTONMAN_ACP_AGENTS_JSON='[
  {"id":"protonman","displayName":"Protonman","command":"protonman","args":["--acp"]},
  {"id":"cline","displayName":"Cline","command":"cline","args":["--acp"]},
  {"id":"opencode","displayName":"OpenCode","command":"opencode","args":["acp"]}
]' bin/protonman-desktop-gio
```

---

## 4. Troubleshooting & Diagnostic Playbook

### Common Connection Failures

#### 1. Flag Not Defined (`flags provided but not defined: -acp`)
- **Symptom**: Agent card displays an orange dot (`Retrying`) with error `Connection failed · exit status 2: flags provided but not defined: -acp`.
- **Cause**: The CLI does not have native ACP support (e.g. running `agy --acp`).
- **Fix**: Remove the entry or replace the command with the proper ACP server or bridge.

#### 2. Executable Not Found (`executable file not found in $PATH`)
- **Symptom**: Agent card shows `Start failed · exec: "<cmd>": executable file not found in $PATH`.
- **Cause**: Binary is not installed or the directory (e.g. `/opt/homebrew/bin` or `~/.local/bin`) is missing from `$PATH`.
- **Fix**: Specify the full absolute binary path in the `Command` editor (e.g. `/opt/homebrew/bin/cline`).

#### 3. Hanging Handshake / Non-JSON Output
- **Symptom**: Card remains stuck in `Starting <Agent>…` until handshake timeout.
- **Cause**: Subprocess printed banner text or prompted for interactive login before JSON-RPC initialize.
- **Fix**: Ensure the agent is logged in before starting the ACP daemon, or verify arguments suppress interactive prompts.

### Manual Verification in Terminal
To test whether an agent executable correctly implements ACP JSON-RPC over stdio:

```bash
# Send an initialize request
echo '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":1,"clientInfo":{"name":"test-client"}}}' | <command> <args>
```

A healthy ACP agent will reply immediately on stdout with:
```json
{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":1,...}}
```
