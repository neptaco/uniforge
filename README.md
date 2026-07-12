# UniForge

Command-line tool for Unity development automation. Build, test, and manage Unity projects with simple commands. Includes a local daemon for real-time Unity Editor integration via MCP.

## Features

- 🤖 **CI/CD optimized** - GitHub Actions annotations, log grouping, noise filtering
- 🖥️ **Cross-platform** - Same commands work on macOS, Windows, Linux
- 🧪 **Test Runner** - Run EditMode/PlayMode tests with XML results
- 📦 **Editor management** - Install Unity versions via Unity Hub CLI
- 📁 **Project management** - Browse and open Unity Hub projects with TUI or CLI
- 📋 **Meta file check** - Detect missing .meta files and duplicate GUIDs
- 🔑 **License management** - Activate/return licenses for CI runners
- 🔌 **MCP Server** - Model Context Protocol server for AI-assisted Unity development
- 🛠️ **Daemon + Tool system** - Local daemon connects to running Unity Editors for real-time tool execution

## Quick Start: GitHub Actions

### Build Workflow (Self-hosted Runner)

Build for multiple platforms using matrix strategy:

```yaml
name: Build

on:
  push:
    tags: ['v*']

jobs:
  build:
    strategy:
      matrix:
        include:
          - runner: unity-windows
            target: StandaloneWindows64
            modules: windows-il2cpp
          - runner: unity-mac
            target: StandaloneOSX
            modules: mac-il2cpp

    runs-on: ${{ matrix.runner }}
    steps:
      - uses: actions/checkout@v4

      - uses: neptaco/setup-uniforge@v1

      - name: Install Unity
        run: uniforge editor install --modules ${{ matrix.modules }}

      - name: Build
        run: uniforge batch run . --ci -- -executeMethod Build.Perform -buildTarget ${{ matrix.target }}
```

### CI Workflow (Self-hosted Runner)

Run tests on every push:

```yaml
name: CI

on: [push, pull_request]

jobs:
  test:
    runs-on: unity
    steps:
      - uses: actions/checkout@v4

      - uses: neptaco/setup-uniforge@v1

      - name: Install Unity
        run: uniforge editor install

      - name: Check .meta files
        run: uniforge meta check .

      - name: Run Tests
        run: uniforge batch test . --platform editmode --ci
```

## Installation

### GitHub Actions

```yaml
- uses: neptaco/setup-uniforge@v1
```

### macOS / Linux

```bash
curl -fsSL https://github.com/neptaco/uniforge/releases/latest/download/install.sh | sh
```

The installer verifies the release checksum and installs to `~/.local/bin` by default. Set `UNIFORGE_INSTALL_DIR` or pass `--install-dir` to choose another directory.

### Windows

```powershell
irm https://github.com/neptaco/uniforge/releases/latest/download/install.ps1 | iex
```

### Update

Standalone installations can update themselves:

```bash
uniforge update
uniforge update --check
uniforge update --version v0.9.0
```

Package-manager and development builds are not modified by `uniforge update`.

### Manual download

