import { useEffect, useMemo, useRef, useState } from "react";
import { Events } from "@wailsio/runtime";
import { DesktopService } from "../bindings/github.com/phongsathornpt/protonman/cmd/protonman-desktop/index";
import {
  PermissionView,
  MCPServerView,
  Snapshot,
  TodoOperationView,
  TimelineView,
} from "../bindings/github.com/phongsathornpt/protonman/internal/adapter/in/desktop/models";

const initialSnapshot = new Snapshot({ status: "Ready to connect", connection: "disconnected" });

export function App() {
  const [snapshot, setSnapshot] = useState(initialSnapshot);
  const [draft, setDraft] = useState("");
  const [workspace, setWorkspace] = useState("");
  const [additionalDirectories, setAdditionalDirectories] = useState("");
  const [mcpName, setMcpName] = useState("");
  const [mcpCommand, setMcpCommand] = useState("");
  const [mcpArgs, setMcpArgs] = useState("");
  const [sessionQuery, setSessionQuery] = useState("");
  const [busy, setBusy] = useState(false);
  const [runningSessionID, setRunningSessionID] = useState("");
  const [pendingAction, setPendingAction] = useState("");
  const [error, setError] = useState("");
  const [showJumpLatest, setShowJumpLatest] = useState(false);
  const transcriptRef = useRef<HTMLDivElement>(null);
  const followTranscript = useRef(true);
  const activeSession = useMemo(() => snapshot.sessions.find((s) => s.id === snapshot.activeSessionId), [snapshot]);
  const visibleSessions = useMemo(() => {
    const query = sessionQuery.trim().toLowerCase();
    if (!query) return snapshot.sessions;
    return snapshot.sessions.filter((session) => `${session.title} ${session.workspace}`.toLowerCase().includes(query));
  }, [sessionQuery, snapshot.sessions]);
  const timelineRevision = useMemo(() => activeSession?.timeline.map((item) => `${item.id}:${item.status}:${item.text.length}`).join("|") ?? "", [activeSession]);

  useEffect(() => {
    const unsubscribe = Events.On("desktop:snapshot", (event) => setSnapshot(Snapshot.createFrom(event.data)));
    void bootstrap();
    return unsubscribe;
  }, []);

  useEffect(() => {
    const transcript = transcriptRef.current;
    if (transcript && followTranscript.current) {
      transcript.scrollTop = transcript.scrollHeight;
      setShowJumpLatest(false);
    }
  }, [activeSession?.id, timelineRevision, busy]);

  useEffect(() => {
    followTranscript.current = true;
    setShowJumpLatest(false);
  }, [activeSession?.id]);

  function jumpToLatest() {
    const transcript = transcriptRef.current;
    if (!transcript) return;
    followTranscript.current = true;
    setShowJumpLatest(false);
    transcript.scrollTo({ top: transcript.scrollHeight, behavior: "smooth" });
  }

  async function bootstrap() {
    if (pendingAction) return;
    setPendingAction("bootstrap");
    try { setError(""); setSnapshot(await DesktopService.Bootstrap()); } catch (cause) { setError(errorMessage(cause)); } finally { setPendingAction(""); }
  }

  async function createSession() {
    if (pendingAction) return;
    setPendingAction("create-session");
    const directories = additionalDirectories.split("\n").map((value) => value.trim()).filter(Boolean);
    const servers = mcpCommand.trim() ? [new MCPServerView({ name: mcpName.trim() || "desktop-mcp", command: mcpCommand.trim(), args: mcpArgs.trim() ? mcpArgs.trim().split(/\s+/) : [] })] : [];
    try { setError(""); setSnapshot(await DesktopService.NewSession(workspace, directories, servers)); setWorkspace(""); setAdditionalDirectories(""); setMcpName(""); setMcpCommand(""); setMcpArgs(""); } catch (cause) { setError(errorMessage(cause)); } finally { setPendingAction(""); }
  }

  async function selectSession(sessionID: string) {
    if (pendingAction || busy) return;
    setPendingAction("select-session");
    try { setError(""); setSnapshot(await DesktopService.SelectSession(sessionID)); } catch (cause) { setError(errorMessage(cause)); } finally { setPendingAction(""); }
  }

  async function setMode(mode: string) {
    if (!activeSession) return;
    if (pendingAction) return;
    setPendingAction("mode");
    try { setError(""); setSnapshot(await DesktopService.SetMode(activeSession.id, mode)); } catch (cause) { setError(errorMessage(cause)); } finally { setPendingAction(""); }
  }

  async function setReasoning(reasoning: string) {
    if (!activeSession) return;
    if (pendingAction) return;
    setPendingAction("reasoning");
    try { setError(""); setSnapshot(await DesktopService.SetReasoning(activeSession.id, reasoning)); } catch (cause) { setError(errorMessage(cause)); } finally { setPendingAction(""); }
  }

  async function setModel(model: string) {
    if (!activeSession || pendingAction || !model) return;
    setPendingAction("model");
    try { setError(""); setSnapshot(await DesktopService.SetModel(activeSession.id, model)); } catch (cause) { setError(errorMessage(cause)); } finally { setPendingAction(""); }
  }

  async function setProvider(provider: string) {
    if (!activeSession || pendingAction || !provider) return;
    setPendingAction("provider");
    try { setError(""); setSnapshot(await DesktopService.SetProvider(activeSession.id, provider)); } catch (cause) { setError(errorMessage(cause)); } finally { setPendingAction(""); }
  }

  async function setLowConcurrency(setting: string) {
    if (!activeSession) return;
    if (pendingAction) return;
    setPendingAction("concurrency");
    try { setError(""); setSnapshot(await DesktopService.SetLowConcurrency(activeSession.id, setting)); } catch (cause) { setError(errorMessage(cause)); } finally { setPendingAction(""); }
  }

  async function closeActiveSession() {
    if (!activeSession || pendingAction || busy) return;
    setPendingAction("close-session");
    try { setError(""); setSnapshot(await DesktopService.CloseSession(activeSession.id)); } catch (cause) { setError(errorMessage(cause)); } finally { setPendingAction(""); }
  }

  async function deleteActiveSession() {
    if (!activeSession || pendingAction || busy || !window.confirm("Delete this session and its saved history?")) return;
    setPendingAction("delete-session");
    try { setError(""); setSnapshot(await DesktopService.DeleteSession(activeSession.id)); } catch (cause) { setError(errorMessage(cause)); } finally { setPendingAction(""); }
  }

  async function forgetMemory(scope: string, memoryID: string) {
    if (!activeSession || pendingAction || busy || !window.confirm("Forget this memory permanently?")) return;
    setPendingAction("forget-memory");
    try { setError(""); setSnapshot(await DesktopService.ForgetMemory(activeSession.id, scope, memoryID)); } catch (cause) { setError(errorMessage(cause)); } finally { setPendingAction(""); }
  }

  async function updateTodoStatus(itemID: string, status: string) {
    if (!activeSession || pendingAction || busy) return;
    setPendingAction("todo");
    try { setError(""); setSnapshot(await DesktopService.PatchTodo(activeSession.id, activeSession.context.todo.revision, new TodoOperationView({ op: "set_status", id: itemID, status }))); } catch (cause) { setError(errorMessage(cause)); } finally { setPendingAction(""); }
  }

  async function patchTodo(operation: TodoOperationView) {
    if (!activeSession || pendingAction || busy) return;
    setPendingAction("todo");
    try { setError(""); setSnapshot(await DesktopService.PatchTodo(activeSession.id, activeSession.context.todo.revision, operation)); } catch (cause) { setError(errorMessage(cause)); } finally { setPendingAction(""); }
  }

  async function submitPrompt() {
    if (!activeSession || !draft.trim() || busy) return;
    const text = draft.trim();
    const sessionID = activeSession.id;
    setDraft(""); setBusy(true); setRunningSessionID(sessionID); setPendingAction("prompt"); setError("");
    try { setSnapshot(await DesktopService.SendPrompt(sessionID, text)); } catch (cause) { setError(errorMessage(cause)); } finally { setBusy(false); setRunningSessionID(""); setPendingAction(""); }
  }

  async function cancelPrompt() {
    if (!runningSessionID || !busy || pendingAction === "cancel") return;
    setPendingAction("cancel");
    try { setSnapshot(await DesktopService.CancelPrompt(runningSessionID)); } catch (cause) { setError(errorMessage(cause)); } finally { setPendingAction(""); }
  }

  async function resolvePermission(request: PermissionView, optionID: string) {
    if (pendingAction) return;
    setPendingAction("permission");
    try { setError(""); setSnapshot(await DesktopService.ResolvePermission(request.requestId, optionID)); } catch (cause) { setError(errorMessage(cause)); } finally { setPendingAction(""); }
  }

  return (
    <main className="app-shell">
      <header className="topbar">
        <div className="brand-lockup"><span className="brand-mark" aria-hidden="true">P</span><div><p className="kicker">PROTONMAN / DESKTOP</p><h1>Code room</h1></div></div>
        <div className="connection-state" data-state={snapshot.connection} role="status"><span className="connection-dot" aria-hidden="true" /><span>{snapshot.connection === "connected" ? "ACP connected" : snapshot.status}</span></div>
      </header>
      <div className="workspace-grid">
        <aside className="session-rail" aria-label="Sessions">
          <div className="rail-heading"><div><p className="kicker">WORKSPACE</p><strong>{activeSession?.workspace || "No workspace"}</strong></div><button className="icon-button" aria-label="Connect or refresh" onClick={() => void bootstrap()} disabled={Boolean(pendingAction)}>↻</button></div>
          <div className="new-session-form"><label htmlFor="workspace">New workspace</label><input id="workspace" value={workspace} onChange={(event) => setWorkspace(event.target.value)} placeholder="Current directory" onKeyDown={(event) => { if (event.key === "Enter") void createSession(); }} disabled={Boolean(pendingAction || busy)} /><details className="advanced-session"><summary>Workspace & MCP</summary><label htmlFor="additional-directories">Additional roots</label><textarea id="additional-directories" value={additionalDirectories} onChange={(event) => setAdditionalDirectories(event.target.value)} placeholder="One absolute path per line" rows={2} disabled={Boolean(pendingAction || busy)} /><label htmlFor="mcp-name">MCP server name</label><input id="mcp-name" value={mcpName} onChange={(event) => setMcpName(event.target.value)} placeholder="local-tools" disabled={Boolean(pendingAction || busy)} /><label htmlFor="mcp-command">MCP stdio command</label><input id="mcp-command" value={mcpCommand} onChange={(event) => setMcpCommand(event.target.value)} placeholder="command" disabled={Boolean(pendingAction || busy)} /><label htmlFor="mcp-args">MCP arguments</label><input id="mcp-args" value={mcpArgs} onChange={(event) => setMcpArgs(event.target.value)} placeholder="--stdio" disabled={Boolean(pendingAction || busy)} /></details><button className="button button-primary" onClick={() => void createSession()} disabled={snapshot.connection !== "connected" || Boolean(pendingAction || busy)}>Start session</button></div>
          <div className="session-list" aria-label="Open sessions"><label className="session-filter" htmlFor="session-filter">Find session</label><input id="session-filter" value={sessionQuery} onChange={(event) => setSessionQuery(event.target.value)} placeholder="Title or workspace" disabled={Boolean(pendingAction || busy)} />
            {snapshot.sessions.length === 0 ? <div className="empty-rail">Your coding sessions will appear here.</div> : visibleSessions.length === 0 ? <div className="empty-rail">No sessions match this filter.</div> : visibleSessions.map((session) => <button className={`session-row ${session.id === activeSession?.id ? "is-active" : ""}`} key={session.id} onClick={() => void selectSession(session.id)} disabled={Boolean(pendingAction || busy)} aria-current={session.id === activeSession?.id ? "page" : undefined}><span className="session-status" data-status={session.status} aria-hidden="true" /><span className="session-copy"><strong title={session.title || "Untitled session"}>{session.title || "Untitled session"}</strong><small title={session.workspace || "Workspace pending"}>{session.workspace || "Workspace pending"}</small></span><span className="session-state" title={session.status}>{session.status}</span></button>)}
          </div>
          <div className="rail-footer"><span className="kicker">RUNTIME</span><span>ACP / local process</span></div>
        </aside>

        <section className="chat-column" aria-label="Chat">
          <div className="chat-heading"><div><p className="kicker">ACTIVE THREAD</p><h2>{activeSession?.title || "Start a coding session"}</h2></div>{activeSession && <div className="heading-controls"><div className="mode-switch" role="group" aria-label="Permission mode">{["ask", "plan", "always-approve"].map((mode) => <button type="button" key={mode} className={activeSession.mode === mode ? "is-selected" : ""} aria-pressed={activeSession.mode === mode} onClick={() => void setMode(mode)} disabled={Boolean(pendingAction || busy)}>{mode.replace("always-approve", "auto")}</button>)}</div><span className="status-chip" data-status={activeSession.status}>{activeSession.status.replace("_", " ")}</span><button className="session-action" type="button" onClick={() => void closeActiveSession()} disabled={Boolean(pendingAction || busy)} title="Close session">Close</button><button className="session-action session-action-danger" type="button" onClick={() => void deleteActiveSession()} disabled={Boolean(pendingAction || busy)} title="Delete saved session">Delete</button></div>}</div>
          {activeSession && <div className="runtime-strip"><label>Provider<select value={activeSession.runtime.provider || ""} onChange={(event) => void setProvider(event.target.value)} disabled={Boolean(pendingAction || busy || activeSession.providerOptions.length === 0)} aria-label="Select provider">{activeSession.providerOptions.length === 0 ? <option value={activeSession.runtime.provider || ""}>{activeSession.runtime.provider || "Provider unavailable"}</option> : activeSession.providerOptions.map((option) => <option key={option.value} value={option.value}>{option.name}</option>)}</select></label><label className="model-control">Model<select value={activeSession.runtime.model || ""} onChange={(event) => void setModel(event.target.value)} disabled={Boolean(pendingAction || busy || activeSession.modelOptions.length === 0)} aria-label="Select model">{activeSession.modelOptions.length === 0 ? <option value={activeSession.runtime.model || ""}>{activeSession.runtime.model || "Model unavailable"}</option> : activeSession.modelOptions.map((option) => <option key={option.value} value={option.value}>{option.name}</option>)}</select></label><label>Reasoning<select value={activeSession.runtime.reasoning || "auto"} onChange={(event) => void setReasoning(event.target.value)} disabled={Boolean(pendingAction || busy)}><option value="auto">Auto</option><option value="none">None</option><option value="minimal">Minimal</option><option value="low">Low</option><option value="medium">Medium</option><option value="high">High</option><option value="xhigh">Extra high</option><option value="max">Max</option></select></label><button className={`runtime-toggle ${activeSession.runtime.lowConcurrency === "on" ? "is-on" : ""}`} onClick={() => void setLowConcurrency(activeSession.runtime.lowConcurrency === "on" ? "off" : "on")} disabled={Boolean(pendingAction || busy)}>Low concurrency: {activeSession.runtime.lowConcurrency || "auto"}</button>{activeSession.modelOptionsError && <span className="model-error" title={activeSession.modelOptionsError}>Model catalog unavailable <button type="button" onClick={() => void setProvider(activeSession.runtime.provider)} disabled={Boolean(pendingAction || busy)}>Retry</button></span>}</div>}
          <div className="transcript" ref={transcriptRef} role="log" aria-live="off" aria-label="Conversation transcript" onScroll={(event) => { const target = event.currentTarget; const atLatest = target.scrollHeight - target.scrollTop - target.clientHeight < 120; followTranscript.current = atLatest; setShowJumpLatest(!atLatest); }}>
            {!activeSession ? <EmptyChat onCreate={() => void createSession()} /> : activeSession.timeline.length === 0 ? <div className="welcome-card"><span className="welcome-index">01</span><p className="kicker">READY FOR INSTRUCTIONS</p><h3>What are we building?</h3><p>Describe the change in plain language. Protonman will inspect the workspace, explain its plan, and ask before it makes consequential changes.</p></div> : compactTimeline(activeSession.timeline).map((item, index) => <TimelineCard item={item} key={`${item.id}-${index}`} />)}
            {busy && <div className="typing-line" role="status" aria-live="polite"><span className="typing-pulse" /> Protonman is working through the workspace</div>}
            {showJumpLatest && <button className="jump-latest" type="button" onClick={jumpToLatest}>Jump to latest</button>}
          </div>
          {error && <div className="error-banner" role="alert">{error}<button onClick={() => setError("")}>Dismiss</button></div>}
          <form className="composer" onSubmit={(event) => { event.preventDefault(); void submitPrompt(); }}><label htmlFor="prompt">Message Protonman</label><textarea id="prompt" value={draft} onChange={(event) => setDraft(event.target.value)} placeholder={activeSession ? "Ask for a change, an explanation, or a review…" : "Create a session to begin"} disabled={!activeSession || snapshot.connection !== "connected" || Boolean(pendingAction && pendingAction !== "prompt")} rows={3} onKeyDown={(event) => { if (event.key === "Enter" && (event.metaKey || event.ctrlKey)) { event.preventDefault(); void submitPrompt(); } }} /><div className="composer-footer"><span>⌘ / Ctrl + Enter to send</span>{busy ? <button className="button button-danger" type="button" onClick={() => void cancelPrompt()} disabled={pendingAction === "cancel"}>Stop work</button> : <button className="button button-primary" type="submit" disabled={!draft.trim() || !activeSession || snapshot.connection !== "connected" || Boolean(pendingAction)}>Send prompt</button>}</div></form>
        </section>

        <aside className="activity-rail" aria-label="Activity"><div className="activity-heading"><p className="kicker">ATTENTION</p><h2>Activity</h2></div>{snapshot.permissionInbox.length > 0 ? <div className="permission-stack">{snapshot.permissionInbox.map((request) => <div className="permission-card" key={request.requestId}><span className="permission-label">DECISION REQUIRED</span><h3>{request.title}</h3><p>{request.detail || "Protonman needs your approval before continuing."}</p><div className="permission-actions">{request.options.map((option) => <button key={option.id} className={option.kind.includes("reject") ? "button button-quiet" : "button button-primary"} onClick={() => void resolvePermission(request, option.id)}>{option.name}</button>)}</div></div>)}</div> : <div className="activity-empty"><span className="activity-line" aria-hidden="true" /><p>No pending decisions.</p><small>Tool calls and permission requests will surface here while the agent works.</small></div>}{activeSession && <ContextPanel session={activeSession} onForget={(scope, memoryID) => void forgetMemory(scope, memoryID)} onTodoStatus={(itemID, status) => void updateTodoStatus(itemID, status)} onTodoPatch={(operation) => void patchTodo(operation)} />}<div className="activity-note"><span className="kicker">DESIGN NOTE</span><p>Every change stays visible. The chat is the narrative; this rail is the control surface.</p></div></aside>
      </div>
    </main>
  );
}

