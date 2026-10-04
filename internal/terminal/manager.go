package terminal

import (
	"fmt"
	"sync"
)

// Def declares a terminal. Config carries the spawn settings; ID is the stable
// key used in URLs.
type Def struct {
	ID string
	Config
}

// Info is a snapshot of a terminal's state for the API.
type Info struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Dir      string `json:"dir"`
	Command  string `json:"command,omitempty"`
	Exited   bool   `json:"exited"`
	ExitCode int    `json:"exitCode"`
}

// Manager owns the set of terminals, in declaration order.
type Manager struct {
	mu    sync.Mutex
	order []string
	defs  map[string]Def
	sess  map[string]*Session
}

// NewManager starts every terminal. On error, already-started ones are closed.
func NewManager(defs []Def) (*Manager, error) {
	m := &Manager{defs: map[string]Def{}, sess: map[string]*Session{}}
	for _, d := range defs {
		if d.ID == "" {
			m.Close()
			return nil, fmt.Errorf("terminal with empty id")
		}
		if _, dup := m.defs[d.ID]; dup {
			m.Close()
			return nil, fmt.Errorf("duplicate terminal id %q", d.ID)
		}
		d.Dir = resolveDir(d.Dir)
		s, err := Start(d.Config)
		if err != nil {
			m.Close()
			return nil, fmt.Errorf("start terminal %q: %w", d.ID, err)
		}
		m.order = append(m.order, d.ID)
		m.defs[d.ID] = d
		m.sess[d.ID] = s
	}
	return m, nil
}

// Get returns the live session for id.
func (m *Manager) Get(id string) (*Session, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sess[id]
	return s, ok
}

// List describes all terminals in declaration order.
func (m *Manager) List() []Info {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Info, 0, len(m.order))
	for _, id := range m.order {
		d, s := m.defs[id], m.sess[id]
		info := Info{ID: id, Name: d.Name, Dir: d.Dir, Command: d.Command}
		select {
		case <-s.Done():
			info.Exited, info.ExitCode = true, s.ExitCode()
		default:
		}
		out = append(out, info)
	}
	return out
}

// ErrRunning is returned by Restart when the shell is still alive.
var ErrRunning = fmt.Errorf("terminal is still running")

// Restart respawns an exited terminal with its original definition.
func (m *Manager) Restart(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	d, ok := m.defs[id]
	if !ok {
		return fmt.Errorf("unknown terminal %q", id)
	}
	select {
	case <-m.sess[id].Done():
	default:
		return ErrRunning
	}
	s, err := Start(d.Config)
	if err != nil {
		return err
	}
	m.sess[id] = s
	return nil
}

// Close hangs up every shell.
func (m *Manager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, s := range m.sess {
		s.Close()
	}
}

func resolveDir(dir string) string {
	if dir == "" {
		return DefaultDir(osGetenv)
	}
	return dir
}
