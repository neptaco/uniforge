# UniForge Setup Guide

First-time setup: installing the CLI, the Unity-side plugin, and connecting an AI agent via MCP. Once set up, everyday usage is covered by the main skill reference.

## Installing the CLI

UniForge is a single static binary with no runtime dependencies. Installation is required, but it is one command and needs no admin rights.

Check whether it is already installed before doing anything else:

```bash
uniforge --version
```

If the command is not found, install it:

```bash
# macOS / Linux (installs to ~/.local/bin by default)
curl -fsSL https://github.com/neptaco/uniforge/releases/latest/download/install.sh | sh

# Windows (PowerShell)
irm https://github.com/neptaco/uniforge/releases/latest/download/install.ps1 | iex
```

```yaml
# GitHub Actions
- uses: neptaco/setup-uniforge@v1
```

Notes:

- The installer verifies the release checksum. Set the `UNIFORGE_INSTALL_DIR` environment variable to change the target directory (works on all platforms; the POSIX script also accepts `--install-dir`). Make sure `~/.local/bin` is on `PATH`.
- Update a standalone install with `uniforge update` (`--check` to only check, `--version vX.Y.Z` to pin).
- **Unity Hub is required** for editor management commands (`editor install`, `editor list`, `project list`).

## Installing the Unity Editor plugin (UPM package)

The Unity-side package is needed **only for live-mode features** (`uniforge tool ...`, MCP tools that talk to the Editor). Batch mode, `meta check`, `logs`, and editor/version management work without it.

Requirements: Unity 6.0 LTS or later (6000.0+).

Install via Unity Package Manager → "Add package from git URL" (or add the same string to `Packages/manifest.json` dependencies):

```
https://github.com/neptaco/uniforge-unity.git?path=Packages/dev.crysta.uniforge#v0.9.0
```

Pin the `#tag` to the latest release from https://github.com/neptaco/uniforge-unity/releases.

Once installed, an open Unity Editor connects to the local daemon automatically. Verify with:

```bash
uniforge tool projects
```

## First-time workflow

1. **Install the CLI** (above) and confirm `uniforge --version` works.
2. **Install a Unity Editor** matching the project:
   ```bash
   uniforge editor install -p .        # auto-detects the project's version
   ```
3. **Pick your mode:**
   - **Editor closed (CI, scripted runs)** — use batch mode directly, nothing else to set up:
     ```bash
     uniforge batch compile .
     uniforge batch test . --platform editmode
     ```
   - **Editor open (interactive / AI-assisted)** — install the UPM package (above), open the project (`uniforge open .`), then:
     ```bash
     uniforge tool projects                      # confirm the Editor is connected
     uniforge tool call editor-state -o json     # first live call
     ```
     The daemon starts automatically on demand; `uniforge daemon start` is not needed.

## Connecting an AI agent via MCP

```bash
claude mcp add uniforge -- uniforge mcp serve
```

or in a generic MCP client config:

```json
{"mcpServers": {"uniforge": {"command": "uniforge", "args": ["mcp", "serve"]}}}
```

## Shell completion (optional)

```bash
# zsh (~/.zshrc)
eval "$(uniforge completion zsh)"

# bash (~/.bashrc)
eval "$(uniforge completion bash)"

# fish
uniforge completion fish | source
```
