import type { Layout, Tab } from "./layout";

type WireLayout = { workspace?: string; tabs: Omit<Tab, "id">[] };

export async function fetchLayout(): Promise<Layout> {
  const res = await fetch("api/layout");
  if (!res.ok) throw new Error(`layout: ${res.status}`);
  const wire: WireLayout = await res.json();
  // Tabs are identified by position; titles need not be unique.
  return { workspace: wire.workspace, tabs: wire.tabs.map((t, i) => ({ ...t, id: `tab${i}` })) };
}

// Relative URLs so the app works behind any proxy path prefix.
export async function restartTerminal(id: string): Promise<void> {
  const res = await fetch(`api/terminals/${encodeURIComponent(id)}/restart`, { method: "POST" });
  if (!res.ok && res.status !== 409) throw new Error(`restart ${id}: ${res.status}`);
}
