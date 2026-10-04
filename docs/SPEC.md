# STAY — Shells That Await You

A persistent web terminal that runs inside a Coder workspace. A single Go daemon owns
the shells; browsers are disposable views onto them.

## 1. Goals and non-goals

**Goals**
- Close the browser, nothing dies. Open it from any device, see where you left off.
- Claude Code in one terminal, a few more for running/testing code.
- One static binary, no external services, no auth UI.

**Non-goals**
- Multi-user support. One workspace owner, one STAY.
- Surviving workspace stop or daemon restart (see risk R6).
- Being a general terminal multiplexer (no scripting API, no session sharing UX).

## 2. Architecture

```
 Browser (xterm.js) ──ws──┐
 Browser (xterm.js) ──ws──┼──▶ Coder app proxy ──▶ 127.0.0.1:7681  stay daemon
 Browser (xterm.js) ──ws──┘     (login, TLS, ACL)       │
                                                        ├─ Session "claude" ── PTY ── $SHELL
                                                        └─ Session "tests"  ── PTY ── $SHELL
```

- **Daemon** (`stay`): HTTP server on `127.0.0.1` only. Serves the embedded frontend,
  a small JSON API, and one websocket per terminal.
- **Session** (`internal/terminal`): owns a PTY and its child process, independent of any
  websocket. Output is fanned out to zero or more attached clients. Input from any client
  is written to the PTY. A session with no clients keeps running.
- **Frontend** (`web/`): React + xterm.js. Pure view; holds no authoritative state.
- **Auth**: none in STAY. Coder (`coder_app` with `subdomain = true`, `share = "owner"`)
  authenticates and terminates TLS. See R1 for what that implies.

### Package layout

```
main.go                     flags, wiring, signal handling
internal/terminal/          Session: PTY lifecycle, fan-out (later: ring buffer, Manager)
internal/server/            HTTP routes, websocket handler, origin check, protocol types
web/                        Vite app; web/embed.go embeds web/dist
web/dist/.gitkeep           placeholder so go:embed compiles before the frontend is built
docs/SPEC.md                this file
```

## 3. Websocket protocol (v1)

Endpoint: `GET /ws/terminals/{id}` upgraded to websocket. One websocket per terminal.
The id is the terminal's stable name/slug. Unknown id → HTTP 404 before upgrade.

Framing:
- **Binary frames** = raw terminal bytes. Server→client: PTY output. Client→server: input
  (keystrokes, paste), written verbatim to the PTY.
- **Text frames** = JSON control messages, each an object with a `type` field. Unknown
  types are ignored (forward compatibility).

### Client → server

| type | fields | meaning |
|---|---|---|
| `resize` | `cols`, `rows` (ints > 0), optional `redraw` (bool) | Set PTY size. With `redraw: true` the server guarantees a SIGWINCH even if the size is unchanged (see §5). |

### Server → client

| type | fields | meaning |
|---|---|---|
| `hello` | `version` (int), `id`, `name` | First message after upgrade. |
| `sync` | `state`: `"start"` \| `"end"` | Brackets the replay of past output (milestone 2). Binary frames between start and end are history; after end they are live. Client resets its terminal on `start`. |
| `exit` | `code` (int) | Child process exited. Server then closes the socket normally. |

Rules:
- Messages on one socket are ordered. Binary data that follows `sync:end` is live output;
  nothing is lost or duplicated between replay and live (the server snapshots the buffer
  and registers the subscriber under one lock).
- Server never sends a binary frame before `hello`.
- Milestone 1 sends `hello` and `exit`; `sync` is specified now but implemented in M2.

### Why this shape supports a future emulator

The client only knows "reset, receive bytes that reconstruct the screen, then live bytes".
Whether the server produces those bytes from a raw ring buffer or from serialising a
server-side terminal emulator's screen state (the way `tmux`/`zellij` do) is invisible to
the protocol. Swapping the implementation changes nothing on the wire.

## 3a. HTTP API

- `GET /api/terminals` → `[{id, name, dir, command?, exited, exitCode}]` in declaration order.
- `POST /api/terminals/{id}/restart` → 204 when an exited terminal was respawned, 409 if
  it is still running, 404 if unknown, 403 on a cross-origin `Origin`. Clients attached to
  the old session already received `exit`; they reconnect to get the new one.

