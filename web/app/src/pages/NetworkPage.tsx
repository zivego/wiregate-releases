import { useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { HeaderBrand } from "../Brand";
import { useAuth } from "../AuthContext";
import { HeaderNavMenu } from "../HeaderNavMenu";
import { DiagnosticsSnapshot, getNetworkDiagnostics, logout } from "../api";
import { ThemeToggleButton } from "../ThemeToggleButton";
import { uiTheme } from "../uiTheme";

export function NetworkPage() {
  const { session, setSession } = useAuth();
  const navigate = useNavigate();
  const [snapshot, setSnapshot] = useState<DiagnosticsSnapshot | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  useEffect(() => {
    let active = true;

    async function load() {
      setLoading(true);
      setError("");
      try {
        const next = await getNetworkDiagnostics();
        if (!active) return;
        setSnapshot(next);
      } catch (err: unknown) {
        if (!active) return;
        setError(err instanceof Error ? err.message : "failed to load network diagnostics");
      } finally {
        if (active) {
          setLoading(false);
        }
      }
    }

    void load();
    return () => {
      active = false;
    };
  }, []);

  async function handleLogout() {
    await logout();
    setSession(null);
    navigate("/login", { replace: true });
  }

  return (
    <div style={s.shell}>
      <header style={s.header}>
        <div style={s.headerLeft}>
          <HeaderBrand />
          <HeaderNavMenu current="network" role={session?.role} />
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
        <section style={s.hero}>
          <div>
            <h2 style={s.heading}>Network Diagnostics</h2>
            <p style={s.subtitle}>
              Inspect gateway assignments, direct versus relay path mode, and route conflicts from the current control-plane state.
            </p>
            <p style={s.meta}>Generated {formatDate(snapshot?.generated_at)}.</p>
          </div>
          <div style={s.stats}>
            <div style={s.statCard}>
              <div style={s.statLabel}>Relay</div>
              <div style={s.statValue}>{snapshot?.relay_status ?? "unknown"}</div>
            </div>
            <div style={s.statCard}>
              <div style={s.statLabel}>Gateway Agents</div>
              <div style={s.statValue}>{snapshot?.summary.gateway_agents ?? 0}</div>
            </div>
            <div style={s.statCard}>
              <div style={s.statLabel}>Conflicts</div>
              <div style={s.statValue}>{snapshot?.summary.conflict_count ?? 0}</div>
            </div>
          </div>
        </section>

        {error && <div style={s.error}>{error}</div>}

        <section style={s.card}>
          {loading ? (
            <p style={s.muted}>Loading network diagnostics...</p>
          ) : !snapshot ? (
            <p style={s.muted}>Diagnostics are currently unavailable.</p>
          ) : (
            <>
              <div style={s.summaryGrid}>
                <div style={s.summaryCard}>
                  <div style={s.summaryLabel}>Total Agents</div>
                  <div style={s.summaryValue}>{snapshot.summary.total_agents}</div>
                </div>
                <div style={s.summaryCard}>
                  <div style={s.summaryLabel}>Direct Paths</div>
                  <div style={s.summaryValue}>{snapshot.summary.direct_agents}</div>
                </div>
                <div style={s.summaryCard}>
                  <div style={s.summaryLabel}>Relay Paths</div>
                  <div style={s.summaryValue}>{snapshot.summary.relay_agents}</div>
                </div>
              </div>

              <div style={s.tableWrap}>
                <table style={s.table}>
                  <thead>
                    <tr>
                      <th style={s.th}>Agent</th>
                      <th style={s.th}>Profile</th>
                      <th style={s.th}>Gateway</th>
                      <th style={s.th}>Path</th>
                      <th style={s.th}>Destinations</th>
                      <th style={s.th}>Conflicts</th>
                    </tr>
                  </thead>
                  <tbody>
                    {snapshot.agents.map((agent) => (
                      <tr key={agent.agent_id} style={s.tr}>
                        <td style={s.td}>
                          <div style={s.primaryCell}>{agent.hostname}</div>
                          <div style={s.secondaryCell}>{agent.platform} · {agent.agent_status}</div>
                        </td>
                        <td style={s.td}>
                          <span style={agent.route_profile === "egress_gateway" || agent.route_profile === "subnet_access" ? s.modeGateway : agent.route_profile === "full_tunnel" ? s.modeFull : s.modeStandard}>
                            {agent.route_profile}
                          </span>
                          <div style={s.secondaryCell}>traffic {agent.traffic_mode}</div>
                        </td>
                        <td style={s.td}>
                          <div>{agent.gateway_mode}</div>
                          <div style={s.secondaryCell}>{agent.gateway_assignment_status}</div>
                        </td>
                        <td style={s.td}>
                          <div>{agent.path_mode}</div>
                          <div style={s.secondaryCell}>{agent.peer_status || "no peer"}</div>
                        </td>
                        <td style={s.td}>{(agent.allowed_destinations ?? []).join(", ") || "none"}</td>
                        <td style={s.td}>
                          {(agent.route_conflicts ?? []).length === 0 ? "none" : (agent.route_conflicts ?? []).join("; ")}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </>
          )}
        </section>
      </main>
    </div>
  );
}

function formatDate(value?: string): string {
  if (!value) return "never";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString();
}

const s: Record<string, React.CSSProperties> = {
  shell: { minHeight: "100vh", background: uiTheme.pageBg, fontFamily: "system-ui, sans-serif" },
  header: { background: uiTheme.headerBg, color: uiTheme.headerText, padding: "0 2rem", height: 56, display: "flex", alignItems: "center", justifyContent: "space-between" },
  headerLeft: { display: "flex", alignItems: "center", gap: "2rem" },
  headerRight: { display: "flex", alignItems: "center", gap: "1rem" },
  userInfo: { fontSize: "0.9rem", color: uiTheme.headerLink },
  roleBadge: { background: uiTheme.headerChipBg, padding: "2px 8px", borderRadius: 4, fontSize: "0.8rem", color: uiTheme.headerChipText },
  logoutBtn: { background: "transparent", border: `1px solid ${uiTheme.border}`, color: uiTheme.headerText, padding: "5px 14px", borderRadius: 5, cursor: "pointer", fontSize: "0.875rem" },
  page: { padding: "2rem", maxWidth: 1240, margin: "0 auto", display: "grid", gap: "1rem" },
  hero: { display: "flex", justifyContent: "space-between", gap: "1rem", flexWrap: "wrap" },
  heading: { margin: 0, color: uiTheme.text, fontSize: "1.5rem" },
  subtitle: { margin: "0.45rem 0 0", color: uiTheme.textMuted, maxWidth: 760, lineHeight: 1.5 },
  meta: { margin: "0.4rem 0 0", color: uiTheme.textMuted, fontSize: "0.85rem" },
  stats: { display: "grid", gridTemplateColumns: "repeat(3, minmax(120px, 1fr))", gap: "0.75rem" },
  statCard: { background: uiTheme.surface, border: `1px solid ${uiTheme.border}`, borderRadius: 12, padding: "0.9rem 1rem", minWidth: 140 },
  statLabel: { fontSize: "0.72rem", letterSpacing: 0.4, textTransform: "uppercase", color: uiTheme.textMuted },
  statValue: { marginTop: 6, fontSize: "1.2rem", fontWeight: 700, color: uiTheme.text },
  error: { background: "#fef2f2", border: "1px solid #fecaca", color: "#991b1b", borderRadius: 12, padding: "0.85rem 1rem" },
  card: { background: uiTheme.surface, borderRadius: 12, padding: "1rem", boxShadow: uiTheme.shadow, display: "grid", gap: "1rem" },
  muted: { margin: 0, color: uiTheme.textMuted },
  summaryGrid: { display: "grid", gridTemplateColumns: "repeat(3, minmax(140px, 1fr))", gap: "0.75rem" },
  summaryCard: { border: `1px solid ${uiTheme.border}`, borderRadius: 12, padding: "0.9rem", background: uiTheme.surfaceAlt },
  summaryLabel: { fontSize: "0.8rem", color: uiTheme.textMuted },
  summaryValue: { marginTop: 6, fontSize: "1.3rem", fontWeight: 700, color: uiTheme.text },
  tableWrap: { overflowX: "auto" },
  table: { width: "100%", borderCollapse: "collapse" },
  th: { textAlign: "left", fontSize: "0.78rem", color: uiTheme.textMuted, textTransform: "uppercase", padding: "0.65rem 0.75rem", borderBottom: `1px solid ${uiTheme.borderTableStrong}`, letterSpacing: 0.5 },
  tr: { borderBottom: `1px solid ${uiTheme.borderTable}` },
  td: { padding: "0.9rem 0.75rem", fontSize: "0.9rem", color: uiTheme.text, verticalAlign: "top" },
  primaryCell: { fontWeight: 600, color: uiTheme.text, marginBottom: "0.2rem" },
  secondaryCell: { fontSize: "0.8rem", color: uiTheme.textMuted, marginTop: "0.2rem" },
  modeStandard: { display: "inline-block", background: "#e0f2fe", color: "#075985", borderRadius: 999, padding: "2px 8px", fontSize: "0.78rem", fontWeight: 600 },
  modeFull: { display: "inline-block", background: "#fff7ed", color: "#9a3412", borderRadius: 999, padding: "2px 8px", fontSize: "0.78rem", fontWeight: 600 },
  modeGateway: { display: "inline-block", background: "#ecfdf5", color: "#166534", borderRadius: 999, padding: "2px 8px", fontSize: "0.78rem", fontWeight: 600 },
};
