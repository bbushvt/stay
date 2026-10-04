package terminal

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func startSh(t *testing.T) *Session {
	t.Helper()
	s, err := Start(Config{Name: "t", Shell: "/bin/sh", Dir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	return s
}

// waitFor reads from sub until the accumulated output contains want.
func waitFor(t *testing.T, sub *Subscriber, want string) string {
	t.Helper()
	var got bytes.Buffer
	timeout := time.After(5 * time.Second)
	for {
		select {
		case b, ok := <-sub.C:
			if !ok {
				t.Fatalf("channel closed before %q; got %q", want, got.String())
			}
			got.Write(b)
			if strings.Contains(got.String(), want) {
				return got.String()
			}
		case <-timeout:
			t.Fatalf("timeout waiting for %q; got %q", want, got.String())
		}
	}
}

func TestSessionEcho(t *testing.T) {
	s := startSh(t)
	sub := s.Subscribe()
	defer sub.Close()
	s.Write([]byte("echo $((6*7))marker\n"))
	waitFor(t, sub, "42marker")
}

func TestSessionSurvivesDetach(t *testing.T) {
	s := startSh(t)
	sub := s.Subscribe()
	sub.Close()
	sub.Close() // idempotent

	select {
	case <-s.Done():
		t.Fatal("session died when its only subscriber left")
	case <-time.After(200 * time.Millisecond):
	}

	sub2 := s.Subscribe()
	defer sub2.Close()
	s.Write([]byte("echo back-again\n"))
	waitFor(t, sub2, "back-again")
}

func TestSessionMultipleSubscribers(t *testing.T) {
	s := startSh(t)
	a, b := s.Subscribe(), s.Subscribe()
	defer a.Close()
	defer b.Close()
	s.Write([]byte("echo fanout\n"))
	waitFor(t, a, "fanout")
	waitFor(t, b, "fanout")
}

func TestSessionResize(t *testing.T) {
	s := startSh(t)
	sub := s.Subscribe()
	defer sub.Close()
	if err := s.Resize(132, 43); err != nil {
		t.Fatal(err)
	}
	s.Write([]byte("stty size\n"))
	waitFor(t, sub, "43 132")
}

func TestSessionExit(t *testing.T) {
	s := startSh(t)
	sub := s.Subscribe()
	s.Write([]byte("exit 7\n"))
	select {
	case <-s.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("session did not finish")
	}
	if got := s.ExitCode(); got != 7 {
		t.Errorf("exit code = %d, want 7", got)
	}
	// Subscriber channel must be closed after drain.
	for range sub.C {
	}
	// Late subscribers get a closed channel.
	if _, ok := <-s.Subscribe().C; ok {
		t.Error("subscribe after exit should yield closed channel")
	}
}

func TestSlowSubscriberDroppedWithoutBlocking(t *testing.T) {
	s := startSh(t)
	old := subscriberQueue
	subscriberQueue = 2
	defer func() { subscriberQueue = old }()
	slow := s.Subscribe() // never read
	fast := s.Subscribe()
	defer fast.Close()

	// Produce far more output chunks than the queue holds.
	s.Write([]byte("i=0; while [ $i -lt 3000 ]; do echo line$i; i=$((i+1)); done; echo finished\n"))

	done := make(chan struct{})
	go func() { waitFor(t, fast, "finished"); close(done) }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("fast subscriber starved by slow one")
	}

	// slow was dropped: draining its queue ends in a closed channel, session alive.
	drained := make(chan struct{})
	go func() {
		for range slow.C {
		}
		close(drained)
	}()
	select {
	case <-drained:
	case <-time.After(5 * time.Second):
		t.Fatal("slow subscriber was never dropped")
	}
	select {
	case <-s.Done():
		t.Fatal("session should survive a dropped subscriber")
	default:
	}
}

func TestAttachReplaysHistoryThenLive(t *testing.T) {
	s := startSh(t)
	first := s.Subscribe()
	s.Write([]byte("echo before-detach\n"))
	waitFor(t, first, "before-detach")
	first.Close()

	snap, sub := s.Attach()
	defer sub.Close()
	if !strings.Contains(string(snap), "before-detach") {
		t.Fatalf("snapshot missing history: %q", snap)
	}
	s.Write([]byte("echo after-attach\n"))
	waitFor(t, sub, "after-attach")
}

func TestRedrawSendsSIGWINCHEvenWhenSizeUnchanged(t *testing.T) {
	s := startSh(t)
	sub := s.Subscribe()
	defer sub.Close()
	s.Write([]byte("trap 'echo got-winch' 28; echo armed\n"))
	waitFor(t, sub, "armed\r\n")
	if err := s.Redraw(80, 24); err != nil { // same as initial size
		t.Fatal(err)
	}
	// dash runs traps only once its blocking read returns; wake it.
	s.Write([]byte("\n"))
	waitFor(t, sub, "got-winch")
}

func TestStartupCommandThenPrompt(t *testing.T) {
	s, err := Start(Config{Name: "t", Shell: "/bin/sh", Dir: t.TempDir(), Command: "echo started-$((2+3))"})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	sub := s.Subscribe()
	defer sub.Close()
	waitFor(t, sub, "started-5")
	// Shell is still usable afterwards.
	s.Write([]byte("echo still-here\n"))
	waitFor(t, sub, "still-here")
}
