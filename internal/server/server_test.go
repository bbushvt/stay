package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/coder/websocket"

	"github.com/bbushvt/stay/internal/config"
	"github.com/bbushvt/stay/internal/terminal"
)

func newTestServer(t *testing.T) (*httptest.Server, *terminal.Session) {
	t.Helper()
	ts, mgr := newTestServerMgr(t)
	sess, _ := mgr.Get("main")
	return ts, sess
}

func newTestServerMgr(t *testing.T) (*httptest.Server, *terminal.Manager) {
	t.Helper()
	dir := t.TempDir()
	mgr, err := terminal.NewManager([]terminal.Def{
		{ID: "main", Config: terminal.Config{Name: "Main", Shell: "/bin/sh", Dir: dir}},
		{ID: "other", Config: terminal.Config{Name: "Other", Shell: "/bin/sh", Dir: dir}},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(mgr.Close)
	assets := fstest.MapFS{"index.html": {Data: []byte("<html>stay</html>")}}
	ts := httptest.NewServer(New(mgr, config.Layout{Workspace: "ws", Tabs: []config.Tab{{Title: "T", Root: config.Node{Kind: "pane", Terminal: "main"}}}}, assets).Handler())
	t.Cleanup(ts.Close)
	return ts, mgr
}

func dial(t *testing.T, ts *httptest.Server, id string) *websocket.Conn {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(ts.URL, "http")+"/ws/terminals/"+id, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.CloseNow() })
	return c
}

func readUntil(t *testing.T, c *websocket.Conn, want string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var got bytes.Buffer
	for {
		typ, data, err := c.Read(ctx)
		if err != nil {
			t.Fatalf("read: %v (got %q, want %q)", err, got.String(), want)
		}
		if typ != websocket.MessageBinary {
			continue
		}
		got.Write(data)
		if strings.Contains(got.String(), want) {
			return
		}
	}
}

func TestServesIndex(t *testing.T) {
	ts, _ := newTestServer(t)
	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var buf bytes.Buffer
	buf.ReadFrom(resp.Body)
	if resp.StatusCode != 200 || !strings.Contains(buf.String(), "stay") {
		t.Fatalf("status %d body %q", resp.StatusCode, buf.String())
	}
}

func TestUnknownTerminal404(t *testing.T) {
	ts, _ := newTestServer(t)
	resp, err := http.Get(ts.URL + "/ws/terminals/nope")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("status %d, want 404", resp.StatusCode)
	}
}

func TestWebsocketHelloInputResize(t *testing.T) {
	ts, _ := newTestServer(t)
	c := dial(t, ts, "main")
	ctx := context.Background()

	typ, data, err := c.Read(ctx)
	if err != nil || typ != websocket.MessageText {
		t.Fatalf("first frame: typ=%v err=%v", typ, err)
	}
	var hello helloMessage
	if err := json.Unmarshal(data, &hello); err != nil || hello.Type != "hello" || hello.Version != ProtocolVersion || hello.ID != "main" || hello.Name != "Main" {
		t.Fatalf("bad hello %s (%v)", data, err)
	}

	c.Write(ctx, websocket.MessageText, mustJSON(ClientMessage{Type: "resize", Cols: 100, Rows: 30}))
	c.Write(ctx, websocket.MessageBinary, []byte("stty size; echo $((20+22))done\n"))
	readUntil(t, c, "30 100")
	readUntil(t, c, "42done")
}

func TestSecondClientSeesSameSession(t *testing.T) {
	ts, _ := newTestServer(t)
	a, b := dial(t, ts, "main"), dial(t, ts, "main")
	a.Write(context.Background(), websocket.MessageBinary, []byte("echo shared-$((1+1))\n"))
	readUntil(t, a, "shared-2")
	readUntil(t, b, "shared-2")
}

func TestDisconnectKeepsShellAlive(t *testing.T) {
	ts, sess := newTestServer(t)
	c := dial(t, ts, "main")
	c.Write(context.Background(), websocket.MessageBinary, []byte("export KEEP=alive\n"))
	time.Sleep(100 * time.Millisecond)
	c.Close(websocket.StatusNormalClosure, "bye")
	time.Sleep(200 * time.Millisecond)

	select {
	case <-sess.Done():
		t.Fatal("shell died when browser disconnected")
	default:
	}
	c2 := dial(t, ts, "main")
	c2.Write(context.Background(), websocket.MessageBinary, []byte("echo state-$KEEP\n"))
	readUntil(t, c2, "state-alive")
}

func TestExitMessage(t *testing.T) {
	ts, _ := newTestServer(t)
	c := dial(t, ts, "main")
	c.Write(context.Background(), websocket.MessageBinary, []byte("exit 3\n"))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for {
		typ, data, err := c.Read(ctx)
		if err != nil {
			t.Fatalf("closed without exit message: %v", err)
		}
		if typ == websocket.MessageText {
			var m exitMessage
			json.Unmarshal(data, &m)
			if m.Type == "exit" {
				if m.Code != 3 {
					t.Fatalf("exit code %d, want 3", m.Code)
				}
				return
			}
		}
	}
}

