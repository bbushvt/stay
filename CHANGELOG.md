# Changelog

Each release's notes are taken from the matching `## vX.Y.Z` section below and published as
the GitHub release body (see `.github/workflows/release.yml`).

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