## 4. Sessions

- Created by the daemon at startup (M1: one hard-coded `main` terminal; M3/M4: from the
  YAML layout). Spawned with `$SHELL` (fallback `/bin/bash`) as a login-ish interactive
  shell, `TERM=xterm-256color`, `COLORTERM=truecolor`, cwd per §6.
- Startup command (M4): typed into the shell as input after spawn, so quitting it leaves
  you in the shell rather than a dead pane.
- Exit: when the child exits, the session is marked dead and attached clients get `exit`.
  M1 leaves restart policy open (a dead session stays dead until the daemon restarts).
- Fan-out: each attached client has a bounded outbound queue. If a client can't keep up
  the client is dropped (it can reconnect and replay); the PTY reader is never blocked by
  a slow browser.

## 5. Reconnect behaviour (milestone 2)

1. Session keeps a ring buffer (default 1 MiB) of recent raw output.
2. On connect: server sends `hello`, `sync:start`, the buffer contents, `sync:end`.
3. Client calls `term.reset()` on `sync:start`, writes replay data, and on `sync:end`
   sends `resize` with `redraw: true`.
4. Server applies the size; if the size equals the current PTY size it first sets a
   one-column-smaller size and then the real one, because the kernel only raises SIGWINCH
   when the size actually changes. Full-screen TUIs redraw and overwrite any replay
   garbage.

### Multiple clients, one PTY size

A PTY has exactly one size. Policy: **the most recent `resize` wins** (tmux's
`window-size latest`). Simple and predictable: the device you're actively using gets the
right geometry; other attached views may wrap oddly until they resize. Alternatives
(smallest-wins) make the active device suffer for a forgotten tab.

## 6. Configuration and Coder integration

### Layout file

Looked up in order: `--config <path>` (must exist), `$XDG_CONFIG_HOME/stay/layout.yaml`
(usually `~/.config/stay/layout.yaml`), else a built-in layout (Claude / Dev + Test). A file
that exists but is invalid stops the daemon with a message naming the problem; unknown
fields are errors so typos don't silently do nothing. Changes need a daemon restart (which
ends the running shells, see R6). A full annotated example is `examples/layout.yaml`.

```yaml
terminals:                    # each id is used in URLs: no slashes/spaces
  - id: claude
    name: Claude              # tab/label text; defaults to id
    dir: .                    # optional, see below
    command: claude --continue  # optional, typed into the shell after start
tabs:
  - title: Claude
    root: claude              # a terminal id, or:
  - title: Work
    root:
      split: horizontal       # horizontal = side by side, vertical = stacked
      children: [dev, {split: vertical, children: [test, logs]}]
```

Rules: ids unique; every pane names a defined terminal; each terminal appears in exactly
one pane (two views of one PTY is allowed by the design but is nearly always a config
slip); splits have >= 2 children. Terminals defined but not placed produce a startup
warning. Pane sizes are not configured; the user drags dividers and the browser remembers.

### Working directory

Default: `$HOME/<repo>` where `<repo>` is the last path segment of `$CODER_GIT_REPO_URL`
with `.git` stripped (`https://github.com/bbushvt/stay.git` -> `stay`), falling back to
`$HOME` if unset or not yet cloned. A terminal's `dir` may be absolute, `~`/`~/x`,
contain `$VARS`, or be relative to the default directory above. It must exist (startup
fails otherwise). Gotcha: quote `"~"` alone, since a bare `~` is YAML null.

### Coder environment

- `CODER_GIT_REPO_URL` -> default directory (above).
- `CODER_WORKSPACE_NAME` -> browser tab title `STAY - <workspace>` (via `GET /api/layout`).
- Shells inherit the daemon's environment, so whatever the Coder agent exports
  (`CODER_*`, `GIT_*`, `SSH_AUTH_SOCK`, ...) is visible in every terminal. The daemon must
  therefore be started from the workspace agent's environment (the milestone 5 script).

Flags: `--listen` (default `127.0.0.1:7681`), `--config`.

