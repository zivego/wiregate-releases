import { useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { AgentInventory, AgentStateAction, EnrollmentToken, GatewayMode, TrafficModeOverride, isPendingSecurityApprovalResponse, listAgents, logout, patchAgentState, patchAgentTrafficMode, postAgentGatewayMode, reissueAgentEnrollment, rotateAgent } from "../api";
import { useAuth } from "../AuthContext";
import { HeaderNavMenu } from "../HeaderNavMenu";
import { HeaderBrand } from "../Brand";
import { ThemeToggleButton } from "../ThemeToggleButton";
import { uiTheme } from "../uiTheme";
import { useStoredState } from "../useStoredState";
import { copyText } from "../clipboard";
import { linuxInstallCommand, windowsInstallCommand } from "../bootstrapCommands";

export function AgentsPage() {
  const { session, setSession } = useAuth();
  const navigate = useNavigate();
  const [agents, setAgents] = useState<AgentInventory[]>([]);
  const [nextCursor, setNextCursor] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [loadingMore, setLoadingMore] = useState(false);
  const [error, setError] = useState("");
  const [query, setQuery] = useStoredState("agents:query", "");
  const [statusFilter, setStatusFilter] = useStoredState("agents:status-filter", "");
  const [platformFilter, setPlatformFilter] = useStoredState("agents:platform-filter", "");
  const [lastUpdatedAt, setLastUpdatedAt] = useState("");
  const [refreshIntervalMs, setRefreshIntervalMs] = useStoredState("agents:refresh-interval-ms", 30_000);
  const [actingAgentID, setActingAgentID] = useState<string | null>(null);
  const [copyMessage, setCopyMessage] = useState("");
  const [notice, setNotice] = useState("");
  const [reissuedToken, setReissuedToken] = useState<EnrollmentToken | null>(null);
  const [serverURL, setServerURL] = useStoredState("agents:server-url", typeof window === "undefined" ? "http://localhost:8080" : window.location.origin);
  const [linuxHostname, setLinuxHostname] = useStoredState("agents:linux-hostname", "");
  const [linuxBinaryPath, setLinuxBinaryPath] = useStoredState("agents:linux-binary-path", "/usr/local/bin/wiregate-agent-linux");
  const [windowsHostname, setWindowsHostname] = useStoredState("agents:windows-hostname", "");
  const [windowsBinaryPath, setWindowsBinaryPath] = useStoredState("agents:windows-binary-path", String.raw`C:\Program Files\Wiregate\wiregate-agent-windows.exe`);
  const canManage = session?.role === "admin" || session?.role === "operator";

  useEffect(() => {
    let active = true;

    async function load() {
      setLoading(true);
      setError("");
      try {
        const inventory = await listAgents({
          q: query.trim(),
          status: statusFilter,
          platform: platformFilter,
          page_size: 50,
        });
        if (!active) return;
        setAgents(inventory.agents);
        setNextCursor(inventory.next_cursor ?? null);
      } catch (e: unknown) {
        if (!active) return;
        setError(e instanceof Error ? e.message : "failed to load agents");
      } finally {
        if (active) {
          setLoading(false);
          setLastUpdatedAt(new Date().toISOString());
        }
      }
    }

    load();
    const timer = refreshIntervalMs > 0 ? window.setInterval(load, refreshIntervalMs) : null;

    return () => {
      active = false;
      if (timer !== null) {
        window.clearInterval(timer);
      }
    };
  }, [query, statusFilter, platformFilter, refreshIntervalMs]);

  async function handleLoadMore() {
    if (!nextCursor) {
      return;
    }
    setLoadingMore(true);
    setError("");
    try {
      const page = await listAgents({
        q: query.trim(),
        status: statusFilter,
        platform: platformFilter,
        page_size: 50,
        cursor: nextCursor,
      });
      setAgents((current) => {
        const seen = new Set(current.map((agent) => agent.id));
        const appended = page.agents.filter((agent) => !seen.has(agent.id));
        return [...current, ...appended];
      });
      setNextCursor(page.next_cursor ?? null);
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : "failed to load more agents");
    } finally {
      setLoadingMore(false);
    }
  }

  async function handleLogout() {
    await logout();
    setSession(null);
    navigate("/login", { replace: true });
  }

  async function handleCopy(label: string, value: string) {
    try {
      await copyText(value);
      setCopyMessage(`${label} copied`);
    } catch (e: unknown) {
      setCopyMessage(e instanceof Error ? e.message : "failed to copy");
    }
  }

  async function handleStateAction(agentID: string, action: AgentStateAction) {
    setActingAgentID(agentID);
    setError("");
    setCopyMessage("");
    setNotice("");
    try {
      const updated = await patchAgentState(agentID, action);
      if (isPendingSecurityApprovalResponse(updated)) {
        setNotice(`${action} requested. Another admin must approve it before this agent changes.`);
        return;
      }
      setAgents((current) => current.map((agent) => (agent.id === agentID ? updated : agent)));
      if (action === "revoke" && reissuedToken) {
        setReissuedToken(null);
      }
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : `failed to ${action} agent`);
    } finally {
      setActingAgentID(null);
    }
  }

  async function handleRotate(agentID: string) {
    setActingAgentID(agentID);
    setError("");
    setNotice("");
    try {
      const updated = await rotateAgent(agentID);
      if (isPendingSecurityApprovalResponse(updated)) {
        setNotice("Rotation requested. Another admin must approve it before the agent enters rotation.");
        return;
      }
      setAgents((current) => current.map((agent) => (agent.id === agentID ? updated : agent)));
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : "failed to request rotation");
    } finally {
      setActingAgentID(null);
    }
  }

  async function handleReissue(agent: AgentInventory) {
    setActingAgentID(agent.id);
    setError("");
    setCopyMessage("");
    setNotice("");
    try {
      const issued = await reissueAgentEnrollment(agent.id);
      if (isPendingSecurityApprovalResponse(issued)) {
        setReissuedToken(null);
        setNotice("Reissue requested. Another admin must approve it before a new enrollment token is issued.");
        return;
      }
      setReissuedToken(issued);
      setLinuxHostname(agent.hostname);
      setWindowsHostname(agent.hostname);
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : "failed to reissue enrollment");
    } finally {
      setActingAgentID(null);
    }
  }

  async function handleTrafficMode(agentID: string, mode: TrafficModeOverride) {
    setActingAgentID(agentID);
    setError("");
    setCopyMessage("");
    try {
      const updated = await patchAgentTrafficMode(agentID, mode);
      setAgents((current) => current.map((agent) => (agent.id === agentID ? updated : agent)));
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : "failed to update traffic mode");
    } finally {
      setActingAgentID(null);
    }
  }

  async function handleGatewayMode(agentID: string, mode: GatewayMode) {
    setActingAgentID(agentID);
    setError("");
    setCopyMessage("");
    try {
      const updated = await postAgentGatewayMode(agentID, mode);
      setAgents((current) => current.map((agent) => (agent.id === agentID ? updated : agent)));
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : "failed to update gateway mode");
    } finally {
      setActingAgentID(null);
    }
  }

  return (
    <div style={s.shell}>
      <header style={s.header}>
        <div style={s.headerLeft}>
          <HeaderBrand />
          <HeaderNavMenu current="agents" role={session?.role} />
        </div>
        <div style={s.headerRight}>
          <ThemeToggleButton />
          <span style={s.userInfo}>
            {session?.email} <span style={s.roleBadge}>{session?.role}</span>
          </span>
          <button onClick={handleLogout} style={s.logoutBtn}>Logout</button>
        </div>
      </header>

      <main style={s.page}>
        <div style={s.headingRow}>
          <div>
            <h2 style={s.heading}>Agents</h2>
            <p style={s.subtitle}>Inventory, heartbeat, and local runtime/apply status reported by enrolled agents.</p>
            <p style={s.meta}>Auto-refresh {refreshLabel(refreshIntervalMs)}. Last updated {formatDate(lastUpdatedAt, true)}.</p>
          </div>
          <select value={String(refreshIntervalMs)} onChange={(e) => setRefreshIntervalMs(Number(e.target.value))} style={s.select}>
            <option value="0">Manual only</option>
            <option value="15000">15s</option>
            <option value="30000">30s</option>
            <option value="60000">60s</option>
          </select>
        </div>

        {reissuedToken?.token && (
          <section style={s.reissueCard}>
            <div style={s.reissueHeader}>
              <div>
                <div style={s.reissueLabel}>Reissued enrollment token</div>
                <code style={s.reissueValue}>{reissuedToken.token}</code>
              </div>
              <button onClick={() => handleCopy("Raw token", reissuedToken.token ?? "")} style={s.inlineBtn}>Copy token</button>
            </div>
            <div style={s.reissueGrid}>
              <label style={s.field}>
                <span style={s.fieldLabel}>Wiregate server URL or host</span>
                <input value={serverURL} onChange={(e) => setServerURL(e.target.value)} style={s.input} />
              </label>
              <label style={s.field}>
                <span style={s.fieldLabel}>Linux hostname override</span>
                <input value={linuxHostname} onChange={(e) => setLinuxHostname(e.target.value)} placeholder={reissuedToken.bound_identity ?? ""} style={s.input} />
              </label>
              <label style={s.field}>
                <span style={s.fieldLabel}>Linux binary path</span>
                <input value={linuxBinaryPath} onChange={(e) => setLinuxBinaryPath(e.target.value)} style={s.input} />
              </label>
              <label style={s.field}>
                <span style={s.fieldLabel}>Windows hostname override</span>
                <input value={windowsHostname} onChange={(e) => setWindowsHostname(e.target.value)} placeholder={reissuedToken.bound_identity ?? ""} style={s.input} />
              </label>
              <label style={s.field}>
                <span style={s.fieldLabel}>Windows binary path</span>
                <input value={windowsBinaryPath} onChange={(e) => setWindowsBinaryPath(e.target.value)} style={s.input} />
              </label>
            </div>
            <div style={s.commandGrid}>
              <div style={s.commandCard}>
                <div style={s.commandTitle}>Linux install command</div>
                <code style={s.commandText}>{linuxInstallCommand(serverURL, reissuedToken.token, linuxHostname || reissuedToken.bound_identity || "", linuxBinaryPath)}</code>
                <button onClick={() => handleCopy("Linux install command", linuxInstallCommand(serverURL, reissuedToken.token ?? "", linuxHostname || reissuedToken.bound_identity || "", linuxBinaryPath))} style={s.inlineBtn}>Copy Linux command</button>
              </div>
              <div style={s.commandCard}>
                <div style={s.commandTitle}>Windows install command</div>
                <code style={s.commandText}>{windowsInstallCommand(serverURL, reissuedToken.token, windowsHostname || reissuedToken.bound_identity || "", windowsBinaryPath)}</code>
                <button onClick={() => handleCopy("Windows install command", windowsInstallCommand(serverURL, reissuedToken.token ?? "", windowsHostname || reissuedToken.bound_identity || "", windowsBinaryPath))} style={s.inlineBtn}>Copy Windows command</button>
              </div>
            </div>
            {copyMessage && <p style={s.meta}>{copyMessage}</p>}
          </section>
        )}

        <section style={s.filtersCard}>
          <input
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="Search hostname or ID"
            style={s.input}
          />
          <select value={statusFilter} onChange={(e) => setStatusFilter(e.target.value)} style={s.select}>
            <option value="">All agent statuses</option>
            <option value="enrolled">enrolled</option>
            <option value="active">active</option>
            <option value="disabled">disabled</option>
            <option value="revoked">revoked</option>
          </select>
          <select value={platformFilter} onChange={(e) => setPlatformFilter(e.target.value)} style={s.select}>
            <option value="">All platforms</option>
            <option value="linux">linux</option>
            <option value="windows">windows</option>
          </select>
          <button
            onClick={() => {
              setQuery("");
              setStatusFilter("");
              setPlatformFilter("");
            }}
            style={s.clearBtn}
          >
            Clear
          </button>
        </section>

        {notice && !error && <p style={s.meta}>{notice}</p>}
        {nextCursor && !loading && !error && <p style={s.meta}>Showing the newest 50 agents first. Load older inventory on demand.</p>}

        <section style={s.card}>
          {loading ? (
            <p style={s.muted}>Loading agents...</p>
          ) : error ? (
            <p style={s.error}>{error}</p>
          ) : agents.length === 0 ? (
            <p style={s.muted}>No agents enrolled yet.</p>
          ) : (
            <div style={s.tableWrap}>
              <table style={s.table}>
                <thead>
                  <tr>
                    <th style={s.th}>Host</th>
                    <th style={s.th}>Platform</th>
                    <th style={s.th}>Agent</th>
                    <th style={s.th}>Runtime</th>
                    <th style={s.th}>Peer</th>
                    <th style={s.th}>Routing</th>
                    <th style={s.th}>Seen</th>
                    <th style={s.th}>Actions</th>
                  </tr>
                </thead>
                <tbody>
                  {agents.map((agent) => (
                    <tr key={agent.id} style={s.tr}>
                      <td style={s.td}>
                        <div style={s.primaryCell}>{agent.hostname}</div>
                        <div style={s.secondaryCell}>{agent.id}</div>
                      </td>
                      <td style={s.td}>{agent.platform}</td>
                      <td style={s.td}>
                        <span style={{ ...s.badge, background: agentStatusColor(agent.status) }}>{agent.status}</span>
                        <div style={s.metaBlock}>
                          <div>version {agent.reported_version ?? "n/a"}</div>
                          <div>created {formatDate(agent.created_at)}</div>
                        </div>
                      </td>
                      <td style={s.td}>
                        <span style={{ ...s.badge, background: runtimeStatusColor(agent.last_apply_status) }}>
                          {agent.last_apply_status ?? "unknown"}
                        </span>
                        <div style={s.metaBlock}>
                          <div>applied {formatDate(agent.last_applied_at)}</div>
                          <div>fp {shortFingerprint(agent.reported_config_fingerprint)}</div>
                          {agent.last_apply_error && <div style={s.errorText}>{agent.last_apply_error}</div>}
                        </div>
                      </td>
                      <td style={s.td}>
                        {agent.peer ? (
                          <>
                            <span style={{ ...s.badge, background: peerStatusColor(agent.peer.status) }}>{agent.peer.status}</span>
                            <div style={s.metaBlock}>
                              <div>{agent.peer.assigned_address ?? "no address"}</div>
                              <div>{(agent.peer.allowed_ips ?? []).join(", ") || "no allowed IPs"}</div>
                            </div>
                          </>
                        ) : (
                          <span style={s.mutedSmall}>no peer</span>
                        )}
                      </td>
                      <td style={s.td}>
                        <div style={s.metaBlock}>
                          <span style={agent.traffic_mode === "full_tunnel" ? s.modeFull : s.modeStandard}>
                            {agent.traffic_mode === "full_tunnel" ? "full tunnel" : "standard"}
                          </span>
                          {canManage ? (
                            <>
                              <select
                                value={agent.traffic_mode_override ?? "inherit"}
                                onChange={(e) => handleTrafficMode(agent.id, e.target.value as TrafficModeOverride)}
                                disabled={actingAgentID === agent.id || agent.status === "revoked"}
                                style={s.inlineSelect}
                              >
                                <option value="inherit">Inherit from policies</option>
                                <option value="standard">Force standard</option>
                                <option value="full_tunnel">Force full tunnel</option>
                              </select>
                              <select
                                value={agent.gateway_mode}
                                onChange={(e) => handleGatewayMode(agent.id, e.target.value as GatewayMode)}
                                disabled={actingAgentID === agent.id || agent.status === "revoked"}
                                style={s.inlineSelect}
                              >
                                <option value="disabled">Gateway disabled</option>
                                <option value="subnet_access">Subnet gateway</option>
                                <option value="egress_gateway">Egress gateway</option>
                              </select>
                            </>
                          ) : (
                            <span style={s.mutedSmall}>
                              {agent.traffic_mode_override ? `override ${agent.traffic_mode_override}` : "inherited"} · gateway {agent.gateway_mode}
                            </span>
                          )}
                        </div>
                      </td>
                      <td style={s.td}>
                        <span style={{ ...s.onlineDot, background: agent.is_online ? "#27ae60" : "#95a5a6" }} title={agent.is_online ? "Online" : "Offline"} />
                        <span style={{ color: agent.is_online ? "#27ae60" : uiTheme.textMuted, fontWeight: agent.is_online ? 600 : 400, fontSize: "0.82rem" }}>
                          {agent.is_online ? "online" : "offline"}
                        </span>
                        <div style={s.metaBlock}>{formatDate(agent.last_seen_at)}</div>
                      </td>
                      <td style={s.td}>
                        {canManage ? (
                          <div style={s.actions}>
                            {agent.status === "disabled" ? (
                              <button onClick={() => handleStateAction(agent.id, "enable")} disabled={actingAgentID === agent.id} style={s.actionBtn}>
                                {actingAgentID === agent.id ? "Working..." : "Enable"}
                              </button>
                            ) : (
                              <button onClick={() => handleStateAction(agent.id, "disable")} disabled={actingAgentID === agent.id || agent.status === "revoked"} style={s.actionBtn}>
                                {actingAgentID === agent.id ? "Working..." : "Disable"}
                              </button>
                            )}
                            <button onClick={() => handleRotate(agent.id)} disabled={actingAgentID === agent.id || agent.status === "revoked"} style={s.actionBtn}>
                              {actingAgentID === agent.id ? "Working..." : "Rotate key"}
                            </button>
                            <button onClick={() => handleStateAction(agent.id, "revoke")} disabled={actingAgentID === agent.id || agent.status === "revoked"} style={s.dangerBtn}>
                              {actingAgentID === agent.id ? "Working..." : "Revoke"}
                            </button>
                            {agent.status === "revoked" && (
                              <button onClick={() => handleReissue(agent)} disabled={actingAgentID === agent.id} style={s.primaryBtn}>
                                {actingAgentID === agent.id ? "Working..." : "Reissue"}
                              </button>
                            )}
                          </div>
                        ) : (
                          <span style={s.mutedSmall}>read-only</span>
                        )}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
          {nextCursor && !loading && !error && (
            <div style={s.loadMoreRow}>
              <button onClick={handleLoadMore} disabled={loadingMore} style={s.inlineBtn}>
                {loadingMore ? "Loading..." : "Load older agents"}
              </button>
            </div>
          )}
        </section>
      </main>
    </div>
  );
}

function formatDate(value?: string, timeOnly?: boolean): string {
  if (!value) return "never";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return timeOnly ? date.toLocaleTimeString() : date.toLocaleString();
}

function refreshLabel(value: number): string {
  if (value <= 0) return "manual only";
  return `every ${Math.round(value / 1000)}s`;
}

function shortFingerprint(value?: string): string {
  if (!value) return "n/a";
  return `${value.slice(0, 10)}...`;
}

function agentStatusColor(status: string): string {
  switch (status) {
    case "enrolled":
      return "#2980b9";
    case "active":
      return "#27ae60";
    case "disabled":
      return "#d35400";
    case "revoked":
      return "#c0392b";
    default:
      return "#7f8c8d";
  }
}

function peerStatusColor(status: string): string {
  switch (status) {
    case "active":
      return "#27ae60";
    case "planned":
      return "#f39c12";
    case "rotation_pending":
      return "#8e44ad";
    case "disabled":
      return "#d35400";
    case "revoked":
      return "#c0392b";
    default:
      return "#7f8c8d";
  }
}

function runtimeStatusColor(status?: string): string {
  switch (status) {
    case "applied":
      return "#27ae60";
    case "staged":
      return "#8e44ad";
    case "apply_failed":
      return "#c0392b";
    case "drifted":
      return "#d35400";
    case "disabled":
      return "#7f8c8d";
    default:
      return "#7f8c8d";
  }
}

const s: Record<string, React.CSSProperties> = {
  shell: { minHeight: "100vh", background: uiTheme.pageBg, fontFamily: "system-ui, sans-serif" },
  header: { background: uiTheme.headerBg, color: uiTheme.headerText, padding: "0 2rem", height: 56, display: "flex", alignItems: "center", justifyContent: "space-between" },
  headerLeft: { display: "flex", alignItems: "center", gap: "2rem" },
  logo: { fontSize: "1.2rem", fontWeight: 700, letterSpacing: 1 },
  nav: { display: "flex", gap: "1rem" },
  navLink: { color: uiTheme.headerLink, textDecoration: "none", fontSize: "0.9rem" },
  headerRight: { display: "flex", alignItems: "center", gap: "1rem" },
  userInfo: { fontSize: "0.9rem", color: uiTheme.headerLink },
  roleBadge: { background: uiTheme.headerChipBg, padding: "2px 8px", borderRadius: 4, fontSize: "0.8rem", color: uiTheme.headerChipText },
  logoutBtn: { background: "transparent", border: `1px solid ${uiTheme.border}`, color: uiTheme.headerText, padding: "5px 14px", borderRadius: 5, cursor: "pointer", fontSize: "0.875rem" },
  page: { padding: "2rem", maxWidth: 1200, margin: "0 auto" },
  headingRow: { display: "flex", alignItems: "flex-start", justifyContent: "space-between", gap: "1rem", marginBottom: "1.5rem" },
  heading: { fontSize: "1.4rem", fontWeight: 600, color: uiTheme.text, margin: 0 },
  subtitle: { margin: "0.35rem 0 0", color: uiTheme.textMuted, fontSize: "0.95rem" },
  reissueCard: { background: uiTheme.surface, borderRadius: 8, padding: "1rem 1.25rem", boxShadow: uiTheme.shadow, marginBottom: "1rem" },
  reissueHeader: { display: "flex", alignItems: "flex-start", justifyContent: "space-between", gap: "1rem", marginBottom: "1rem" },
  reissueLabel: { fontSize: "0.8rem", color: uiTheme.textMuted, marginBottom: "0.35rem" },
  reissueValue: { display: "block", color: uiTheme.text, wordBreak: "break-all" },
  reissueGrid: { display: "grid", gridTemplateColumns: "repeat(auto-fit, minmax(220px, 1fr))", gap: "0.75rem", marginBottom: "0.9rem" },
  field: { display: "grid", gap: "0.3rem" },
  fieldLabel: { fontSize: "0.8rem", color: uiTheme.textMuted },
  filtersCard: { background: uiTheme.surface, borderRadius: 8, padding: "1rem 1.25rem", boxShadow: uiTheme.shadow, marginBottom: "1rem", display: "flex", flexWrap: "wrap", gap: "0.75rem" },
  card: { background: uiTheme.surface, borderRadius: 8, padding: "1.25rem", boxShadow: uiTheme.shadow },
  input: { flex: "1 1 240px", minWidth: 220, padding: "0.65rem 0.8rem", borderRadius: 6, border: `1px solid ${uiTheme.border}`, fontSize: "0.9rem", background: uiTheme.inputBg, color: uiTheme.inputText },
  select: { padding: "0.65rem 0.8rem", borderRadius: 6, border: `1px solid ${uiTheme.border}`, fontSize: "0.9rem", background: uiTheme.inputBg, color: uiTheme.inputText },
  inlineSelect: { padding: "0.4rem 0.5rem", borderRadius: 6, border: `1px solid ${uiTheme.border}`, fontSize: "0.8rem", background: uiTheme.inputBg, color: uiTheme.inputText },
  clearBtn: { background: "transparent", color: uiTheme.text, border: `1px solid ${uiTheme.border}`, padding: "0.6rem 0.9rem", borderRadius: 6, cursor: "pointer", fontSize: "0.9rem" },
  tableWrap: { overflowX: "auto" },
  loadMoreRow: { display: "flex", justifyContent: "center", marginTop: "1rem" },
  table: { width: "100%", borderCollapse: "collapse" },
  th: { textAlign: "left", fontSize: "0.78rem", color: uiTheme.textMuted, textTransform: "uppercase", padding: "0.65rem 0.75rem", borderBottom: `1px solid ${uiTheme.borderTableStrong}`, letterSpacing: 0.5 },
  tr: { borderBottom: `1px solid ${uiTheme.borderTable}` },
  td: { padding: "0.9rem 0.75rem", fontSize: "0.9rem", color: uiTheme.text, verticalAlign: "top" },
  primaryCell: { fontWeight: 600, color: uiTheme.text, marginBottom: "0.2rem" },
  secondaryCell: { fontSize: "0.8rem", color: uiTheme.textMuted },
  badge: { display: "inline-block", color: "#fff", padding: "2px 8px", borderRadius: 999, fontSize: "0.78rem", lineHeight: 1.5 },
  modeStandard: { display: "inline-block", width: "fit-content", background: "#e0f2fe", color: "#075985", borderRadius: 999, padding: "2px 8px", fontSize: "0.78rem", fontWeight: 600 },
  modeFull: { display: "inline-block", width: "fit-content", background: "#fff7ed", color: "#9a3412", borderRadius: 999, padding: "2px 8px", fontSize: "0.78rem", fontWeight: 600 },
  metaBlock: { marginTop: "0.45rem", display: "grid", gap: "0.2rem", color: uiTheme.textMuted, fontSize: "0.8rem" },
  meta: { margin: "0.35rem 0 0", color: uiTheme.textMuted, fontSize: "0.8rem" },
  actions: { display: "flex", flexWrap: "wrap", gap: "0.45rem" },
  actionBtn: { background: uiTheme.surface, color: uiTheme.text, border: `1px solid ${uiTheme.border}`, padding: "0.45rem 0.7rem", borderRadius: 6, cursor: "pointer", fontSize: "0.8rem" },
  dangerBtn: { background: uiTheme.surface, color: "#b42318", border: "1px solid #f0b4af", padding: "0.45rem 0.7rem", borderRadius: 6, cursor: "pointer", fontSize: "0.8rem" },
  primaryBtn: { background: "#1f6feb", color: "#fff", border: "none", padding: "0.45rem 0.7rem", borderRadius: 6, cursor: "pointer", fontSize: "0.8rem" },
  inlineBtn: { background: "transparent", color: uiTheme.text, border: `1px solid ${uiTheme.border}`, padding: "0.45rem 0.7rem", borderRadius: 6, cursor: "pointer", fontSize: "0.8rem" },
  commandGrid: { display: "grid", gridTemplateColumns: "repeat(auto-fit, minmax(280px, 1fr))", gap: "0.75rem" },
  commandCard: { background: uiTheme.surfaceAlt, borderRadius: 8, padding: "0.9rem" },
  commandTitle: { fontWeight: 600, color: uiTheme.text, marginBottom: "0.45rem" },
  commandText: { display: "block", whiteSpace: "pre-wrap", wordBreak: "break-word", color: uiTheme.text, background: uiTheme.inputBg, borderRadius: 8, padding: "0.75rem", fontSize: "0.82rem", marginBottom: "0.7rem" },
  onlineDot: { display: "inline-block", width: 8, height: 8, borderRadius: "50%", marginRight: 6, verticalAlign: "middle" },
  muted: { color: uiTheme.textMuted, margin: 0 },
  mutedSmall: { color: uiTheme.textMuted, fontSize: "0.8rem" },
  error: { color: "#c0392b", margin: 0 },
  errorText: { color: "#c0392b" },
};
