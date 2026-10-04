package terminal

import (
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// DefaultDir returns $HOME/<repo> where repo is derived from CODER_GIT_REPO_URL,
// falling back to $HOME (and "/" if even that is unset).
func DefaultDir(getenv func(string) string) string {
	home := getenv("HOME")
	if home == "" {
		home = "/"
	}
	if repo := RepoName(getenv("CODER_GIT_REPO_URL")); repo != "" {
		dir := filepath.Join(home, repo)
		if fi, err := os.Stat(dir); err == nil && fi.IsDir() {
			return dir
		}
	}
	return home
}

// RepoName extracts the repository name from a git URL, handling both
// https://host/org/repo.git and scp-style git@host:org/repo.git forms.
func RepoName(repoURL string) string {
	s := strings.TrimSpace(repoURL)
	if s == "" {
		return ""
	}
	if u, err := url.Parse(s); err == nil && u.Path != "" {
		s = u.Path
	} else if i := strings.LastIndex(s, ":"); i >= 0 {
		s = s[i+1:]
	}
	s = strings.TrimRight(s, "/")
	s = strings.TrimSuffix(s, ".git")
	name := path.Base(s)
	if name == "." || name == "/" {
		return ""
	}
	return name
}