### HTTP API addition

`GET /api/layout` -> `{workspace?, tabs: [{title, root}]}` where `root` is
`{kind: "pane", terminal}` or `{kind: "split", direction, children}`.

## 7. Security model

The listener has no authentication. Safety rests on (a) binding to loopback and (b) Coder
being the only way in from outside the workspace. Consequences handled in code:

- **Bind**: `--listen` must resolve to a loopback address; the daemon refuses otherwise.
- **Origin check**: websocket upgrades require `Origin` host to equal the `Host` header
  (same-origin), which is what `github.com/coder/websocket` does by default. This blocks a
  malicious web page the owner visits from opening a websocket to `localhost:7681` in
  their browser or via DNS rebinding.
- A terminal is a shell as the workspace user; anything that can reach the port can run
  code. That is the accepted trade-off for "no auth", hence the risks below.

## 8. Risks and decisions I would flag

**R1 — Unauthenticated shell on a loopback port (medium).** Any process in the workspace
(a compromised dependency, a dev server, an `npm` postinstall script) can connect to
`127.0.0.1:7681` and get a shell as the user. In practice such a process already *is* that
user, so the escalation is nil on a single-user workspace. It matters if the workspace
hosts other users or sandboxed processes (e.g. untrusted code in a container sharing the
host network). Browser-originated attacks are real and are covered by the Origin check
plus loopback bind. Mitigation option, if wanted later: a random per-start token in a
`0600` file, required via header/cookie, injected by the Coder script. Not built.

**R2 — Replaying a raw byte ring buffer is lossy (medium).** The buffer can start in the
middle of an escape sequence or UTF-8 character, and cannot reconstruct state set long ago
(alternate screen, scroll regions, modes, title). Mitigations: reset the terminal before
replay, drop leading bytes up to a safe boundary (the first newline or ESC), and force a
redraw via SIGWINCH. This works well for Claude Code and similar TUIs which repaint on
resize; it works poorly for TUIs that don't (some `less`/`vim` states). The long-term fix
is the server-side emulator already left open by the protocol. Accepted for M2.

**R3 — "Resize to trigger redraw" does nothing if the size is unchanged (low, known).**
Addressed by the shrink-then-restore nudge in §5.

**R4 — PTY has one size but many viewers (low).** Addressed by latest-resize-wins (§5).

**R5 — Coder app proxy and websockets (low, verify in M5).** Subdomain apps proxy
websockets fine, but idle connections may be cut by intermediaries. Client reconnects
automatically (exponential backoff) and relies on replay, so this is survivable. Use
relative URLs everywhere so the app also works behind path-based proxies. The server pings
every 30 s to keep connections warm.

**R6 — Daemon lifetime ≠ "never lose work" (medium).** Sessions live as long as the
daemon. A workspace stop, an OOM kill of the daemon, or an upgrade-restart kills every
shell. Browsers closing is safe; the workspace sleeping is not. Mitigations to consider:
make the Coder script restart the daemon (sessions are lost but the layout is recreated),
set the daemon's `oom_score_adj` low, and exclude it from autostop by keeping the app
"connected" semantics in mind. True survival across restart would require delegating to
tmux/dtach, which I recommend *against* — it brings back the clunkiness you left.

**R7 — Claude Code sessions are the cargo, not just shells (note).** If the daemon dies,
`claude --continue`/`--resume` recovers conversation state; consider documenting that in the
startup command for the Claude terminal.

**R8 — Clipboard addon needs a secure context and user gesture (low).** Coder subdomain
apps are HTTPS so OSC 52 write works; reading the clipboard prompts the browser. Fine.

No decision above is wrong enough to change the plan; R2 and R6 are the ones to watch.

## 9. Milestones

1. One terminal in the browser (Go server, websocket, xterm.js page). Done.
2. Persistence: ring buffer + replay + redraw nudge. Done.
3. Multiple terminals: tabs and splits via react-resizable-panels. Done (layout is
   derived in the frontend; terminals are defined in `main.go` until M4).
4. YAML layout + Coder environment integration. Done.
5. GoReleaser builds, Coder Terraform module.