function TimelineCard({ item }: { item: TimelineView }) {
  const isUser = item.kind === "user";
  const isTool = item.kind === "tool";
  const isHistorical = isTool && item.title.startsWith("Historical tool ");
  const title = isHistorical ? item.title.replace("Historical tool ", "") : item.title;
  return <article className={`message-card ${isUser ? "message-user" : ""} ${isTool ? "message-tool" : ""} ${isHistorical ? "message-historical" : ""}`}><div className="message-meta"><span>{isUser ? "YOU" : isTool ? "TOOL ACTIVITY" : "PROTONMAN"}</span>{item.status && <span className="message-status">{item.status.replace("_", " ")}</span>}</div>{isHistorical ? <details><summary className="historical-summary"><strong>{title}</strong><span>show output</span></summary><div className="message-text">{item.text}</div></details> : <>{title && <h3>{title}</h3>}<div className="message-text">{item.text}</div></>}</article>;
}

function EmptyChat({ onCreate }: { onCreate: () => void }) {
  return <div className="empty-chat"><span className="empty-glyph" aria-hidden="true">+</span><p className="kicker">NO ACTIVE THREAD</p><h3>Open a workspace to begin.</h3><button className="button button-primary" onClick={onCreate}>Create session</button></div>;
}

function compactTimeline(items: TimelineView[]) {
  return items.reduce<TimelineView[]>((result, item) => { const previous = result[result.length - 1]; if (previous && previous.kind === item.kind && item.kind === "assistant" && !item.id) { previous.text += item.text; return result; } result.push(new TimelineView({ ...item })); return result; }, []);
}

