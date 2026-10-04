// Mirrors internal/config (JSON form). The server owns the layout; see docs/SPEC.md §6.
export type Node =
  | { kind: "pane"; terminal: string }
  | { kind: "split"; direction: "horizontal" | "vertical"; children: Node[] };

export type Tab = { id: string; title: string; root: Node };

export type Layout = { workspace?: string; tabs: Tab[] };

export function paneIds(node: Node): string[] {
  return node.kind === "pane" ? [node.terminal] : node.children.flatMap(paneIds);
}
