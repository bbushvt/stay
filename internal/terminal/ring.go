package terminal

import "bytes"

// ring keeps the most recent N bytes of output.
type ring struct {
	buf     []byte
	start   int // index of oldest byte
	n       int // bytes stored
	wrapped bool
}

func newRing(size int) *ring { return &ring{buf: make([]byte, size)} }

func (r *ring) write(p []byte) {
	if len(p) >= len(r.buf) {
		copy(r.buf, p[len(p)-len(r.buf):])
		r.start, r.n, r.wrapped = 0, len(r.buf), true
		return
	}
	for len(p) > 0 {
		end := (r.start + r.n) % len(r.buf)
		c := copy(r.buf[end:], p)
		if r.n+c > len(r.buf) {
			over := r.n + c - len(r.buf)
			r.start = (r.start + over) % len(r.buf)
			r.n = len(r.buf)
			r.wrapped = true
		} else {
			r.n += c
		}
		p = p[c:]
	}
}

// snapshot returns a copy of the buffered bytes. If older output has been
// discarded, the start is trimmed to a safe boundary (just after the first
// newline, else past any partial UTF-8 sequence) so the replay doesn't begin
// mid-line or mid-character.
func (r *ring) snapshot() []byte {
	out := make([]byte, r.n)
	k := copy(out, r.buf[r.start:min(r.start+r.n, len(r.buf))])
	copy(out[k:], r.buf[:r.n-k])
	if !r.wrapped {
		return out
	}
	if i := bytes.IndexByte(out, '\n'); i >= 0 && i < 4096 {
		return out[i+1:]
	}
	for len(out) > 0 && out[0]&0xC0 == 0x80 {
		out = out[1:]
	}
	return out
}
