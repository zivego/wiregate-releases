import { useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { HeaderBrand } from "../Brand";
import { useAuth } from "../AuthContext";
import { HeaderNavMenu } from "../HeaderNavMenu";
import { DNSConfig, getDNSConfig, logout, updateDNSConfig } from "../api";
import { ThemeToggleButton } from "../ThemeToggleButton";
import { uiTheme } from "../uiTheme";

const emptyConfig: DNSConfig = {
  enabled: false,
  servers: [],
  search_domains: [],
};

export function DNSPage() {
  const { session, setSession } = useAuth();
  const navigate = useNavigate();
  const canManage = session?.role === "admin";
  const canView = session?.role === "admin" || session?.role === "operator";
  const [config, setConfig] = useState<DNSConfig>(emptyConfig);
  const [serverDraft, setServerDraft] = useState("");
  const [searchDomainDraft, setSearchDomainDraft] = useState("");
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");

  useEffect(() => {
    if (!canView) {
      navigate("/dashboard", { replace: true });
      return;
    }
    let active = true;

    async function load() {
      setLoading(true);
      setError("");
      try {
        const next = await getDNSConfig();
        if (!active) return;
        setConfig({
          enabled: next.enabled,
          servers: Array.isArray(next.servers) ? next.servers : [],
          search_domains: Array.isArray(next.search_domains) ? next.search_domains : [],
          updated_at: next.updated_at,
        });
      } catch (err: unknown) {
        if (!active) return;
        setError(err instanceof Error ? err.message : "failed to load dns configuration");
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
  }, [canView, navigate]);

  async function handleLogout() {
    await logout();
    setSession(null);
    navigate("/login", { replace: true });
  }

  async function handleSave() {
    setSaving(true);
    setError("");
    setNotice("");
    try {
      const updated = await updateDNSConfig({
        enabled: config.enabled,
        servers: uniqueNormalized(config.servers),
        search_domains: uniqueNormalized(config.search_domains),
      });
      setConfig(updated);
      setNotice("DNS configuration updated.");
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : "failed to update dns configuration");
    } finally {
      setSaving(false);
    }
  }

  if (!canView) {
    return null;
  }

  return (
    <div style={s.shell}>
      <header style={s.header}>
        <div style={s.headerLeft}>
          <HeaderBrand />
          <HeaderNavMenu current="dns" role={session?.role} />
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
            <h2 style={s.heading}>Managed DNS</h2>
            <p style={s.subtitle}>
              Define control-plane DNS servers and search domains that become part of the desired WireGuard config for enrolled agents.
            </p>
          </div>
          <div style={s.heroStats}>
            <div style={s.statCard}>
              <div style={s.statLabel}>Mode</div>
              <div style={s.statValue}>{config.enabled ? "Enabled" : "Disabled"}</div>
            </div>
            <div style={s.statCard}>
              <div style={s.statLabel}>Resolvers</div>
              <div style={s.statValue}>{config.servers.length}</div>
            </div>
            <div style={s.statCard}>
              <div style={s.statLabel}>Search Domains</div>
              <div style={s.statValue}>{config.search_domains.length}</div>
            </div>
          </div>
        </section>

        {error && <div style={s.error}>{error}</div>}
        {notice && <div style={s.notice}>{notice}</div>}

        <section style={s.card}>
          {loading ? (
            <p style={s.muted}>Loading DNS configuration...</p>
          ) : (
            <>
              <div style={s.sectionHeader}>
                <div>
                  <h3 style={s.sectionTitle}>Desired-State DNS Policy</h3>
                  <p style={s.sectionText}>Agents receive these settings during enrollment and later check-ins whenever a reconfigure is required.</p>
                </div>
                <label style={s.toggleRow}>
                  <span>Enabled</span>
                  <input
                    type="checkbox"
                    checked={config.enabled}
                    disabled={!canManage}
                    onChange={(e) => setConfig((current) => ({ ...current, enabled: e.target.checked }))}
                  />
                </label>
              </div>

              <div style={s.columns}>
                <div style={s.panel}>
                  <div style={s.panelTitle}>Resolvers</div>
                  <div style={s.inlineForm}>
                    <input
                      value={serverDraft}
                      disabled={!canManage}
                      onChange={(e) => setServerDraft(e.target.value)}
                      placeholder="1.1.1.1"
                      style={s.input}
                    />
                    <button
                      type="button"
                      disabled={!canManage}
                      onClick={() => {
                        const next = serverDraft.trim();
                        if (!next) return;
                        setConfig((current) => ({ ...current, servers: uniqueNormalized([...current.servers, next]) }));
                        setServerDraft("");
                      }}
                      style={s.secondaryBtn}
                    >
                      Add
                    </button>
                  </div>
                  {config.servers.length === 0 ? (
                    <p style={s.muted}>No DNS servers configured.</p>
                  ) : (
                    <div style={s.tagList}>
                      {config.servers.map((server) => (
                        <div key={server} style={s.tag}>
                          <span>{server}</span>
                          {canManage && (
                            <button type="button" onClick={() => setConfig((current) => ({ ...current, servers: current.servers.filter((value) => value !== server) }))} style={s.tagBtn}>
                              Remove
                            </button>
                          )}
                        </div>
                      ))}
                    </div>
                  )}
                </div>

                <div style={s.panel}>
                  <div style={s.panelTitle}>Search Domains</div>
                  <div style={s.inlineForm}>
                    <input
                      value={searchDomainDraft}
                      disabled={!canManage}
                      onChange={(e) => setSearchDomainDraft(e.target.value)}
                      placeholder="corp.example"
                      style={s.input}
                    />
                    <button
                      type="button"
                      disabled={!canManage}
                      onClick={() => {
                        const next = searchDomainDraft.trim();
                        if (!next) return;
                        setConfig((current) => ({ ...current, search_domains: uniqueNormalized([...current.search_domains, next]) }));
                        setSearchDomainDraft("");
                      }}
                      style={s.secondaryBtn}
                    >
                      Add
                    </button>
                  </div>
                  {config.search_domains.length === 0 ? (
                    <p style={s.muted}>No search domains configured.</p>
                  ) : (
                    <div style={s.tagList}>
                      {config.search_domains.map((domain) => (
                        <div key={domain} style={s.tag}>
                          <span>{domain}</span>
                          {canManage && (
                            <button type="button" onClick={() => setConfig((current) => ({ ...current, search_domains: current.search_domains.filter((value) => value !== domain) }))} style={s.tagBtn}>
                              Remove
                            </button>
                          )}
                        </div>
                      ))}
                    </div>
                  )}
                </div>
              </div>

              <div style={s.preview}>
                <div style={s.panelTitle}>Agent Render Preview</div>
                <code style={s.codeBlock}>{renderPreview(config)}</code>
              </div>

              <div style={s.footer}>
                <div style={s.meta}>Last updated: {formatDate(config.updated_at)}</div>
                {canManage ? (
                  <button type="button" onClick={() => void handleSave()} disabled={saving} style={s.primaryBtn}>
                    {saving ? "Saving..." : "Save DNS config"}
                  </button>
                ) : (
                  <span style={s.readonlyBadge}>Operator access is read-only.</span>
                )}
              </div>
            </>
          )}
        </section>
      </main>
    </div>
  );
}

function uniqueNormalized(values: string[]): string[] {
  const seen = new Set<string>();
  const out: string[] = [];
  for (const value of values) {
    const trimmed = value.trim();
    if (!trimmed) continue;
    const key = trimmed.toLowerCase();
    if (seen.has(key)) continue;
    seen.add(key);
    out.push(trimmed);
  }
  return out;
}

function renderPreview(config: DNSConfig): string {
  const lines = ["[Interface]", "Address = 10.77.0.23/32"];
  if (config.enabled && (config.servers.length > 0 || config.search_domains.length > 0)) {
    lines.push(`DNS = ${[...config.servers, ...config.search_domains].join(", ")}`);
  }
  lines.push("");
  lines.push("[Peer]");
  lines.push("Endpoint = vpn.example.com:55182");
  return lines.join("\n");
}

function formatDate(value?: string): string {
  if (!value) return "never";
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
    gridTemplateColumns: "repeat(3, minmax(120px, 1fr))",
    gap: "0.75rem",
  },
  statCard: {
    background: uiTheme.surface,
    border: `1px solid ${uiTheme.border}`,
    borderRadius: 14,
    padding: "0.9rem 1rem",
    minWidth: 140,
  },
  statLabel: {
    fontSize: "0.72rem",
    letterSpacing: 0.4,
    textTransform: "uppercase",
    color: uiTheme.textMuted,
  },
  statValue: {
    marginTop: 6,
    fontSize: "1.35rem",
    fontWeight: 700,
    color: uiTheme.text,
  },
  error: {
    background: "#fef2f2",
    border: "1px solid #fecaca",
    color: "#991b1b",
    borderRadius: 12,
    padding: "0.85rem 1rem",
  },
  notice: {
    background: "#ecfdf5",
    border: "1px solid #a7f3d0",
    color: "#065f46",
    borderRadius: 12,
    padding: "0.85rem 1rem",
  },
  card: {
    background: uiTheme.surface,
    border: `1px solid ${uiTheme.border}`,
    borderRadius: 18,
    padding: "1.25rem",
    display: "grid",
    gap: "1rem",
  },
  muted: {
    margin: 0,
    color: uiTheme.textMuted,
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
    fontSize: "1.05rem",
    color: uiTheme.text,
  },
  sectionText: {
    margin: "0.35rem 0 0",
    color: uiTheme.textMuted,
    maxWidth: 680,
    lineHeight: 1.5,
  },
  toggleRow: {
    display: "inline-flex",
    alignItems: "center",
    gap: "0.6rem",
    color: uiTheme.text,
    fontWeight: 600,
  },
  columns: {
    display: "grid",
    gridTemplateColumns: "repeat(auto-fit, minmax(280px, 1fr))",
    gap: "1rem",
  },
  panel: {
    border: `1px solid ${uiTheme.border}`,
    borderRadius: 14,
    padding: "1rem",
    display: "grid",
    gap: "0.9rem",
    background: uiTheme.surfaceAlt,
  },
  panelTitle: {
    fontWeight: 700,
    color: uiTheme.text,
  },
  inlineForm: {
    display: "flex",
    gap: "0.6rem",
    flexWrap: "wrap",
  },
  input: {
    flex: "1 1 180px",
    border: `1px solid ${uiTheme.border}`,
    borderRadius: 10,
    padding: "0.7rem 0.8rem",
    background: uiTheme.surface,
    color: uiTheme.text,
  },
  secondaryBtn: {
    border: `1px solid ${uiTheme.border}`,
    background: uiTheme.surface,
    color: uiTheme.text,
    borderRadius: 10,
    padding: "0.7rem 0.9rem",
    cursor: "pointer",
    fontWeight: 600,
  },
  tagList: {
    display: "flex",
    gap: "0.6rem",
    flexWrap: "wrap",
  },
  tag: {
    display: "inline-flex",
    alignItems: "center",
    gap: "0.5rem",
    background: uiTheme.surface,
    border: `1px solid ${uiTheme.border}`,
    borderRadius: 999,
    padding: "0.45rem 0.7rem",
    color: uiTheme.text,
  },
  tagBtn: {
    border: "none",
    background: "transparent",
    color: uiTheme.textMuted,
    cursor: "pointer",
    fontWeight: 600,
  },
  preview: {
    border: `1px solid ${uiTheme.border}`,
    borderRadius: 14,
    padding: "1rem",
    background: "#111827",
    display: "grid",
    gap: "0.7rem",
  },
  codeBlock: {
    whiteSpace: "pre-wrap",
    color: "#d1fae5",
    fontSize: "0.9rem",
    lineHeight: 1.6,
  },
  footer: {
    display: "flex",
    justifyContent: "space-between",
    alignItems: "center",
    gap: "1rem",
    flexWrap: "wrap",
  },
  meta: {
    color: uiTheme.textMuted,
    fontSize: "0.9rem",
  },
  primaryBtn: {
    border: "none",
    background: uiTheme.headerBg,
    color: uiTheme.headerText,
    borderRadius: 10,
    padding: "0.8rem 1rem",
    cursor: "pointer",
    fontWeight: 700,
  },
  readonlyBadge: {
    color: uiTheme.textMuted,
    fontSize: "0.9rem",
    fontWeight: 600,
  },
};
