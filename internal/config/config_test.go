package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func parse(t *testing.T, s string) (*Config, error) {
	t.Helper()
	return Parse(strings.NewReader(s))
}

func TestParseExample(t *testing.T) {
	f, err := os.Open("../../examples/layout.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	c, err := Parse(f)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Terminals) != 4 || len(c.Tabs) != 2 {
		t.Fatalf("got %d terminals, %d tabs", len(c.Terminals), len(c.Tabs))
	}
	if dirs := []string{c.Terminals[2].Dir, c.Terminals[3].Dir}; dirs[0] != "." || dirs[1] != "~" {
		t.Fatalf("example dirs not parsed as written (is ~ quoted?): %q", dirs)
	}
	root := c.Tabs[1].Root
	if root.Kind != "split" || root.Direction != "horizontal" || len(root.Children) != 2 {
		t.Fatalf("bad root: %+v", root)
	}
	if inner := root.Children[1]; inner.Kind != "split" || inner.Direction != "vertical" || inner.Children[1].Terminal != "logs" {
		t.Fatalf("bad nested split: %+v", inner)
	}
	if c.Tabs[0].Root.Kind != "pane" || c.Tabs[0].Root.Terminal != "claude" {
		t.Fatalf("bare string should be a pane: %+v", c.Tabs[0].Root)
	}
}

func TestDefaultIsValid(t *testing.T) {
	for _, claude := range []bool{true, false} {
		if err := Default(claude).Validate(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestValidationErrors(t *testing.T) {
	cases := map[string]struct{ yaml, want string }{
		"empty":        {"", "empty"},
		"no terminals": {"tabs: [{title: a, root: x}]", "no terminals"},
		"no tabs":      {"terminals: [{id: a}]", "no tabs"},
		"dup id":       {"terminals: [{id: a}, {id: a}]\ntabs: [{title: t, root: a}]", "duplicate"},
		"missing id":   {"terminals: [{name: a}]\ntabs: [{title: t, root: a}]", "id is required"},
		"bad id":       {"terminals: [{id: 'a b'}]\ntabs: [{title: t, root: 'a b'}]", "may not contain"},
		"unknown ref":  {"terminals: [{id: a}]\ntabs: [{title: t, root: b}]", `unknown terminal "b"`},
		"no title":     {"terminals: [{id: a}]\ntabs: [{root: a}]", "title is required"},
		"bad split":    {"terminals: [{id: a},{id: b}]\ntabs: [{title: t, root: {split: diagonal, children: [a, b]}}]", "horizontal"},
		"one child":    {"terminals: [{id: a}]\ntabs: [{title: t, root: {split: horizontal, children: [a]}}]", "at least 2"},
		"pane+split":   {"terminals: [{id: a}]\ntabs: [{title: t, root: {pane: a, split: horizontal}}]", "exactly one"},
		"used twice":   {"terminals: [{id: a}]\ntabs: [{title: t, root: a}, {title: u, root: a}]", "once"},
		"typo field":   {"terminals: [{id: a, comand: ls}]\ntabs: [{title: t, root: a}]", "comand"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := parse(t, tc.yaml)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %v, want containing %q", err, tc.want)
			}
		})
	}
}

func TestUnplaced(t *testing.T) {
	c, err := parse(t, "terminals: [{id: a}, {id: b}]\ntabs: [{title: t, root: a}]")
	if err != nil {
		t.Fatal(err)
	}
	if got := c.Unplaced(); len(got) != 1 || got[0] != "b" {
		t.Fatalf("unplaced = %v", got)
	}
}

func TestResolveDir(t *testing.T) {
	home := t.TempDir()
	base := filepath.Join(home, "repo")
	for _, d := range []string{base, filepath.Join(base, "sub"), filepath.Join(home, "other")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	env := map[string]string{"HOME": home, "PROJ": "other"}
	getenv := func(k string) string { return env[k] }

	ok := map[string]string{
		"":                           base,
		".":                          base,
		"sub":                        filepath.Join(base, "sub"),
		"~":                          home,
		"~/other":                    filepath.Join(home, "other"),
		"$HOME/other":                filepath.Join(home, "other"),
		"${HOME}/$PROJ":              filepath.Join(home, "other"),
		filepath.Join(home, "other"): filepath.Join(home, "other"),
	}
	for in, want := range ok {
		got, err := ResolveDir(in, base, getenv)
		if err != nil || filepath.Clean(got) != want {
			t.Errorf("ResolveDir(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	if _, err := ResolveDir("nope", base, getenv); err == nil {
		t.Error("missing dir should be an error")
	}
}

func TestDefsAndLayout(t *testing.T) {
	home := t.TempDir()
	if err := os.Mkdir(filepath.Join(home, "stay"), 0o755); err != nil {
		t.Fatal(err)
	}
	env := map[string]string{"HOME": home, "CODER_GIT_REPO_URL": "https://github.com/x/stay.git", "CODER_WORKSPACE_NAME": "ws1"}
	getenv := func(k string) string { return env[k] }

	c := Default(true)
	defs, err := c.Defs(getenv)
	if err != nil {
		t.Fatal(err)
	}
	if len(defs) != 3 || defs[0].ID != "claude" || defs[0].Command != "claude" || defs[0].Dir != filepath.Join(home, "stay") {
		t.Fatalf("defs: %+v", defs)
	}
	if l := c.Layout(getenv); l.Workspace != "ws1" || len(l.Tabs) != 2 {
		t.Fatalf("layout: %+v", l)
	}

	// Name falls back to id.
	c2, _ := parse(t, "terminals: [{id: solo}]\ntabs: [{title: t, root: solo}]")
	d2, _ := c2.Defs(getenv)
	if d2[0].Name != "solo" {
		t.Errorf("name fallback: %q", d2[0].Name)
	}
}

func TestLoadErrorIncludesPath(t *testing.T) {
	p := filepath.Join(t.TempDir(), "bad.yaml")
	os.WriteFile(p, []byte("terminals: []\n"), 0o644)
	if _, err := Load(p); err == nil || !strings.Contains(err.Error(), "bad.yaml") {
		t.Fatalf("err = %v", err)
	}
}

func TestAllExamplesValid(t *testing.T) {
	files, _ := filepath.Glob("../../examples/*.yaml")
	if len(files) < 2 {
		t.Fatalf("expected example files, found %v", files)
	}
	for _, f := range files {
		if _, err := Load(f); err != nil {
			t.Errorf("%s: %v", f, err)
		}
	}
}
