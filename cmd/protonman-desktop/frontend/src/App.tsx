import { useState } from "react";

type Snapshot = {
  status: string;
};

declare global {
  interface Window {
    go?: {
      main?: {
			DesktopApp?: {
          Snapshot: () => Promise<Snapshot>;
          SetStatus: (status: string) => Promise<void>;
        };
      };
    };
  }
}

const initialSnapshot: Snapshot = { status: "Connecting to Wails shell…" };

export function App() {
  const [snapshot, setSnapshot] = useState(initialSnapshot);

  async function loadSnapshot() {
	const api = window.go?.main?.DesktopApp;
    if (!api) {
      setSnapshot({ status: "Wails bindings unavailable in browser mode" });
      return;
    }

    setSnapshot(await api.Snapshot());
  }

  async function markReady() {
	const api = window.go?.main?.DesktopApp;
    if (!api) {
      return;
    }

    await api.SetStatus("ready");
    await loadSnapshot();
  }

  return (
    <main className="shell">
      <aside className="sidebar">
        <p className="eyebrow">PROTONMAN</p>
        <h1>Desktop migration</h1>
        <p className="muted">React and TypeScript are now the target presentation layer.</p>
      </aside>
      <section className="content">
        <header>
          <span className="eyebrow">WAILS BRIDGE</span>
          <h2>Runtime shell</h2>
        </header>
        <div className="status-card">
          <span className="status-dot" />
          <div>
            <strong>{snapshot.status}</strong>
            <p className="muted">ACP and session behavior will move behind the Go controller in the next slice.</p>
          </div>
        </div>
        <div className="actions">
          <button onClick={loadSnapshot}>Refresh snapshot</button>
          <button className="primary" onClick={markReady}>Exercise bridge</button>
        </div>
      </section>
    </main>
  );
}
