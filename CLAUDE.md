# STAY — Shells That Await You

Persistent web terminal for a Coder workspace. One Go daemon owns the PTYs; browsers are
disposable views. Closing a browser must never kill a process. Full design, protocol and
risk analysis: `docs/SPEC.md` — read it before changing behaviour.

## Stack
- Backend: Go 1.27, `github.com/creack/pty`, `github.com/coder/websocket`, `go.yaml.in/yaml/v3`,
  stdlib `net/http`.
  Keep dependencies minimal; ask before adding one.
- Frontend (`web/`): TypeScript, React, Vite, **npm** (not pnpm), xterm.js (`@xterm/xterm`)
  with fit/webgl/clipboard/web-links addons. `react-resizable-panels` arrives in milestone 3.
- The built frontend (`web/dist`) is embedded via `go:embed` (`web/embed.go`): one binary.
- Toolchain pinned in `mise.toml` (Go, Node).

## Architecture
- `main.go` — flags, wiring, signal handling. Refuses non-loopback `--listen`.
- `internal/terminal` — `Session` = PTY + child, independent of websockets; ring-buffer
  history; fan-out to `Subscriber`s with bounded queues (slow clients are dropped, PTY
  reader never blocks). `Manager` owns the set of terminals and restarts exited ones.
- `internal/config` — YAML layout parse/validate, dir resolution, `Layout` JSON for the
  frontend. Example: `examples/layout.yaml`. YAML gotcha: bare `~` is null; quote it.
- `internal/server` — HTTP routes, websocket handler, protocol types (`protocol.go`).
- `web/src` — `Terminal.tsx` (xterm + websocket + reconnect), `Panes.tsx` (split tree via
  react-resizable-panels), `App.tsx` (tabs), `layout.ts` (layout types; server-provided via
  `GET /api/layout`), `api.ts`, `protocol.ts` (mirror of `protocol.go`).
- HTTP API: `GET /api/layout`, `GET /api/terminals`, `POST /api/terminals/{id}/restart` (exited only; same-origin).
- Protocol: one websocket per terminal at `/ws/terminals/{id}`. Binary frames = terminal
  bytes; JSON text frames = control messages. Keep `protocol.go` and `protocol.ts` in sync.
- Binds 127.0.0.1 only, **no auth** — Coder (`coder_app`, subdomain, share=owner) fronts it.
  The websocket Origin check (same-origin) must stay on.

## Commands
```
make build        # npm install + vite build + go build -> ./stay
make run          # build and run on 127.0.0.1:7681
make test         # go test -race ./...
make check        # tests + tsc typecheck + gofmt + go vet
cd web && npm run dev    # vite dev server, proxies /ws to 127.0.0.1:7681
./stay --listen 127.0.0.1:7681 [--config examples/layout.yaml]
```
`go test` works without building the frontend (`web/dist/.gitkeep` keeps `go:embed` valid).

## Conventions
- Standard Go style (`gofmt`), small packages under `internal/`, table tests, `-race`.
- Tests must not need a browser; drive the websocket with `coder/websocket` against
  `httptest`. Shells in tests use `/bin/sh` with a temp dir.
- Frontend URLs are relative (no leading `/`) so the app works behind any proxy prefix.
- Build only the current milestone; keep structure ready, don't build ahead.
- Don't commit `web/dist` contents or the `stay` binary (gitignored).

## Release and deployment
- `.goreleaser.yaml` + `.github/workflows/{ci,release}.yml`; tag `vX.Y.Z` to release.
  Dry run: `goreleaser release --snapshot --clean`. Archive names are a contract with
  `terraform/run.sh`.
- `terraform/` is the Coder module (`main.tf`, `run.sh`, README). Settings are exported as
  `STAY_*` env vars ahead of `run.sh`, so keep `run.sh` free of Terraform templating and
  keep variable validations strict (inputs reach a shell).
- Test `run.sh` without network via `STAY_DOWNLOAD_BASE=file://...` and a throwaway `HOME`.

## Working inside a STAY/Coder workspace
You may be running inside one of the user's terminals. Never kill processes you didn't
start. Only bind the STAY port (7681) when testing; no other ports.

## Milestones
1. One terminal in the browser — **done**
2. Persistence: ring buffer, replay (`sync` messages), SIGWINCH redraw nudge — **done**
3. Multiple terminals: tabs and splits — **done**
4. YAML layout config + Coder env integration — **done**
5. GoReleaser + Coder Terraform module — **done** (untested inside a real Coder deployment)
