import { useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { HeaderBrand } from "../Brand";
import { useAuth } from "../AuthContext";
import { HeaderNavMenu } from "../HeaderNavMenu";
import {
  LogCategory,
  LogDeliveryFailure,
  LogDeliveryStatus,
  LogRouteRule,
  LogSeverity,
  LogSink,
  LogSinkInput,
  createLoggingSink,
  deleteLoggingSink,
  getLoggingRoutes,
  getLoggingStatus,
  listLoggingSinks,
  logout,
  testLoggingDelivery,
  updateLoggingRoutes,
  updateLoggingSink,
} from "../api";
import { ThemeToggleButton } from "../ThemeToggleButton";
import { uiTheme } from "../uiTheme";

const allCategories: LogCategory[] = ["auth", "session", "user_mgmt", "policy", "agent", "enrollment", "reconcile", "security", "system"];
const allSeverities: LogSeverity[] = ["debug", "info", "warn", "error"];

interface RouteDraft {
  id?: string;
  sink_id: string;
  categories: string;
  min_severity: LogSeverity;
  enabled: boolean;
}

const defaultSinkInput: LogSinkInput = {
  name: "",
  type: "syslog",
  enabled: true,
  syslog: {
    transport: "tcp",
    host: "127.0.0.1",
    port: 6514,
    format: "json",
    facility: 16,
    app_name: "wiregate",
    hostname_override: "",
    ca_cert_file: "",
    client_cert_file: "",
    client_key_file: "",
  },
};

export function LoggingPage() {
  const { session, setSession } = useAuth();
  const navigate = useNavigate();
  const canManage = session?.role === "admin";
  const canView = session?.role === "admin" || session?.role === "operator";
  const [sinks, setSinks] = useState<LogSink[]>([]);
  const [routeDrafts, setRouteDrafts] = useState<RouteDraft[]>([]);
  const [status, setStatus] = useState<LogDeliveryStatus[]>([]);
  const [recentFailures, setRecentFailures] = useState<LogDeliveryFailure[]>([]);
  const [redactedFields, setRedactedFields] = useState<string[]>([]);
  const [queueCapacity, setQueueCapacity] = useState(0);
  const [currentQueued, setCurrentQueued] = useState(0);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [busyKey, setBusyKey] = useState("");
  const [createInput, setCreateInput] = useState<LogSinkInput>(defaultSinkInput);

  useEffect(() => {
    if (session?.role === "readonly") {
      navigate("/dashboard", { replace: true });
    }
  }, [navigate, session?.role]);

  useEffect(() => {
    if (!canView) return;
    let active = true;

    async function load() {
      setLoading(true);
      setError("");
      try {
        const [sinkInventory, routesResp, statusResp] = await Promise.all([
          listLoggingSinks(),
          getLoggingRoutes(),
          getLoggingStatus(),
        ]);
        if (!active) return;
        setSinks(sinkInventory);
        setRouteDrafts(routesResp.routes.map(routeToDraft));
        setRedactedFields(uniqueStrings([...routesResp.redacted_fields, ...statusResp.redacted_fields]));
        setStatus(statusResp.sinks);
        setRecentFailures(statusResp.recent_failures);
        setQueueCapacity(statusResp.queue_capacity);
        setCurrentQueued(statusResp.current_queued);
      } catch (err: unknown) {
        if (!active) return;
        setError(err instanceof Error ? err.message : "failed to load logging configuration");
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
  }, [canView]);

  async function refreshAll() {
    const [sinkInventory, routesResp, statusResp] = await Promise.all([
      listLoggingSinks(),
      getLoggingRoutes(),
      getLoggingStatus(),
    ]);
    setSinks(sinkInventory);
    setRouteDrafts(routesResp.routes.map(routeToDraft));
    setRedactedFields(uniqueStrings([...routesResp.redacted_fields, ...statusResp.redacted_fields]));
    setStatus(statusResp.sinks);
    setRecentFailures(statusResp.recent_failures);
    setQueueCapacity(statusResp.queue_capacity);
    setCurrentQueued(statusResp.current_queued);
  }

  async function handleLogout() {
    await logout();
    setSession(null);
    navigate("/login", { replace: true });
  }

  async function handleCreateSink() {
    setBusyKey("create");
    setError("");
    setNotice("");
    try {
      await createLoggingSink(createInput);
      setCreateInput(defaultSinkInput);
      await refreshAll();
      setNotice("Syslog sink saved.");
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : "failed to create sink");
    } finally {
      setBusyKey("");
    }
  }

  async function handleToggleSinkEnabled(sink: LogSink) {
    setBusyKey(`toggle:${sink.id}`);
    setError("");
    setNotice("");
    try {
      await updateLoggingSink(sink.id, { ...sink, enabled: !sink.enabled });
      await refreshAll();
      setNotice(`Sink ${!sink.enabled ? "enabled" : "disabled"}.`);
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : "failed to update sink");
    } finally {
      setBusyKey("");
    }
  }

  async function handleDeleteSink(sinkID: string) {
    setBusyKey(`delete:${sinkID}`);
    setError("");
    setNotice("");
    try {
      await deleteLoggingSink(sinkID);
      await refreshAll();
      setNotice("Sink removed.");
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : "failed to delete sink");
    } finally {
      setBusyKey("");
    }
  }

  async function handleTestDelivery(sinkID?: string) {
    setBusyKey(`test:${sinkID ?? "all"}`);
    setError("");
    setNotice("");
    try {
      await testLoggingDelivery(sinkID);
      await new Promise((resolve) => window.setTimeout(resolve, 150));
      await refreshAll();
      setNotice(sinkID ? "Test delivery accepted for selected sink." : "Broadcast test delivery accepted.");
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : "failed to test delivery");
    } finally {
      setBusyKey("");
    }
  }

  async function handleSaveRoutes() {
    setBusyKey("routes");
    setError("");
    setNotice("");
    try {
      const payload: LogRouteRule[] = routeDrafts.map((draft) => ({
        id: draft.id,
        sink_id: draft.sink_id.trim(),
        categories: normalizeCategories(draft.categories),
        min_severity: draft.min_severity,
        enabled: draft.enabled,
      }));
      await updateLoggingRoutes(payload);
      await refreshAll();
      setNotice("Routing rules updated.");
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : "failed to update routes");
    } finally {
      setBusyKey("");
    }
  }

  if (!canView || session?.role === "readonly") {
    return null;
  }

  const enabledSinks = sinks.filter((sink) => sink.enabled).length;
  const failingSinks = status.filter((item) => item.consecutive_failures > 0 || item.last_error).length;

  return (
    <div style={s.shell}>
      <header style={s.header}>
        <div style={s.headerLeft}>
          <HeaderBrand />
          <HeaderNavMenu current="logging" role={session?.role} />
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
            <h2 style={s.heading}>Logging &amp; SIEM</h2>
            <p style={s.subtitle}>
              Manage syslog export sinks, category/severity routing, and delivery health. Recommended Wazuh profile is
              TCP/TLS with JSON payloads.
            </p>
          </div>
          <div style={s.heroStats}>
            <div style={s.statCard}>
              <div style={s.statLabel}>Configured Sinks</div>
              <div style={s.statValue}>{sinks.length}</div>
            </div>
            <div style={s.statCard}>
              <div style={s.statLabel}>Enabled</div>
              <div style={s.statValue}>{enabledSinks}</div>
            </div>
            <div style={s.statCard}>
              <div style={s.statLabel}>Queue</div>
              <div style={s.statValue}>{currentQueued}/{queueCapacity}</div>
            </div>
            <div style={s.statCard}>
              <div style={s.statLabel}>Delivery Issues</div>
              <div style={s.statValue}>{failingSinks}</div>
            </div>
          </div>
        </section>

        {!canManage && (
          <section style={s.readOnlyBanner}>
            Operator access is read-only here: sink health, routes, and redaction policy remain visible, but edits stay locked to admin users.
          </section>
        )}

        {error && <div style={s.error}>{error}</div>}
        {notice && <div style={s.notice}>{notice}</div>}

        {canManage && (
          <section style={s.card}>
            <div style={s.sectionHeader}>
              <div>
                <h3 style={s.sectionTitle}>Add Syslog Sink</h3>
                <p style={s.sectionCopy}>TLS fields are file references only. PEM contents stay outside SQLite and outside git.</p>
              </div>
              <button type="button" onClick={() => void handleCreateSink()} disabled={busyKey === "create"} style={s.primaryBtn}>
                {busyKey === "create" ? "Saving..." : "Create sink"}
              </button>
            </div>
            <div style={s.formGrid}>
              <label style={s.field}>
                <span style={s.fieldLabel}>Name</span>
                <input value={createInput.name} onChange={(e) => setCreateInput((current) => ({ ...current, name: e.target.value }))} style={s.input} />
              </label>
              <label style={s.field}>
                <span style={s.fieldLabel}>Transport</span>
                <select value={createInput.syslog.transport} onChange={(e) => setCreateInput((current) => ({ ...current, syslog: { ...current.syslog, transport: e.target.value as LogSinkInput["syslog"]["transport"] } }))} style={s.select}>
                  <option value="udp">udp</option>
                  <option value="tcp">tcp</option>
                  <option value="tls">tls</option>
                </select>
              </label>
              <label style={s.field}>
                <span style={s.fieldLabel}>Format</span>
                <select value={createInput.syslog.format} onChange={(e) => setCreateInput((current) => ({ ...current, syslog: { ...current.syslog, format: e.target.value as LogSinkInput["syslog"]["format"] } }))} style={s.select}>
                  <option value="json">json</option>
                  <option value="rfc5424">rfc5424</option>
                </select>
              </label>
              <label style={s.field}>
                <span style={s.fieldLabel}>Host</span>
                <input value={createInput.syslog.host} onChange={(e) => setCreateInput((current) => ({ ...current, syslog: { ...current.syslog, host: e.target.value } }))} style={s.input} />
              </label>
              <label style={s.field}>
                <span style={s.fieldLabel}>Port</span>
                <input type="number" value={String(createInput.syslog.port)} onChange={(e) => setCreateInput((current) => ({ ...current, syslog: { ...current.syslog, port: Number(e.target.value) || 0 } }))} style={s.input} />
              </label>
              <label style={s.field}>
                <span style={s.fieldLabel}>Facility</span>
                <input type="number" value={String(createInput.syslog.facility)} onChange={(e) => setCreateInput((current) => ({ ...current, syslog: { ...current.syslog, facility: Number(e.target.value) || 0 } }))} style={s.input} />
              </label>
              <label style={s.field}>
                <span style={s.fieldLabel}>App Name</span>
                <input value={createInput.syslog.app_name} onChange={(e) => setCreateInput((current) => ({ ...current, syslog: { ...current.syslog, app_name: e.target.value } }))} style={s.input} />
              </label>
              <label style={s.field}>
                <span style={s.fieldLabel}>Hostname Override</span>
                <input value={createInput.syslog.hostname_override ?? ""} onChange={(e) => setCreateInput((current) => ({ ...current, syslog: { ...current.syslog, hostname_override: e.target.value } }))} style={s.input} />
              </label>
              <label style={s.field}>
                <span style={s.fieldLabel}>CA Cert File</span>
                <input value={createInput.syslog.ca_cert_file ?? ""} onChange={(e) => setCreateInput((current) => ({ ...current, syslog: { ...current.syslog, ca_cert_file: e.target.value } }))} style={s.input} />
              </label>
              <label style={s.field}>
                <span style={s.fieldLabel}>Client Cert File</span>
                <input value={createInput.syslog.client_cert_file ?? ""} onChange={(e) => setCreateInput((current) => ({ ...current, syslog: { ...current.syslog, client_cert_file: e.target.value } }))} style={s.input} />
              </label>
              <label style={s.field}>
                <span style={s.fieldLabel}>Client Key File</span>
                <input value={createInput.syslog.client_key_file ?? ""} onChange={(e) => setCreateInput((current) => ({ ...current, syslog: { ...current.syslog, client_key_file: e.target.value } }))} style={s.input} />
              </label>
              <label style={{ ...s.field, ...s.checkboxField }}>
                <input type="checkbox" checked={createInput.enabled} onChange={(e) => setCreateInput((current) => ({ ...current, enabled: e.target.checked }))} />
                <span style={s.checkboxLabel}>Enabled on save</span>
              </label>
            </div>
          </section>
        )}

        <section style={s.card}>
          <div style={s.sectionHeader}>
            <div>
              <h3 style={s.sectionTitle}>Sink Inventory</h3>
              <p style={s.sectionCopy}>Generic architecture, first production sink type is syslog. Wazuh works best with TCP/TLS + JSON.</p>
            </div>
            {canManage && (
              <button type="button" onClick={() => void handleTestDelivery()} disabled={busyKey === "test:all"} style={s.secondaryBtn}>
                {busyKey === "test:all" ? "Sending..." : "Test all routes"}
              </button>
            )}
          </div>
          {loading ? (
            <p style={s.muted}>Loading logging inventory...</p>
          ) : sinks.length === 0 ? (
            <p style={s.muted}>No logging sinks are configured yet.</p>
          ) : (
            <div style={s.sinkGrid}>
              {sinks.map((sink) => {
                const sinkStatus = status.find((item) => item.sink_id === sink.id);
                return (
                  <article key={sink.id} style={s.sinkCard}>
                    <div style={s.cardTop}>
                      <div>
                        <div style={s.sessionTitle}>{sink.name}</div>
                        <div style={s.sessionMeta}>{sink.type} · {sink.syslog.transport}/{sink.syslog.format}</div>
                      </div>
                      <span style={sink.enabled ? { ...s.badge, ...s.badgeCurrent } : s.badge}>
                        {sink.enabled ? "Enabled" : "Disabled"}
                      </span>
                    </div>
                    <dl style={s.details}>
                      <div style={s.detailRow}>
                        <dt style={s.term}>Endpoint</dt>
                        <dd style={s.value}>{sink.syslog.host}:{sink.syslog.port}</dd>
                      </div>
                      <div style={s.detailRow}>
                        <dt style={s.term}>Facility</dt>
                        <dd style={s.value}>{sink.syslog.facility}</dd>
                      </div>
                      <div style={s.detailRow}>
                        <dt style={s.term}>App Name</dt>
                        <dd style={s.value}>{sink.syslog.app_name}</dd>
                      </div>
                      <div style={s.detailRow}>
                        <dt style={s.term}>Delivered</dt>
                        <dd style={s.value}>{sinkStatus?.total_delivered ?? 0}</dd>
                      </div>
                      <div style={s.detailRow}>
                        <dt style={s.term}>Failed</dt>
                        <dd style={s.value}>{sinkStatus?.total_failed ?? 0}</dd>
                      </div>
                      <div style={s.detailColumn}>
                        <dt style={s.term}>Last Error</dt>
                        <dd style={s.agentValue}>{sinkStatus?.last_error || "none"}</dd>
                      </div>
                    </dl>
                    <div style={s.actions}>
                      <button type="button" onClick={() => void handleTestDelivery(sink.id)} disabled={busyKey === `test:${sink.id}`} style={s.secondaryAction}>
                        {busyKey === `test:${sink.id}` ? "Sending..." : "Test delivery"}
                      </button>
                      {canManage && (
                        <>
                          <button type="button" onClick={() => void handleToggleSinkEnabled(sink)} disabled={busyKey === `toggle:${sink.id}`} style={s.secondaryAction}>
                            {busyKey === `toggle:${sink.id}` ? "Saving..." : sink.enabled ? "Disable" : "Enable"}
                          </button>
                          <button type="button" onClick={() => void handleDeleteSink(sink.id)} disabled={busyKey === `delete:${sink.id}`} style={s.dangerAction}>
                            {busyKey === `delete:${sink.id}` ? "Deleting..." : "Delete"}
                          </button>
                        </>
                      )}
                    </div>
                  </article>
                );
              })}
            </div>
          )}
        </section>

        <section style={s.card}>
          <div style={s.sectionHeader}>
            <div>
              <h3 style={s.sectionTitle}>Routing Rules</h3>
              <p style={s.sectionCopy}>Rules match by category and minimum severity before events enter the export queue.</p>
            </div>
            {canManage && (
              <div style={s.inlineActions}>
                <button type="button" onClick={() => setRouteDrafts((current) => [...current, { sink_id: sinks[0]?.id ?? "", categories: "system", min_severity: "info", enabled: true }])} style={s.secondaryBtn}>
                  Add rule
                </button>
                <button type="button" onClick={() => void handleSaveRoutes()} disabled={busyKey === "routes"} style={s.primaryBtn}>
                  {busyKey === "routes" ? "Saving..." : "Save routes"}
                </button>
              </div>
            )}
          </div>
          {routeDrafts.length === 0 ? (
            <p style={s.muted}>No route rules configured yet.</p>
          ) : (
            <div style={s.tableWrap}>
              <table style={s.table}>
                <thead>
                  <tr>
                    <th style={s.th}>Sink</th>
                    <th style={s.th}>Categories</th>
                    <th style={s.th}>Min Severity</th>
                    <th style={s.th}>Enabled</th>
                    {canManage && <th style={s.th}>Actions</th>}
                  </tr>
                </thead>
                <tbody>
                  {routeDrafts.map((route, index) => (
                    <tr key={route.id ?? `${route.sink_id}-${index}`}>
                      <td style={s.td}>
                        {canManage ? (
                          <select value={route.sink_id} onChange={(e) => setRouteDrafts((current) => current.map((item, itemIndex) => itemIndex === index ? { ...item, sink_id: e.target.value } : item))} style={s.selectInline}>
                            <option value="">Select sink</option>
                            {sinks.map((sink) => (
                              <option key={sink.id} value={sink.id}>{sink.name}</option>
                            ))}
                          </select>
                        ) : (
                          sinks.find((sink) => sink.id === route.sink_id)?.name ?? route.sink_id
                        )}
                      </td>
                      <td style={s.td}>
                        {canManage ? (
                          <input value={route.categories} onChange={(e) => setRouteDrafts((current) => current.map((item, itemIndex) => itemIndex === index ? { ...item, categories: e.target.value } : item))} style={s.inputInline} />
                        ) : (
                          route.categories
                        )}
                      </td>
                      <td style={s.td}>
                        {canManage ? (
                          <select value={route.min_severity} onChange={(e) => setRouteDrafts((current) => current.map((item, itemIndex) => itemIndex === index ? { ...item, min_severity: e.target.value as LogSeverity } : item))} style={s.selectInline}>
                            {allSeverities.map((severity) => (
                              <option key={severity} value={severity}>{severity}</option>
                            ))}
                          </select>
                        ) : (
                          route.min_severity
                        )}
                      </td>
                      <td style={s.td}>
                        {canManage ? (
                          <input type="checkbox" checked={route.enabled} onChange={(e) => setRouteDrafts((current) => current.map((item, itemIndex) => itemIndex === index ? { ...item, enabled: e.target.checked } : item))} />
                        ) : (
                          route.enabled ? "yes" : "no"
                        )}
                      </td>
                      {canManage && (
                        <td style={s.td}>
                          <button type="button" onClick={() => setRouteDrafts((current) => current.filter((_, itemIndex) => itemIndex !== index))} style={s.textBtn}>
                            Remove
                          </button>
                        </td>
                      )}
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
          <div style={s.ruleHint}>Available categories: {allCategories.join(", ")}</div>
        </section>

        <section style={s.card}>
          <div style={s.sectionHeader}>
            <div>
              <h3 style={s.sectionTitle}>Delivery Health</h3>
              <p style={s.sectionCopy}>Per-sink queue depth, retry counters, dead-letter retention, and the latest delivery errors mirrored from the exporter.</p>
            </div>
          </div>
          {status.length === 0 ? (
            <p style={s.muted}>No delivery status is available yet.</p>
          ) : (
            <div style={s.tableWrap}>
              <table style={s.table}>
                <thead>
                  <tr>
                    <th style={s.th}>Sink</th>
                    <th style={s.th}>Queue</th>
                    <th style={s.th}>Dropped</th>
                    <th style={s.th}>Delivered</th>
                    <th style={s.th}>Failed</th>
                    <th style={s.th}>Dead Letters</th>
                    <th style={s.th}>Last Delivered</th>
                    <th style={s.th}>Last Error</th>
                  </tr>
                </thead>
                <tbody>
                  {status.map((item) => (
                    <tr key={item.sink_id}>
                      <td style={s.td}>{item.sink_name || item.sink_id}</td>
                      <td style={s.td}>{item.queue_depth}</td>
                      <td style={s.td}>{item.dropped_events}</td>
                      <td style={s.td}>{item.total_delivered}</td>
                      <td style={s.td}>{item.total_failed}</td>
                      <td style={s.td}>{item.dead_letter_count}</td>
                      <td style={s.td}>{formatDateTime(item.last_delivered_at)}</td>
                      <td style={s.td}>{item.last_error || "none"}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </section>

        <section style={s.card}>
          <div style={s.sectionHeader}>
            <div>
              <h3 style={s.sectionTitle}>Recent Delivery Failures</h3>
              <p style={s.sectionCopy}>Latest exhausted retries retained per sink so operators can see what actually failed to leave the process.</p>
            </div>
          </div>
          {recentFailures.length === 0 ? (
            <p style={s.muted}>No persisted delivery failures right now.</p>
          ) : (
            <div style={s.tableWrap}>
              <table style={s.table}>
                <thead>
                  <tr>
                    <th style={s.th}>Occurred</th>
                    <th style={s.th}>Sink</th>
                    <th style={s.th}>Event</th>
                    <th style={s.th}>Error</th>
                  </tr>
                </thead>
                <tbody>
                  {recentFailures.map((failure) => (
                    <tr key={failure.id}>
                      <td style={s.td}>{formatDateTime(failure.occurred_at)}</td>
                      <td style={s.td}>{failure.sink_name || failure.sink_id}</td>
                      <td style={s.td}>
                        <div style={s.failureTitle}>{failure.message || failure.action || failure.category}</div>
                        <div style={s.failureMeta}>
                          {failure.category}/{failure.severity}
                          {failure.test_delivery ? " • test delivery" : ""}
                        </div>
                      </td>
                      <td style={s.td}>{failure.error_message}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </section>

        <section style={s.card}>
          <div style={s.sectionHeader}>
            <div>
              <h3 style={s.sectionTitle}>Redaction Policy</h3>
              <p style={s.sectionCopy}>These fields are masked before events enter the queue or leave the process.</p>
            </div>
          </div>
          <div style={s.redactionWrap}>
            {redactedFields.map((field) => (
              <span key={field} style={s.redactionChip}>{field}</span>
            ))}
          </div>
        </section>
      </main>
    </div>
  );
}

function routeToDraft(route: LogRouteRule): RouteDraft {
  return {
    id: route.id,
    sink_id: route.sink_id,
    categories: route.categories.join(", "),
    min_severity: route.min_severity,
    enabled: route.enabled,
  };
}

function normalizeCategories(raw: string): LogCategory[] {
  return raw
    .split(",")
    .map((item) => item.trim())
    .filter((item): item is LogCategory => allCategories.includes(item as LogCategory));
}

function uniqueStrings(values: string[]): string[] {
  return Array.from(new Set(values.filter(Boolean)));
}

function formatDateTime(value?: string): string {
  if (!value) return "never";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
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
    gridTemplateColumns: "repeat(4, minmax(120px, 1fr))",
    gap: "0.75rem",
  },
  statCard: {
    background: uiTheme.surface,
    border: `1px solid ${uiTheme.border}`,
    borderRadius: 14,
    padding: "0.9rem 1rem",
    minWidth: 130,
  },
  statLabel: {
    color: uiTheme.textMuted,
    fontSize: "0.78rem",
    textTransform: "uppercase",
    letterSpacing: 0.6,
  },
  statValue: {
    marginTop: "0.35rem",
    color: uiTheme.text,
    fontSize: "1.4rem",
    fontWeight: 700,
  },
  readOnlyBanner: {
    background: "#fff8e5",
    border: "1px solid #f0d795",
    color: "#8a5a00",
    borderRadius: 12,
    padding: "0.9rem 1rem",
    lineHeight: 1.55,
  },
  error: {
    background: "#fdecea",
    border: "1px solid #f5c2c7",
    color: "#b42318",
    borderRadius: 12,
    padding: "0.85rem 1rem",
  },
  notice: {
    background: "#edfdf3",
    border: "1px solid #abe8c6",
    color: "#067647",
    borderRadius: 12,
    padding: "0.85rem 1rem",
  },
  card: {
    background: uiTheme.surface,
    border: `1px solid ${uiTheme.border}`,
    borderRadius: 18,
    padding: "1.15rem",
    boxShadow: uiTheme.shadow,
    display: "grid",
    gap: "1rem",
  },
  sectionHeader: {
    display: "flex",
    justifyContent: "space-between",
    alignItems: "flex-start",
    gap: "1rem",
    flexWrap: "wrap",
  },
  sectionTitle: {
    margin: 0,
    color: uiTheme.text,
    fontSize: "1.05rem",
  },
  sectionCopy: {
    margin: "0.3rem 0 0",
    color: uiTheme.textMuted,
    lineHeight: 1.5,
    maxWidth: 760,
  },
  formGrid: {
    display: "grid",
    gridTemplateColumns: "repeat(auto-fit, minmax(220px, 1fr))",
    gap: "0.85rem",
  },
  field: {
    display: "grid",
    gap: "0.35rem",
  },
  fieldLabel: {
    fontSize: "0.82rem",
    color: uiTheme.textMuted,
    fontWeight: 600,
  },
  checkboxField: {
    alignItems: "center",
    gridAutoFlow: "column",
    justifyContent: "start",
    paddingTop: "1.7rem",
  },
  checkboxLabel: {
    color: uiTheme.text,
    fontSize: "0.9rem",
  },
  input: {
    width: "100%",
    padding: "0.7rem 0.85rem",
    borderRadius: 10,
    border: `1px solid ${uiTheme.border}`,
    background: uiTheme.surfaceAlt,
    color: uiTheme.text,
  },
  select: {
    width: "100%",
    padding: "0.7rem 0.85rem",
    borderRadius: 10,
    border: `1px solid ${uiTheme.border}`,
    background: uiTheme.surfaceAlt,
    color: uiTheme.text,
  },
  primaryBtn: {
    border: "none",
    borderRadius: 10,
    background: uiTheme.headerBg,
    color: uiTheme.headerText,
    padding: "0.75rem 1rem",
    fontWeight: 700,
    cursor: "pointer",
  },
  secondaryBtn: {
    border: `1px solid ${uiTheme.border}`,
    borderRadius: 10,
    background: uiTheme.surfaceAlt,
    color: uiTheme.text,
    padding: "0.7rem 0.95rem",
    fontWeight: 600,
    cursor: "pointer",
  },
  sinkGrid: {
    display: "grid",
    gridTemplateColumns: "repeat(auto-fit, minmax(280px, 1fr))",
    gap: "0.9rem",
  },
  sinkCard: {
    border: `1px solid ${uiTheme.border}`,
    borderRadius: 14,
    padding: "1rem",
    background: uiTheme.surfaceAlt,
    display: "grid",
    gap: "0.9rem",
  },
  cardTop: {
    display: "flex",
    justifyContent: "space-between",
    gap: "0.8rem",
    alignItems: "flex-start",
  },
  sessionTitle: {
    fontSize: "1rem",
    fontWeight: 700,
    color: uiTheme.text,
  },
  sessionMeta: {
    color: uiTheme.textMuted,
    marginTop: "0.2rem",
    fontSize: "0.88rem",
  },
  badge: {
    alignSelf: "flex-start",
    padding: "0.3rem 0.55rem",
    borderRadius: 999,
    border: `1px solid ${uiTheme.border}`,
    color: uiTheme.textMuted,
    fontSize: "0.76rem",
    fontWeight: 700,
  },
  badgeCurrent: {
    background: uiTheme.headerBg,
    color: uiTheme.headerText,
    borderColor: uiTheme.headerBg,
  },
  details: {
    display: "grid",
    gap: "0.5rem",
    margin: 0,
  },
  detailRow: {
    display: "flex",
    justifyContent: "space-between",
    gap: "0.8rem",
    alignItems: "baseline",
  },
  detailColumn: {
    display: "grid",
    gap: "0.25rem",
  },
  term: {
    color: uiTheme.textMuted,
    fontSize: "0.8rem",
  },
  value: {
    margin: 0,
    color: uiTheme.text,
    fontSize: "0.9rem",
  },
  agentValue: {
    margin: 0,
    color: uiTheme.text,
    fontSize: "0.82rem",
    lineHeight: 1.45,
    wordBreak: "break-word",
  },
  actions: {
    display: "flex",
    gap: "0.55rem",
    flexWrap: "wrap",
  },
  secondaryAction: {
    border: `1px solid ${uiTheme.border}`,
    borderRadius: 10,
    background: uiTheme.surface,
    color: uiTheme.text,
    padding: "0.65rem 0.85rem",
    cursor: "pointer",
    fontWeight: 600,
  },
  dangerAction: {
    border: "1px solid #f5c2c7",
    borderRadius: 10,
    background: "#fff1f3",
    color: "#b42318",
    padding: "0.65rem 0.85rem",
    cursor: "pointer",
    fontWeight: 600,
  },
  muted: {
    color: uiTheme.textMuted,
    margin: 0,
  },
  failureTitle: {
    color: uiTheme.text,
    fontWeight: 600,
  },
  failureMeta: {
    color: uiTheme.textMuted,
    fontSize: "0.85rem",
    marginTop: "0.2rem",
  },
  tableWrap: {
    overflowX: "auto",
  },
  table: {
    width: "100%",
    borderCollapse: "collapse",
  },
  th: {
    textAlign: "left",
    padding: "0.75rem",
    fontSize: "0.78rem",
    textTransform: "uppercase",
    letterSpacing: 0.5,
    color: uiTheme.textMuted,
    borderBottom: `1px solid ${uiTheme.border}`,
  },
  td: {
    padding: "0.75rem",
    color: uiTheme.text,
    borderBottom: `1px solid ${uiTheme.border}`,
    verticalAlign: "top",
  },
  inlineActions: {
    display: "flex",
    gap: "0.6rem",
    flexWrap: "wrap",
  },
  selectInline: {
    width: "100%",
    padding: "0.5rem 0.65rem",
    borderRadius: 8,
    border: `1px solid ${uiTheme.border}`,
    background: uiTheme.surfaceAlt,
    color: uiTheme.text,
  },
  inputInline: {
    width: "100%",
    padding: "0.5rem 0.65rem",
    borderRadius: 8,
    border: `1px solid ${uiTheme.border}`,
    background: uiTheme.surfaceAlt,
    color: uiTheme.text,
  },
  textBtn: {
    border: "none",
    background: "transparent",
    color: uiTheme.text,
    cursor: "pointer",
    textDecoration: "underline",
  },
  ruleHint: {
    color: uiTheme.textMuted,
    fontSize: "0.82rem",
  },
  redactionWrap: {
    display: "flex",
    gap: "0.55rem",
    flexWrap: "wrap",
  },
  redactionChip: {
    padding: "0.45rem 0.7rem",
    borderRadius: 999,
    background: uiTheme.surfaceAlt,
    border: `1px solid ${uiTheme.border}`,
    color: uiTheme.text,
    fontSize: "0.82rem",
    fontWeight: 600,
  },
};
