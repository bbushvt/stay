// Package terminal owns PTY-backed shell sessions. A Session lives independently
// of any websocket: clients subscribe and unsubscribe, the shell keeps running.
package terminal

import (
	"errors"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"
)

// subscriberQueue (a var so tests can shrink it) is how many output chunks a client may fall behind before it
// is dropped. The PTY reader never blocks on a slow client.
var subscriberQueue = 256

var osGetenv = os.Getenv

var redrawNudgeDelay = 50 * time.Millisecond

// HistoryBytes is the per-session replay buffer size.
const HistoryBytes = 1 << 20

// Config describes how to start a session.
type Config struct {
	Name  string
	Shell string   // defaults to $SHELL, then /bin/bash
	Args  []string // defaults to ["-l"] when Shell is defaulted or has no args
	Dir   string   // defaults to DefaultDir()
	// Command, if set, is typed into the shell after it starts, so quitting it
	// leaves a usable prompt rather than a dead terminal.
	Command string
	Env     []string // extra KEY=VALUE entries appended to the environment
	Cols    uint16
	Rows    uint16
}

// Session is a running shell attached to a PTY.
type Session struct {
	Name string

	ptmx *os.File
	cmd  *exec.Cmd

	mu     sync.Mutex
	subs   map[*Subscriber]struct{}
	closed bool
	hist   *ring
	cols   uint16
	rows   uint16

	done     chan struct{}
	exitCode int
}

// Subscriber receives live PTY output. C is closed when the session exits or
// the subscriber is dropped for being too slow; check Session.Done to tell which.
type Subscriber struct {
	C <-chan []byte

	ch   chan []byte
	sess *Session
}

// Start spawns the shell and begins reading its output.
func Start(cfg Config) (*Session, error) {
	shell := cfg.Shell
	args := cfg.Args
	if shell == "" {
		shell = os.Getenv("SHELL")
		if shell == "" {
			shell = "/bin/bash"
		}
		if args == nil {
			args = []string{"-l"}
		}
	}
	dir := cfg.Dir
	if dir == "" {
		dir = DefaultDir(osGetenv)
	}
	cols, rows := cfg.Cols, cfg.Rows
	if cols == 0 {
		cols = 80
	}
	if rows == 0 {
		rows = 24
	}

	cmd := exec.Command(shell, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "TERM=xterm-256color", "COLORTERM=truecolor")
	cmd.Env = append(cmd.Env, cfg.Env...)

	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: cols, Rows: rows})
	if err != nil {
		return nil, err
	}

	s := &Session{
		Name: cfg.Name,
		ptmx: ptmx,
		cmd:  cmd,
		subs: make(map[*Subscriber]struct{}),
		hist: newRing(HistoryBytes),
		cols: cols,
		rows: rows,
		done: make(chan struct{}),
	}
	go s.readLoop()
	if cfg.Command != "" {
		// The tty buffers this until the shell reads its first line.
		_, _ = ptmx.Write([]byte(cfg.Command + "\n"))
	}
	return s, nil
}

func (s *Session) readLoop() {
	buf := make([]byte, 32*1024)
	for {
		n, err := s.ptmx.Read(buf)
		if n > 0 {
			s.broadcast(append([]byte(nil), buf[:n]...))
		}
		if err != nil {
			break // EIO once the child side closes, or closed by Close()
		}
	}

	err := s.cmd.Wait()
	code := 0
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		code = ee.ExitCode() // -1 when killed by a signal
	}

	s.mu.Lock()
	s.exitCode = code
	s.closed = true
	// done must close before subscriber channels so a subscriber seeing its
	// channel close can reliably tell "session exited" from "dropped".
	close(s.done)
	for sub := range s.subs {
		close(sub.ch)
		delete(s.subs, sub)
	}
	s.mu.Unlock()
	s.ptmx.Close()
}

func (s *Session) broadcast(p []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.hist.write(p)
	for sub := range s.subs {
		select {
		case sub.ch <- p:
		default: // too slow: drop it, it can reconnect
			close(sub.ch)
			delete(s.subs, sub)
		}
	}
}

// Subscribe attaches a new output listener without history. If the session has
// already exited the returned channel is closed immediately.
func (s *Session) Subscribe() *Subscriber {
	_, sub := s.Attach()
	return sub
}

// Attach atomically snapshots recent output and subscribes to what follows, so
// the replay and live stream have no gap or overlap.
func (s *Session) Attach() ([]byte, *Subscriber) {
	ch := make(chan []byte, subscriberQueue)
	sub := &Subscriber{C: ch, ch: ch, sess: s}
	s.mu.Lock()
	defer s.mu.Unlock()
	snap := s.hist.snapshot()
	if s.closed {
		close(ch)
		return snap, sub
	}
	s.subs[sub] = struct{}{}
	return snap, sub
}

// Close detaches the subscriber. Safe to call more than once and after the
// session has dropped it.
func (sub *Subscriber) Close() {
	s := sub.sess
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.subs[sub]; ok {
		delete(s.subs, sub)
		close(sub.ch)
	}
}

// Write sends input to the shell.
func (s *Session) Write(p []byte) (int, error) { return s.ptmx.Write(p) }

// Resize sets the PTY size; the kernel signals SIGWINCH if it changed.
func (s *Session) Resize(cols, rows uint16) error {
	s.mu.Lock()
	s.cols, s.rows = cols, rows
	s.mu.Unlock()
	return pty.Setsize(s.ptmx, &pty.Winsize{Cols: cols, Rows: rows})
}

// Redraw sets the size like Resize but guarantees the foreground program gets
// a SIGWINCH: the kernel only signals on an actual change, so if the size is
// unchanged it is briefly shrunk by one column first.
func (s *Session) Redraw(cols, rows uint16) error {
	s.mu.Lock()
	same := s.cols == cols && s.rows == rows
	s.mu.Unlock()
	if same && cols > 1 {
		if err := pty.Setsize(s.ptmx, &pty.Winsize{Cols: cols - 1, Rows: rows}); err != nil {
			return err
		}
		time.Sleep(redrawNudgeDelay) // let the signal be delivered before restoring
	}
	return s.Resize(cols, rows)
}

// Done is closed once the shell has exited and its output has been read.
func (s *Session) Done() <-chan struct{} { return s.done }

// ExitCode is valid after Done is closed. -1 means killed by a signal.
func (s *Session) ExitCode() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.exitCode
}

// Close hangs up the shell's process group (only processes STAY started).
func (s *Session) Close() {
	if s.cmd.Process != nil {
		_ = syscall.Kill(-s.cmd.Process.Pid, syscall.SIGHUP)
	}
}
