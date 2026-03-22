import type { CSSProperties } from "react";
import { useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { HeaderBrand } from "../Brand";
import { useAuth } from "../AuthContext";
import { HeaderNavMenu } from "../HeaderNavMenu";
import {
  AnalyticsBucket,
  AnalyticsRange,
  AuditAnalytics,
  AuditEvent,
  getAuditAnalytics,
  getLoggingStatus,
  listAuditEvents,
  LogDeliveryStatus,
  logout,
} from "../api";
import { ThemeToggleButton } from "../ThemeToggleButton";
import { HeatmapGrid, StackedBars, TimeSeriesChart } from "../components/charts/SvgCharts";
import { uiTheme } from "../uiTheme";
import { useStoredState } from "../useStoredState";

const actionPalette = ["#0f766e", "#175cd3", "#b54708", "#c2410c", "#7a5af8", "#087443"];

export function AuditEventsPage() {
  const { session, setSession } = useAuth();
  const navigate = useNavigate();
  const [events, setEvents] = useState<AuditEvent[]>([]);
  const [analytics, setAnalytics] = useState<AuditAnalytics | null>(null);
  const [deliveryIssues, setDeliveryIssues] = useState<LogDeliveryStatus[]>([]);
  const [nextCursor, setNextCursor] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [loadingMore, setLoadingMore] = useState(false);
  const [error, setError] = useState("");
  const [action, setAction] = useStoredState("audit:action", "");
  const [resourceType, setResourceType] = useStoredState("audit:resource-type", "");
  const [result, setResult] = useStoredState("audit:result", "");
  const [actorUserID, setActorUserID] = useStoredState("audit:actor-user-id", "");
  const [limit, setLimit] = useStoredState("audit:limit", "50");
  const [range, setRange] = useStoredState<AnalyticsRange>("audit:analytics-range", "24h");
  const [bucket, setBucket] = useStoredState<AnalyticsBucket>("audit:analytics-bucket", "hour");
  const [expandedEventIDs, setExpandedEventIDs] = useStoredState<string[]>("audit:expanded-event-ids", []);

  const canViewLogging = session?.role === "admin" || session?.role === "operator";
  const effectiveBucket = normalizeAuditBucket(range, bucket);
  const analyticsBucket = analytics?.bucket ?? effectiveBucket;

  useEffect(() => {
    if (effectiveBucket !== bucket) {
      setBucket(effectiveBucket);
    }
  }, [bucket, effectiveBucket, setBucket]);

  useEffect(() => {
    let active = true;

    async function load() {
      setLoading(true);
      setError("");
      try {
        const [auditEventInventory, auditAnalytics, maybeLogging] = await Promise.all([
          listAuditEvents({
            action: action.trim(),
            resource_type: resourceType.trim(),
            result: result.trim(),
            actor_user_id: actorUserID.trim(),
            page_size: Number(limit) || 50,
          }),
          getAuditAnalytics(range, effectiveBucket),
          canViewLogging ? getLoggingStatus() : Promise.resolve(null),
        ]);
        if (!active) return;
        setEvents(auditEventInventory.events);
        setNextCursor(auditEventInventory.next_cursor ?? null);
        setAnalytics(auditAnalytics);
        setDeliveryIssues((maybeLogging?.sinks ?? []).filter((item) => item.last_error || item.total_failed > 0 || item.dropped_events > 0));
      } catch (e: unknown) {
        if (!active) return;
        setError(e instanceof Error ? e.message : "failed to load audit events");
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
  }, [action, actorUserID, canViewLogging, effectiveBucket, limit, range, resourceType, result]);

  async function handleLogout() {
    await logout();
    setSession(null);
    navigate("/login", { replace: true });
  }

  async function handleLoadMore() {
    if (!nextCursor) return;
    setLoadingMore(true);
    setError("");
    try {
      const auditEventInventory = await listAuditEvents({
        action: action.trim(),
        resource_type: resourceType.trim(),
        result: result.trim(),
        actor_user_id: actorUserID.trim(),
        page_size: Number(limit) || 50,
        cursor: nextCursor,
      });
      setEvents((current) => {
        const seen = new Set(current.map((event) => event.id));
        return [...current, ...auditEventInventory.events.filter((event) => !seen.has(event.id))];
      });
      setNextCursor(auditEventInventory.next_cursor ?? null);
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : "failed to load more audit events");
    } finally {
      setLoadingMore(false);
    }
  }

  function applyPreset(kind: "reconcile" | "enrollment" | "auth" | "users" | "lifecycle" | "rotation" | "reissue") {
    if (kind === "reconcile") {
      setAction("peer.reconcile");
      setResourceType("peer");
      setResult("");
      return;
    }
    if (kind === "enrollment") {
      setAction("enrollment.perform");
      setResourceType("");
      setResult("");
      return;
    }
    if (kind === "auth") {
      setAction("");
      setResourceType("session");
      setResult("");
      return;
    }
    if (kind === "lifecycle") {
      setAction("agent.");
      setResourceType("agent");
      setResult("");
      return;
    }
    if (kind === "rotation") {
      setAction("agent.rotate.");
      setResourceType("agent");
      setResult("");
      return;
    }
    if (kind === "reissue") {
      setAction("agent.reissue");
      setResourceType("agent");
      setResult("");
      return;
    }
    setAction("");
    setResourceType("user");
    setResult("");
  }

  function toggleExpanded(eventID: string) {
    setExpandedEventIDs((current) =>
      current.includes(eventID) ? current.filter((id) => id !== eventID) : [...current, eventID],
    );
  }

  const trendPoints = analytics?.event_trend.map((point) => ({
    label: formatBucket(point.bucket_start, analyticsBucket),
    value: point.count,
  })) ?? [];
  const heatmapCells = analytics?.activity_heatmap.map((cell) => ({
    x: cell.hour,
    y: cell.weekday,
    value: cell.count,
  })) ?? [];
  const actionBars = analytics?.action_distribution.map((item, index) => ({
    label: item.category,
    segments: [{ value: item.count, color: actionPalette[index % actionPalette.length] }],
  })) ?? [];

  return (
    <div style={s.shell}>
      <header style={s.header}>
        <div style={s.headerLeft}>
          <HeaderBrand />
          <HeaderNavMenu current="audit" role={session?.role} />
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
            <h2 style={s.heading}>Audit Analytics</h2>
            <p style={s.subtitle}>
              Trend and distribution views over the authoritative audit trail, with delivery-failure visibility layered in for operators and admins.
            </p>
          </div>
          <div style={s.heroControls}>
            <select value={range} onChange={(e) => setRange(e.target.value as AnalyticsRange)} style={s.select}>
              <option value="24h">24 hours</option>
              <option value="7d">7 days</option>
              <option value="30d">30 days</option>
            </select>
            <select value={effectiveBucket} onChange={(e) => setBucket(e.target.value as AnalyticsBucket)} style={s.select}>
              <option value="hour">hour</option>
              <option value="day">day</option>
            </select>
          </div>
        </section>

        {error && <div style={s.error}>{error}</div>}

        <section style={s.analyticsGrid}>
          <article style={s.panelWide}>
            <div style={s.panelHeader}>
              <div>
                <div style={s.panelEyebrow}>Audit Trend</div>
                <h3 style={s.panelTitle}>Events Over Time</h3>
              </div>
              <div style={s.panelMetric}>{sum(trendPoints.map((point) => point.value))} events</div>
            </div>
            {loading ? (
              <p style={s.muted}>Loading audit trend…</p>
            ) : (
              <>
                <div style={s.chartWrap}>
                  <TimeSeriesChart points={trendPoints} />
                </div>
                <div style={s.legendRow}>
                  {compressLabels(trendPoints).map((point) => (
                    <span key={point.label} style={s.legendItem}>
                      <span style={s.legendDot} />
                      {point.label}: {point.value}
                    </span>
                  ))}
                </div>
              </>
            )}
          </article>

          <article style={s.panel}>
            <div style={s.panelHeader}>
              <div>
                <div style={s.panelEyebrow}>Action Distribution</div>
                <h3 style={s.panelTitle}>Category Weight</h3>
              </div>
            </div>
            {loading ? (
              <p style={s.muted}>Loading action mix…</p>
            ) : actionBars.length === 0 ? (
              <p style={s.muted}>No actions in the current range.</p>
            ) : (
              <>
                <div style={s.chartWrapSmall}>
                  <StackedBars bars={actionBars} />
                </div>
                <div style={s.metricList}>
                  {analytics?.action_distribution.map((item, index) => (
                    <div key={item.category} style={s.metricRow}>
                      <span style={s.metricLabel}>
                        <span style={{ ...s.legendDot, background: actionPalette[index % actionPalette.length] }} />
                        {item.category}
                      </span>
                      <strong>{item.count}</strong>
                    </div>
                  ))}
                </div>
              </>
            )}
          </article>

          <article style={s.panelWide}>
            <div style={s.panelHeader}>
              <div>
                <div style={s.panelEyebrow}>Hourly Activity Heatmap</div>
                <h3 style={s.panelTitle}>When Changes Happen</h3>
              </div>
              <div style={s.panelMetric}>{effectiveBucket}</div>
            </div>
            {loading ? (
              <p style={s.muted}>Loading activity heatmap…</p>
            ) : (
              <>
                <div style={s.chartWrapHeatmap}>
                  <HeatmapGrid cells={heatmapCells} columns={24} rows={7} />
                </div>
                <div style={s.heatmapLabels}>
                  <span>Sun</span>
                  <span>Mon</span>
                  <span>Tue</span>
                  <span>Wed</span>
                  <span>Thu</span>
                  <span>Fri</span>
                  <span>Sat</span>
                </div>
              </>
            )}
          </article>

          {canViewLogging && (
            <article style={s.panel}>
              <div style={s.panelHeader}>
                <div>
                  <div style={s.panelEyebrow}>Export Issues</div>
                  <h3 style={s.panelTitle}>Delivery Failures</h3>
                </div>
              </div>
              {deliveryIssues.length === 0 ? (
                <p style={s.muted}>No recent export issues are visible.</p>
              ) : (
                <div style={s.issueList}>
                  {deliveryIssues.slice(0, 5).map((issue) => (
                    <div key={issue.sink_id} style={s.issueCard}>
                      <div style={s.issueTitle}>{issue.sink_name || issue.sink_id}</div>
                      <div style={s.issueMeta}>
                        failed {issue.total_failed} · dropped {issue.dropped_events}
                      </div>
                      <div style={s.issueError}>{issue.last_error || "delivery retry pressure detected"}</div>
                    </div>
                  ))}
                </div>
              )}
            </article>
          )}
        </section>

        <section style={s.filtersCard}>
          <div style={s.presetRow}>
            <button onClick={() => applyPreset("reconcile")} style={s.presetBtn}>Reconcile</button>
            <button onClick={() => applyPreset("enrollment")} style={s.presetBtn}>Enrollments</button>
            <button onClick={() => applyPreset("lifecycle")} style={s.presetBtn}>Lifecycle</button>
            <button onClick={() => applyPreset("rotation")} style={s.presetBtn}>Rotation</button>
            <button onClick={() => applyPreset("reissue")} style={s.presetBtn}>Reissue</button>
            <button onClick={() => applyPreset("auth")} style={s.presetBtn}>Sessions</button>
            <button onClick={() => applyPreset("users")} style={s.presetBtn}>Users</button>
          </div>
          <input value={action} onChange={(e) => setAction(e.target.value)} placeholder="Filter by action" style={s.input} />
          <input value={resourceType} onChange={(e) => setResourceType(e.target.value)} placeholder="Filter by resource type" style={s.input} />
          <select value={result} onChange={(e) => setResult(e.target.value)} style={s.select}>
            <option value="">All results</option>
            <option value="success">success</option>
            <option value="failure">failure</option>
          </select>
          <input value={actorUserID} onChange={(e) => setActorUserID(e.target.value)} placeholder="Actor user ID" style={s.input} />
          <select value={limit} onChange={(e) => setLimit(e.target.value)} style={s.select}>
            <option value="25">25</option>
            <option value="50">50</option>
            <option value="100">100</option>
          </select>
          <button
            onClick={() => {
              setAction("");
              setResourceType("");
              setResult("");
              setActorUserID("");
              setLimit("50");
              setNextCursor(null);
              setExpandedEventIDs([]);
            }}
            style={s.clearBtn}
          >
            Clear
          </button>
        </section>

        <section style={s.card}>
          {loading ? (
            <p style={s.muted}>Loading audit events...</p>
          ) : error ? (
            <p style={s.errorText}>{error}</p>
          ) : events.length === 0 ? (
            <p style={s.muted}>No audit events match the current filter.</p>
          ) : (
            <div>
              <div style={s.tableWrap}>
                <table style={s.table}>
                  <thead>
                    <tr>
                      <th style={s.th}>When</th>
                      <th style={s.th}>Action</th>
                      <th style={s.th}>Resource</th>
                      <th style={s.th}>Actor</th>
                      <th style={s.th}>Result</th>
                      <th style={s.th}>Metadata</th>
                    </tr>
                  </thead>
                  <tbody>
                    {events.map((event) => (
                      <tr key={event.id}>
                        <td style={s.td}>
                          <div>{formatDate(event.created_at)}</div>
                          <div style={s.secondaryCell}>{formatRelativeTime(event.created_at)}</div>
                        </td>
                        <td style={s.td}>
                          <div style={s.primaryCell}>{event.action}</div>
                          <div style={s.secondaryCell}>{event.id}</div>
                        </td>
                        <td style={s.td}>
                          <div style={s.primaryCell}>{event.resource_type}</div>
                          <div style={s.secondaryCell}>{event.resource_id ?? "n/a"}</div>
                        </td>
                        <td style={s.td}>{event.actor_user_id ?? "system"}</td>
                        <td style={s.td}>
                          <span style={{ ...s.badge, background: event.result === "success" ? "#067647" : "#b42318" }}>
                            {event.result}
                          </span>
                        </td>
                        <td style={s.td}>
                          <button onClick={() => toggleExpanded(event.id)} style={s.metadataToggle}>
                            {expandedEventIDs.includes(event.id) ? "Hide metadata" : "Show metadata"}
                          </button>
                          {expandedEventIDs.includes(event.id) && (
                            <pre style={s.metadata}>{JSON.stringify(event.metadata ?? {}, null, 2)}</pre>
                          )}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
              {nextCursor && (
                <div style={s.loadMoreRow}>
                  <button onClick={handleLoadMore} style={s.clearBtn} disabled={loadingMore}>
                    {loadingMore ? "Loading..." : "Load older events"}
                  </button>
                </div>
              )}
            </div>
          )}
        </section>
      </main>
    </div>
  );
}

function normalizeAuditBucket(range: AnalyticsRange, bucket: AnalyticsBucket): AnalyticsBucket {
  if (range !== "24h" && bucket === "hour") {
    return "day";
  }
  return bucket;
}

function formatDate(value: string): string {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString();
}

function formatRelativeTime(value: string): string {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  const deltaMs = Date.now() - date.getTime();
  const deltaMinutes = Math.round(deltaMs / 60000);
  if (deltaMinutes < 1) return "just now";
  if (deltaMinutes < 60) return `${deltaMinutes}m ago`;
  const deltaHours = Math.round(deltaMinutes / 60);
  if (deltaHours < 24) return `${deltaHours}h ago`;
  const deltaDays = Math.round(deltaHours / 24);
  return `${deltaDays}d ago`;
}

function formatBucket(value: string, bucket: AuditAnalytics["bucket"]): string {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return bucket === "hour"
    ? date.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })
    : date.toLocaleDateString([], { month: "short", day: "numeric" });
}

function compressLabels(points: Array<{ label: string; value: number }>): Array<{ label: string; value: number }> {
  if (points.length <= 6) return points;
  const step = Math.max(1, Math.floor(points.length / 6));
  return points.filter((_, index) => index === 0 || index === points.length - 1 || index % step === 0);
}

function sum(values: number[]): number {
  return values.reduce((total, value) => total + value, 0);
}

const s: Record<string, CSSProperties> = {
  shell: { minHeight: "100vh", background: uiTheme.pageBg, fontFamily: "system-ui, sans-serif" },
  header: { background: uiTheme.headerBg, color: uiTheme.headerText, padding: "0 2rem", height: 56, display: "flex", alignItems: "center", justifyContent: "space-between" },
  headerLeft: { display: "flex", alignItems: "center", gap: "2rem" },
  headerRight: { display: "flex", alignItems: "center", gap: "1rem" },
  userInfo: { fontSize: "0.9rem", color: uiTheme.headerLink },
  roleBadge: { background: uiTheme.headerChipBg, padding: "2px 8px", borderRadius: 4, fontSize: "0.8rem", color: uiTheme.headerChipText },
  logoutBtn: { background: "transparent", border: `1px solid ${uiTheme.border}`, color: uiTheme.headerText, padding: "5px 14px", borderRadius: 5, cursor: "pointer", fontSize: "0.875rem" },
  page: { padding: "2rem", maxWidth: 1300, margin: "0 auto", display: "grid", gap: "1.25rem" },
  hero: { display: "flex", justifyContent: "space-between", alignItems: "flex-start", gap: "1rem", flexWrap: "wrap" },
  heroControls: { display: "flex", gap: "0.75rem", flexWrap: "wrap" },
  heading: { margin: 0, fontSize: "1.6rem", color: uiTheme.text },
  subtitle: { margin: "0.45rem 0 0", color: uiTheme.textMuted, maxWidth: 760, lineHeight: 1.55 },
  error: { background: "#fdecea", border: "1px solid #f5c2c7", color: "#b42318", borderRadius: 12, padding: "0.85rem 1rem" },
  analyticsGrid: { display: "grid", gridTemplateColumns: "repeat(auto-fit, minmax(320px, 1fr))", gap: "1rem" },
  panel: { background: uiTheme.surface, borderRadius: 18, padding: "1.1rem", boxShadow: uiTheme.shadow, border: `1px solid ${uiTheme.border}`, display: "grid", gap: "0.9rem" },
  panelWide: { gridColumn: "1 / -1", background: uiTheme.surface, borderRadius: 18, padding: "1.1rem", boxShadow: uiTheme.shadow, border: `1px solid ${uiTheme.border}`, display: "grid", gap: "0.9rem" },
  panelHeader: { display: "flex", justifyContent: "space-between", alignItems: "flex-start", gap: "1rem", flexWrap: "wrap" },
  panelEyebrow: { color: uiTheme.textMuted, textTransform: "uppercase", letterSpacing: 0.6, fontSize: "0.76rem", fontWeight: 700 },
  panelTitle: { margin: "0.25rem 0 0", color: uiTheme.text, fontSize: "1.08rem" },
  panelMetric: { color: uiTheme.text, fontWeight: 700, fontSize: "0.95rem" },
  chartWrap: { color: "#175cd3", minHeight: 180 },
  chartWrapSmall: { minHeight: 160 },
  chartWrapHeatmap: { minHeight: 210 },
  legendRow: { display: "flex", flexWrap: "wrap", gap: "0.65rem 1rem" },
  legendItem: { color: uiTheme.textMuted, fontSize: "0.82rem", display: "inline-flex", alignItems: "center", gap: "0.4rem" },
  legendDot: { width: 10, height: 10, borderRadius: 999, display: "inline-block", background: "#175cd3" },
  metricList: { display: "grid", gap: "0.45rem" },
  metricRow: { display: "flex", justifyContent: "space-between", gap: "0.75rem", color: uiTheme.textMuted, fontSize: "0.9rem" },
  metricLabel: { display: "inline-flex", alignItems: "center", gap: "0.45rem" },
  heatmapLabels: { display: "grid", gridTemplateColumns: "repeat(7, minmax(0, 1fr))", gap: "0.35rem", color: uiTheme.textMuted, fontSize: "0.8rem" },
  issueList: { display: "grid", gap: "0.75rem" },
  issueCard: { background: uiTheme.surfaceAlt, borderRadius: 12, border: `1px solid ${uiTheme.border}`, padding: "0.9rem", display: "grid", gap: "0.35rem" },
  issueTitle: { color: uiTheme.text, fontWeight: 700 },
  issueMeta: { color: uiTheme.textMuted, fontSize: "0.84rem" },
  issueError: { color: uiTheme.textSoft, fontSize: "0.86rem", lineHeight: 1.45, wordBreak: "break-word" },
  filtersCard: { background: uiTheme.surface, borderRadius: 12, padding: "1rem 1.25rem", boxShadow: uiTheme.shadow, display: "flex", flexWrap: "wrap", gap: "0.75rem" },
  presetRow: { display: "flex", gap: "0.5rem", flexWrap: "wrap", width: "100%" },
  presetBtn: { background: "#eef4ff", color: "#175cd3", border: "1px solid #c7d7fe", borderRadius: 999, padding: "0.45rem 0.8rem", cursor: "pointer", fontSize: "0.82rem", fontWeight: 600 },
  card: { background: uiTheme.surface, borderRadius: 12, padding: "1.25rem", boxShadow: uiTheme.shadow },
  input: { flex: "1 1 220px", minWidth: 200, padding: "0.7rem 0.85rem", borderRadius: 8, border: `1px solid ${uiTheme.border}`, fontSize: "0.95rem", background: uiTheme.inputBg, color: uiTheme.inputText },
  select: { padding: "0.7rem 0.85rem", borderRadius: 8, border: `1px solid ${uiTheme.border}`, fontSize: "0.95rem", background: uiTheme.inputBg, color: uiTheme.inputText },
  clearBtn: { background: uiTheme.surface, color: uiTheme.textSoft, border: `1px solid ${uiTheme.border}`, borderRadius: 8, padding: "0.7rem 0.95rem", cursor: "pointer" },
  tableWrap: { overflowX: "auto" },
  table: { width: "100%", borderCollapse: "collapse" },
  th: { textAlign: "left", padding: "0.75rem", fontSize: "0.8rem", color: uiTheme.textMuted, borderBottom: `1px solid ${uiTheme.borderTableStrong}` },
  td: { padding: "0.9rem 0.75rem", verticalAlign: "top", borderBottom: `1px solid ${uiTheme.borderTable}` },
  primaryCell: { fontWeight: 600, color: uiTheme.text },
  secondaryCell: { fontSize: "0.84rem", color: uiTheme.textMuted, marginTop: "0.2rem" },
  badge: { display: "inline-block", color: "#fff", borderRadius: 999, padding: "0.2rem 0.55rem", fontSize: "0.78rem", fontWeight: 600 },
  metadataToggle: { background: uiTheme.surface, color: uiTheme.textSoft, border: `1px solid ${uiTheme.border}`, borderRadius: 8, padding: "0.45rem 0.7rem", cursor: "pointer", fontSize: "0.8rem" },
  loadMoreRow: { display: "flex", justifyContent: "center", marginTop: "1rem" },
  metadata: { margin: 0, whiteSpace: "pre-wrap", wordBreak: "break-word", fontSize: "0.8rem", color: uiTheme.textMuted, background: uiTheme.surfaceAlt, borderRadius: 8, padding: "0.65rem" },
  muted: { color: uiTheme.textMuted, margin: 0 },
  errorText: { color: "#b42318", margin: 0 },
};
