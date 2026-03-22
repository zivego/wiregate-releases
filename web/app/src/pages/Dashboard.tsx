import type { CSSProperties } from "react";
import { useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { HeaderBrand } from "../Brand";
import { useAuth } from "../AuthContext";
import { HeaderNavMenu } from "../HeaderNavMenu";
import {
  AnalyticsRange,
  DashboardAnalytics,
  FailingAgent,
  fetchLiveHealth,
  fetchReconcileHealth,
  getDashboardAnalytics,
  getLoggingStatus,
  LogDeliveryStatus,
  logout,
} from "../api";
import { ThemeToggleButton } from "../ThemeToggleButton";
import { DonutOrCoverageRing, HealthSparkline, StackedBars, TimeSeriesChart } from "../components/charts/SvgCharts";
import { uiTheme } from "../uiTheme";
import { useStoredState } from "../useStoredState";

const chartPalette = {
  auth: "#0f766e",
  enrollUsed: "#0f766e",
  enrollInactive: "#f59e0b",
  deliveryHealthy: "#175cd3",
  deliveryRisk: "#c2410c",
};

export function Dashboard() {
  const { session, setSession } = useAuth();
  const navigate = useNavigate();
  const [liveStatus, setLiveStatus] = useState<string>("...");
  const [reconcileStatus, setReconcileStatus] = useState<string>("...");
  const [analytics, setAnalytics] = useState<DashboardAnalytics | null>(null);
  const [loggingStatus, setLoggingStatus] = useState<LogDeliveryStatus[]>([]);
  const [loggingQueue, setLoggingQueue] = useState({ current: 0, capacity: 0 });
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [lastUpdatedAt, setLastUpdatedAt] = useState<string>("");
  const [range, setRange] = useStoredState<AnalyticsRange>("dashboard:analytics-range", "24h");
  const [refreshIntervalMs, setRefreshIntervalMs] = useStoredState("dashboard:refresh-interval-ms", 30_000);

  const canViewLogging = session?.role === "admin" || session?.role === "operator";
  const analyticsBucket = analytics?.bucket ?? "hour";

  useEffect(() => {
    let active = true;

    async function load() {
      setLoading(true);
      setError("");
      try {
        const [live, reconcile, dashboard, maybeLogging] = await Promise.all([
          fetchLiveHealth(),
          fetchReconcileHealth(),
          getDashboardAnalytics(range),
          canViewLogging ? getLoggingStatus() : Promise.resolve(null),
        ]);
        if (!active) return;
        setLiveStatus(live.status);
        setReconcileStatus(reconcile.status);
        setAnalytics(dashboard);
        if (maybeLogging) {
          setLoggingStatus(maybeLogging.sinks);
          setLoggingQueue({ current: maybeLogging.current_queued, capacity: maybeLogging.queue_capacity });
        } else {
          setLoggingStatus([]);
          setLoggingQueue({ current: 0, capacity: 0 });
        }
      } catch (err: unknown) {
        if (!active) return;
        setError(err instanceof Error ? err.message : "failed to load dashboard analytics");
        setLiveStatus("error");
        setReconcileStatus("error");
        setAnalytics(null);
        setLoggingStatus([]);
        setLoggingQueue({ current: 0, capacity: 0 });
      } finally {
        if (active) {
          setLoading(false);
          setLastUpdatedAt(new Date().toISOString());
        }
      }
    }

    void load();
    const timer = refreshIntervalMs > 0 ? window.setInterval(load, refreshIntervalMs) : null;

    return () => {
      active = false;
      if (timer !== null) {
        window.clearInterval(timer);
      }
    };
  }, [canViewLogging, range, refreshIntervalMs]);

  async function handleLogout() {
    await logout();
    setSession(null);
    navigate("/login", { replace: true });
  }

  const authTrendPoints = analytics?.auth_security_trend.map((point) => ({
    label: formatBucket(point.bucket_start, analyticsBucket),
    value: point.count,
  })) ?? [];
  const failureSparkline = analytics?.top_failing_agents.map((agent) => agent.failure_score) ?? [];
  const healthySinks = loggingStatus.filter((item) => item.enabled && !item.last_error && item.consecutive_failures === 0).length;
  const degradedSinks = loggingStatus.filter((item) => item.enabled && (item.last_error || item.consecutive_failures > 0)).length;

  return (
    <div style={styles.page}>
      <header style={styles.header}>
        <div style={styles.headerLeft}>
          <HeaderBrand />
          <HeaderNavMenu current="dashboard" role={session?.role} />
        </div>
        <div style={styles.headerRight}>
          <ThemeToggleButton />
          <span style={styles.userInfo}>
            {session?.email} <span style={styles.role}>{session?.role}</span>
          </span>
          <button onClick={handleLogout} style={styles.logoutBtn}>
            Logout
          </button>
        </div>
      </header>

      <main style={styles.main}>
        <section style={styles.hero}>
          <div>
            <h2 style={styles.heading}>Operational Dashboard</h2>
            <p style={styles.heroCopy}>
              Auth, enrollment, policy, and runtime health analytics from the live control-plane inventory. Auto-refresh {refreshLabel(refreshIntervalMs)}.
            </p>
            <div style={styles.heroMeta}>Last updated {formatTimestamp(lastUpdatedAt)}</div>
          </div>
          <div style={styles.controls}>
            <select value={range} onChange={(e) => setRange(e.target.value as AnalyticsRange)} style={styles.select}>
              <option value="24h">24 hours</option>
              <option value="7d">7 days</option>
              <option value="30d">30 days</option>
            </select>
            <select value={String(refreshIntervalMs)} onChange={(e) => setRefreshIntervalMs(Number(e.target.value))} style={styles.select}>
              <option value="0">Manual only</option>
              <option value="15000">15s</option>
              <option value="30000">30s</option>
              <option value="60000">60s</option>
            </select>
          </div>
        </section>

        {error && <div style={styles.error}>{error}</div>}

        <section style={styles.statusGrid}>
          <StatusCard label="API Health" value={liveStatus} tone={liveStatus === "ok" ? "good" : "bad"} detail="liveness endpoint" />
          <StatusCard label="Reconciler" value={reconcileStatus} tone={reconcileStatus === "ready" ? "good" : "bad"} detail="runtime sync loop" />
          <StatusCard label="Total Agents" value={String(analytics?.health_cards.total_agents ?? 0)} detail="inventory records" />
          <StatusCard label="Seen Recently" value={String(analytics?.health_cards.recently_seen_agents ?? 0)} tone="info" detail="within the active window" />
          <StatusCard label="Drifted" value={String(analytics?.health_cards.drifted_agents ?? 0)} tone={(analytics?.health_cards.drifted_agents ?? 0) > 0 ? "warn" : "good"} detail="runtime mismatch" />
          <StatusCard label="Apply Failed" value={String(analytics?.health_cards.failed_agents ?? 0)} tone={(analytics?.health_cards.failed_agents ?? 0) > 0 ? "bad" : "good"} detail="last apply attempt" />
          <StatusCard label="Pending Reconcile" value={String(analytics?.health_cards.pending_reconcile ?? 0)} tone={(analytics?.health_cards.pending_reconcile ?? 0) > 0 ? "warn" : "good"} detail="runtime follow-up needed" />
          {canViewLogging && (
            <StatusCard
              label="Export Queue"
              value={`${loggingQueue.current}/${loggingQueue.capacity || 0}`}
              tone={loggingQueue.current > 0 ? "warn" : "good"}
              detail="SIEM delivery backlog"
            />
          )}
        </section>

        <section style={styles.analyticsGrid}>
          <article style={styles.panelWide}>
            <div style={styles.panelHeader}>
              <div>
                <div style={styles.panelEyebrow}>Auth &amp; Security</div>
                <h3 style={styles.panelTitle}>Event Trend</h3>
              </div>
              <div style={styles.panelMetric}>
                {sum(authTrendPoints.map((point) => point.value))} events
              </div>
            </div>
            {loading ? (
              <p style={styles.muted}>Loading trend…</p>
            ) : (
              <>
                <div style={styles.chartWrap}>
                  <TimeSeriesChart points={authTrendPoints} />
                </div>
                <div style={styles.legendRow}>
                  {compressLabels(authTrendPoints).map((point) => (
                    <span key={point.label} style={styles.legendItem}>
                      <span style={{ ...styles.legendDot, background: chartPalette.auth }} />
                      {point.label}: {point.value}
                    </span>
                  ))}
                </div>
              </>
            )}
          </article>

          <article style={styles.panel}>
            <div style={styles.panelHeader}>
              <div>
                <div style={styles.panelEyebrow}>Enrollment Funnel</div>
                <h3 style={styles.panelTitle}>Issued to Used</h3>
              </div>
            </div>
            {loading || !analytics ? (
              <p style={styles.muted}>Loading funnel…</p>
            ) : (
              <>
                <div style={styles.chartWrapTight}>
                  <StackedBars
                    bars={[
                      {
                        label: "Enrollment",
                        segments: [
                          { value: analytics.enrollment_funnel.used, color: chartPalette.enrollUsed },
                          { value: analytics.enrollment_funnel.revoked_or_expired, color: chartPalette.enrollInactive },
                        ],
                      },
                    ]}
                  />
                </div>
                <div style={styles.metricList}>
                  <MetricRow label="Issued" value={analytics.enrollment_funnel.issued} />
                  <MetricRow label="Used" value={analytics.enrollment_funnel.used} />
                  <MetricRow label="Revoked / Expired" value={analytics.enrollment_funnel.revoked_or_expired} />
                </div>
              </>
            )}
          </article>

          <article style={styles.panel}>
            <div style={styles.panelHeader}>
              <div>
                <div style={styles.panelEyebrow}>Policy Coverage</div>
                <h3 style={styles.panelTitle}>Active Assignment Reach</h3>
              </div>
            </div>
            {loading || !analytics ? (
              <p style={styles.muted}>Loading coverage…</p>
            ) : (
              <>
                <div style={styles.coverageWrap}>
                  <DonutOrCoverageRing
                    value={analytics.policy_coverage.agents_with_policy}
                    total={analytics.policy_coverage.total_agents}
                    fillColor={chartPalette.auth}
                  />
                </div>
                <div style={styles.metricList}>
                  <MetricRow label="Policies" value={analytics.policy_coverage.policies_total} />
                  <MetricRow label="Active Assignments" value={analytics.policy_coverage.active_assignments} />
                  <MetricRow label="Without Policy" value={analytics.policy_coverage.agents_without_policy} />
                </div>
              </>
            )}
          </article>

          <article style={styles.panelWide}>
            <div style={styles.panelHeader}>
              <div>
                <div style={styles.panelEyebrow}>Top Failing Agents</div>
                <h3 style={styles.panelTitle}>Failure Pressure</h3>
              </div>
              {!loading && failureSparkline.length > 1 && (
                <div style={styles.sparklineWrap}>
                  <HealthSparkline values={failureSparkline} />
                </div>
              )}
            </div>
            {loading || !analytics ? (
              <p style={styles.muted}>Loading failing-agent ranking…</p>
            ) : analytics.top_failing_agents.length === 0 ? (
              <p style={styles.muted}>No failing agents in the current range.</p>
            ) : (
              <div style={styles.agentList}>
                {analytics.top_failing_agents.map((agent) => (
                  <FailingAgentCard key={agent.agent_id} agent={agent} />
                ))}
              </div>
            )}
          </article>

          {canViewLogging && (
            <article style={styles.panelWide}>
              <div style={styles.panelHeader}>
                <div>
                  <div style={styles.panelEyebrow}>Log Delivery Health</div>
                  <h3 style={styles.panelTitle}>SIEM Export Pipeline</h3>
                </div>
                <div style={styles.deliverySummary}>
                  <span style={styles.deliveryBadge}>{healthySinks} healthy</span>
                  <span style={{ ...styles.deliveryBadge, ...(degradedSinks > 0 ? styles.deliveryBadgeWarn : null) }}>{degradedSinks} degraded</span>
                </div>
              </div>
              {loggingStatus.length === 0 ? (
                <p style={styles.muted}>No sink health is visible yet.</p>
              ) : (
                <>
                  <div style={styles.chartWrapTight}>
                    <StackedBars
                      bars={loggingStatus.map((item) => ({
                        label: item.sink_name || item.sink_id,
                        segments: [
                          { value: item.total_delivered, color: chartPalette.deliveryHealthy },
                          { value: item.total_failed + item.dropped_events, color: chartPalette.deliveryRisk },
                        ],
                      }))}
                    />
                  </div>
                  <div style={styles.deliveryGrid}>
                    {loggingStatus.slice(0, 4).map((item) => (
                      <div key={item.sink_id} style={styles.deliveryCard}>
                        <div style={styles.deliveryTitle}>{item.sink_name || item.sink_id}</div>
                        <div style={styles.deliveryMeta}>
                          queue {item.queue_depth} · delivered {item.total_delivered} · failed {item.total_failed}
                        </div>
                        <div style={styles.deliveryError}>{item.last_error || "No recent delivery errors."}</div>
                      </div>
                    ))}
                  </div>
                </>
              )}
            </article>
          )}
        </section>
      </main>
    </div>
  );
}

function StatusCard({ label, value, detail, tone = "neutral" }: { label: string; value: string; detail: string; tone?: "neutral" | "good" | "warn" | "bad" | "info" }) {
  return (
    <div style={styles.statusCard}>
      <div style={styles.statusLabel}>{label}</div>
      <div style={{ ...styles.statusValue, ...toneStyle(tone) }}>{value}</div>
      <div style={styles.statusDetail}>{detail}</div>
    </div>
  );
}

function MetricRow({ label, value }: { label: string; value: number }) {
  return (
    <div style={styles.metricRow}>
      <span>{label}</span>
      <strong>{value}</strong>
    </div>
  );
}

function FailingAgentCard({ agent }: { agent: FailingAgent }) {
  return (
    <div style={styles.agentCard}>
      <div style={styles.agentHeader}>
        <div>
          <div style={styles.agentName}>{agent.hostname || agent.agent_id}</div>
          <div style={styles.agentMeta}>{agent.platform} · {agent.status}</div>
        </div>
        <div style={styles.agentScore}>{agent.failure_score}</div>
      </div>
      <div style={styles.agentIssue}>{agent.last_apply_error || agent.failure_categories?.join(", ") || "runtime failure pressure detected"}</div>
    </div>
  );
}

function toneStyle(tone: "neutral" | "good" | "warn" | "bad" | "info"): CSSProperties {
  switch (tone) {
    case "good":
      return { color: "#067647" };
    case "warn":
      return { color: "#b54708" };
    case "bad":
      return { color: "#b42318" };
    case "info":
      return { color: "#175cd3" };
    default:
      return { color: uiTheme.text };
  }
}

function formatTimestamp(value?: string): string {
  if (!value) return "never";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString();
}

function formatBucket(value: string, bucket: DashboardAnalytics["bucket"]): string {
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

function refreshLabel(value: number): string {
  if (value <= 0) return "manual only";
  return `every ${Math.round(value / 1000)}s`;
}

function sum(values: number[]): number {
  return values.reduce((total, value) => total + value, 0);
}

const styles: Record<string, CSSProperties> = {
  page: {
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
  role: {
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
  main: {
    padding: "2rem",
    maxWidth: 1240,
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
    fontSize: "1.6rem",
    fontWeight: 700,
    color: uiTheme.text,
  },
  heroCopy: {
    margin: "0.45rem 0 0",
    color: uiTheme.textMuted,
    lineHeight: 1.55,
    maxWidth: 760,
  },
  heroMeta: {
    marginTop: "0.55rem",
    color: uiTheme.textSoft,
    fontSize: "0.88rem",
  },
  controls: {
    display: "flex",
    gap: "0.75rem",
    flexWrap: "wrap",
  },
  select: {
    padding: "0.7rem 0.9rem",
    borderRadius: 10,
    border: `1px solid ${uiTheme.border}`,
    background: uiTheme.surface,
    color: uiTheme.text,
    minWidth: 140,
  },
  error: {
    background: "#fdecea",
    border: "1px solid #f5c2c7",
    color: "#b42318",
    borderRadius: 12,
    padding: "0.85rem 1rem",
  },
  statusGrid: {
    display: "grid",
    gridTemplateColumns: "repeat(auto-fit, minmax(180px, 1fr))",
    gap: "0.85rem",
  },
  statusCard: {
    background: uiTheme.surface,
    border: `1px solid ${uiTheme.border}`,
    borderRadius: 16,
    padding: "1rem",
    boxShadow: uiTheme.shadow,
    display: "grid",
    gap: "0.3rem",
  },
  statusLabel: {
    color: uiTheme.textMuted,
    fontSize: "0.78rem",
    letterSpacing: 0.5,
    textTransform: "uppercase",
  },
  statusValue: {
    fontSize: "1.5rem",
    fontWeight: 700,
  },
  statusDetail: {
    color: uiTheme.textSoft,
    fontSize: "0.86rem",
  },
  analyticsGrid: {
    display: "grid",
    gridTemplateColumns: "repeat(auto-fit, minmax(320px, 1fr))",
    gap: "1rem",
  },
  panel: {
    background: uiTheme.surface,
    border: `1px solid ${uiTheme.border}`,
    borderRadius: 20,
    padding: "1.15rem",
    boxShadow: uiTheme.shadow,
    display: "grid",
    gap: "0.9rem",
  },
  panelWide: {
    gridColumn: "1 / -1",
    background: uiTheme.surface,
    border: `1px solid ${uiTheme.border}`,
    borderRadius: 20,
    padding: "1.15rem",
    boxShadow: uiTheme.shadow,
    display: "grid",
    gap: "0.9rem",
  },
  panelHeader: {
    display: "flex",
    justifyContent: "space-between",
    alignItems: "flex-start",
    gap: "1rem",
    flexWrap: "wrap",
  },
  panelEyebrow: {
    color: uiTheme.textMuted,
    textTransform: "uppercase",
    letterSpacing: 0.6,
    fontSize: "0.76rem",
    fontWeight: 700,
  },
  panelTitle: {
    margin: "0.25rem 0 0",
    color: uiTheme.text,
    fontSize: "1.1rem",
  },
  panelMetric: {
    color: uiTheme.text,
    fontWeight: 700,
    fontSize: "0.95rem",
  },
  chartWrap: {
    color: chartPalette.auth,
    minHeight: 180,
  },
  chartWrapTight: {
    minHeight: 160,
  },
  legendRow: {
    display: "flex",
    flexWrap: "wrap",
    gap: "0.65rem 1rem",
  },
  legendItem: {
    color: uiTheme.textMuted,
    fontSize: "0.82rem",
    display: "inline-flex",
    alignItems: "center",
    gap: "0.4rem",
  },
  legendDot: {
    width: 10,
    height: 10,
    borderRadius: 999,
    display: "inline-block",
  },
  metricList: {
    display: "grid",
    gap: "0.45rem",
  },
  metricRow: {
    display: "flex",
    justifyContent: "space-between",
    gap: "0.75rem",
    color: uiTheme.textMuted,
    fontSize: "0.9rem",
  },
  coverageWrap: {
    display: "flex",
    justifyContent: "center",
    alignItems: "center",
  },
  muted: {
    color: uiTheme.textMuted,
    margin: 0,
  },
  sparklineWrap: {
    color: chartPalette.deliveryHealthy,
    width: 160,
  },
  agentList: {
    display: "grid",
    gridTemplateColumns: "repeat(auto-fit, minmax(220px, 1fr))",
    gap: "0.8rem",
  },
  agentCard: {
    background: uiTheme.surfaceAlt,
    border: `1px solid ${uiTheme.border}`,
    borderRadius: 14,
    padding: "0.95rem",
    display: "grid",
    gap: "0.65rem",
  },
  agentHeader: {
    display: "flex",
    justifyContent: "space-between",
    gap: "0.8rem",
  },
  agentName: {
    color: uiTheme.text,
    fontWeight: 700,
  },
  agentMeta: {
    color: uiTheme.textMuted,
    fontSize: "0.84rem",
    marginTop: "0.2rem",
  },
  agentScore: {
    minWidth: 36,
    height: 36,
    borderRadius: 999,
    display: "inline-flex",
    alignItems: "center",
    justifyContent: "center",
    background: "#fff4e8",
    color: "#b54708",
    fontWeight: 700,
  },
  agentIssue: {
    color: uiTheme.textSoft,
    fontSize: "0.88rem",
    lineHeight: 1.5,
  },
  deliverySummary: {
    display: "flex",
    gap: "0.5rem",
    flexWrap: "wrap",
  },
  deliveryBadge: {
    background: "#eaf2ff",
    color: "#175cd3",
    borderRadius: 999,
    padding: "0.3rem 0.65rem",
    fontSize: "0.82rem",
    fontWeight: 700,
  },
  deliveryBadgeWarn: {
    background: "#fff3e8",
    color: "#b54708",
  },
  deliveryGrid: {
    display: "grid",
    gridTemplateColumns: "repeat(auto-fit, minmax(220px, 1fr))",
    gap: "0.8rem",
  },
  deliveryCard: {
    background: uiTheme.surfaceAlt,
    border: `1px solid ${uiTheme.border}`,
    borderRadius: 14,
    padding: "0.95rem",
    display: "grid",
    gap: "0.4rem",
  },
  deliveryTitle: {
    color: uiTheme.text,
    fontWeight: 700,
  },
  deliveryMeta: {
    color: uiTheme.textMuted,
    fontSize: "0.84rem",
  },
  deliveryError: {
    color: uiTheme.textSoft,
    fontSize: "0.86rem",
    lineHeight: 1.45,
    wordBreak: "break-word",
  },
};
