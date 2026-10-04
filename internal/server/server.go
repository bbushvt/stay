// Package server serves the embedded frontend and the per-terminal websocket.
// It performs no authentication; see docs/SPEC.md §7.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/coder/websocket"

	"github.com/bbushvt/stay/internal/config"
	"github.com/bbushvt/stay/internal/terminal"
)

const (
	maxMessageBytes = 1 << 20
	writeTimeout    = 10 * time.Second
	pingInterval    = 30 * time.Second
	maxDim          = 1000
	replayChunk     = 64 << 10
)

// Server routes HTTP requests to sessions and static assets.
type Server struct {
	terms  *terminal.Manager
	layout config.Layout
	assets fs.FS
}

// New builds a Server. assets is the frontend root (index.html at its top level).
func New(terms *terminal.Manager, layout config.Layout, assets fs.FS) *Server {
	return &Server{terms: terms, layout: layout, assets: assets}
}

// Handler returns the HTTP handler.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/layout", s.handleLayout)
	mux.HandleFunc("GET /api/terminals", s.handleList)
	mux.HandleFunc("POST /api/terminals/{id}/restart", s.handleRestart)
	mux.HandleFunc("GET /ws/terminals/{id}", s.handleTerminal)
	mux.Handle("GET /", http.FileServerFS(s.assets))
	return mux
}

func (s *Server) handleLayout(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(s.layout)
}

func (s *Server) handleList(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(s.terms.List())
}

// handleRestart respawns an exited terminal. Browsers attach an Origin header
// to POSTs, so the same-origin check blocks cross-site request forgery.
func (s *Server) handleRestart(w http.ResponseWriter, r *http.Request) {
	if o := r.Header.Get("Origin"); o != "" {
		if u, err := url.Parse(o); err != nil || !strings.EqualFold(u.Host, r.Host) {
			http.Error(w, "cross-origin request refused", http.StatusForbidden)
			return
		}
	}
	err := s.terms.Restart(r.PathValue("id"))
	switch {
	case err == nil:
		w.WriteHeader(http.StatusNoContent)
	case errors.Is(err, terminal.ErrRunning):
		http.Error(w, err.Error(), http.StatusConflict)
	default:
		http.Error(w, err.Error(), http.StatusNotFound)
	}
}

func (s *Server) handleTerminal(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sess, ok := s.terms.Get(id)
	if !ok {
		http.NotFound(w, r)
		return
	}

	// Default AcceptOptions enforce same-origin (Origin host == Host), which is
	// our protection against cross-site websocket hijacking.
	c, err := websocket.Accept(w, r, nil)
	if err != nil {
		return // Accept already replied
	}
	defer c.CloseNow()
	c.SetReadLimit(maxMessageBytes)

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	history, sub := sess.Attach()
	defer sub.Close()

	if err := write(ctx, c, websocket.MessageText, mustJSON(helloMessage{
		Type: "hello", Version: ProtocolVersion, ID: id, Name: sess.Name,
	})); err != nil {
		return
	}

	// Replay: bracketed by sync messages so the client can reset first and
	// knows when live output begins (SPEC §5).
	if err := write(ctx, c, websocket.MessageText, mustJSON(syncMessage{Type: "sync", State: "start"})); err != nil {
		return
	}
	for len(history) > 0 {
		n := min(len(history), replayChunk)
		if err := write(ctx, c, websocket.MessageBinary, history[:n]); err != nil {
			return
		}
		history = history[n:]
	}
	if err := write(ctx, c, websocket.MessageText, mustJSON(syncMessage{Type: "sync", State: "end"})); err != nil {
		return
	}

	go func() {
		defer cancel()
		readLoop(ctx, c, sess)
	}()

	ping := time.NewTicker(pingInterval)
	defer ping.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case data, ok := <-sub.C:
			if !ok {
				finish(ctx, c, sess)
				return
			}
			if err := write(ctx, c, websocket.MessageBinary, data); err != nil {
				return
			}
		case <-ping.C:
			pctx, pcancel := context.WithTimeout(ctx, writeTimeout)
			err := c.Ping(pctx)
			pcancel()
			if err != nil {
				return
			}
		}
	}
}

// finish handles the subscriber channel closing: either the shell exited
// (tell the client) or this client was too slow and was dropped.
func finish(ctx context.Context, c *websocket.Conn, sess *terminal.Session) {
	select {
	case <-sess.Done():
		_ = write(ctx, c, websocket.MessageText, mustJSON(exitMessage{Type: "exit", Code: sess.ExitCode()}))
		c.Close(websocket.StatusNormalClosure, "exited")
	default:
		c.Close(websocket.StatusPolicyViolation, "client too slow")
	}
}

func readLoop(ctx context.Context, c *websocket.Conn, sess *terminal.Session) {
	for {
		typ, data, err := c.Read(ctx)
		if err != nil {
			return
		}
		switch typ {
		case websocket.MessageBinary:
			if _, err := sess.Write(data); err != nil {
				return
			}
		case websocket.MessageText:
			var m ClientMessage
			if err := json.Unmarshal(data, &m); err != nil {
				continue
			}
			if m.Type == "resize" && m.Cols > 0 && m.Rows > 0 && m.Cols <= maxDim && m.Rows <= maxDim {
				apply := sess.Resize
				if m.Redraw {
					apply = sess.Redraw
				}
				if err := apply(uint16(m.Cols), uint16(m.Rows)); err != nil {
					log.Printf("resize %q: %v", sess.Name, err)
				}
			}
		}
	}
}

func write(ctx context.Context, c *websocket.Conn, typ websocket.MessageType, data []byte) error {
	ctx, cancel := context.WithTimeout(ctx, writeTimeout)
	defer cancel()
	return c.Write(ctx, typ, data)
}
