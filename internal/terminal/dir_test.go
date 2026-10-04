package terminal

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRepoName(t *testing.T) {
	cases := map[string]string{
		"https://github.com/bbushvt/stay.git":  "stay",
		"https://github.com/bbushvt/stay":      "stay",
		"https://github.com/bbushvt/stay.git/": "stay",
		"git@github.com:bbushvt/stay.git":      "stay",
		"ssh://git@host:2222/org/proj.git":     "proj",
		"":                                     "",
		"   ":                                  "",
	}
	for in, want := range cases {
		if got := RepoName(in); got != want {
			t.Errorf("RepoName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDefaultDir(t *testing.T) {
	home := t.TempDir()
	if err := os.Mkdir(filepath.Join(home, "stay"), 0o755); err != nil {
		t.Fatal(err)
	}
	env := func(m map[string]string) func(string) string {
		return func(k string) string { return m[k] }
	}

	got := DefaultDir(env(map[string]string{"HOME": home, "CODER_GIT_REPO_URL": "https://github.com/x/stay.git"}))
	if want := filepath.Join(home, "stay"); got != want {
		t.Errorf("repo dir: got %q want %q", got, want)
	}
	// Repo not cloned yet -> HOME.
	if got := DefaultDir(env(map[string]string{"HOME": home, "CODER_GIT_REPO_URL": "https://github.com/x/missing.git"})); got != home {
		t.Errorf("missing repo: got %q want %q", got, home)
	}
	// No URL -> HOME.
	if got := DefaultDir(env(map[string]string{"HOME": home})); got != home {
		t.Errorf("no url: got %q want %q", got, home)
	}
}
