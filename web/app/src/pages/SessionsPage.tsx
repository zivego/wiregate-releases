import { useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { HeaderBrand } from "../Brand";
import { useAuth } from "../AuthContext";
import { HeaderNavMenu } from "../HeaderNavMenu";
import { SessionInventoryItem, listSessions, logout, revokeSession } from "../api";
import { ThemeToggleButton } from "../ThemeToggleButton";
import { uiTheme } from "../uiTheme";

export function SessionsPage() {
  const { session, setSession } = useAuth();
  const navigate = useNavigate();
  const [sessions, setSessions] = useState<SessionInventoryItem[]>([]);
  const [nextCursor, setNextCursor] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [loadingMore, setLoadingMore] = useState(false);
  const [error, setError] = useState("");
  const [busySessionID, setBusySessionID] = useState("");

  useEffect(() => {
    let active = true;

    async function load() {
      setLoading(true);
      setError("");
      try {
        const inventory = await listSessions();
        if (!active) return;
        setSessions(inventory.sessions);
        setNextCursor(inventory.next_cursor ?? null);
      } catch (err: unknown) {
        if (!active) return;
        setError(err instanceof Error ? err.message : "failed to load sessions");
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
  }, []);

  async function handleLogout() {
    await logout();
    setSession(null);
    navigate("/login", { replace: true });
  }

  async function handleRevoke(item: SessionInventoryItem) {
    setBusySessionID(item.session_id);
    setError("");
    try {
      await revokeSession(item.session_id);
      if (item.current) {
        setSession(null);
        navigate("/login", { replace: true, state: { notice: "Session revoked. Sign in again." } });
        return;
      }
      setSessions((current) => current.filter((entry) => entry.session_id !== item.session_id));
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : "failed to revoke session");
    } finally {
      setBusySessionID("");
    }
  }

  async function handleLoadMore() {
    if (!nextCursor) return;
    setLoadingMore(true);
    setError("");
    try {
      const page = await listSessions(nextCursor);
      setSessions((current) => {
        const seen = new Set(current.map((entry) => entry.session_id));
        return [...current, ...page.sessions.filter((entry) => !seen.has(entry.session_id))];
      });
      setNextCursor(page.next_cursor ?? null);
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : "failed to load more sessions");
    } finally {
      setLoadingMore(false);
    }
  }

  return (
    <div style={s.shell}>
      <header style={s.header}>
        <div style={s.headerLeft}>
          <HeaderBrand />
          <HeaderNavMenu current="sessions" role={session?.role} />
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
            <h2 style={s.heading}>Active Sessions</h2>
            <p style={s.subtitle}>Review current browser and API sessions, inspect device metadata, and revoke any session that should be forced out.</p>
          </div>
          <div style={s.heroStats}>
            <div style={s.statCard}>
              <div style={s.statLabel}>Visible Sessions</div>
              <div style={s.statValue}>{sessions.length}</div>
            </div>
            <div style={s.statCard}>
              <div style={s.statLabel}>SSO Sessions</div>
              <div style={s.statValue}>{sessions.filter((item) => item.auth_provider === "oidc").length}</div>
            </div>
          </div>
        </section>

        {error && <div style={s.error}>{error}</div>}

        <section style={s.card}>
          {loading ? (
            <p style={s.muted}>Loading active sessions...</p>
          ) : sessions.length === 0 ? (
            <p style={s.muted}>No active sessions are visible for the current access scope.</p>
          ) : (
            <div style={s.grid}>
              {sessions.map((item) => (
                <article key={item.session_id} style={item.current ? { ...s.sessionCard, ...s.sessionCardCurrent } : s.sessionCard}>
                  <div style={s.cardTop}>
                    <div>
                      <div style={s.sessionTitle}>
                        {item.current ? "Current session" : item.auth_provider === "oidc" ? "SSO session" : "Local session"}
                      </div>
                      <div style={s.sessionMeta}>{item.email} · {item.role}</div>
                    </div>
                    <span style={item.current ? { ...s.badge, ...s.badgeCurrent } : s.badge}>
                      {item.current ? "Current" : "Active"}
                    </span>
                  </div>

                  <dl style={s.details}>
                    <div style={s.detailRow}>
                      <dt style={s.term}>Auth</dt>
                      <dd style={s.value}>{item.auth_provider}</dd>
                    </div>
                    <div style={s.detailRow}>
                      <dt style={s.term}>IP</dt>
                      <dd style={s.value}>{item.source_ip || "unknown"}</dd>
                    </div>
                    <div style={s.detailRow}>
                      <dt style={s.term}>Last seen</dt>
                      <dd style={s.value}>{formatDateTime(item.last_seen_at)}</dd>
                    </div>
                    <div style={s.detailRow}>
                      <dt style={s.term}>Issued</dt>
                      <dd style={s.value}>{formatDateTime(item.issued_at)}</dd>
                    </div>
                    <div style={s.detailRow}>
                      <dt style={s.term}>Expires</dt>
                      <dd style={s.value}>{formatDateTime(item.expires_at)}</dd>
                    </div>
                    <div style={s.detailColumn}>
                      <dt style={s.term}>User agent</dt>
                      <dd style={s.agentValue}>{item.user_agent || "not captured"}</dd>
                    </div>
                  </dl>

                  <div style={s.actions}>
                    <button
                      type="button"
                      onClick={() => void handleRevoke(item)}
                      disabled={busySessionID === item.session_id}
                      style={item.current ? s.primaryAction : s.secondaryAction}
                    >
                      {busySessionID === item.session_id ? "Revoking..." : item.current ? "Revoke current session" : "Force logout"}
                    </button>
                  </div>
                </article>
              ))}
            </div>
          )}
          {!loading && nextCursor && (
            <div style={s.loadMoreRow}>
              <button type="button" onClick={() => void handleLoadMore()} disabled={loadingMore} style={s.secondaryAction}>
                {loadingMore ? "Loading..." : "Load older sessions"}
              </button>
            </div>
          )}
        </section>
      </main>
    </div>
  );
}

function formatDateTime(value: string): string {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }
  return date.toLocaleString();
}

