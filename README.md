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

- [Quick start in Coder](#quick-start-in-coder) (start here)
- [Using STAY](#using-stay)
- [Coder module reference](#coder-module-reference) (options, variations, what the script does)
- [Build from source](#build-from-source)
- [Run it outside Coder](#run-it-outside-coder)
- [Configure terminals and layout](#configure-terminals-and-layout)
- [Development](#development)
- [Releasing](#releasing)
- [Troubleshooting](#troubleshooting)
- [Security model](#security-model)

---

## Quick start in Coder

You need:

- an existing Coder template you can edit, with a `coder_agent`
- a Linux workspace (amd64 or arm64)
- a Coder deployment with wildcard app hostnames enabled (`--wildcard-access-url`), which Coder
  requires for any subdomain app

Do these three steps in order.

### Step 1. Add the module to your template

Add this block to your template's `main.tf`, anywhere alongside your existing `coder_agent`.
It is the only thing STAY needs:

```tf
module "stay" {
  source   = "git::https://github.com/bbushvt/stay.git//terraform?ref=v0.2.0"
  agent_id = coder_agent.main.id   # use your agent's reference if it isn't named "main"
}
```

The module installs the STAY binary, starts the daemon, and adds the **STAY** button to the
workspace page. `ref=v0.2.0` is the STAY release to use; pinning a tag keeps template updates
deliberate.

**Recommended: make sure your agent exports `CODER_GIT_REPO_URL`.** STAY uses it to start each
terminal in `$HOME/<repo>` (for `https://github.com/org/my-app.git`, that is `$HOME/my-app`).
If your existing `coder_agent` doesn't already set it, add it to that agent's `env` map:

```hcl
env = {
  CODER_GIT_REPO_URL = "https://github.com/org/my-app.git"   # or your template's repo variable
}
```

Without it, terminals simply start in `$HOME`. STAY does not clone the repository; clone it
with whatever your template already does, using the same URL. If the directory isn't there yet
when STAY starts, terminals start in `$HOME`.

### Step 2. Push the template and (re)start a workspace

```sh
coder templates push <your-template-name>
```

Then **restart** an existing workspace (or create a new one) so the module's script runs.
The script runs on every workspace start; the first run downloads and installs STAY.

### Step 3. Click **STAY**

Open the workspace page and click the **STAY** button. You get a terminal in your browser.
See [Using STAY](#using-stay) for what to do next.

If the button does not appear or the page does not load, see
[Troubleshooting](#troubleshooting).

---

## Using STAY

Once the **STAY** button opens the page:

- **First launch:** with no layout file you get three terminals: **Claude** (runs `claude` if
  it is installed), **Dev** and **Test**. Claude has its own tab; Dev and Test share a
  second tab, side by side.
- **Type like any terminal.** Each one is a real shell in your repo directory (or `$HOME`).
  Links in the output are clickable, and programs can copy to your clipboard.
- **Copy and paste** work like a desktop terminal. On Linux and Windows, **Ctrl-C** copies
  when text is selected (with nothing selected it still sends an interrupt) and **Ctrl-V**
  pastes; **Ctrl-Shift-C/V** always copy and paste. On a Mac, use **Cmd-C** and **Cmd-V**;
  **Ctrl-C** always interrupts.
- **Switch tabs** with the tab bar. Terminals on hidden tabs keep running and stay connected.
- **Resize splits** by dragging the divider. Sizes and the selected tab are remembered in
  that browser only.
- **Close the browser whenever you like.** Nothing stops. Your shells, running programs and
  Claude Code keep going.
- **Come back from any device.** Click **STAY** again. Each terminal replays its recent
  output and full-screen programs redraw, so you see where you left off. Several browsers can
  be attached at the same time; whichever one resized last decides the terminal's size.
- **If a program exits** (for example you type `exit`), the pane shows "process exited with
  code N" and a **Restart** button that starts a fresh shell there.
- **If your network drops,** the page reconnects by itself and replays.
- **What does end your terminals:** stopping the workspace, or the STAY daemon being stopped
  or restarted. That is the one thing STAY cannot survive. Using `claude --continue` as the
  Claude terminal's command picks the conversation back up afterwards.
- **Change the terminals and layout** (names, directories, startup commands, tabs, splits) by
  providing a layout file. See [Configure terminals and layout](#configure-terminals-and-layout)
  and the [module variations](#variations-each-one-independent).

---

## Coder module reference

The `terraform/` directory in this repo is the module. This section is reference material;
you do not need any of it for the [quick start](#quick-start-in-coder).

### Variations (each one independent)

These are **not steps** and do not build on each other. Each one is a change you can make to
the `module "stay" { ... }` block from Step 1; apply whichever you want, in any combination.

**Pin the STAY version** (the default `latest` installs the newest release when the workspace
starts):

```tf
module "stay" {
  source       = "git::https://github.com/bbushvt/stay.git//terraform?ref=v0.2.0"
  agent_id     = coder_agent.main.id
  stay_version = "0.2.0"
}
```

**Ship a layout inline** (written to `~/.config/stay/layout.yaml` each time the workspace
starts):

```tf
module "stay" {
  source   = "git::https://github.com/bbushvt/stay.git//terraform?ref=v0.2.0"
  agent_id = coder_agent.main.id

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
}
```

**Ship a layout from a file** next to your template (use this *instead of* the inline form,
not in addition to it):

```tf
module "stay" {
  source   = "git::https://github.com/bbushvt/stay.git//terraform?ref=v0.2.0"
  agent_id = coder_agent.main.id
  layout   = file("${path.module}/stay-layout.yaml")
}
```

A ready-made layout to start from is [`examples/my-workspace.yaml`](examples/my-workspace.yaml).

**Change the port or where the button appears** (`order = 1` puts the button ahead of apps with
a higher number; `group` collects it under a named section, see [All module inputs](#all-module-inputs)):

```tf
module "stay" {
  source   = "git::https://github.com/bbushvt/stay.git//terraform?ref=v0.2.0"
  agent_id = coder_agent.main.id
  port     = 7700
  order    = 1
  group    = "Terminals"
}
```

### All module inputs

| Input | Default | Notes |
|---|---|---|
| `agent_id` | required | the `coder_agent` to attach to |
| `stay_version` | `"latest"` | or a version such as `"0.1.0"` (no leading `v`) |
| `port` | `7681` | loopback port inside the workspace (1024–65535) |
| `layout` | `""` | layout YAML; empty leaves any existing `~/.config/stay/layout.yaml` alone |
| `share` | `"owner"` | `owner`, `authenticated` or `public`. STAY has no login and a terminal is a shell: keep `owner` |
| `repo` | `"bbushvt/stay"` | GitHub `owner/name` that publishes releases (set this if you fork) |
| `install_dir` | `"$HOME/.local/bin"` | where the binary is installed |
| `order` | `null` | sort position of the **STAY** button among the workspace's apps: lower numbers come first, apps with the same (or no) order are sorted by name. Use it to put STAY first, e.g. `1` |
| `group` | `null` | name of a group to put the button under, e.g. `"Terminals"`. Apps with the same group name are collected into one expandable section on the workspace page. Unset means a standalone button |

Output: `app_url` (the in-workspace URL).

### What the module does on each workspace start

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
- The app is a subdomain app (`subdomain = true`) shared with the owner only by default.

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

## Run it outside Coder

```sh
./stay
# 2026/10/04 19:00:04 no layout file; using built-in layout
# 2026/10/04 19:00:04 stay dev listening on http://127.0.0.1:7681
```

Then open <http://127.0.0.1:7681>. (Inside a Coder workspace, use the module and the **STAY**
button instead; see the [quick start](#quick-start-in-coder). The daemon is only reachable
from inside the machine it runs on.)

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

What the page does and how to use it is described in [Using STAY](#using-stay).

---

## Configure terminals and layout

A layout file says which shells STAY runs and how they are arranged in the browser. This
section is the full reference, with examples you can copy.

- [Where the file goes](#where-the-file-goes)
- [How the layout works](#how-the-layout-works)
- [Terminals](#terminals)
- [Tabs and splits](#tabs-and-splits)
- [Sizes](#sizes)
- [Examples](#examples)
- [Validation and common mistakes](#validation-and-common-mistakes)

### Where the file goes

The daemon looks for a layout in this order:

1. `--config <path>` (an error if the file is missing)
2. `~/.config/stay/layout.yaml` (honours `$XDG_CONFIG_HOME`)
3. the built-in layout: a **Claude** tab, and a **Shells** tab with **Dev** and **Test** side by side

With the Coder module, pass the layout through the module's `layout` input (see the
[module variations](#variations-each-one-independent)), or put the file at the path above
yourself. The file is read once, **when the daemon starts**; editing it has no effect until
the daemon restarts, and a restart ends all running shells.

### How the layout works

A layout has two parts, kept separate on purpose:

1. **`terminals`** defines *what runs*: one entry per shell. The daemon starts every
   terminal when it starts, whether or not it is shown anywhere.
2. **`tabs`** defines *where it is shown*: each tab has a `title` and a `root`. A root is
   either a terminal `id` (one full-size pane) or a **split** containing two or more
   children, each of which is a terminal or another split. Nesting splits builds grids.

```
Tab "Work":  root = split horizontal [ dev, split vertical [ test, logs ] ]

  ┌──────────┬──────────┐
  │          │   test   │
  │   dev    ├──────────┤
  │          │   logs   │
  └──────────┴──────────┘
```

- **Each terminal appears in exactly one pane.** One that is defined but not placed in any
  tab still runs but is invisible; the daemon logs a warning at startup.
- **Terminals and views are independent.** Switching tabs, closing the browser or opening it
  on another device only changes what you are looking at. The shells keep running, and every
  browser shows the same layout attached to the same terminals.
- If a terminal's process exits, its pane stays where it is with a **Restart** button.

### Terminals

```
terminals:
  - id: claude
    name: Claude
    dir: .
    command: claude --continue
```

| Field | Required | Meaning |
|---|---|---|
| `id` | yes | Unique name used to place the terminal in `tabs` and in its URL. No slashes, spaces or URL punctuation (`? # %`) |
| `name` | no | Label shown in the pane. Defaults to the `id` |
| `dir` | no | Working directory (below). Defaults to `$HOME/<repo>`, else `$HOME` |
| `command` | no | Typed into the shell after it starts, as if you had typed it and pressed Enter |

**`command`** runs inside a normal interactive shell, so quitting the program (for example
`claude`) leaves you at a prompt instead of a dead pane. It can be a full command line:
`command: tail -f /tmp/app.log`.

**`dir`** may be:

| Value | Resolves to |
|---|---|
| _(omitted)_ | `$HOME/<repo>`, where `<repo>` is the last part of `$CODER_GIT_REPO_URL` without `.git`; `$HOME` if that is unset or the directory doesn't exist yet |
| `.` or `src` | relative to that default directory |
| `/var/log` | an absolute path |
| `"~"` or `~/notes` | your home directory, or under it. **Quote a lone `"~"`**: bare `~` is YAML null, which means "not set" |
| `$HOME/data` | environment variables are expanded |

The directory must exist, otherwise the daemon refuses to start and names the terminal.

### Tabs and splits

```
tabs:
  - title: Work
    root:
      split: horizontal
      children: [dev, test]
```

Each tab has a `title` (required) and a `root`. A node, anywhere in the tree, is one of:

| Form | Meaning |
|---|---|
| `dev` | a pane showing terminal `dev` |
| `{pane: dev}` | the same, in mapping form. Needed to attach a `size` to a pane |
| `{split: horizontal, children: [...]}` | children side by side, left to right |
| `{split: vertical, children: [...]}` | children stacked, top to bottom |

A split needs at least two children. Each child is any node, so splits nest as deep as you
like. Tabs appear in the tab bar in the order listed.

### Sizes

By default every child of a split gets an equal share. Give children a **`size`** to change
that. A size is a **percentage of the parent split** (a number above 0 and below 100), and
it can be set on any node that is a child of a split. A bare terminal id has no place to
write a size, so use `{pane: id, size: N}` instead. A tab's own `root` cannot have a size.

```yaml
terminals: [{id: dev}, {id: test}]
tabs:
  - title: Work
    root:
      split: horizontal
      children:
        - {pane: dev, size: 70}     # 70% of the width
        - {pane: test, size: 30}    # 30%
```

How sizes are resolved:

- **All children sized:** the sizes must add up to 100.
- **Some children sized:** the rest share what is left equally. In
  `[{pane: a, size: 50}, b, c]`, `a` gets 50% and `b` and `c` get 25% each. The sized
  children must add up to less than 100 so there is room for the others.
- **None sized:** equal shares.
- Sizes are relative to the **parent split only**, so a nested split's children are a
  percentage of that nested split, not of the whole tab.
- A pane cannot be dragged below 10% of its split.

**Sizes are starting values.** You can still drag dividers in the browser. STAY remembers
your dragged sizes in that browser only. If you later change a `size` in the layout file,
that split goes back to the file's values in every browser, and you can drag again from
there. Splits whose sizes you didn't touch in the file keep their remembered sizes.

### Examples

**One terminal**, the simplest layout:

```yaml
terminals:
  - id: main
tabs:
  - title: Shell
    root: main
```

**Two tabs**: Claude on its own, and a work tab with side-by-side shells (this is the
built-in layout, plus `command` for Claude):

```yaml
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
  - title: Shells
    root:
      split: horizontal
      children: [dev, test]
```

**Stacked panes**: a big editor shell above a short one for running things:

```yaml
terminals:
  - id: edit
  - id: run
tabs:
  - title: Work
    root:
      split: vertical
      children:
        - {pane: edit, size: 75}
        - {pane: run, size: 25}
```

**Nested splits**: `dev` on the left half; `test` over `logs` on the right half. Sizes at
each level are relative to that split:

```yaml
terminals:
  - id: dev
  - id: test
  - id: logs
    dir: /var/log
    command: tail -f syslog
tabs:
  - title: Work
    root:
      split: horizontal
      children:
        - {pane: dev, size: 50}          # 50% of the tab's width
        - split: vertical
          size: 50                       # the other 50%
          children:
            - {pane: test, size: 70}     # 70% of this column's height
            - {pane: logs, size: 30}     # 30%
```

**Partial sizing**: pin the narrow sidebar and let the rest share the remainder:

```yaml
terminals:
  - id: files
  - id: dev
  - id: test
tabs:
  - title: Work
    root:
      split: horizontal
      children:
        - {pane: files, size: 20}   # 20%
        - dev                       # 40%
        - test                      # 40%
```

**Everything together**: a full layout with directories, commands and sizes. The Claude
terminal starts in the repo; `notes` starts in `~/notes`; `server` runs a command from a
subdirectory of the repo:

```yaml
terminals:
  - id: claude
    name: Claude
    command: claude --continue
  - id: server
    name: Server
    dir: backend                 # relative to the repo directory
    command: make run
  - id: test
    name: Test
    dir: backend
  - id: notes
    name: Notes
    dir: ~/notes
tabs:
  - title: Claude
    root: claude
  - title: Backend
    root:
      split: horizontal
      children:
        - {pane: server, size: 60}
        - {pane: test, size: 40}
  - title: Notes
    root: notes
```

Ready-to-copy files:

- [`examples/my-workspace.yaml`](examples/my-workspace.yaml): Claude tab plus a Work tab with
  Dev, Run and Test.
- [`examples/layout.yaml`](examples/layout.yaml): every option, annotated.

```sh
mkdir -p ~/.config/stay
cp examples/my-workspace.yaml ~/.config/stay/layout.yaml
```

### Validation and common mistakes

A file that exists but is invalid stops the daemon with a message naming the problem.
Unknown fields are errors too, so a typo such as `comand:` is caught instead of ignored.

| Mistake | Error says |
|---|---|
| a pane names a terminal that isn't defined | `unknown terminal "x"` |
| the same terminal in two panes | `each terminal may appear once` |
| duplicate `id` | `duplicate terminal id` |
| split with fewer than two children | `a split needs at least 2 children` |
| `split` other than `horizontal`/`vertical` | `split must be horizontal or vertical` |
| a node with both `pane` and `split`, or neither | `exactly one of pane or split` |
| `size` of 0, negative, or 100 and above | `size must be a percentage greater than 0 and less than 100` |
| every child sized but not summing to 100 | `sizes in a split must add up to 100` |
| sized children already use up 100 | `leave no room for the children without one` |
| `size` on a tab's `root` | `size only applies to a child of a split` |
| `dir` that doesn't exist | `working directory "..." does not exist` |
| `dir: ~` unquoted | no error, but the terminal gets the default directory (bare `~` is YAML null) |

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
git tag v0.2.0
git push origin v0.2.0     # the "release" GitHub Actions workflow does the rest
```

Before tagging, add a `## vX.Y.Z` section to [CHANGELOG.md](CHANGELOG.md); the workflow
publishes that section as the release notes and fails if it is missing.

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
| Terminals start in `$HOME`, not the repo | `CODER_GIT_REPO_URL` is not in the daemon's environment, or the repo is not cloned yet. See [quick start, Step 1](#step-1-add-the-module-to-your-template) |
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
