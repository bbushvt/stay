# Changelog

Each release's notes are taken from the matching `## vX.Y.Z` section below and published as
the GitHub release body (see `.github/workflows/release.yml`).

## v0.3.0

### Added
- **Copy and paste shortcuts in the terminal.** On Linux and Windows, Ctrl-C copies when text
  is selected (with no selection it still sends an interrupt to the program) and Ctrl-V
  pastes. Ctrl-Shift-C and Ctrl-Shift-V always copy and paste. On macOS, Cmd-C and Cmd-V
  copy and paste, and Ctrl-C always interrupts. Paste uses the browser's native paste event,
  so bracketed paste works and no clipboard permission prompt appears.

### Changed
- Ctrl-V now pastes on Linux and Windows instead of sending the literal-next character
  (`^V`) to the shell. Use Ctrl-Shift-V or your shell's own binding if you relied on it.

### Development
- Added `vitest` (dev dependency) with unit tests for the key handling; `make check` runs it.

### Documentation
- README: the new shortcuts are described under "Using STAY".

### Upgrading
This is a frontend change inside the daemon binary. A running daemon is never restarted by
the module, so the new version takes effect the next time the daemon starts (for example
after a workspace restart). Restarting the daemon ends running shells. To use this release,
set `ref=v0.3.0` in your template's `module "stay"` source (`stay_version` defaults to
`latest`).

## v0.2.0

### Added
- **Layout `size` option.** Any child of a split can set `size: <percent>` (above 0, below
  100) to choose its share of the split, e.g. `{pane: dev, size: 70}`. If every child is
  sized the sizes must add up to 100; otherwise unsized children share the remainder equally.
  Sizes are starting values: you can still drag dividers, and the browser remembers your
  dragged sizes. Changing a `size` in the layout file resets that split to the new values.
- Layout validation errors for `size` (out of range, sizes not adding up, no room for
  unsized children, `size` on a tab root).

### Documentation
- README: a full "Configure terminals and layout" reference with field tables, sizing rules,
  six copy-ready examples and a table of validation errors.
- README and `terraform/README.md`: the `order` and `group` module inputs are explained
  separately (button sort position, and grouping under a named section).
- `docs/SPEC.md` and `examples/layout.yaml` describe `size`.

### Upgrading
Existing layouts keep working unchanged. A running daemon is never restarted by the module,
so the new binary and any layout change take effect the next time the daemon starts (for
example after a workspace restart). Restarting the daemon ends running shells.
To use the new module docs and binary, set `ref=v0.2.0` in your template's `module "stay"`
source (`stay_version` defaults to `latest`).

## v0.1.0

Initial release: persistent browser terminals backed by a Go daemon, tabs and splits, YAML
layout, Coder Terraform module, GoReleaser releases.
