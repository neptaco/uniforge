---
name: uniforge
description: Automate Unity development with the UniForge CLI — run EditMode/PlayMode tests, compile projects, manage Unity Editor versions and lifecycle, validate .meta files, read Unity logs, diagnose stale runtime state, and execute live editor tools via the local daemon using tool call. Use when working on a Unity project and you need to run tests after C# changes, compile-check, open/close/restart Unity Editor, check .meta integrity, inspect Unity logs, or connect an AI agent to a running Unity Editor.
---

# UniForge CLI Reference

UniForge (`uniforge`) is a cross-platform CLI for Unity development automation.
It has two modes of operation:

- **Batch mode** — drives Unity in batchmode while the Editor is *closed* (`test`, `compile`, `run`)
- **Live mode** — talks to a *running* Unity Editor through a local daemon (`tool call`); requires the Unity-side UPM package

**Setup:** if `uniforge --version` fails, or the Unity-side plugin / live-tool connection is not set up yet, read [references/setup.md](references/setup.md) for installation and first-time workflow.

**CI:** for GitHub Actions workflows (installing the CLI on runners, test/build jobs, license handling), read [references/github-actions.md](references/github-actions.md).

## Checking status and diagnosing problems

| Command | What it tells you |
|---|---|
| `uniforge doctor [project]` | Diagnoses stale Unity runtime state (leftover lock/runtime files, orphan licensing clients) that can block Editor startup or batch mode. Read-only by default. |
| `uniforge doctor [project] --fix` | Repairs only verified-stale files; never touches state while a matching Unity process is running. |
| `uniforge daemon status` | Whether the local daemon is running. |
| `uniforge tool projects` | Which running Unity Editors are connected to the daemon. |
| `uniforge editor list` | Installed Unity Editor versions. |
| `uniforge license status` | Unity license state (mainly for CI runners). |
| `uniforge logs` | Recent Unity Editor log output (`-f` to follow, `--trace` for project stack traces). |

Typical triage order when "Unity won't start" or "batch mode hangs": `doctor` → `logs --trace` → `doctor --fix`.

## Command reference

### Batch mode (Editor must be closed)

```bash
uniforge compile <project>
uniforge test <project> --platform editmode|playmode
uniforge test <project> --platform editmode --filter MyTestClass
uniforge test <project> --platform editmode --results ./test-results.xml
uniforge run <project> -- -executeMethod Build.Perform -buildTarget StandaloneOSX
```

Common flags: `--ci` (GitHub Actions annotations, log grouping), `--log-file <path>`, `--timeout <seconds>`, `-t/--timestamp`.

### Live tools (Editor must be open, UPM package installed)

```bash
uniforge tool list                                   # available tools
uniforge tool describe <tool-name>                   # argument schema
uniforge tool call editor-state -o json
uniforge tool call run-tests '{"mode":"EditMode"}' -o json
uniforge tool call run-tests '{"mode":"EditMode","test_names":"MyTestClass"}' -o json
uniforge tool call <tool-name> --project my-game -o json   # choose among connected projects
```

Key tools: `run-tests` (waits for completion and returns results), `editor-state`, `refresh` (AssetDatabase refresh), `hierarchy`, `list-tests`.

### Editor & project management

```bash
uniforge editor install [version] [--modules ios,android] [-p <project>]
uniforge editor list
uniforge editor available --lts --latest -o json
uniforge open <project|name>          # open Unity Editor
uniforge close <project> [--force]
uniforge restart <project>
uniforge project list [-o json]
uniforge project path <name>
```

### .meta files, logs, daemon

```bash
uniforge meta check <project>                 # missing/orphan .meta, duplicate GUIDs
uniforge meta check <project> --fix [--force]
uniforge logs [-n 500] [-f] [--trace] [--full-trace] [--editor]
uniforge daemon start|stop|restart|status     # rarely needed; auto-started on demand
```

## Tips

- **Batch mode and an open Editor are mutually exclusive.** If the project is open in the Editor, `compile|test|run` fails with a lock error — use `uniforge tool call` instead (e.g. `run-tests`). Conversely, close the Editor (`uniforge close`) before CI-style runs in batch mode.
- **Choosing a test platform:** type/namespace/asmdef/Editor-extension changes → `editmode`; runtime or UI behavior → `playmode`; just want a compile check → `compile`.
- **Run `meta check` after file operations** — especially after `git mv`, directory restructuring, or `git merge`/`git rebase`. Missing or orphan `.meta` files and duplicate GUIDs cause subtle asset breakage.
- **The daemon is self-managing.** `tool call`, `tool list`, and `tool projects` auto-start it (`--auto-start-daemon` defaults to true). Only reach for `daemon restart` if `tool projects` shows a stale connection.
- **Output formats:** live-tool output defaults to YAML; pass `-o json` for machine-readable output. `project list` and `editor available` also accept `-o json` (`--output`).
- **Timeout units differ:** `--timeout` on `compile|test|run` is in **seconds**; `tool call --timeout` is in **milliseconds** (default 300000 = 5 min). Raise it for long PlayMode suites.
- **`uniforge restart <project>`** is the quickest fix when the Editor becomes unresponsive or stops picking up script changes.
- **Use `--ci` in CI** for grouped logs, GitHub Actions annotations, and stack-trace noise filtering.
- **Prefer root-level `test|compile|run`** — the `batch` forms remain as hidden deprecated aliases.
- **Useful environment variables:** `UNIFORGE_HUB_PATH` (Unity Hub location), `UNIFORGE_EDITOR_BASE_PATH` (custom Editor base dir), `UNIFORGE_EDITOR` (external editor for the `project` TUI), `UNIFORGE_BIN` (binary path for daemon auto-start), `UNIFORGE_LOG_LEVEL`, `UNIFORGE_TIMEOUT`, `UNIFORGE_NO_COLOR`, and for CI licensing: `UNITY_USERNAME`, `UNITY_PASSWORD`, `UNITY_SERIAL`.
