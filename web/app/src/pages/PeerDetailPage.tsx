import { useEffect, useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
import { getPeer, logout, PeerView, reconcilePeer } from "../api";
import { useAuth } from "../AuthContext";
import { HeaderNavMenu } from "../HeaderNavMenu";
import { HeaderBrand } from "../Brand";
import { ThemeToggleButton } from "../ThemeToggleButton";
import { uiTheme } from "../uiTheme";

export function PeerDetailPage() {
  const { peerId } = useParams<{ peerId: string }>();
  const { session, setSession } = useAuth();
  const navigate = useNavigate();
  const [peer, setPeer] = useState<PeerView | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [reconciling, setReconciling] = useState(false);

  useEffect(() => {
    if (!peerId) {
      setError("peer id is required");
      setLoading(false);
      return;
    }

    let active = true;
    const id = peerId;
    async function load() {
      setLoading(true);
      setError("");
      try {
        const result = await getPeer(id);
        if (!active) return;
        setPeer(result);
      } catch (e: unknown) {
        if (!active) return;
        setError(e instanceof Error ? e.message : "failed to load peer");
      } finally {
        if (active) {
          setLoading(false);
        }
      }
    }

    load();
    return () => {
      active = false;
    };
  }, [peerId]);

  async function handleLogout() {
    await logout();
    setSession(null);
    navigate("/login", { replace: true });
  }

  async function handleReconcile() {
    if (!peerId) return;
    setReconciling(true);
    setError("");
    try {
      const updated = await reconcilePeer(peerId);
      setPeer(updated);
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : "failed to reconcile peer");
    } finally {
      setReconciling(false);
    }
  }

  const canReconcile = session?.role === "admin" || session?.role === "operator";

  return (
    <div style={s.shell}>
      <header style={s.header}>
        <div style={s.headerLeft}>
          <HeaderBrand />
          <HeaderNavMenu current="peers" role={session?.role} />
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
            <Link to="/peers" style={s.backLink}>Back to peers</Link>
            <h2 style={s.heading}>Peer Detail</h2>
            <p style={s.subtitle}>Drill-down for one peer intent, runtime state, and reconcile action.</p>
          </div>
          {canReconcile && (
            <button onClick={handleReconcile} disabled={reconciling || !peer || peer.status === "disabled" || peer.status === "revoked"} style={s.reconcileBtn}>
              {reconciling ? "Reconciling..." : "Reconcile"}
            </button>
          )}
        </div>

        {loading ? (
          <section style={s.card}><p style={s.muted}>Loading peer...</p></section>
        ) : error ? (
          <section style={s.card}><p style={s.error}>{error}</p></section>
        ) : !peer ? (
          <section style={s.card}><p style={s.muted}>Peer not found.</p></section>
        ) : (
          <div style={s.grid}>
            <section style={s.card}>
              <h3 style={s.cardTitle}>Identity</h3>
              <div style={s.detailRow}><span style={s.label}>Peer ID</span><span style={s.value}>{peer.id}</span></div>
              <div style={s.detailRow}><span style={s.label}>Agent ID</span><span style={s.value}>{peer.agent_id}</span></div>
              <div style={s.detailRow}><span style={s.label}>Hostname</span><span style={s.value}>{peer.hostname ?? "unknown"}</span></div>
              <div style={s.detailRow}><span style={s.label}>Address</span><span style={s.value}>{peer.assigned_address ?? "none"}</span></div>
              <div style={s.detailRow}><span style={s.label}>Public key</span><span style={s.smallValue}>{peer.public_key}</span></div>
            </section>

            <section style={s.card}>
              <h3 style={s.cardTitle}>Intent</h3>
              <div style={s.detailRow}><span style={s.label}>Status</span><span style={{ ...s.badge, background: peerStatusColor(peer.status) }}>{peer.status}</span></div>
              <div style={s.stack}>
                {peer.allowed_ips.length > 0 ? peer.allowed_ips.map((cidr) => (
                  <span key={cidr} style={s.pill}>{cidr}</span>
                )) : <span style={s.muted}>No intended allowed IPs</span>}
              </div>
            </section>

            <section style={s.card}>
              <h3 style={s.cardTitle}>Runtime</h3>
              <div style={s.detailRow}><span style={s.label}>Drift</span><span style={{ ...s.badge, background: driftColor(peer.drift) }}>{peer.drift}</span></div>
              <div style={s.stack}>
                {(peer.runtime_allowed_ips ?? []).length > 0 ? (peer.runtime_allowed_ips ?? []).map((cidr) => (
                  <span key={cidr} style={s.pill}>{cidr}</span>
                )) : <span style={s.muted}>No runtime peer state</span>}
              </div>
            </section>
          </div>
        )}
      </main>
    </div>
  );
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

function driftColor(drift: string): string {
  switch (drift) {
    case "in_sync":
      return "#27ae60";
    case "missing_runtime":
      return "#d35400";
    case "allowed_ips_mismatch":
      return "#c0392b";
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
  backLink: { display: "inline-block", color: "#2980b9", textDecoration: "none", fontSize: "0.85rem", marginBottom: "0.65rem" },
  heading: { fontSize: "1.4rem", fontWeight: 600, color: uiTheme.text, margin: 0 },
  subtitle: { margin: "0.35rem 0 0", color: uiTheme.textMuted, fontSize: "0.95rem" },
  grid: { display: "grid", gridTemplateColumns: "repeat(auto-fit, minmax(260px, 1fr))", gap: "1rem" },
  card: { background: uiTheme.surface, borderRadius: 8, padding: "1.25rem", boxShadow: uiTheme.shadow },
  cardTitle: { margin: "0 0 1rem", fontSize: "1rem", color: uiTheme.text },
  detailRow: { display: "flex", justifyContent: "space-between", gap: "1rem", padding: "0.45rem 0", borderBottom: `1px solid ${uiTheme.borderTable}` },
  label: { color: uiTheme.textMuted, fontSize: "0.85rem" },
  value: { color: uiTheme.text, fontSize: "0.9rem", textAlign: "right" },
  smallValue: { color: uiTheme.text, fontSize: "0.8rem", textAlign: "right", wordBreak: "break-all" },
  stack: { display: "flex", flexWrap: "wrap", gap: "0.5rem", marginTop: "0.75rem" },
  pill: { background: uiTheme.surfaceAlt, color: uiTheme.textSoft, padding: "0.35rem 0.6rem", borderRadius: 999, fontSize: "0.8rem" },
  badge: { display: "inline-block", color: "#fff", padding: "2px 8px", borderRadius: 999, fontSize: "0.78rem", lineHeight: 1.5 },
  reconcileBtn: { background: uiTheme.headerBg, color: uiTheme.headerText, border: "none", padding: "0.65rem 1rem", borderRadius: 6, cursor: "pointer", fontSize: "0.9rem" },
  muted: { color: uiTheme.textMuted, margin: 0 },
  error: { color: "#c0392b", margin: 0 },
};
