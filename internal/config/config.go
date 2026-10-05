// Package config loads the YAML layout file: which terminals exist and how
// they are arranged in tabs and splits. See docs/SPEC.md §6.
package config

import (
	"bytes"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/bbushvt/stay/internal/terminal"
)

// Terminal declares one shell.
type Terminal struct {
	ID      string `yaml:"id"`
	Name    string `yaml:"name"`
	Dir     string `yaml:"dir"`
	Command string `yaml:"command"`
}

// Node is a layout tree node: a pane showing one terminal, or a split of
// children. The JSON form is what the frontend consumes (web/src/layout.ts).
type Node struct {
	Kind      string `json:"kind"` // "pane" | "split"
	Terminal  string `json:"terminal,omitempty"`
	Direction string `json:"direction,omitempty"` // "horizontal" | "vertical"
	Children  []Node `json:"children,omitempty"`
	// Size is this node's share of its parent split, in percent (0 < size < 100).
	// Zero means unset: unsized siblings share what the sized ones leave over.
	Size float64 `json:"size,omitempty"`
}

// UnmarshalYAML accepts a bare string (a pane), {pane: id}, or
// {split: horizontal|vertical, children: [...]}. A mapping may also carry
// `size: <percent>` to size it within its parent split.
func (n *Node) UnmarshalYAML(v *yaml.Node) error {
	switch v.Kind {
	case yaml.ScalarNode:
		*n = Node{Kind: "pane", Terminal: v.Value}
		return nil
	case yaml.MappingNode:
		var m struct {
			Pane     string   `yaml:"pane"`
			Split    string   `yaml:"split"`
			Children []Node   `yaml:"children"`
			Size     *float64 `yaml:"size"`
		}
		if err := v.Decode(&m); err != nil {
			return err
		}
		switch {
		case m.Pane != "" && m.Split == "":
			*n = Node{Kind: "pane", Terminal: m.Pane}
		case m.Split != "" && m.Pane == "":
			*n = Node{Kind: "split", Direction: m.Split, Children: m.Children}
		default:
			return fmt.Errorf("line %d: a layout node needs exactly one of `pane` or `split`", v.Line)
		}
		if m.Size != nil {
			n.Size = *m.Size
			if n.Size == 0 {
				n.Size = -1 // explicit `size: 0`: keep it distinct from unset so validation rejects it
			}
		}
		return nil
	}
	return fmt.Errorf("line %d: a layout node must be a terminal id or a mapping", v.Line)
}

// Tab is a titled layout tree.
type Tab struct {
	Title string `yaml:"title" json:"title"`
	Root  Node   `yaml:"root" json:"root"`
}

// Layout is what the frontend renders.
type Layout struct {
	Workspace string `json:"workspace,omitempty"` // CODER_WORKSPACE_NAME, for the page title
	Tabs      []Tab  `json:"tabs"`
}

// Config is the parsed file.
type Config struct {
	Terminals []Terminal `yaml:"terminals"`
	Tabs      []Tab      `yaml:"tabs"`
}

// Default is used when no config file exists.
func Default(hasClaude bool) *Config {
	claude := Terminal{ID: "claude", Name: "Claude"}
	if hasClaude {
		claude.Command = "claude"
	}
	pane := func(id string) Node { return Node{Kind: "pane", Terminal: id} }
	return &Config{
		Terminals: []Terminal{claude, {ID: "dev", Name: "Dev"}, {ID: "test", Name: "Test"}},
		Tabs: []Tab{
			{Title: "Claude", Root: pane("claude")},
			{Title: "Shells", Root: Node{Kind: "split", Direction: "horizontal", Children: []Node{pane("dev"), pane("test")}}},
		},
	}
}

// Parse reads and validates a YAML document. Unknown fields are errors so
// typos don't silently do nothing.
func Parse(r io.Reader) (*Config, error) {
	dec := yaml.NewDecoder(r)
	dec.KnownFields(true)
	var c Config
	if err := dec.Decode(&c); err != nil {
		if err == io.EOF {
			return nil, fmt.Errorf("config is empty")
		}
		return nil, err
	}
	return &c, c.Validate()
}

// Load parses the file at path.
func Load(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	c, err := Parse(bytes.NewReader(b))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return c, nil
}

// DefaultPath is where Load looks when --config isn't given:
// $XDG_CONFIG_HOME/stay/layout.yaml (usually ~/.config/stay/layout.yaml).
func DefaultPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "stay", "layout.yaml")
}

