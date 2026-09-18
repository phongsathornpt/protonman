import { useState } from "react";
import { DesktopService } from "../bindings/github.com/phongsathornpt/protonman/cmd/protonman-desktop/index";
import type { Snapshot } from "../bindings/github.com/phongsathornpt/protonman/internal/adapter/in/desktop/models";

const initialSnapshot: Snapshot = { status: "Connecting to Wails shell…" };

export function App() {
  const [snapshot, setSnapshot] = useState(initialSnapshot);

  async function loadSnapshot() {
    setSnapshot(await DesktopService.Snapshot());
  }

  async function markReady() {
    await DesktopService.SetStatus("ready");
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
