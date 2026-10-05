# STAY — Shells That Await You

A persistent web terminal for [Coder](https://coder.com) workspaces. Open a browser, click a
button, and you are back in your running terminals: Claude Code in one, a few more for
running and testing code. **Close the browser and everything keeps running.** Open it from
any other device and the terminals are exactly where you left them.

- One static Go binary (the web UI is embedded). No database, no extra services.
- Tabs and splits, each terminal with its own working directory and optional startup command.
- Several browsers can watch the same terminals at once.
- No login of its own: it listens on `127.0.0.1` only and relies on Coder for authentication,
  TLS and access control.

```
Browser ──▶ Coder app proxy (login, TLS) ──▶ 127.0.0.1:7681  stay daemon ──▶ PTYs (your shells)
```

The daemon owns the shells. Browsers only view them, so closing a tab never kills a process.
When a browser reconnects, the daemon replays the recent output and nudges the terminal
(SIGWINCH) so full-screen programs like Claude Code redraw cleanly. Design details, the
websocket protocol and a list of known risks are in [docs/SPEC.md](docs/SPEC.md).

**What STAY does not do:** survive the workspace stopping or the daemon restarting. Those end
every shell. Closing browsers, switching devices and losing your network are all safe.

---

## Contents

- [Use it in a Coder template](#use-it-in-a-coder-template)
- [Build from source](#build-from-source)
- [Run it](#run-it)
- [Configure terminals and layout](#configure-terminals-and-layout)
- [Development](#development)
- [Releasing](#releasing)
- [Troubleshooting](#troubleshooting)
- [Security model](#security-model)

---

## Use it in a Coder template

The `terraform/` directory in this repo is a Coder module. It installs the release binary
(checksum-verified), starts the daemon, and adds an **STAY** button to the workspace page.
It needs a published release first; see [Releasing](#releasing).

### 1. Minimal

Add the module to your template's `main.tf`, next to your `coder_agent`:

```tf
module "stay" {
  source   = "git::https://github.com/bbushvt/stay.git//terraform?ref=v0.1.0"
  agent_id = coder_agent.main.id
}
```

Push the template (`coder templates push`), restart your workspace, and click **STAY**.
`ref=` is the release tag; pinning a tag keeps template updates deliberate.

### 2. With the repo directory (recommended)

Each terminal starts in `$HOME/<repo>`, where `<repo>` comes from `CODER_GIT_REPO_URL`. That
variable is **not** set automatically; expose it in the agent's environment:

```tf
data "coder_parameter" "repo_url" {
  name         = "repo_url"
  display_name = "Git repository"
  type         = "string"
  default      = "https://github.com/bbushvt/stay.git"
  mutable      = false
}

resource "coder_agent" "main" {
  os   = "linux"
  arch = "amd64"

  # STAY reads this to choose each terminal's default directory:
  #   https://github.com/org/my-app.git  ->  $HOME/my-app   (falls back to $HOME)
  env = {
    CODER_GIT_REPO_URL = data.coder_parameter.repo_url.value
  }
}

module "stay" {
  source   = "git::https://github.com/bbushvt/stay.git//terraform?ref=v0.1.0"
  agent_id = coder_agent.main.id
}
```

STAY does not clone the repository. Use whatever you already use for that (for example the
`git-clone` module from the Coder registry, or your own `coder_script`) and give it the same
URL. If the directory does not exist yet when STAY starts, terminals start in `$HOME`.

### 3. With a layout shipped in the template

Pass the layout as a string or load it from a file next to your template. It is written to
`~/.config/stay/layout.yaml` each time the workspace starts.

```tf
module "stay" {
  source       = "git::https://github.com/bbushvt/stay.git//terraform?ref=v0.1.0"
  agent_id     = coder_agent.main.id
  stay_version = "0.1.0"        # pin the binary too; default is "latest"
  order        = 1
  group        = "Terminals"

  # Inline...
  layout = <<-YAML
    terminals:
      - id: claude
        name: Claude
        command: claude --continue
      - id: dev
        name: Dev
      - id: test
        name: Test
    tabs:
      - title: Claude
        root: claude
      - title: Work
        root: {split: horizontal, children: [dev, test]}
  YAML

  # ...or from a file (use one or the other):
  # layout = file("${path.module}/stay-layout.yaml")
}
```

### 4. Everything the module accepts

| Input | Default | Notes |
|---|---|---|
| `agent_id` | required | the `coder_agent` to attach to |
| `stay_version` | `"latest"` | or a version such as `"0.1.0"` (no leading `v`) |
| `port` | `7681` | loopback port inside the workspace (1024–65535) |
| `layout` | `""` | layout YAML; empty leaves any existing `~/.config/stay/layout.yaml` alone |
| `share` | `"owner"` | `owner`, `authenticated` or `public`. STAY has no login and a terminal is a shell: keep `owner` |
| `repo` | `"bbushvt/stay"` | GitHub `owner/name` that publishes releases (set this if you fork) |
| `install_dir` | `"$HOME/.local/bin"` | where the binary is installed |
| `order`, `group` | `null` | placement of the app button |

Output: `app_url` (the in-workspace URL).

### What happens on each workspace start

1. Resolves the release (`latest` follows GitHub's `/releases/latest` redirect; no API token
   or rate limit).
2. Downloads `stay_<version>_linux_<amd64|arm64>.tar.gz` and verifies it against the
   release's `checksums.txt`. A mismatch aborts the install.
3. Installs `~/.local/bin/stay`, writes the layout if you passed one, and starts the daemon
   detached, logging to `~/.local/state/stay/stay.log`.

Things to know:

- **A running daemon is never restarted by the script**, because restarting ends every shell.
  A new binary version or layout takes effect the next time the daemon starts (for example
  after a workspace restart). To apply it right now, stop `stay` yourself and re-run the
  module's script from the Coder UI.
- If an upgrade fails (offline, bad checksum) the already-installed binary still starts.
- Terminals inherit the daemon's environment, which is the agent's: `CODER_*`,
  `SSH_AUTH_SOCK`, your `coder_agent.env`, and so on.
- The app uses `subdomain = true`. Your Coder deployment needs wildcard app hostnames
  configured (`--wildcard-access-url`); that is a Coder requirement for any subdomain app.

More module detail: [terraform/README.md](terraform/README.md).

---

## Build from source

### Prerequisites

| Tool | Version | Why |
|---|---|---|
| Go | 1.27 | backend |
| Node.js + npm | 24 | builds the web UI (npm, not pnpm) |
| make | any | convenience targets |

The repo pins Go and Node in `mise.toml`. With [mise](https://mise.jdx.dev) installed:

```sh
mise install        # installs the pinned Go and Node
```

Otherwise install those versions however you like.

### Build

```sh
git clone https://github.com/bbushvt/stay.git
cd stay
make build          # npm install + vite build, then go build -> ./stay
```

That produces a single `./stay` binary with the web UI embedded. Without `make`:

```sh
cd web && npm install && npm run build && cd ..
go build -o stay .
```

Build order matters: the UI must be built **before** `go build`, because Go embeds
`web/dist` at compile time. If you skip it the binary still compiles (a placeholder file
keeps the embed valid) but serves a bare directory listing instead of the app.

Check the build:

```sh
./stay --version    # prints "stay dev" for a local build
```

### Make targets

| Command | Does |
|---|---|
| `make build` | build the web UI, then the `./stay` binary |
| `make run` | build and run on `127.0.0.1:7681` |
| `make test` | `go test -race ./...` |
| `make check` | tests, TypeScript typecheck, `gofmt`, `go vet` (what CI runs) |
| `make clean` | remove `./stay` and built UI files |

`go test ./...` works without building the web UI.

---

## Run it

```sh
./stay
# 2026/10/04 19:00:04 no layout file; using built-in layout
# 2026/10/04 19:00:04 stay dev listening on http://127.0.0.1:7681
```

Then open <http://127.0.0.1:7681>. Inside a Coder workspace, use the **STAY** button
instead (see above); the daemon is only reachable from inside the workspace.

### Flags

| Flag | Default | Meaning |
|---|---|---|
| `--listen` | `127.0.0.1:7681` | address to listen on. **Must be a loopback address**; STAY refuses anything else because it has no authentication |
| `--config` | _(see below)_ | layout file. If given, it must exist |
| `--version` | | print the version and exit |

### Running it in the background yourself

Outside Coder (inside Coder the module does this for you):

```sh
mkdir -p ~/.local/state/stay
setsid nohup ./stay > ~/.local/state/stay/stay.log 2>&1 < /dev/null &
echo $! > ~/.local/state/stay/stay.pid

# stop it (ends all its shells):
kill "$(cat ~/.local/state/stay/stay.pid)"
```

On SIGINT/SIGTERM the daemon shuts down and hangs up the shells it started. It only touches
processes it spawned.

### What you will see

With no layout file, STAY starts three terminals: **Claude** (runs `claude` if it is on
your `PATH`), **Dev** and **Test**, arranged as a Claude tab and a Shells tab with Dev and
Test side by side. Drag the dividers to resize; sizes and the selected tab are remembered in
that browser only. If a shell exits, its pane shows the exit code and a **Restart** button.

---

## Configure terminals and layout

The daemon looks for a layout in this order:

1. `--config <path>` (an error if the file is missing)
2. `~/.config/stay/layout.yaml` (honours `$XDG_CONFIG_HOME`)
3. the built-in layout described above

A file that exists but is invalid stops the daemon with a message naming the problem
(unknown fields, undefined terminals, bad splits, duplicate ids, and so on). Changes need a
daemon restart, which ends the running shells.

```yaml
terminals:
  - id: claude                  # used in URLs: no slashes or spaces
    name: Claude                # label; defaults to the id
    command: claude --continue  # optional; typed into the shell after it starts
  - id: dev
    name: Dev
    dir: .                      # optional working directory
  - id: logs
    name: Logs
    dir: "~"                    # quote it: a bare ~ is YAML null, i.e. "not set"
  - id: test
    name: Test

tabs:
  - title: Claude
    root: claude                # a terminal id...
  - title: Work
    root:                       # ...or a split of two or more nodes
      split: horizontal         # horizontal = side by side, vertical = stacked
      children:
        - dev
        - split: vertical       # splits can nest
          children: [logs, test]
```

- Each terminal may appear in exactly one pane.
- `command` is typed into a normal shell, so quitting it (for example `claude`) leaves you at
  a prompt rather than a dead pane.
- `dir` may be absolute, `~` or `~/x`, contain `$VARS`, or be relative to the default
  directory. It must exist. When omitted it defaults to `$HOME/<repo>`, with `<repo>` from
  `$CODER_GIT_REPO_URL`, falling back to `$HOME`.

Ready-to-copy files:

- [`examples/my-workspace.yaml`](examples/my-workspace.yaml): Claude tab plus a Work tab with
  Dev, Run and Test.
- [`examples/layout.yaml`](examples/layout.yaml): every option, annotated.

```sh
mkdir -p ~/.config/stay
cp examples/my-workspace.yaml ~/.config/stay/layout.yaml
```

---

## Development

```sh
# terminal 1: the daemon (serves the API and websockets on 7681)
make build && ./stay

# terminal 2: the web UI with hot reload
cd web
npm install
npm run dev          # Vite dev server; proxies /ws and /api to 127.0.0.1:7681
```

Open the URL Vite prints. The Vite server needs the daemon running on `127.0.0.1:7681`.

| Task | Command |
|---|---|
| Go tests (race detector) | `make test` |
| Everything CI runs | `make check` |
| TypeScript typecheck only | `cd web && npm run typecheck` |
| Production UI build only | `cd web && npm run build` |

Project layout:

```
main.go                  flags, wiring, signals
internal/terminal/       PTY sessions, ring-buffer history, terminal manager
internal/server/         HTTP routes, websocket protocol, origin checks
internal/config/         YAML layout parsing and validation
web/                     React + xterm.js frontend (embedded via web/embed.go)
terraform/               Coder module
examples/                sample layouts
docs/SPEC.md             design, protocol, risks
CLAUDE.md                conventions for AI-assisted work in this repo
```

Tests never need a browser: they drive the websocket directly against an in-process server.

---

## Releasing

Releases are built by [GoReleaser](https://goreleaser.com) from `.goreleaser.yaml`.

```sh
git tag v0.1.0
git push origin v0.1.0     # the "release" GitHub Actions workflow does the rest
```

The workflow builds the web UI, then static Linux amd64 and arm64 binaries, packs them as
`stay_<version>_linux_<arch>.tar.gz`, and attaches those plus `checksums.txt` to a GitHub
release. The archive names are a contract with `terraform/run.sh`; do not rename them.

Dry run locally, publishing nothing (needs the `goreleaser` CLI):

```sh
goreleaser check
goreleaser release --snapshot --clean      # artifacts land in ./dist
```

CI (`.github/workflows/ci.yml`) runs `make check` and a full build on every push and pull
request.

---

## Troubleshooting

| Symptom | Likely cause and fix |
|---|---|
| Page loads but the terminal never connects, or the websocket returns **403** | STAY only accepts same-origin websocket upgrades (`Origin` host must equal `Host`). Reach it through the Coder **STAY** button or `http://127.0.0.1:7681` directly. If it fails only behind Coder, the proxy may not be preserving `Host`; please open an issue with the response headers |
| `./stay` shows a file listing (just `.gitkeep`) instead of the app | the web UI was not built before `go build`. Run `make build` |
| `address already in use` | something else holds the port. Pick another with `--listen 127.0.0.1:PORT` (and set the module's `port` to match) |
| `--listen ... is not a loopback address` | intentional. STAY has no auth, so it refuses to bind publicly |
| `working directory "..." does not exist` | a `dir` in your layout points at a missing path. Fix it or remove it to use the default |
| Terminals start in `$HOME`, not the repo | `CODER_GIT_REPO_URL` is not in the daemon's environment, or the repo is not cloned yet. See [template example 2](#2-with-the-repo-directory-recommended) |
| Everything vanished after the workspace restarted | expected: STAY survives closing browsers, not stopping workspaces. Use `claude --continue` as the Claude terminal's command to resume the conversation |
| Install script fails with "no releases yet" | the module downloads from GitHub releases; publish one first (see [Releasing](#releasing)) |
| Terminal looks garbled after reconnecting | programs that do not repaint on resize cannot be fully restored from replayed output; run `reset` or `clear`. Claude Code and similar full-screen apps redraw on their own |

Logs: `~/.local/state/stay/stay.log` when started by the Coder module, otherwise wherever you
redirected stdout/stderr.

---

## Security model

STAY has **no authentication**. It protects itself by listening on loopback only and by
rejecting cross-origin websocket and restart requests, and it relies on Coder to authenticate
who can reach it.

- Anything that can reach the port gets a shell as the workspace user. Keep the module's
  `share = "owner"`.
- Other processes inside the workspace can also connect to `127.0.0.1:7681`. On a single-user
  workspace they already run as you, so nothing is gained; on a shared workspace, reconsider.
- Layout `command` values and the module's inputs run as you, in your shell. Treat layout
  files like any other script.

The full threat discussion and the other known risks are in [docs/SPEC.md §7–8](docs/SPEC.md).
