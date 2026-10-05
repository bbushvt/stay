// Mirrors internal/config (JSON form). The server owns the layout; see docs/SPEC.md §6.
// `size` is the node's percentage of its parent split (absent = unset).
export type Node =
  | { kind: "pane"; terminal: string; size?: number }
  | { kind: "split"; direction: "horizontal" | "vertical"; children: Node[]; size?: number };

export type Tab = { id: string; title: string; root: Node };

export type Layout = { workspace?: string; tabs: Tab[] };

export function paneIds(node: Node): string[] {
  return node.kind === "pane" ? [node.terminal] : node.children.flatMap(paneIds);
}

// Percent for each child of a split: configured sizes as given, the rest shared
// equally among unsized children. Returns undefined when no child has a size.
export function childSizes(children: Node[]): number[] | undefined {
  if (!children.some((c) => c.size)) return undefined;
  const set = children.reduce((sum, c) => sum + (c.size ?? 0), 0);
  const unsized = children.filter((c) => !c.size).length;
  const share = unsized ? (100 - set) / unsized : 0;
  return children.map((c) => c.size ?? share);
}
