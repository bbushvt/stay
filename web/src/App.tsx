import { useEffect, useState } from "react";
import { fetchLayout } from "./api";
import type { Tab } from "./layout";
import { About } from "./About";
import { Panes } from "./Panes";

const ACTIVE_TAB_KEY = "stay:activeTab";

function loadActive(): string | null {
  try {
    return localStorage.getItem(ACTIVE_TAB_KEY);
  } catch {
    return null;
  }
}

export function App() {
  const [tabs, setTabs] = useState<Tab[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [active, setActive] = useState<string | null>(loadActive);
  const [version, setVersion] = useState("");
  const [aboutOpen, setAboutOpen] = useState(false);

  useEffect(() => {
    fetchLayout()
      .then((layout) => {
        setTabs(layout.tabs);
        setVersion(layout.version);
        document.title = layout.workspace ? `STAY — ${layout.workspace}` : "STAY";
      })
      .catch((e) => setError(String(e)));
  }, []);

  if (error) return <div className="notice">{error}</div>;
  if (!tabs) return null;
  if (tabs.length === 0) return <div className="notice">No terminals configured.</div>;

  const current = tabs.some((t) => t.id === active) ? active! : tabs[0].id;
  const select = (id: string) => {
    setActive(id);
    try {
      localStorage.setItem(ACTIVE_TAB_KEY, id);
    } catch {
      // per-viewer convenience only
    }
  };

  return (
    <div className="app">
      <nav className="tabs">
        <button className="about-btn" onClick={() => setAboutOpen(true)}>
          About
        </button>
        {tabs.map((t) => (
          <button key={t.id} className={t.id === current ? "tab selected" : "tab"} onClick={() => select(t.id)}>
            {t.title}
          </button>
        ))}
      </nav>
      <About version={version} open={aboutOpen} onClose={() => setAboutOpen(false)} />
      <div className="stage">
        {tabs.map((t) => (
          <div key={t.id} className="tab-body" style={{ display: t.id === current ? "block" : "none" }}>
            <Panes node={t.root} active={t.id === current} path={t.id} />
          </div>
        ))}
      </div>
    </div>
  );
}
