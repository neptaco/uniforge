# UniForge Setup Guide

First-time setup: installing the CLI and Unity-side package, then checking the live Editor connection. Once set up, everyday usage is covered by the main skill reference.

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

## Installing the Unity Editor package

The Unity-side package is needed **only for live-mode features** (`uniforge tool ...` commands that talk to the Editor). Batch mode, `meta check`, `logs`, and editor/version management work without it.

Requirement: Unity 6.0 LTS or later (6000.0+). Required Unity packages, including Unity UI and the Test Framework, are resolved automatically as package dependencies.

Move to the Unity project root—the directory that contains `Assets`, `Packages`, and `ProjectSettings`:

```bash
cd /path/to/MyUnityProject
```

Then add the latest released UniForge package:

```bash
uniforge package add neptaco/uniforge-unity/Packages/dev.crysta.uniforge
```

When the project argument is omitted, UniForge detects the Unity project containing the current directory. Pass the project path before the package source when running from elsewhere. The GitHub shorthand expands to the full HTTPS Git URL, and the highest semantic-version tag is selected automatically. Existing dependencies are preserved. Interactive terminals show the resolved project, URL, tag, package reference, and manifest path before changing the file. Non-interactive runs proceed without prompting; use `--yes` to skip the prompt in a terminal. Use `--tag v0.11.0` to pin a release. Alternatively, use Unity Package Manager → "Add package from git URL":

```
https://github.com/neptaco/uniforge-unity.git?path=Packages/dev.crysta.uniforge#v0.11.0
```

Pin the `#tag` to the latest version from https://github.com/neptaco/uniforge-unity/tags.

Once installed, an open Unity Editor connects to the local daemon automatically. Verify with:

```bash
uniforge tool projects
```

## First-time workflow

1. **Install the CLI** (above) and confirm `uniforge --version` works.
2. **Pick your mode:**
   - **Editor closed (CI, scripted runs)** — use batch mode directly, nothing else to set up:
     ```bash
     uniforge compile
     uniforge test --platform editmode
     ```
   - **Editor open (interactive / AI-assisted)** — install the UPM package (above), open the project (`uniforge open`), then:
     ```bash
     uniforge tool projects                      # confirm the Editor is connected
     uniforge tool call editor-state -o json     # first live call
     ```
     The daemon starts automatically on demand; `uniforge daemon start` is not needed.
3. **Only if the project requires a Unity version that is not installed:**
   ```bash
   uniforge editor install -p .        # auto-detects the project's version
   ```

## Connecting an AI agent

```bash
uniforge tool list
uniforge tool call <tool> [json-args] -o json
```

An AI agent can run these commands directly; no server registration is required.

## Shell completion (optional)

```bash
# zsh (~/.zshrc)
eval "$(uniforge completion zsh)"

# bash (~/.bashrc)
eval "$(uniforge completion bash)"

# fish
uniforge completion fish | source
```
