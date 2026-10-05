export type KeyAction = "copy" | "paste" | "pass";

type KeyLike = Pick<KeyboardEvent, "type" | "key" | "ctrlKey" | "metaKey" | "altKey" | "shiftKey">;

export const isMac = /Mac|iPhone|iPad/.test(navigator.platform);

// Decides what a keydown means for clipboard handling. On macOS Cmd is the
// clipboard modifier and Ctrl-C always reaches the shell. Elsewhere Ctrl-C
// copies only while text is selected (otherwise it is SIGINT), and Ctrl-Shift-C/V
// always copy/paste as in most Linux terminals.
export function keyAction(e: KeyLike, hasSelection: boolean, mac: boolean): KeyAction {
  if (e.type !== "keydown" || e.altKey) return "pass";
  const key = e.key.toLowerCase();
  if (key !== "c" && key !== "v") return "pass";
  if (mac) {
    if (!e.metaKey || e.ctrlKey || e.shiftKey) return "pass";
    return key === "v" ? "paste" : hasSelection ? "copy" : "pass";
  }
  if (!e.ctrlKey || e.metaKey) return "pass";
  if (key === "v") return "paste";
  return e.shiftKey || hasSelection ? "copy" : "pass";
}