const s: Record<string, React.CSSProperties> = {
  shell: {
    minHeight: "100vh",
    background: uiTheme.pageBg,
    fontFamily: "system-ui, sans-serif",
  },
  header: {
    background: uiTheme.headerBg,
    color: uiTheme.headerText,
    padding: "0 2rem",
    height: 56,
    display: "flex",
    alignItems: "center",
    justifyContent: "space-between",
  },
  headerLeft: {
    display: "flex",
    alignItems: "center",
    gap: "2rem",
  },
  headerRight: {
    display: "flex",
    alignItems: "center",
    gap: "1rem",
  },
  userInfo: {
    fontSize: "0.9rem",
    color: uiTheme.headerLink,
  },
  roleBadge: {
    background: uiTheme.headerChipBg,
    padding: "2px 8px",
    borderRadius: 4,
    fontSize: "0.8rem",
    color: uiTheme.headerChipText,
  },
  logoutBtn: {
    background: "transparent",
    border: `1px solid ${uiTheme.border}`,
    color: uiTheme.headerText,
    padding: "5px 14px",
    borderRadius: 5,
    cursor: "pointer",
    fontSize: "0.875rem",
  },
  page: {
    padding: "2rem",
    maxWidth: 1200,
    margin: "0 auto",
    display: "grid",
    gap: "1.25rem",
  },
  hero: {
    display: "flex",
    justifyContent: "space-between",
    alignItems: "flex-start",
    gap: "1rem",
    flexWrap: "wrap",
  },
  heading: {
    margin: 0,
    color: uiTheme.text,
    fontSize: "1.5rem",
  },
  subtitle: {
    margin: "0.45rem 0 0",
    color: uiTheme.textMuted,
    maxWidth: 760,
    lineHeight: 1.55,
  },
  heroStats: {
    display: "grid",
    gridTemplateColumns: "repeat(2, minmax(120px, 1fr))",
    gap: "0.75rem",
  },
  statCard: {
    background: uiTheme.surface,
    border: `1px solid ${uiTheme.border}`,
    borderRadius: 14,
    padding: "0.9rem 1rem",
    minWidth: 140,
    boxShadow: uiTheme.shadow,
  },
  statLabel: {
    color: uiTheme.textMuted,
    fontSize: "0.8rem",
    textTransform: "uppercase",
    letterSpacing: 0.6,
  },
  statValue: {
    color: uiTheme.text,
    fontSize: "1.45rem",
    fontWeight: 700,
    marginTop: "0.3rem",
  },
  card: {
    background: uiTheme.surface,
    border: `1px solid ${uiTheme.border}`,
    borderRadius: 18,
    padding: "1rem",
    boxShadow: uiTheme.shadow,
  },
  grid: {
    display: "grid",
    gridTemplateColumns: "repeat(auto-fit, minmax(280px, 1fr))",
    gap: "1rem",
  },
  sessionCard: {
    border: `1px solid ${uiTheme.border}`,
    borderRadius: 16,
    padding: "1rem",
    background: uiTheme.surfaceAlt,
    display: "grid",
    gap: "0.9rem",
  },
  sessionCardCurrent: {
    background: uiTheme.surface,
    boxShadow: uiTheme.shadow,
  },
  cardTop: {
    display: "flex",
    alignItems: "flex-start",
    justifyContent: "space-between",
    gap: "0.75rem",
  },
  sessionTitle: {
    color: uiTheme.text,
    fontWeight: 700,
    fontSize: "1rem",
  },
  sessionMeta: {
    color: uiTheme.textMuted,
    fontSize: "0.9rem",
    marginTop: "0.25rem",
  },
  badge: {
    display: "inline-flex",
    alignItems: "center",
    justifyContent: "center",
    padding: "0.25rem 0.55rem",
    borderRadius: 999,
    fontSize: "0.78rem",
    fontWeight: 700,
    background: uiTheme.surface,
    color: uiTheme.textMuted,
    border: `1px solid ${uiTheme.border}`,
  },
  badgeCurrent: {
    color: uiTheme.headerText,
    background: uiTheme.headerBg,
  },
  details: {
    display: "grid",
    gap: "0.6rem",
    margin: 0,
  },
  detailRow: {
    display: "grid",
    gridTemplateColumns: "88px 1fr",
    gap: "0.5rem",
    alignItems: "baseline",
  },
  detailColumn: {
    display: "grid",
    gap: "0.25rem",
  },
  term: {
    color: uiTheme.textMuted,
    fontSize: "0.82rem",
    fontWeight: 700,
    textTransform: "uppercase",
    letterSpacing: 0.45,
    margin: 0,
  },
  value: {
    margin: 0,
    color: uiTheme.text,
    fontSize: "0.92rem",
  },
  agentValue: {
    margin: 0,
    color: uiTheme.text,
    fontSize: "0.92rem",
    lineHeight: 1.5,
    overflowWrap: "anywhere",
  },
  actions: {
    display: "flex",
    justifyContent: "flex-end",
  },
  loadMoreRow: {
    display: "flex",
    justifyContent: "center",
  },
  primaryAction: {
    border: "none",
    borderRadius: 10,
    background: uiTheme.headerBg,
    color: uiTheme.headerText,
    padding: "0.7rem 0.95rem",
    fontWeight: 700,
    cursor: "pointer",
  },
  secondaryAction: {
    border: `1px solid ${uiTheme.border}`,
    borderRadius: 10,
    background: uiTheme.surface,
    color: uiTheme.text,
    padding: "0.7rem 0.95rem",
    fontWeight: 700,
    cursor: "pointer",
  },
  muted: {
    margin: "0.5rem 0",
    color: uiTheme.textMuted,
  },
  error: {
    color: "#c0392b",
    background: "#fdecea",
    border: "1px solid #f5c6cb",
    borderRadius: 12,
    padding: "0.8rem 1rem",
  },
};
