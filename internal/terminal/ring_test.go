package terminal

import "testing"

func TestRingNoWrap(t *testing.T) {
	r := newRing(16)
	r.write([]byte("abc"))
	r.write([]byte("def"))
	if got := string(r.snapshot()); got != "abcdef" {
		t.Fatalf("got %q", got)
	}
}

func TestRingWrapTrimsToLine(t *testing.T) {
	r := newRing(10)
	r.write([]byte("0123\nabc"))
	r.write([]byte("def\n")) // total 12 > 10: oldest 2 dropped -> "23\nabcdef\n"
	if got := string(r.snapshot()); got != "abcdef\n" {
		t.Fatalf("got %q", got)
	}
}

func TestRingOversizedWrite(t *testing.T) {
	r := newRing(8)
	r.write([]byte("xxxxxxxxxxxx\nyyy"))
	if got := string(r.snapshot()); got != "yyy" {
		t.Fatalf("got %q", got)
	}
}

func TestRingSkipsPartialUTF8(t *testing.T) {
	r := newRing(4)
	r.write([]byte("é€")) // c3 a9 e2 82 ac = 5 bytes; keeps a9 e2 82 ac
	if got := string(r.snapshot()); got != "€" {
		t.Fatalf("got %q", got)
	}
}
