import { FormEvent, useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import {
  AccessPolicy,
  CreateEnrollmentTokenInput,
  EnrollmentToken,
  createEnrollmentToken,
  listAllAccessPolicies,
  listEnrollmentTokens,
  logout,
  revokeEnrollmentToken,
} from "../api";
import { useAuth } from "../AuthContext";
import { HeaderNavMenu } from "../HeaderNavMenu";
import { HeaderBrand } from "../Brand";
import { ThemeToggleButton } from "../ThemeToggleButton";
import { uiTheme } from "../uiTheme";
import { useStoredState } from "../useStoredState";
import { copyText } from "../clipboard";
import { linuxInstallCommand, windowsInstallCommand } from "../bootstrapCommands";

export function EnrollmentTokensPage() {
  const { session, setSession } = useAuth();
  const navigate = useNavigate();
  const [tokens, setTokens] = useState<EnrollmentToken[]>([]);
  const [nextCursor, setNextCursor] = useState<string | null>(null);
  const [policies, setPolicies] = useState<AccessPolicy[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadingMore, setLoadingMore] = useState(false);
  const [error, setError] = useState("");
  const [createError, setCreateError] = useState("");
  const [creating, setCreating] = useState(false);
  const [revokingID, setRevokingID] = useState<string | null>(null);
  const [createdToken, setCreatedToken] = useState<EnrollmentToken | null>(null);
  const [copyMessage, setCopyMessage] = useState("");

  const [model, setModel] = useStoredState<"A" | "B">("tokens:model", "B");
  const [scope, setScope] = useStoredState("tokens:scope", "enrollment");
  const [boundIdentity, setBoundIdentity] = useStoredState("tokens:bound-identity", "");
  const [ttlMinutes, setTTLMinutes] = useStoredState("tokens:ttl-minutes", "30");
  const [selectedPolicyIDs, setSelectedPolicyIDs] = useStoredState<string[]>("tokens:selected-policy-ids", []);
  const [serverURL, setServerURL] = useStoredState("tokens:server-url", typeof window === "undefined" ? "http://localhost:8080" : window.location.origin);
  const [linuxHostname, setLinuxHostname] = useStoredState("tokens:linux-hostname", "");
  const [linuxBinaryPath, setLinuxBinaryPath] = useStoredState("tokens:linux-binary-path", "/usr/local/bin/wiregate-agent-linux");
  const [windowsHostname, setWindowsHostname] = useStoredState("tokens:windows-hostname", "");
  const [windowsBinaryPath, setWindowsBinaryPath] = useStoredState("tokens:windows-binary-path", String.raw`C:\Program Files\Wiregate\wiregate-agent-windows.exe`);

  const canManage = session?.role === "admin" || session?.role === "operator";
  const modelBMissingIdentity = model === "B" && boundIdentity.trim() === "";

  useEffect(() => {
    let active = true;

    async function load() {
      setLoading(true);
      setError("");
      try {
        const [tokenInventory, policyInventory] = await Promise.all([
          listEnrollmentTokens(),
          listAllAccessPolicies(),
        ]);
        if (!active) return;
        setTokens(tokenInventory.tokens);
        setNextCursor(tokenInventory.next_cursor ?? null);
        setPolicies(policyInventory);
      } catch (e: unknown) {
        if (!active) return;
        setError(e instanceof Error ? e.message : "failed to load enrollment tokens");
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

  async function handleLoadMore() {
    if (!nextCursor) {
      return;
    }
    setLoadingMore(true);
    setError("");
    try {
      const page = await listEnrollmentTokens(nextCursor, 50);
      setTokens((current) => {
        const seen = new Set(current.map((token) => token.id));
        const appended = page.tokens.filter((token) => !seen.has(token.id));
        return [...current, ...appended];
      });
      setNextCursor(page.next_cursor ?? null);
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : "failed to load more enrollment tokens");
    } finally {
      setLoadingMore(false);
    }
  }

  async function handleLogout() {
    await logout();
    setSession(null);
    navigate("/login", { replace: true });
  }

  async function handleCreate(e: FormEvent) {
    e.preventDefault();
    if (!canManage) return;

    setCreating(true);
    setCreateError("");
    setCreatedToken(null);
    setCopyMessage("");

    const input: CreateEnrollmentTokenInput = {
      model,
      scope,
      access_policy_ids: selectedPolicyIDs,
    };
    if (boundIdentity.trim()) input.bound_identity = boundIdentity.trim();
    if (ttlMinutes.trim()) input.ttl_minutes = Number(ttlMinutes);

    try {
      const created = await createEnrollmentToken(input);
      setTokens((current) => [created, ...current]);
      setCreatedToken(created);
      setBoundIdentity("");
      setSelectedPolicyIDs([]);
    } catch (e: unknown) {
      setCreateError(e instanceof Error ? e.message : "failed to create enrollment token");
    } finally {
      setCreating(false);
    }
  }

  async function handleCopy(label: string, value: string) {
    try {
      await copyText(value);
      setCopyMessage(`${label} copied`);
    } catch (e: unknown) {
      setCopyMessage(e instanceof Error ? e.message : "failed to copy");
    }
  }

  async function handleRevoke(tokenID: string) {
    if (!canManage) return;
    setRevokingID(tokenID);
    setError("");
    try {
      const updated = await revokeEnrollmentToken(tokenID);
      setTokens((current) => current.map((token) => (token.id === tokenID ? updated : token)));
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : "failed to revoke token");
    } finally {
      setRevokingID(null);
    }
  }

  function togglePolicy(policyID: string) {
    setSelectedPolicyIDs((current) =>
      current.includes(policyID) ? current.filter((id) => id !== policyID) : [...current, policyID],
    );
  }

  return (
    <div style={s.shell}>
      <header style={s.header}>
        <div style={s.headerLeft}>
          <HeaderBrand />
          <HeaderNavMenu current="tokens" role={session?.role} />
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
            <h2 style={s.heading}>Enrollment Tokens</h2>
            <p style={s.subtitle}>Issue short-lived bootstrap tokens and bind them to pre-authorized access policies.</p>
          </div>
        </div>

        {canManage && (
          <section style={s.card}>
            <h3 style={s.subheading}>Create token</h3>
            <form onSubmit={handleCreate} style={s.form}>
              <select value={model} onChange={(e) => setModel(e.target.value as "A" | "B")} style={s.input}>
                <option value="B">Model B</option>
                <option value="A">Model A</option>
              </select>
              <input value={scope} onChange={(e) => setScope(e.target.value)} placeholder="Scope" style={s.input} />
              <input
                value={boundIdentity}
                onChange={(e) => setBoundIdentity(e.target.value)}
                placeholder={model === "B" ? "Bound hostname (required for B)" : "Bound identity (optional for A)"}
                style={s.input}
              />
              <p style={model === "B" ? s.infoText : s.warning}>
                {model === "B"
                  ? "Model B is the default MVP path and should be bound to the expected hostname."
                  : "Model A is less constrained and may carry higher misuse risk."}
              </p>
              <input
                value={ttlMinutes}
                onChange={(e) => setTTLMinutes(e.target.value)}
                placeholder="TTL minutes"
                inputMode="numeric"
                style={s.input}
              />
              <div style={s.policyBox}>
                <strong style={s.policyHeading}>Bind access policies</strong>
                {policies.length === 0 ? (
                  <p style={s.muted}>No access policies yet.</p>
                ) : (
                  <div style={s.policyList}>
                    {policies.map((policy) => (
                      <label key={policy.id} style={s.policyItem}>
                        <input
                          type="checkbox"
                          checked={selectedPolicyIDs.includes(policy.id)}
                          onChange={() => togglePolicy(policy.id)}
                        />
                        <span>
                          <strong>{policy.name}</strong>
                          <span style={s.policyMeta}>{policy.destinations.join(", ")}</span>
                        </span>
                      </label>
                    ))}
                  </div>
                )}
              </div>
              {createError && <p style={s.error}>{createError}</p>}
              {copyMessage && <p style={s.infoText}>{copyMessage}</p>}
              <button type="submit" disabled={creating || modelBMissingIdentity} style={s.primaryBtn}>
                {creating ? "Creating..." : modelBMissingIdentity ? "Bound hostname required" : "Create token"}
              </button>
            </form>

            {createdToken?.token && (
              <div style={s.secretCard}>
                <div style={s.secretLabel}>One-time token</div>
                <code style={s.secretValue}>{createdToken.token}</code>
                <button onClick={() => handleCopy("Raw token", createdToken.token ?? "")} style={s.secretBtn}>
                  Copy token
                </button>
                {createdToken.risk_warning && <p style={s.warning}>{createdToken.risk_warning}</p>}
                <div style={s.commandBox}>
                  <strong style={s.commandHeading}>Bootstrap snippets</strong>
                  <input
                    value={serverURL}
                    onChange={(e) => setServerURL(e.target.value)}
                    placeholder="Wiregate server URL or host"
                    style={s.commandInput}
                  />
                  <p style={s.commandHint}>If hostname/domain override is empty, the generated command will use the server host or IP.</p>
                  <div style={s.commandGrid}>
                    <div style={s.commandCard}>
                      <div style={s.commandTitle}>Linux</div>
                      <input
                        value={linuxHostname}
                        onChange={(e) => setLinuxHostname(e.target.value)}
                        placeholder="Hostname/domain override (optional)"
                        style={s.commandInput}
                      />
                      <input
                        value={linuxBinaryPath}
                        onChange={(e) => setLinuxBinaryPath(e.target.value)}
                        placeholder="Binary path"
                        style={s.commandInput}
                      />
                      <code style={s.commandText}>{linuxInstallCommand(serverURL, createdToken.token, linuxHostname, linuxBinaryPath)}</code>
                      <button
                        onClick={() => handleCopy("Linux install command", linuxInstallCommand(serverURL, createdToken.token ?? "", linuxHostname, linuxBinaryPath))}
                        style={s.secretBtn}
                      >
                        Copy Linux install command
                      </button>
                    </div>
                    <div style={s.commandCard}>
                      <div style={s.commandTitle}>Windows</div>
                      <input
                        value={windowsHostname}
                        onChange={(e) => setWindowsHostname(e.target.value)}
                        placeholder="Hostname/domain override (optional)"
                        style={s.commandInput}
                      />
                      <input
                        value={windowsBinaryPath}
                        onChange={(e) => setWindowsBinaryPath(e.target.value)}
                        placeholder="Binary path"
                        style={s.commandInput}
                      />
                      <code style={s.commandText}>{windowsInstallCommand(serverURL, createdToken.token, windowsHostname, windowsBinaryPath)}</code>
                      <button
                        onClick={() => handleCopy("Windows install command", windowsInstallCommand(serverURL, createdToken.token ?? "", windowsHostname, windowsBinaryPath))}
                        style={s.secretBtn}
                      >
                        Copy Windows install command
                      </button>
                    </div>
                  </div>
                </div>
              </div>
            )}
          </section>
        )}

        <section style={s.card}>
          <h3 style={s.subheading}>Issued tokens</h3>
          {nextCursor && !loading && !error && <p style={s.muted}>Showing the newest 50 tokens first. Load older history on demand.</p>}
          {loading ? (
            <p style={s.muted}>Loading tokens...</p>
          ) : error ? (
            <p style={s.error}>{error}</p>
          ) : tokens.length === 0 ? (
            <p style={s.muted}>No enrollment tokens issued yet.</p>
          ) : (
            <div style={s.tableWrap}>
              <table style={s.table}>
                <colgroup>
                  <col style={{ width: "14%" }} />
                  <col style={{ width: "24%" }} />
                  <col style={{ width: "22%" }} />
                  <col style={{ width: "26%" }} />
                  <col style={{ width: "14%" }} />
                </colgroup>
                <thead>
                  <tr>
                    <th style={s.th}>Token</th>
                    <th style={s.th}>Binding</th>
                    <th style={s.th}>Policies</th>
                    <th style={s.th}>Lifecycle</th>
                    <th style={s.th}>Action</th>
                  </tr>
                </thead>
                <tbody>
                  {tokens.map((token) => (
                    <tr key={token.id}>
                      <td style={s.tdToken}>
                        <div style={s.primaryCell} title={token.id}>{shortID(token.id)}</div>
                        <div style={s.secondaryCell}>{token.model} / {token.scope}</div>
                        <button onClick={() => handleCopy("Token ID", token.id)} style={s.inlineBtn}>Copy ID</button>
                      </td>
                      <td style={s.td}>
                        <span style={{ ...s.badge, background: token.status === "issued" ? "#2980b9" : "#7f8c8d" }}>{token.status}</span>
                        <div style={s.metaBlock}>
                          <div>{token.bound_identity || "unbound"}</div>
                          <div style={s.creatorLine} title={token.created_by_user_id}>creator {shortID(token.created_by_user_id)}</div>
                        </div>
                      </td>
                      <td style={s.td}>
                        {(token.access_policy_ids ?? []).length > 0 ? (
                          <div style={s.metaBlock}>
                            {(token.access_policy_ids ?? []).map((policyID) => (
                              <div key={policyID} title={policyID}>{policyName(policies, policyID)}</div>
                            ))}
                          </div>
                        ) : (
                          <span style={s.muted}>none</span>
                        )}
                      </td>
                      <td style={s.td}>
                        <div style={s.metaBlock}>
                          <div>expires {formatDate(token.expires_at)}</div>
                          <div>used {formatDate(token.used_at)}</div>
                          <div>revoked {formatDate(token.revoked_at)}</div>
                        </div>
                      </td>
                      <td style={s.td}>
                        {canManage ? (
                          <button
                            onClick={() => handleRevoke(token.id)}
                            disabled={token.status !== "issued" || revokingID === token.id}
                            style={s.secondaryBtn}
                          >
                            {revokingID === token.id ? "Revoking..." : "Revoke"}
                          </button>
                        ) : (
                          <span style={s.muted}>read-only</span>
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
                {loadingMore ? "Loading..." : "Load older tokens"}
              </button>
            </div>
          )}
        </section>
      </main>
    </div>
  );
}

function policyName(policies: AccessPolicy[], policyID: string): string {
  const match = policies.find((policy) => policy.id === policyID);
  return match ? match.name : shortID(policyID);
}

function shortID(id: string): string {
  return id.length > 12 ? id.slice(0, 8) + "..." : id;
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
  logo: { fontSize: "1.2rem", fontWeight: 700, letterSpacing: 1 },
  nav: { display: "flex", gap: "1rem" },
  navLink: { color: uiTheme.headerLink, textDecoration: "none", fontSize: "0.9rem" },
  headerRight: { display: "flex", alignItems: "center", gap: "1rem" },
  userInfo: { fontSize: "0.9rem", color: uiTheme.headerLink },
  roleBadge: { background: uiTheme.headerChipBg, padding: "2px 8px", borderRadius: 4, fontSize: "0.8rem", color: uiTheme.headerChipText },
  logoutBtn: { background: "transparent", border: `1px solid ${uiTheme.border}`, color: uiTheme.headerText, padding: "5px 14px", borderRadius: 5, cursor: "pointer", fontSize: "0.875rem" },
  page: { padding: "2rem", maxWidth: 1100, margin: "0 auto" },
  headingRow: { marginBottom: "1.5rem" },
  heading: { margin: 0, fontSize: "1.5rem", color: uiTheme.text },
  subtitle: { margin: "0.5rem 0 0", color: uiTheme.textMuted },
  card: { background: uiTheme.surface, borderRadius: 12, padding: "1.25rem", boxShadow: uiTheme.shadow, marginBottom: "1.25rem" },
  subheading: { marginTop: 0, color: uiTheme.text },
  form: { display: "grid", gap: "0.9rem" },
  input: { padding: "0.75rem 0.9rem", borderRadius: 8, border: `1px solid ${uiTheme.border}`, fontSize: "0.95rem", background: uiTheme.inputBg, color: uiTheme.inputText },
  primaryBtn: { background: "#1f6feb", color: "#fff", border: "none", borderRadius: 8, padding: "0.75rem 1rem", cursor: "pointer", fontWeight: 600 },
  secondaryBtn: { background: uiTheme.surface, color: "#b42318", border: "1px solid #f0b4af", borderRadius: 8, padding: "0.65rem 0.9rem", cursor: "pointer" },
  inlineBtn: { marginTop: "0.4rem", background: "transparent", color: uiTheme.textMuted, border: `1px solid ${uiTheme.border}`, borderRadius: 6, padding: "0.35rem 0.55rem", cursor: "pointer", fontSize: "0.8rem" },
  tableWrap: { overflowX: "auto" },
  loadMoreRow: { display: "flex", justifyContent: "center", marginTop: "1rem" },
  table: { width: "100%", borderCollapse: "collapse", minWidth: 720, tableLayout: "fixed" as const },
  th: { textAlign: "left", padding: "0.75rem", fontSize: "0.8rem", color: uiTheme.textMuted, borderBottom: `1px solid ${uiTheme.borderTableStrong}`, overflow: "hidden" },
  td: { padding: "0.9rem 0.75rem", verticalAlign: "top", borderBottom: `1px solid ${uiTheme.borderTable}`, overflow: "hidden" },
  tdToken: { padding: "0.9rem 0.75rem", verticalAlign: "top", borderBottom: `1px solid ${uiTheme.borderTable}`, overflow: "hidden", width: "14%" },
  badge: { display: "inline-block", color: "#fff", borderRadius: 999, padding: "0.2rem 0.55rem", fontSize: "0.78rem", fontWeight: 600 },
  primaryCell: { fontWeight: 600, color: uiTheme.text },
  secondaryCell: { fontSize: "0.84rem", color: uiTheme.textMuted, marginTop: "0.2rem" },
  metaBlock: { display: "grid", gap: "0.25rem", fontSize: "0.84rem", color: uiTheme.textMuted, overflow: "hidden" },
  creatorLine: { overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" as const },
  muted: { color: uiTheme.textMuted },
  error: { color: "#b42318", margin: 0 },
  infoText: { color: "#175cd3", margin: 0 },
  warning: { color: "#b54708", marginBottom: 0 },
  policyBox: { border: `1px solid ${uiTheme.borderSubtle}`, borderRadius: 10, padding: "0.9rem", background: uiTheme.surfaceAlt },
  policyHeading: { display: "block", marginBottom: "0.75rem", color: uiTheme.textSoft },
  policyList: { display: "grid", gap: "0.5rem" },
  policyItem: { display: "flex", gap: "0.75rem", alignItems: "flex-start", color: uiTheme.text },
  policyMeta: { display: "block", fontSize: "0.84rem", color: uiTheme.textMuted, marginTop: "0.2rem" },
  secretCard: { marginTop: "1rem", background: uiTheme.surfaceInverse, color: uiTheme.headerText, borderRadius: 10, padding: "1rem" },
  secretLabel: { fontSize: "0.78rem", color: uiTheme.textInverseMuted, marginBottom: "0.5rem" },
  secretValue: { display: "block", whiteSpace: "pre-wrap", wordBreak: "break-all", fontSize: "0.95rem" },
  secretBtn: { marginTop: "0.75rem", background: uiTheme.surfaceInverseAlt, color: uiTheme.headerText, border: `1px solid ${uiTheme.border}`, borderRadius: 8, padding: "0.55rem 0.8rem", cursor: "pointer" },
  commandBox: { marginTop: "1rem", borderTop: `1px solid ${uiTheme.border}`, paddingTop: "1rem" },
  commandHeading: { display: "block", marginBottom: "0.75rem", color: "#f8fafc" },
  commandGrid: { display: "grid", gridTemplateColumns: "repeat(auto-fit, minmax(280px, 1fr))", gap: "0.9rem", marginTop: "0.75rem" },
  commandCard: { background: uiTheme.surfaceInverseAlt, borderRadius: 10, padding: "0.9rem" },
  commandTitle: { fontWeight: 700, marginBottom: "0.6rem" },
  commandInput: { width: "100%", padding: "0.7rem 0.85rem", borderRadius: 8, border: `1px solid ${uiTheme.border}`, background: uiTheme.inputBgInverse, color: uiTheme.inputTextInverse, fontSize: "0.9rem", boxSizing: "border-box" as const, marginBottom: "0.75rem" },
  commandText: { display: "block", whiteSpace: "pre-wrap", wordBreak: "break-word", color: "#d1e9ff", background: uiTheme.inputBgInverse, borderRadius: 8, padding: "0.75rem", fontSize: "0.84rem" },
  commandHint: { margin: "0.65rem 0 0", color: uiTheme.textInverseMuted, fontSize: "0.8rem" },
};