Download the latest release from [GitHub Releases](https://github.com/neptaco/uniforge/releases).

### Shell Completion

```bash
# zsh (~/.zshrc)
eval "$(uniforge completion zsh)"

# bash (~/.bashrc)
eval "$(uniforge completion bash)"

# fish
uniforge completion fish | source
```

## Prerequisites

- Unity Hub installed

## Usage

### Manage Unity Editor

```bash
# Interactive TUI (when no version specified)
uniforge editor install

# Install from project (auto-detect version)
uniforge editor install -p .

# Install specific version with modules
uniforge editor install 2022.3.10f1 --modules ios,android

# List installed Unity Editors
uniforge editor list

# List available versions (for scripting)
uniforge editor available --lts --latest -o json

# Clear cached Unity release information
uniforge cache clear
```

### Batch Mode (Unity Editor closed)

```bash
# Compile project
uniforge batch compile ./MyProject

# Run EditMode tests
uniforge batch test ./MyProject --platform editmode

# Run PlayMode tests with filter and results
uniforge batch test ./MyProject --platform playmode \
  --filter MyTestClass \
  --results ./test-results.xml

# Run custom method
uniforge batch run ./MyProject -- -executeMethod Build.Execute

# CI mode (optimized output)
uniforge batch run ./MyProject --ci -- -executeMethod Build.Execute
```

**Options:**
- `--ci`: CI mode (GitHub Actions annotations, log grouping, stack trace filtering)
- `--log-file <path>`: Save log to file
- `--timeout <seconds>`: Timeout in seconds
- `-t, --timestamp`: Show timestamps

### Daemon & Tool System (Unity Editor open)

The daemon connects to running Unity Editors for real-time tool execution:

```bash
# Start the local daemon
uniforge daemon start

# Check daemon status
uniforge daemon status

# Restart daemon
uniforge daemon restart

# List connected Unity projects
uniforge tool projects

# List available tools
uniforge tool list

# Describe tool schema
uniforge tool describe editor-state

# Execute a tool
uniforge tool call editor-state -o json

# Execute with arguments
uniforge tool call run-tests '{"mode":"EditMode"}' -o json

# Stop daemon
uniforge daemon stop
```

Default output is YAML. Use `-o json` when machine-readable output is needed.

### MCP Server

Start a Model Context Protocol server for AI-assisted Unity development:

```bash
uniforge mcp serve
```

The MCP server exposes Unity tools over stdio, enabling AI agents to interact with Unity Editor.

### Check .meta File Integrity

```bash
# Check for missing/orphan .meta files and duplicate GUIDs
uniforge meta check ./MyProject

# Auto-fix (generate missing, remove orphans)
uniforge meta check ./MyProject --fix

# Fix without confirmation (for CI)
uniforge meta check ./MyProject --fix --force
```

### Manage Unity Hub Projects

```bash
# Interactive TUI
uniforge project list

# List in JSON format
uniforge project list -o json

# Open project by name (partial match)
uniforge project open my-game

# Get project path (for shell scripts)
uniforge project path my-game

# Show project details (Unity version, packages, asmdefs)
uniforge project info my-game
```

### Open/Close Unity Editor

```bash
# Open Unity Editor with a project
uniforge open ./MyProject

# Open by project name
uniforge open my-game

# Close / force close / restart
uniforge close ./MyProject
uniforge close ./MyProject --force
uniforge restart ./MyProject
```

### View Unity Logs

```bash
# Show last 100 lines
uniforge logs

# Follow in real-time
uniforge logs -f

# Show project stack traces
uniforge logs --trace

# Show full stack traces (including Unity internals)
uniforge logs --full-trace

# Open in text editor
uniforge logs --editor
```

### Diagnose Stale Unity Runtime State

```bash
# Read-only diagnosis
uniforge doctor unity ./MyProject

# Remove only verified stale files and stop orphan licensing clients
uniforge doctor unity ./MyProject --fix
```

The doctor never removes runtime files when the process state cannot be verified or a matching Unity process is active. Use `uniforge clean unity` when you explicitly want to remove a selected runtime file:

```bash
# Remove Temp/UnityLockfile after verifying the editor is not running
uniforge clean unity ./MyProject --target lockfile

# Preview what would be removed
uniforge clean unity ./MyProject --target lockfile --dry-run
```

### Manage Unity License

```bash
# Check license status
uniforge license status

# Activate (Personal: no serial, Plus/Pro: serial required)
uniforge license activate -u user@example.com -p password -s SERIAL-KEY

# Return license
uniforge license return
```

**Environment variables:** `UNITY_USERNAME`, `UNITY_PASSWORD`, `UNITY_SERIAL`

## Configuration

### Environment Variables

```bash
UNIFORGE_HUB_PATH           # Path to Unity Hub executable
UNIFORGE_EDITOR_BASE_PATH   # Custom Unity Editor base directory
UNIFORGE_EDITOR              # External editor for "project" TUI
UNIFORGE_BIN                 # Go binary path for daemon auto-start
UNIFORGE_LOG_LEVEL           # Log level (debug, info, warn, error)
UNIFORGE_TIMEOUT             # Default timeout in seconds
UNIFORGE_NO_COLOR            # Disable colored output
```

## Development

### Prerequisites

- [Go](https://go.dev/) 1.25+
- [Task](https://taskfile.dev/) - Task runner

### Setup

```bash
git clone https://github.com/neptaco/uniforge.git
cd uniforge
task deps
```

### Commands

```bash
task build    # Build binary to dist/uniforge
task test     # Run tests
task check    # Run all checks (fmt, vet, lint, test)
```

### Notes

- Use `./dist/uniforge` for testing, not `go run .` — `task build` embeds version info via ldflags, and the daemon requires a real binary path.
- `batch run|compile|test` is the canonical form. Root-level `run|compile|test` are deprecated aliases.

## License

MIT License - see [LICENSE](LICENSE) file for details.

## Contributing

Contributions are welcome! See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.
