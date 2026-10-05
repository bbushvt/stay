import { useEffect, useRef, useState } from "react";
import { Terminal as XTerm } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import { WebglAddon } from "@xterm/addon-webgl";
import { ClipboardAddon } from "@xterm/addon-clipboard";
import { WebLinksAddon } from "@xterm/addon-web-links";
import "@xterm/xterm/css/xterm.css";
import { restartTerminal } from "./api";
import { isMac, keyAction } from "./keys";
import { type ClientMessage, type ServerMessage, terminalSocketURL } from "./protocol";

// `active` is whether this pane's tab is showing. Hidden terminals stay
// connected (no replay on every tab switch) but skip fitting while zero-sized.
export function Terminal({ id, active }: { id: string; active: boolean }) {
  const host = useRef<HTMLDivElement>(null);
  const focusRef = useRef<() => void>(() => {});
  const [exitCode, setExitCode] = useState<number | null>(null);
  const [generation, setGeneration] = useState(0);

  useEffect(() => {
    const el = host.current!;
    setExitCode(null);
    const term = new XTerm({
      cursorBlink: true,
      fontFamily: 'ui-monospace, "SF Mono", Menlo, Consolas, monospace',
      fontSize: 14,
      scrollback: 5000,
      allowProposedApi: true,
    });
    const fit = new FitAddon();
    term.loadAddon(fit);
    term.loadAddon(new ClipboardAddon());
    term.loadAddon(new WebLinksAddon());
    term.open(el);
    try {
      const gl = new WebglAddon();
      gl.onContextLoss(() => gl.dispose());
      term.loadAddon(gl);
    } catch {
      // WebGL unavailable: xterm falls back to its DOM/canvas renderer.
    }
    const doFit = () => {
      if (el.clientWidth > 0 && el.clientHeight > 0) fit.fit();
    };
    doFit();
    focusRef.current = () => term.focus();

    // Copy/paste shortcuts (see keys.ts). Paste returns false without
    // preventDefault so the browser's native paste event reaches xterm, which
    // handles bracketed paste and needs no clipboard-read permission.
    term.attachCustomKeyEventHandler((e) => {
      const action = keyAction(e, term.hasSelection(), isMac);
      if (action === "copy") {
        e.preventDefault();
        void navigator.clipboard.writeText(term.getSelection()).catch(() => {});
        term.clearSelection();
      }
      return action === "pass";
    });

    const enc = new TextEncoder();
    let ws: WebSocket | null = null;
    let disposed = false;
    let exited = false;
    let retry = 0;
    let retryTimer: number | undefined;

    const send = (m: ClientMessage) => ws?.readyState === WebSocket.OPEN && ws.send(JSON.stringify(m));

    const connect = () => {
      const sock = new WebSocket(terminalSocketURL(id));
      ws = sock;
      sock.binaryType = "arraybuffer";
      sock.onopen = () => {
        retry = 0;
      };
      sock.onmessage = (ev) => {
        if (typeof ev.data !== "string") {
          term.write(new Uint8Array(ev.data as ArrayBuffer));
          return;
        }
        const msg = JSON.parse(ev.data) as ServerMessage;
        if (msg.type === "sync" && msg.state === "start") {
          // Server is replaying history: start from a clean screen (SPEC §5).
          term.reset();
        } else if (msg.type === "sync" && msg.state === "end") {
          // Fit to this browser, then force SIGWINCH so TUIs repaint.
          doFit();
          send({ type: "resize", cols: term.cols, rows: term.rows, redraw: true });
        } else if (msg.type === "exit") {
          exited = true;
          setExitCode(msg.code);
        }
      };
      sock.onclose = () => {
        if (disposed || exited || ws !== sock) return;
        const delay = Math.min(500 * 2 ** retry++, 10000);
        retryTimer = window.setTimeout(connect, delay);
      };
    };
    connect();

    const subs = [
      term.onData((d) => ws?.readyState === WebSocket.OPEN && ws.send(enc.encode(d))),
      term.onBinary((d) => {
        if (ws?.readyState === WebSocket.OPEN) ws.send(Uint8Array.from(d, (c) => c.charCodeAt(0)));
      }),
      term.onResize(({ cols, rows }) => send({ type: "resize", cols, rows })),
    ];

    const ro = new ResizeObserver(doFit);
    ro.observe(el);

    return () => {
      disposed = true;
      window.clearTimeout(retryTimer);
      ro.disconnect();
      subs.forEach((s) => s.dispose());
      ws?.close();
      term.dispose();
    };
  }, [id, generation]);

  useEffect(() => {
    if (active) focusRef.current();
  }, [active]);

  const restart = async () => {
    await restartTerminal(id);
    setGeneration((g) => g + 1);
  };

  return (
    <div className="term">
      <div ref={host} className="term-host" />
      {exitCode !== null && (
        <div className="term-exited">
          <span>process exited with code {exitCode}</span>
          <button onClick={restart}>Restart</button>
        </div>
      )}
    </div>
  );
}
