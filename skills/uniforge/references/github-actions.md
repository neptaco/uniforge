# UniForge on GitHub Actions

Using UniForge in CI/CD workflows: installing the CLI on runners, running tests and builds, and managing Unity licenses. Unity itself runs on self-hosted runners with Unity Hub installed.

## Installing the CLI on a runner

Use the official action (Linux/macOS/Windows, amd64/arm64):

```yaml
- uses: neptaco/setup-uniforge@v1
```

Inputs:

| Input | Default | Description |
|---|---|---|
| `version` | `latest` | Release to install. Accepts `latest`, a full tag (`v0.9.0`), or a partial version (`v0`, `v0.9`) resolved to the newest matching release. |
| `ref` | — | Build from source at a branch/tag/commit instead of downloading a release. Requires Go on the runner. |
| `token` | — | GitHub token for API requests; set it to avoid rate limits when using a partial `version`. |

The action downloads the release archive, verifies its checksum, and adds `uniforge` to `PATH`.

## CI workflow: tests on every push

```yaml
name: CI

on: [push, pull_request]

jobs:
  test:
    runs-on: unity   # self-hosted runner with Unity Hub
    steps:
      - uses: actions/checkout@v4

      - uses: neptaco/setup-uniforge@v1

      - name: Install Unity
        run: uniforge editor install   # auto-detects the version from the project

      - name: Check .meta files
        run: uniforge meta check .

      - name: Run Tests
        run: uniforge test . --platform editmode --ci
```

## Build workflow: multi-platform matrix

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
        run: uniforge run . --ci -- -executeMethod Build.Perform -buildTarget ${{ matrix.target }}
```

## The `--ci` flag

Always pass `--ci` to `test|compile|run` in CI. It enables:

- GitHub Actions annotations (`::error::`/`::warning::`) so failures surface on the PR
- Log grouping (collapsible sections in the run log)
- Stack-trace and startup-noise filtering for readable output

## License management on CI runners

Personal licenses activate without a serial; Plus/Pro require one. Store credentials as repository secrets and pass them via the environment variables the CLI reads (`UNITY_USERNAME`, `UNITY_PASSWORD`, `UNITY_SERIAL`):

```yaml
- name: Activate license
  run: uniforge license activate
  env:
    UNITY_USERNAME: ${{ secrets.UNITY_USERNAME }}
    UNITY_PASSWORD: ${{ secrets.UNITY_PASSWORD }}
    UNITY_SERIAL: ${{ secrets.UNITY_SERIAL }}

# ... build/test steps ...

- name: Return license
  if: always()
  run: uniforge license return
```

`uniforge license status` reports the active license source (serial file, Unity Hub login, or licensing server). Persistent self-hosted runners that stay logged in to Unity Hub don't need per-run activation.

## Tips for CI

- **Raise timeouts for long jobs**: `test --timeout <seconds>` (default 600) and `run --timeout <seconds>` (default 3600). PlayMode suites and IL2CPP builds often need more.
- **Recover from a crashed previous run**: `uniforge doctor . --fix` removes verified-stale lock/runtime files that would otherwise block batch mode on a persistent runner.
- **Fail fast on asset issues**: run `uniforge meta check .` before tests; add `--fix --force` only if you want CI to auto-repair (it modifies the working tree).
- **Save artifacts**: `test --results ./test-results.xml` writes the test results XML; `--log-file <path>` captures the full Unity log for upload with `actions/upload-artifact`.
- **Ephemeral runners**: return the license with `if: always()` so a failed build doesn't leak a seat.