// Validate checks ids, references and split shapes.
func (c *Config) Validate() error {
	if len(c.Terminals) == 0 {
		return fmt.Errorf("no terminals defined")
	}
	if len(c.Tabs) == 0 {
		return fmt.Errorf("no tabs defined")
	}
	ids := map[string]bool{}
	for i, t := range c.Terminals {
		if t.ID == "" {
			return fmt.Errorf("terminals[%d]: id is required", i)
		}
		if strings.ContainsAny(t.ID, "/ \t?#%") {
			return fmt.Errorf("terminal %q: id may not contain slashes, spaces or URL punctuation", t.ID)
		}
		if ids[t.ID] {
			return fmt.Errorf("duplicate terminal id %q", t.ID)
		}
		ids[t.ID] = true
	}
	used := map[string]int{}
	for i, tab := range c.Tabs {
		if tab.Title == "" {
			return fmt.Errorf("tabs[%d]: title is required", i)
		}
		if tab.Root.Size != 0 {
			return fmt.Errorf("tabs[%d] (%s): size only applies to a child of a split", i, tab.Title)
		}
		if err := validateNode(tab.Root, ids, used, fmt.Sprintf("tabs[%d] (%s)", i, tab.Title)); err != nil {
			return err
		}
	}
	for id, n := range used {
		if n > 1 {
			// Two panes on one PTY are allowed by the design, but in a config
			// file it is almost always a copy/paste slip.
			return fmt.Errorf("terminal %q is placed in %d panes; each terminal may appear once", id, n)
		}
	}
	return nil
}

func validateNode(n Node, ids map[string]bool, used map[string]int, where string) error {
	if n.Size != 0 && (n.Size <= 0 || n.Size >= 100) {
		return fmt.Errorf("%s: size must be a percentage greater than 0 and less than 100", where)
	}
	switch n.Kind {
	case "pane":
		if !ids[n.Terminal] {
			return fmt.Errorf("%s: unknown terminal %q", where, n.Terminal)
		}
		used[n.Terminal]++
	case "split":
		if n.Direction != "horizontal" && n.Direction != "vertical" {
			return fmt.Errorf("%s: split must be `horizontal` or `vertical`, got %q", where, n.Direction)
		}
		if len(n.Children) < 2 {
			return fmt.Errorf("%s: a split needs at least 2 children", where)
		}
		var total float64
		sized := 0
		for _, ch := range n.Children {
			if err := validateNode(ch, ids, used, where); err != nil {
				return err
			}
			if ch.Size != 0 {
				total += ch.Size
				sized++
			}
		}
		switch {
		case sized == len(n.Children) && math.Abs(total-100) > 0.01:
			return fmt.Errorf("%s: sizes in a split must add up to 100 when every child has one, got %g", where, total)
		case sized < len(n.Children) && total >= 100:
			return fmt.Errorf("%s: sizes in a split leave no room for the children without one (they add up to %g)", where, total)
		}
	default:
		return fmt.Errorf("%s: invalid layout node", where)
	}
	return nil
}

// Unplaced returns ids of terminals not shown in any tab (they would run
// invisibly), so the caller can warn.
func (c *Config) Unplaced() []string {
	used := map[string]bool{}
	var walk func(Node)
	walk = func(n Node) {
		if n.Kind == "pane" {
			used[n.Terminal] = true
		}
		for _, ch := range n.Children {
			walk(ch)
		}
	}
	for _, t := range c.Tabs {
		walk(t.Root)
	}
	var out []string
	for _, t := range c.Terminals {
		if !used[t.ID] {
			out = append(out, t.ID)
		}
	}
	return out
}

// Defs converts to terminal definitions, resolving each working directory
// against getenv's HOME and the repo-derived default directory.
func (c *Config) Defs(getenv func(string) string) ([]terminal.Def, error) {
	base := terminal.DefaultDir(getenv)
	out := make([]terminal.Def, 0, len(c.Terminals))
	for _, t := range c.Terminals {
		dir, err := ResolveDir(t.Dir, base, getenv)
		if err != nil {
			return nil, fmt.Errorf("terminal %q: %w", t.ID, err)
		}
		name := t.Name
		if name == "" {
			name = t.ID
		}
		out = append(out, terminal.Def{ID: t.ID, Config: terminal.Config{Name: name, Dir: dir, Command: t.Command}})
	}
	return out, nil
}

// Layout returns the frontend's view of the tabs.
func (c *Config) Layout(getenv func(string) string) Layout {
	return Layout{Workspace: getenv("CODER_WORKSPACE_NAME"), Tabs: c.Tabs}
}

// ResolveDir expands $VARS and ~, resolves relative paths against base (the
// repo directory), and requires the result to exist. Empty means base.
func ResolveDir(dir, base string, getenv func(string) string) (string, error) {
	if dir == "" {
		return base, nil
	}
	dir = os.Expand(dir, getenv)
	home := getenv("HOME")
	switch {
	case dir == "~":
		dir = home
	case strings.HasPrefix(dir, "~/"):
		dir = filepath.Join(home, dir[2:])
	case !filepath.IsAbs(dir):
		dir = filepath.Join(base, dir)
	}
	fi, err := os.Stat(dir)
	if err != nil || !fi.IsDir() {
		return "", fmt.Errorf("working directory %q does not exist", dir)
	}
	return dir, nil
}
