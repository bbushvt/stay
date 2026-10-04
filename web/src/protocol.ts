// Mirrors internal/server/protocol.go; see docs/SPEC.md §3.

export type ClientMessage = { type: "resize"; cols: number; rows: number; redraw?: boolean };

export type ServerMessage =
  | { type: "hello"; version: number; id: string; name: string }
  | { type: "sync"; state: "start" | "end" }
  | { type: "exit"; code: number };

export function terminalSocketURL(id: string): string {
  // Relative to the page so it works behind any proxy path prefix.
  const url = new URL(`ws/terminals/${encodeURIComponent(id)}`, window.location.href);
  url.protocol = url.protocol === "https:" ? "wss:" : "ws:";
  return url.toString();
}
