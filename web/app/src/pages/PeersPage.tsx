import { useEffect, useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import { PeerView, listPeers, logout, reconcilePeer } from "../api";
import { useAuth } from "../AuthContext";
import { HeaderNavMenu } from "../HeaderNavMenu";
import { HeaderBrand } from "../Brand";
import { ThemeToggleButton } from "../ThemeToggleButton";
import { uiTheme } from "../uiTheme";
import { useStoredState } from "../useStoredState";

interface BulkSummary {
  attempted: number;
  succeeded: number;
  failed: number;
  failedPeerIDs: string[];
  messages: string[];
}

export function PeersPage() {
  const { session, setSession } = useAuth();
  const navigate = useNavigate();
  const [peers, setPeers] = useState<PeerView[]>([]);
  const [nextCursor, setNextCursor] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [loadingMore, setLoadingMore] = useState(false);
  const [error, setError] = useState("");
  const [reconcilingPeerID, setReconcilingPeerID] = useState<string | null>(null);
  const [lastUpdatedAt, setLastUpdatedAt] = useState("");
  const [query, setQuery] = useStoredState("peers:query", "");
  const [statusFilter, setStatusFilter] = useStoredState("peers:status-filter", "");
  const [agentIDFilter, setAgentIDFilter] = useStoredState("peers:agent-id-filter", "");
  const [refreshIntervalMs, setRefreshIntervalMs] = useStoredState("peers:refresh-interval-ms", 30_000);
  const [selectedPeerIDs, setSelectedPeerIDs] = useState<string[]>([]);
  const [bulkReconciling, setBulkReconciling] = useState(false);
  const [bulkSummary, setBulkSummary] = useState<BulkSummary | null>(null);

  useEffect(() => {
    let active = true;

    async function load() {
      setLoading(true);
      setError("");
      try {
        const inventory = await listPeers({
          q: query.trim(),
          status: statusFilter,
          agent_id: agentIDFilter.trim(),
          page_size: 50,
        });
        if (!active) return;
        setPeers(inventory.peers);
        setNextCursor(inventory.next_cursor ?? null);
        setSelectedPeerIDs([]);
        setBulkSummary(null);
      } catch (e: unknown) {
        if (!active) return;
        setError(e instanceof Error ? e.message : "failed to load peers");
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
  }, [query, statusFilter, agentIDFilter, refreshIntervalMs]);

  async function handleLoadMore() {
    if (!nextCursor) {
      return;
    }
    setLoadingMore(true);
    setError("");
    try {
      const page = await listPeers({
        q: query.trim(),
        status: statusFilter,
        agent_id: agentIDFilter.trim(),
        page_size: 50,
        cursor: nextCursor,
      });
      setPeers((current) => {
        const seen = new Set(current.map((peer) => peer.id));
        const appended = page.peers.filter((peer) => !seen.has(peer.id));
        return [...current, ...appended];
      });
      setNextCursor(page.next_cursor ?? null);
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : "failed to load more peers");
    } finally {
      setLoadingMore(false);
    }
  }

  async function handleLogout() {
    await logout();
    setSession(null);
    navigate("/login", { replace: true });
  }

  async function handleReconcile(peerID: string) {
    setReconcilingPeerID(peerID);
    setError("");
    setBulkSummary(null);
    try {
      const updated = await reconcilePeer(peerID);
      setPeers((current) => current.map((peer) => (peer.id === peerID ? updated : peer)));
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : "failed to reconcile peer");
    } finally {
      setReconcilingPeerID(null);
    }
  }

  const canReconcile = session?.role === "admin" || session?.role === "operator";
  const selectedPeers = peers.filter((peer) => selectedPeerIDs.includes(peer.id));

  function togglePeerSelection(peerID: string) {
    setSelectedPeerIDs((current) =>
      current.includes(peerID) ? current.filter((id) => id !== peerID) : [...current, peerID],
    );
  }

  async function handleBulkReconcile() {
    if (!canReconcile || selectedPeerIDs.length === 0) return;
    setBulkReconciling(true);
    setError("");
    setBulkSummary(null);
    try {
      const results = await Promise.allSettled(
        selectedPeerIDs.map(async (peerID) => {
          try {
            const peer = await reconcilePeer(peerID);
            return { peerID, peer };
          } catch (error) {
            throw { peerID, error };
          }
        }),
      );
      const updates = new Map<string, PeerView>();
      const failures: string[] = [];
      const failedPeerIDs: string[] = [];

      for (const result of results) {
        if (result.status === "fulfilled") {
          updates.set(result.value.peer.id, result.value.peer);
        } else {
          const reason = result.reason as { peerID?: string; error?: unknown };
          if (typeof reason?.peerID === "string") {
            failedPeerIDs.push(reason.peerID);
          }
          failures.push(reason?.error instanceof Error ? reason.error.message : "reconcile failed");
        }
      }

      setPeers((current) => current.map((peer) => updates.get(peer.id) ?? peer));
      setBulkSummary({
        attempted: selectedPeerIDs.length,
        succeeded: updates.size,
        failed: failures.length,
        failedPeerIDs,
        messages: failures,
      });
      if (failures.length === 0) {
        setSelectedPeerIDs([]);
      } else {
        setSelectedPeerIDs(failedPeerIDs);
      }
    } finally {
      setBulkReconciling(false);
    }
  }

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
            <h2 style={s.heading}>Peers</h2>
            <p style={s.subtitle}>Policy-rendered peer intent, observed runtime state, and manual reconcile control.</p>
            <p style={s.meta}>Auto-refresh {refreshLabel(refreshIntervalMs)}. Last updated {formatTime(lastUpdatedAt)}.</p>
          </div>
          <select value={String(refreshIntervalMs)} onChange={(e) => setRefreshIntervalMs(Number(e.target.value))} style={s.select}>
            <option value="0">Manual only</option>
            <option value="15000">15s</option>
            <option value="30000">30s</option>
            <option value="60000">60s</option>
          </select>
        </div>

        <section style={s.filtersCard}>
          <input
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="Search hostname or peer ID"
            style={s.input}
          />
          <input
            value={agentIDFilter}
            onChange={(e) => setAgentIDFilter(e.target.value)}
            placeholder="Filter by agent ID"
            style={s.input}
          />
          <select value={statusFilter} onChange={(e) => setStatusFilter(e.target.value)} style={s.select}>
            <option value="">All peer statuses</option>
            <option value="planned">planned</option>
            <option value="active">active</option>
            <option value="rotation_pending">rotation_pending</option>
            <option value="disabled">disabled</option>
            <option value="revoked">revoked</option>
          </select>
          <button
            onClick={() => {
              setQuery("");
              setStatusFilter("");
              setAgentIDFilter("");
            }}
            style={s.clearBtn}
          >
            Clear
          </button>
        </section>

        {canReconcile && (
          <section style={s.bulkCard}>
            <div style={s.bulkMeta}>
              <strong>{selectedPeerIDs.length}</strong> selected
              {selectedPeers.length > 0 && (
                <span style={s.bulkHint}>
                  drifted {selectedPeers.filter((peer) => peer.drift !== "in_sync").length}
                </span>
              )}
            </div>
            <div style={s.bulkActions}>
              <button onClick={() => setSelectedPeerIDs(peers.map((peer) => peer.id))} style={s.bulkBtn}>Select all</button>
              <button onClick={() => setSelectedPeerIDs(peers.filter((peer) => peer.drift !== "in_sync").map((peer) => peer.id))} style={s.bulkBtn}>Select drifted</button>
              <button onClick={() => setSelectedPeerIDs([])} style={s.bulkBtn}>Clear selection</button>
              <button onClick={handleBulkReconcile} disabled={selectedPeerIDs.length === 0 || bulkReconciling} style={s.reconcileBtn}>
                {bulkReconciling ? "Reconciling..." : "Reconcile selected"}
              </button>
            </div>
            {bulkSummary && (
              <div
                style={{
                  ...s.bulkSummary,
                  borderColor: bulkSummary.failed > 0 ? "#f0b4af" : "#b7e4c7",
                  background: bulkSummary.failed > 0 ? "#fff7f6" : uiTheme.surfaceAlt,
                }}
              >
                <strong>
                  Attempted {bulkSummary.attempted}, succeeded {bulkSummary.succeeded}, failed {bulkSummary.failed}
                </strong>
                {bulkSummary.failedPeerIDs.length > 0 && (
                  <div style={s.bulkSummaryText}>Still selected: {bulkSummary.failedPeerIDs.join(", ")}</div>
                )}
                {bulkSummary.messages.length > 0 && (
                  <div style={s.bulkSummaryText}>{bulkSummary.messages.slice(0, 3).join(" | ")}</div>
                )}
              </div>
            )}
          </section>
        )}
        {nextCursor && !loading && !error && <p style={s.meta}>Showing the newest 50 peers first. Load older peer intent as needed.</p>}

        <section style={s.card}>
          {loading ? (
            <p style={s.muted}>Loading peers...</p>
          ) : error ? (
            <p style={s.error}>{error}</p>
          ) : peers.length === 0 ? (
            <p style={s.muted}>No peers found.</p>
          ) : (
            <div style={s.tableWrap}>
              <table style={s.table}>
                <thead>
                  <tr>
                    {canReconcile && <th style={s.th}></th>}
                    <th style={s.th}>Peer</th>
                    <th style={s.th}>Intent</th>
                    <th style={s.th}>Runtime</th>
                    <th style={s.th}>Drift</th>
                    <th style={s.th}>Action</th>
                  </tr>
                </thead>
                <tbody>
                  {peers.map((peer) => (
                    <tr key={peer.id} style={s.tr}>
                      {canReconcile && (
                        <td style={s.td}>
                          <input
                            type="checkbox"
                            checked={selectedPeerIDs.includes(peer.id)}
                            onChange={() => togglePeerSelection(peer.id)}
                          />
                        </td>
                      )}
                      <td style={s.td}>
                        <Link to={`/peers/${peer.id}`} style={s.primaryLink}>{peer.hostname ?? "unknown host"}</Link>
                        <div style={s.secondaryCell}>{peer.id}</div>
                        <div style={s.secondaryCell}>{peer.assigned_address ?? "no address"}</div>
                      </td>
                      <td style={s.td}>
                        <span style={{ ...s.badge, background: peerStatusColor(peer.status) }}>{peer.status}</span>
                        <div style={s.metaBlock}>
                          <div>{peer.allowed_ips.join(", ") || "no allowed IPs"}</div>
                          <div>agent {peer.agent_id}</div>
                        </div>
                      </td>
                      <td style={s.td}>
                        <div style={s.metaBlock}>
                          <div>{(peer.runtime_allowed_ips ?? []).join(", ") || "no runtime peer"}</div>
                          <div>pub {shortKey(peer.public_key)}</div>
                        </div>
                      </td>
                      <td style={s.td}>
                        <span style={{ ...s.badge, background: driftColor(peer.drift) }}>{peer.drift}</span>
                      </td>
                      <td style={s.td}>
                        {canReconcile ? (
                          <button
                            onClick={() => handleReconcile(peer.id)}
                            disabled={reconcilingPeerID === peer.id || peer.status === "disabled" || peer.status === "revoked"}
                            style={s.reconcileBtn}
                          >
                            {reconcilingPeerID === peer.id ? "Reconciling..." : "Reconcile"}
                          </button>
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
              <button onClick={handleLoadMore} disabled={loadingMore} style={s.bulkBtn}>
                {loadingMore ? "Loading..." : "Load older peers"}
              </button>
            </div>
          )}
        </section>
      </main>
    </div>
  );
}

function shortKey(value: string): string {
  return value.length <= 14 ? value : `${value.slice(0, 14)}...`;
}

function formatTime(value?: string): string {
  if (!value) return "never";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleTimeString();
}

function refreshLabel(value: number): string {
  if (value <= 0) return "manual only";
  return `every ${Math.round(value / 1000)}s`;
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
  heading: { fontSize: "1.4rem", fontWeight: 600, color: uiTheme.text, margin: 0 },
  subtitle: { margin: "0.35rem 0 0", color: uiTheme.textMuted, fontSize: "0.95rem" },
  filtersCard: { background: uiTheme.surface, borderRadius: 8, padding: "1rem 1.25rem", boxShadow: uiTheme.shadow, marginBottom: "1rem", display: "flex", flexWrap: "wrap", gap: "0.75rem" },
  bulkCard: { background: uiTheme.surface, borderRadius: 8, padding: "1rem 1.25rem", boxShadow: uiTheme.shadow, marginBottom: "1rem", display: "flex", alignItems: "center", justifyContent: "space-between", gap: "1rem", flexWrap: "wrap" },
  bulkMeta: { display: "flex", alignItems: "center", gap: "0.75rem", color: uiTheme.text, fontSize: "0.9rem" },
  bulkHint: { color: uiTheme.textMuted, fontSize: "0.85rem" },
  bulkActions: { display: "flex", gap: "0.6rem", flexWrap: "wrap" },
  bulkSummary: { width: "100%", marginTop: "0.85rem", border: "1px solid", borderRadius: 10, padding: "0.85rem 1rem" },
  bulkSummaryText: { marginTop: "0.35rem", color: uiTheme.textMuted, fontSize: "0.88rem" },
  bulkBtn: { background: "transparent", color: uiTheme.text, border: `1px solid ${uiTheme.border}`, padding: "0.55rem 0.85rem", borderRadius: 6, cursor: "pointer", fontSize: "0.85rem" },
  card: { background: uiTheme.surface, borderRadius: 8, padding: "1.25rem", boxShadow: uiTheme.shadow },
  input: { flex: "1 1 240px", minWidth: 220, padding: "0.65rem 0.8rem", borderRadius: 6, border: `1px solid ${uiTheme.border}`, fontSize: "0.9rem", background: uiTheme.inputBg, color: uiTheme.inputText },
  select: { padding: "0.65rem 0.8rem", borderRadius: 6, border: `1px solid ${uiTheme.border}`, fontSize: "0.9rem", background: uiTheme.inputBg, color: uiTheme.inputText },
  clearBtn: { background: "transparent", color: uiTheme.text, border: `1px solid ${uiTheme.border}`, padding: "0.6rem 0.9rem", borderRadius: 6, cursor: "pointer", fontSize: "0.9rem" },
  tableWrap: { overflowX: "auto" },
  loadMoreRow: { display: "flex", justifyContent: "center", marginTop: "1rem" },
  table: { width: "100%", borderCollapse: "collapse" },
  th: { textAlign: "left", fontSize: "0.78rem", color: uiTheme.textMuted, textTransform: "uppercase", padding: "0.65rem 0.75rem", borderBottom: `1px solid ${uiTheme.borderTableStrong}`, letterSpacing: 0.5 },
  tr: { borderBottom: `1px solid ${uiTheme.borderTable}` },
  td: { padding: "0.9rem 0.75rem", fontSize: "0.9rem", color: uiTheme.text, verticalAlign: "top" },
  primaryCell: { fontWeight: 600, color: uiTheme.text, marginBottom: "0.2rem" },
  primaryLink: { display: "inline-block", fontWeight: 600, color: uiTheme.text, textDecoration: "none", marginBottom: "0.2rem" },
  secondaryCell: { fontSize: "0.8rem", color: uiTheme.textMuted, marginTop: "0.15rem" },
  badge: { display: "inline-block", color: "#fff", padding: "2px 8px", borderRadius: 999, fontSize: "0.78rem", lineHeight: 1.5 },
  metaBlock: { display: "grid", gap: "0.2rem", color: uiTheme.textMuted, fontSize: "0.8rem" },
  meta: { margin: "0.35rem 0 0", color: uiTheme.textMuted, fontSize: "0.8rem" },
  reconcileBtn: { background: uiTheme.headerBg, color: uiTheme.headerText, border: "none", padding: "0.55rem 0.85rem", borderRadius: 6, cursor: "pointer", fontSize: "0.85rem" },
  muted: { color: uiTheme.textMuted, margin: 0 },
  mutedSmall: { color: uiTheme.textMuted, fontSize: "0.8rem" },
  error: { color: "#c0392b", margin: 0 },
};
