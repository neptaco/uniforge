---
name: uniforge
description: Automate Unity development with the UniForge CLI — run EditMode/PlayMode tests, compile projects, manage Unity Editor versions and lifecycle, validate .meta files, read Unity logs, diagnose stale runtime state, and execute live editor tools via the local daemon using tool call. Use when working on a Unity project and you need to run tests after C# changes, compile-check, open/close/restart Unity Editor, check .meta integrity, inspect Unity logs, or connect an AI agent to a running Unity Editor.
---

# UniForge CLI Reference

UniForge (`uniforge`) is a cross-platform CLI for Unity development automation.
It has two modes of operation:

- **Batch mode** — drives Unity in batchmode while the Editor is *closed* (`test`, `compile`, `run`)
- **Live mode** — talks to a *running* Unity Editor through a local daemon (`tool call`); requires the Unity-side UPM package

**Setup:** if `uniforge --version` fails, or the Unity-side package / live-tool connection is not set up yet, read [references/setup.md](references/setup.md) for installation and first-time workflow.

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
uniforge compile
uniforge test --platform editmode|playmode
uniforge test --platform editmode --filter MyTestClass
uniforge test --platform editmode --results ./test-results.xml
uniforge run -- -executeMethod Build.Perform -buildTarget StandaloneOSX
```

These commands detect the Unity project containing the current directory. Pass an explicit project path as the first argument when running elsewhere. Common flags: `--ci` (GitHub Actions annotations, log grouping), `--log-file <path>`, `--timeout <seconds>`, `-t/--timestamp`.

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
uniforge open [project|name]          # open Unity Editor
uniforge close [project] [--force]
uniforge restart [project]
uniforge project list [-o json]
uniforge project path <name>
```

### .meta files, logs, daemon

```bash
uniforge meta check [project]                 # missing/orphan .meta, duplicate GUIDs
uniforge meta check [project] --fix [--force]
uniforge logs [-n 500] [-f] [--trace] [--full-trace] [--editor]
uniforge daemon start|stop|restart|status     # rarely needed; auto-started on demand
```

## Tips

- **Install a GitHub UPM package:** From inside the Unity project, run `uniforge package add <owner/repository/path/to/package>`. Pass an explicit project path before the package source when running elsewhere. The latest semantic-version tag is selected automatically; use `--tag vX.Y.Z` to pin it. Before editing the project manifest, UniForge compares the resolved package's minimum Unity version with `ProjectSettings/ProjectVersion.txt`. Newer streams, including alpha and beta releases, are accepted when their ordered Unity version meets the minimum; do not invent an upper support bound. Interactive terminals show the versions and resolved values for confirmation. Non-interactive runs skip only the prompt and still perform the compatibility check. Use `--yes` to skip a terminal prompt, or `--force` only to intentionally bypass a failed or unavailable compatibility check.
- **Update from a package notification:** From inside the Unity project, run `uniforge package update`. Pass a project path when running elsewhere.
- **Pin the package to a release tag:** Use `uniforge package add <owner/repository/path/to/package> --tag vX.Y.Z` for the first installation.
- **Batch mode and an open Editor are mutually exclusive.** If the project is open in the Editor, `compile|test|run` fails with a lock error — use `uniforge tool call` instead (e.g. `run-tests`). Conversely, close the Editor (`uniforge close`) before CI-style runs in batch mode.
- **Choosing a test platform:** type/namespace/asmdef/Editor-extension changes → `editmode`; runtime or UI behavior → `playmode`; just want a compile check → `compile`.
- **For repeatable gameplay automation, prefer an existing bot, autoplay, or test-driver mode.** If none exists and reliable PlayMode or end-to-end coverage requires one, propose a minimal opt-in test mode before implementing it. Do not add or enable bot behavior without user approval, and keep it disabled in normal builds.
- **Run `meta check` after file operations** — especially after `git mv`, directory restructuring, or `git merge`/`git rebase`. Missing or orphan `.meta` files and duplicate GUIDs cause subtle asset breakage.
- **A failing test run is not a tool malfunction.** `tool call run-tests` exits non-zero when tests fail, but the result payload (`run_id`, `fail_count`, `message`) is still printed. Read `message` to distinguish "tests failed" from "the run could not start" (e.g. `No tests matched`, `A test run is already in progress`).
- **Correlate test results by `run_id`.** When fetching details with `tool call test-results`, pass the `run_id` from the `run-tests` response (`'{"run_id":"<id>"}'`). A bare `test-results` returns the most recently created run (including one still running) — check `run_id`/`completed`/`running` in the response before treating it as the run you just started. If `run-tests` failed to start, no new run exists and you will see the previous one.
- **Lost the `run-tests` response? Use `after_run_id`.** `tool call test-results '{"after_run_id":"<previous-run-id>","wait":true,"timeout":300000}'` waits for the run started after the anchor and returns its completed results — no polling loops needed. An unknown anchor errors instead of falling back to the latest run.
- **A minimized or fully hidden editor may never start a run** (OS-level loop starvation; an unfocused-but-visible editor is fine). If a timeout says the run "has not reported starting", bring the editor forward (`osascript -e 'tell application "Unity" to activate'` on macOS) and retry.
- **The daemon is self-managing.** `tool call`, `tool list`, and `tool projects` auto-start it (`--auto-start-daemon` defaults to true). Only reach for `daemon restart` if `tool projects` shows a stale connection.
- **Output formats:** live-tool output defaults to YAML; pass `-o json` for machine-readable output. `project list` and `editor available` also accept `-o json` (`--output`).
- **Timeout units differ:** `--timeout` on `compile|test|run` is in **seconds**; `tool call --timeout` is in **milliseconds** (default 300000 = 5 min). Raise it for long PlayMode suites.
- **Run `uniforge restart` from inside the project** when the Editor becomes unresponsive or stops picking up script changes. Pass a project path when running elsewhere.
- **Use `--ci` in CI** for grouped logs, GitHub Actions annotations, and stack-trace noise filtering.
- **Prefer root-level `test|compile|run`** — the `batch` forms remain as hidden deprecated aliases.
- **Useful environment variables:** `UNIFORGE_HUB_PATH` (Unity Hub location), `UNIFORGE_EDITOR_BASE_PATH` (custom Editor base dir), `UNIFORGE_EDITOR` (external editor for the `project` TUI), `UNIFORGE_BIN` (binary path for daemon auto-start), `UNIFORGE_LOG_LEVEL`, `UNIFORGE_TIMEOUT`, `UNIFORGE_NO_COLOR`, and for CI licensing: `UNITY_USERNAME`, `UNITY_PASSWORD`, `UNITY_SERIAL`.
