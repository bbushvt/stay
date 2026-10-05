import { Group, Panel, Separator, useDefaultLayout } from "react-resizable-panels";
import { type Node, childSizes, paneIds } from "./layout";
import { Terminal } from "./Terminal";

// Recursively renders a split tree. Sizes persist per split in localStorage.
export function Panes({ node, active, path }: { node: Node; active: boolean; path: string }) {
  if (node.kind === "pane") return <Terminal id={node.terminal} active={active} />;
  return <Split node={node} active={active} path={path} />;
}

function Split({ node, active, path }: { node: Extract<Node, { kind: "split" }>; active: boolean; path: string }) {
  const panelIds = node.children.map((c, i) => `${i}:${paneIds(c).join("+")}`);
  const sizes = childSizes(node.children);
  // Configured sizes are part of the storage key: dragging is remembered per browser, but
  // editing sizes in the layout file starts fresh from the new values.
  const { defaultLayout, onLayoutChanged } = useDefaultLayout({
    id: `stay:${path}${sizes ? `:${sizes.join(",")}` : ""}`,
    panelIds,
    storage: safeStorage(),
  });
  return (
    <Group orientation={node.direction} defaultLayout={defaultLayout} onLayoutChanged={onLayoutChanged}>
      {node.children.flatMap((child, i) => [
        i > 0 ? <Separator key={`sep${i}`} className={`sep ${node.direction}`} /> : null,
        <Panel key={panelIds[i]} id={panelIds[i]} minSize="10%" defaultSize={sizes ? `${sizes[i]}%` : undefined}>
          <Panes node={child} active={active} path={`${path}.${i}`} />
        </Panel>,
      ])}
    </Group>
  );
}

// localStorage can throw (private windows, blocked site data); fall back to memory.
function safeStorage() {
  try {
    localStorage.getItem("stay:probe");
    return localStorage;
  } catch {
    const mem = new Map<string, string>();
    return { getItem: (k: string) => mem.get(k) ?? null, setItem: (k: string, v: string) => void mem.set(k, v) };
  }
}
