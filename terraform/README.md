# STAY Coder module

Installs [STAY](../README.md) in a workspace, starts it, and adds an **STAY** button to
the workspace page. STAY is a persistent web terminal: close the browser and your shells
(and Claude Code) keep running.

```tf
module "stay" {
  source   = "git::https://github.com/bbushvt/stay.git//terraform?ref=v0.1.0"
  agent_id = coder_agent.main.id

  # optional: pin a release, and ship your layout with the template
  # stay_version = "0.1.0"
  # layout       = file("${path.module}/stay-layout.yaml")   # see examples/
}
```

## What it does

- `coder_script` (runs on every workspace start, never blocks login):
  1. Resolves the release (`latest` follows GitHub's redirect, no API token or rate limit).
  2. Downloads `stay_<version>_linux_<amd64|arm64>.tar.gz` and **verifies it against the
     release's `checksums.txt`** before installing to `~/.local/bin/stay`.
  3. Writes `~/.config/stay/layout.yaml` if `layout` is set.
  4. Starts the daemon on `127.0.0.1:<port>` (detached, log in `~/.local/state/stay/stay.log`)
     unless it is already running.
- `coder_app` with `subdomain = true` and `share = "owner"`, with a healthcheck on `/`.

## Inputs

| name | default | notes |
|---|---|---|
| `agent_id` | required | |
| `stay_version` | `"latest"` | or `"0.1.0"` (no `v`) |
| `port` | `7681` | loopback only |
| `layout` | `""` | YAML written to `~/.config/stay/layout.yaml` each start; empty leaves any existing file |
| `share` | `"owner"` | STAY has no login of its own and a terminal is a shell: keep `owner` |
| `repo` | `"bbushvt/stay"` | where releases come from |
| `install_dir` | `"$HOME/.local/bin"` | |
| `order` | `null` | sort position of the button: lower numbers first, ties sorted by name |
| `group` | `null` | group name; apps sharing it are collected into one section on the workspace page |

## Behaviour to know about

- **A running daemon is never restarted by the script.** Restarting ends every shell. A new
  version or layout takes effect the next time the daemon starts (for example after a
  workspace restart). To apply it now, stop `stay` yourself and re-run the script.
- A failed upgrade (network down, bad checksum) still starts the already-installed binary.
- **Default working directory** comes from `CODER_GIT_REPO_URL`, so the template must expose
  that variable in the agent's environment (for example via `coder_agent.env`); otherwise
  terminals start in `$HOME`. Terminals inherit the daemon's environment, which is the
  script's, i.e. the agent's.
- Workspace stop kills all shells; STAY survives closing browsers, not stopping workspaces.

## Releasing

Push a tag: `git tag v0.1.0 && git push origin v0.1.0`. The `release` workflow runs
GoReleaser, which builds the frontend, then the Linux amd64/arm64 binaries, archives,
and `checksums.txt`. The archive naming in `.goreleaser.yaml` is a contract with
`run.sh`. Dry run locally: `goreleaser release --snapshot --clean`.