func TestCrossOriginRejected(t *testing.T) {
	ts, _ := newTestServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, resp, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(ts.URL, "http")+"/ws/terminals/main", &websocket.DialOptions{
		HTTPHeader: http.Header{"Origin": []string{"https://evil.example"}},
	})
	if err == nil {
		t.Fatal("cross-origin websocket was accepted")
	}
	if resp == nil || resp.StatusCode != http.StatusForbidden {
		t.Fatalf("response %v, want 403", resp)
	}
}

// readControl returns the next text frame's "type"/"state" pair, skipping binary.
func collectUntilSyncEnd(t *testing.T, c *websocket.Conn) (replay string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var buf bytes.Buffer
	started := false
	for {
		typ, data, err := c.Read(ctx)
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		if typ == websocket.MessageBinary {
			if !started {
				t.Fatal("binary frame before sync:start")
			}
			buf.Write(data)
			continue
		}
		var m syncMessage
		json.Unmarshal(data, &m)
		if m.Type == "sync" && m.State == "start" {
			started = true
		}
		if m.Type == "sync" && m.State == "end" {
			return buf.String()
		}
	}
}

func TestReconnectReplaysHistory(t *testing.T) {
	ts, _ := newTestServer(t)
	a := dial(t, ts, "main")
	a.Write(context.Background(), websocket.MessageBinary, []byte("echo remembered-$((3+4))\n"))
	readUntil(t, a, "remembered-7")
	a.Close(websocket.StatusNormalClosure, "bye")

	b := dial(t, ts, "main")
	replay := collectUntilSyncEnd(t, b)
	if !strings.Contains(replay, "remembered-7") {
		t.Fatalf("replay missing history: %q", replay)
	}
}

func TestListTerminals(t *testing.T) {
	ts, _ := newTestServerMgr(t)
	resp, err := http.Get(ts.URL + "/api/terminals")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var infos []terminal.Info
	if err := json.NewDecoder(resp.Body).Decode(&infos); err != nil {
		t.Fatal(err)
	}
	if len(infos) != 2 || infos[0].ID != "main" || infos[1].ID != "other" || infos[0].Exited {
		t.Fatalf("unexpected list: %+v", infos)
	}
}

func TestTerminalsAreIndependent(t *testing.T) {
	ts, _ := newTestServerMgr(t)
	a, b := dial(t, ts, "main"), dial(t, ts, "other")
	a.Write(context.Background(), websocket.MessageBinary, []byte("export WHO=main\n"))
	b.Write(context.Background(), websocket.MessageBinary, []byte("export WHO=other\n"))
	time.Sleep(100 * time.Millisecond)
	a.Write(context.Background(), websocket.MessageBinary, []byte("echo who-$WHO\n"))
	readUntil(t, a, "who-main")
}

func post(t *testing.T, url, origin string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest("POST", url, nil)
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	return resp
}

func TestRestart(t *testing.T) {
	ts, mgr := newTestServerMgr(t)
	if got := post(t, ts.URL+"/api/terminals/main/restart", "").StatusCode; got != http.StatusConflict {
		t.Fatalf("restart of running terminal: %d, want 409", got)
	}
	if got := post(t, ts.URL+"/api/terminals/nope/restart", "").StatusCode; got != http.StatusNotFound {
		t.Fatalf("restart unknown: %d, want 404", got)
	}

	old, _ := mgr.Get("main")
	old.Write([]byte("exit 0\n"))
	<-old.Done()
	if got := post(t, ts.URL+"/api/terminals/main/restart", ts.URL).StatusCode; got != http.StatusNoContent {
		t.Fatalf("restart: %d, want 204", got)
	}
	fresh, _ := mgr.Get("main")
	if fresh == old {
		t.Fatal("session was not replaced")
	}
	c := dial(t, ts, "main")
	c.Write(context.Background(), websocket.MessageBinary, []byte("echo reborn\n"))
	readUntil(t, c, "reborn")
}

func TestRestartCrossOriginRefused(t *testing.T) {
	ts, mgr := newTestServerMgr(t)
	s, _ := mgr.Get("main")
	s.Write([]byte("exit 0\n"))
	<-s.Done()
	if got := post(t, ts.URL+"/api/terminals/main/restart", "https://evil.example").StatusCode; got != http.StatusForbidden {
		t.Fatalf("cross-origin restart: %d, want 403", got)
	}
}

func TestLayoutEndpoint(t *testing.T) {
	ts, _ := newTestServerMgr(t)
	resp, err := http.Get(ts.URL + "/api/layout")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var raw map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		t.Fatal(err)
	}
	tabs := raw["tabs"].([]any)
	root := tabs[0].(map[string]any)["root"].(map[string]any)
	if raw["workspace"] != "ws" || len(tabs) != 1 || root["kind"] != "pane" || root["terminal"] != "main" {
		t.Fatalf("unexpected layout JSON: %v", raw)
	}
}