function errorMessage(error: unknown) { return error instanceof Error ? error.message : String(error); }

function ContextPanel({ session, onForget, onTodoStatus, onTodoPatch }: { session: Snapshot["sessions"][number]; onForget: (scope: string, memoryID: string) => void; onTodoStatus: (itemID: string, status: string) => void; onTodoPatch: (operation: TodoOperationView) => void }) {
  const todo = session.context.todo.items;
  const memories = [...session.context.memory.workspace.map((entry) => ({ ...entry, displayScope: "workspace" })), ...session.context.memory.global.map((entry) => ({ ...entry, displayScope: "global" }))];
  function addTodo() { const text = window.prompt("Task text"); if (text?.trim()) onTodoPatch(new TodoOperationView({ op: "add", id: `task-${Date.now()}`, text: text.trim(), status: "pending" })); }
  function editTodo(itemID: string, text: string) { const next = window.prompt("Edit task", text); if (next?.trim()) onTodoPatch(new TodoOperationView({ op: "set_text", id: itemID, text: next.trim() })); }
  function removeTodo(itemID: string) { if (window.confirm("Remove this task?")) onTodoPatch(new TodoOperationView({ op: "remove", id: itemID })); }
  return <section className="context-panel"><div className="context-heading"><span className="kicker">CONTEXT</span><span>{session.subagents.length} agents</span></div>{session.context.goal && <div className="context-goal"><span className="context-label">ACTIVE GOAL</span><p>{session.context.goal}</p></div>}<div className="context-stats"><span><strong>{todo.filter((item) => item.status !== "completed").length}</strong> open tasks</span><span><strong>{memories.length}</strong> memories</span></div><div className="todo-list"><div className="context-label todo-heading">TASKS <button type="button" onClick={addTodo}>+ Add</button></div>{todo.map((item) => <div className="todo-row" key={item.id}><div><strong data-status={item.status}>{item.status.replace("_", " ")}</strong><small>{item.text}</small></div><span className="todo-actions"><button type="button" onClick={() => onTodoStatus(item.id, nextTodoStatus(item.status))} title="Advance task status">→</button><button type="button" onClick={() => editTodo(item.id, item.text)} title="Edit task">✎</button><button type="button" onClick={() => removeTodo(item.id)} title="Remove task">×</button></span></div>)}</div>{memories.length > 0 && <div className="memory-list"><span className="context-label">MEMORIES</span>{memories.map((entry) => <div className="memory-row" key={`${entry.displayScope}:${entry.id}`}><div><strong>{entry.key || entry.kind}</strong><small>{entry.value}</small></div><button type="button" onClick={() => onForget(entry.displayScope, entry.id)} title="Forget memory">×</button></div>)}</div>}{session.subagents.length > 0 && <div className="agent-list">{session.subagents.map((agent) => <div className="agent-row" key={agent.id}><span className="agent-dot" data-status={agent.status} /><div><strong>{agent.profile}</strong><small>{agent.task || agent.summary || agent.status}</small></div></div>)}</div>}</section>;
}

function nextTodoStatus(status: string) {
  if (status === "pending") return "in_progress";
  if (status === "in_progress") return "completed";
  return "pending";
}
