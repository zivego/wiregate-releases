import { FormEvent, useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import {
  AccessPolicy,
  AgentInventory,
  AuditEvent,
  PolicySimulationResult,
  TrafficMode,
  assignAccessPolicy,
  createAccessPolicy,
  listAccessPolicies,
  listAllAgents,
  listAuditEvents,
  logout,
  simulateAccessPolicy,
  unassignAccessPolicy,
  updateAccessPolicy,
} from "../api";
import { useAuth } from "../AuthContext";
import { HeaderNavMenu } from "../HeaderNavMenu";
import { HeaderBrand } from "../Brand";
import { ThemeToggleButton } from "../ThemeToggleButton";
import { uiTheme } from "../uiTheme";
import { useStoredState } from "../useStoredState";
import { copyText } from "../clipboard";

export function AccessPoliciesPage() {
  const { session, setSession } = useAuth();
  const navigate = useNavigate();
  const [policies, setPolicies] = useState<AccessPolicy[]>([]);
  const [nextCursor, setNextCursor] = useState<string | null>(null);
  const [agents, setAgents] = useState<AgentInventory[]>([]);
  const [assignmentEvents, setAssignmentEvents] = useState<AuditEvent[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadingMore, setLoadingMore] = useState(false);
  const [error, setError] = useState("");
  const [saving, setSaving] = useState(false);
  const [assigningPolicyID, setAssigningPolicyID] = useState<string | null>(null);
  const [editingPolicyID, setEditingPolicyID] = useState<string | null>(null);
  const [assignmentSelections, setAssignmentSelections] = useState<Record<string, string>>({});
  const [assignmentMessages, setAssignmentMessages] = useState<Record<string, string>>({});
  const [copyMessage, setCopyMessage] = useState("");
  const [simulatorAgentID, setSimulatorAgentID] = useState("");
  const [simulatorOverride, setSimulatorOverride] = useState<"auto" | "inherit" | "standard" | "full_tunnel">("auto");
  const [simulatorPolicyIDs, setSimulatorPolicyIDs] = useState<string[]>([]);
  const [simulatorBusy, setSimulatorBusy] = useState(false);
  const [simulatorResult, setSimulatorResult] = useState<PolicySimulationResult | null>(null);
  const [simulatorError, setSimulatorError] = useState("");

  const [name, setName] = useStoredState("policies:draft-name", "");
  const [description, setDescription] = useStoredState("policies:draft-description", "");
  const [destinationsText, setDestinationsText] = useStoredState("policies:draft-destinations", "");
  const [trafficMode, setTrafficMode] = useStoredState<TrafficMode>("policies:draft-traffic-mode", "standard");

  const canManage = session?.role === "admin" || session?.role === "operator";

  useEffect(() => {
    let active = true;

    async function load() {
      setLoading(true);
      setError("");
      try {
        const [policyInventory, agentInventory, auditInventory] = await Promise.all([
          listAccessPolicies(),
          listAllAgents(),
          listAuditEvents({ resource_type: "access_policy", limit: 20 }),
        ]);
        if (!active) return;
        setPolicies(policyInventory.policies);
        setNextCursor(policyInventory.next_cursor ?? null);
        setAgents(agentInventory);
        setAssignmentEvents(auditInventory.events.filter((event) => event.action === "access_policy.assign" || event.action === "access_policy.unassign"));
      } catch (e: unknown) {
        if (!active) return;
        setError(e instanceof Error ? e.message : "failed to load access policies");
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
      const page = await listAccessPolicies(nextCursor, 50);
      setPolicies((current) => {
        const seen = new Set(current.map((policy) => policy.id));
        const appended = page.policies.filter((policy) => !seen.has(policy.id));
        return [...current, ...appended];
      });
      setNextCursor(page.next_cursor ?? null);
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : "failed to load more access policies");
    } finally {
      setLoadingMore(false);
    }
  }

  useEffect(() => {
    if (!simulatorAgentID) {
      return;
    }
    const assigned = policies
      .filter((policy) => policy.assigned_agent_ids.includes(simulatorAgentID))
      .map((policy) => policy.id);
    setSimulatorPolicyIDs(assigned);
  }, [policies, simulatorAgentID]);

  async function handleLogout() {
    await logout();
    setSession(null);
    navigate("/login", { replace: true });
  }

  function resetForm() {
    setEditingPolicyID(null);
    setName("");
    setDescription("");
    setDestinationsText("");
    setTrafficMode("standard");
  }

  function parseDestinations(): string[] {
    return destinationsText
      .split(/\r?\n|,/)
      .map((item) => item.trim())
      .filter(Boolean);
  }

  async function handleSave(e: FormEvent) {
    e.preventDefault();
    if (!canManage) return;
    setSaving(true);
    setError("");
    try {
      const payload = {
        name: name.trim(),
        description: description.trim(),
        destinations: parseDestinations(),
        traffic_mode: trafficMode,
      };
      const saved = editingPolicyID
        ? await updateAccessPolicy(editingPolicyID, payload)
        : await createAccessPolicy(payload);
      setPolicies((current) => {
        if (editingPolicyID) {
          return current.map((policy) => (policy.id === editingPolicyID ? saved : policy));
        }
        return [saved, ...current];
      });
      resetForm();
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : "failed to save access policy");
    } finally {
      setSaving(false);
    }
  }

  async function handleAssign(policyID: string) {
    const agentID = assignmentSelections[policyID];
    if (!agentID) return;
    const currentPolicy = policies.find((policy) => policy.id === policyID);
    if (!currentPolicy) return;

    const isAssigned = currentPolicy.assigned_agent_ids.includes(agentID);
    setAssigningPolicyID(policyID);
    setError("");
    setCopyMessage("");
    try {
      if (isAssigned) {
        await unassignAccessPolicy(policyID, agentID);
      } else {
        await assignAccessPolicy(policyID, agentID);
      }
      const assignedAgent = agents.find((agent) => agent.id === agentID);
      const createdAt = new Date().toISOString();
      setPolicies((current) =>
        current.map((policy) => {
          if (policy.id !== policyID) return policy;
          if (isAssigned) {
            return {
              ...policy,
              assigned_agent_ids: policy.assigned_agent_ids.filter((id) => id !== agentID),
            };
          }
          if (policy.assigned_agent_ids.includes(agentID)) {
            return policy;
          }
          return {
            ...policy,
            assigned_agent_ids: [...policy.assigned_agent_ids, agentID],
          };
        }),
      );
      setAssignmentMessages((current) => ({
        ...current,
        [policyID]: assignedAgent
          ? isAssigned
            ? `Removed from ${assignedAgent.hostname} (${assignedAgent.platform})`
            : `Assigned to ${assignedAgent.hostname} (${assignedAgent.platform})`
          : isAssigned
            ? `Removed from ${agentID}`
            : `Assigned to ${agentID}`,
      }));
      setAssignmentEvents((current) => [
        {
          id: `local-${policyID}-${agentID}-${createdAt}`,
          action: isAssigned ? "access_policy.unassign" : "access_policy.assign",
          actor_user_id: session?.user_id,
          resource_type: "access_policy",
          resource_id: policyID,
          result: "success",
          created_at: createdAt,
          metadata: { agent_id: agentID, source: "ui-local" },
        },
        ...current,
      ]);
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : isAssigned ? "failed to remove access policy assignment" : "failed to assign access policy");
    } finally {
      setAssigningPolicyID(null);
    }
  }

  function startEdit(policy: AccessPolicy) {
    setEditingPolicyID(policy.id);
    setName(policy.name);
    setDescription(policy.description ?? "");
    setDestinationsText(policy.destinations.join("\n"));
    setTrafficMode(policy.traffic_mode);
  }

  async function handleCopy(label: string, value: string) {
    try {
      await copyText(value);
      setCopyMessage(`${label} copied`);
    } catch (e: unknown) {
      setCopyMessage(e instanceof Error ? e.message : "failed to copy");
    }
  }

  async function handleSimulate() {
    setSimulatorBusy(true);
    setSimulatorError("");
    try {
      const result = await simulateAccessPolicy({
        agent_id: simulatorAgentID || undefined,
        policy_ids: simulatorPolicyIDs,
        traffic_mode_override: simulatorOverride === "auto" ? undefined : simulatorOverride,
      });
      setSimulatorResult(result);
    } catch (e: unknown) {
      setSimulatorError(e instanceof Error ? e.message : "failed to simulate policy intent");
    } finally {
      setSimulatorBusy(false);
    }
  }

  return (
    <div style={s.shell}>
      <header style={s.header}>
        <div style={s.headerLeft}>
          <HeaderBrand />
          <HeaderNavMenu current="policies" role={session?.role} />
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
            <h2 style={s.heading}>Access Policies</h2>
            <p style={s.subtitle}>Server-authoritative destination sets and explicit assignment of those rights onto enrolled agents.</p>
          </div>
        </div>

        {canManage && (
          <section style={s.card}>
            <h3 style={s.subheading}>{editingPolicyID ? "Edit policy" : "Create policy"}</h3>
            <form onSubmit={handleSave} style={s.form}>
              <input value={name} onChange={(e) => setName(e.target.value)} placeholder="Policy name" style={s.input} />
              <input value={description} onChange={(e) => setDescription(e.target.value)} placeholder="Description" style={s.input} />
              <textarea
                value={destinationsText}
                onChange={(e) => setDestinationsText(e.target.value)}
                placeholder="Destinations, one CIDR per line or comma-separated"
                style={s.textarea}
                rows={5}
              />
              <select value={trafficMode} onChange={(e) => setTrafficMode(e.target.value as TrafficMode)} style={s.input}>
                <option value="standard">Standard (policy CIDRs only)</option>
                <option value="full_tunnel">Full tunnel (0.0.0.0/0)</option>
              </select>
              <p style={s.infoText}>Use canonical CIDRs. The backend will normalize order before rendering peer intent.</p>
              {error && <p style={s.error}>{error}</p>}
              {copyMessage && <p style={s.infoText}>{copyMessage}</p>}
              <div style={s.formActions}>
                <button type="submit" disabled={saving} style={s.primaryBtn}>
                  {saving ? "Saving..." : editingPolicyID ? "Update policy" : "Create policy"}
                </button>
                {editingPolicyID && (
                  <button type="button" onClick={resetForm} style={s.secondaryBtn}>
                    Cancel edit
                  </button>
                )}
              </div>
            </form>
          </section>
        )}

        <section style={s.card}>
          <h3 style={s.subheading}>Policies</h3>
          {nextCursor && !loading && !error && <p style={s.muted}>Showing the newest 50 policies first. Load older policy history on demand.</p>}
          {loading ? (
            <p style={s.muted}>Loading policies...</p>
          ) : error ? (
            <p style={s.error}>{error}</p>
          ) : policies.length === 0 ? (
            <p style={s.muted}>No access policies created yet.</p>
          ) : (
            <div style={s.tableWrap}>
              <table style={s.table}>
                <thead>
                  <tr>
                    <th style={s.th}>Policy</th>
                    <th style={s.th}>Destinations</th>
                    <th style={s.th}>Traffic mode</th>
                    <th style={s.th}>Updated</th>
                    <th style={s.th}>Assign</th>
                    <th style={s.th}>Edit</th>
                  </tr>
                </thead>
                <tbody>
                  {policies.map((policy) => (
                    <tr key={policy.id}>
                      <td style={s.td}>
                        <div style={s.primaryCell}>{policy.name}</div>
                        <div style={s.secondaryCell}>{policy.id}</div>
                        <button onClick={() => handleCopy("Policy ID", policy.id)} style={s.inlineBtn}>Copy ID</button>
                        {policy.description && <div style={s.metaBlock}>{policy.description}</div>}
                      </td>
                      <td style={s.td}>
                        <div style={s.destinationList}>
                          {policy.destinations.map((destination) => (
                            <span key={destination} style={s.destinationChip}>{destination}</span>
                          ))}
                        </div>
                      </td>
                      <td style={s.td}>
                        <span style={policy.traffic_mode === "full_tunnel" ? s.modeFull : s.modeStandard}>
                          {policy.traffic_mode === "full_tunnel" ? "full tunnel" : "standard"}
                        </span>
                      </td>
                      <td style={s.td}>{formatDate(policy.updated_at)}</td>
                      <td style={s.td}>
                        {canManage ? (
                          <div style={s.assignBox}>
                            <select
                              value={assignmentSelections[policy.id] ?? ""}
                              onChange={(e) => setAssignmentSelections((current) => ({ ...current, [policy.id]: e.target.value }))}
                              style={s.input}
                            >
                              <option value="">Select agent</option>
                              {agents.map((agent) => (
                                <option key={agent.id} value={agent.id}>
                                  {agent.hostname} ({agent.platform})
                                  {policy.assigned_agent_ids.includes(agent.id) ? " ✓ assigned" : ""}
                                </option>
                              ))}
                            </select>
                            <button
                              onClick={() => handleAssign(policy.id)}
                              disabled={!assignmentSelections[policy.id] || assigningPolicyID === policy.id}
                              style={s.primaryBtn}
                            >
                              {assigningPolicyID === policy.id
                                ? policy.assigned_agent_ids.includes(assignmentSelections[policy.id] ?? "")
                                  ? "Removing..."
                                  : "Assigning..."
                                : policy.assigned_agent_ids.includes(assignmentSelections[policy.id] ?? "")
                                  ? "Remove"
                                  : "Assign"}
                            </button>
                            {assignmentMessages[policy.id] && <span style={s.assignmentNote}>{assignmentMessages[policy.id]}</span>}
                          </div>
                        ) : (
                          <span style={s.muted}>read-only</span>
                        )}
                      </td>
                      <td style={s.td}>
                        {canManage ? (
                          <button onClick={() => startEdit(policy)} style={s.secondaryBtn}>
                            Edit
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
              <button type="button" onClick={handleLoadMore} disabled={loadingMore} style={s.secondaryBtn}>
                {loadingMore ? "Loading..." : "Load older policies"}
              </button>
            </div>
          )}
        </section>

        <section style={s.card}>
          <div style={s.simulatorHeader}>
            <div>
              <h3 style={s.subheading}>Policy Simulator</h3>
              <p style={s.muted}>
                Preview the exact rendered intent path that drives `AllowedIPs`, using the same backend logic as real peer rendering.
              </p>
            </div>
            <button type="button" onClick={() => void handleSimulate()} disabled={simulatorBusy} style={s.primaryBtn}>
              {simulatorBusy ? "Simulating..." : "Run simulation"}
            </button>
          </div>

          <div style={s.simulatorGrid}>
            <label style={s.field}>
              <span style={s.fieldLabel}>Agent context</span>
              <select value={simulatorAgentID} onChange={(e) => setSimulatorAgentID(e.target.value)} style={s.input}>
                <option value="">No agent context</option>
                {agents.map((agent) => (
                  <option key={agent.id} value={agent.id}>
                    {agent.hostname} ({agent.platform})
                  </option>
                ))}
              </select>
            </label>
            <label style={s.field}>
              <span style={s.fieldLabel}>Traffic override</span>
              <select value={simulatorOverride} onChange={(e) => setSimulatorOverride(e.target.value as "auto" | "inherit" | "standard" | "full_tunnel")} style={s.input}>
                <option value="auto">Auto / current agent behavior</option>
                <option value="inherit">Inherit from selected policies</option>
                <option value="standard">Force standard</option>
                <option value="full_tunnel">Force full tunnel</option>
              </select>
            </label>
          </div>

          <div style={s.simulatorPolicies}>
            {policies.map((policy) => {
              const checked = simulatorPolicyIDs.includes(policy.id);
              return (
                <label key={policy.id} style={checked ? { ...s.policyToggle, ...s.policyToggleActive } : s.policyToggle}>
                  <input
                    type="checkbox"
                    checked={checked}
                    onChange={(e) => setSimulatorPolicyIDs((current) => e.target.checked ? [...current, policy.id] : current.filter((id) => id !== policy.id))}
                  />
                  <span style={s.policyToggleText}>
                    <strong>{policy.name}</strong>
                    <span>{policy.traffic_mode}</span>
                  </span>
                </label>
              );
            })}
          </div>

          {simulatorError && <p style={s.error}>{simulatorError}</p>}

          {simulatorResult && (
            <div style={s.simulatorResult}>
              <div style={s.resultStat}>
                <span style={s.resultLabel}>Effective mode</span>
                <span style={simulatorResult.effective_traffic_mode === "full_tunnel" ? s.modeFull : s.modeStandard}>
                  {simulatorResult.effective_traffic_mode}
                </span>
              </div>
              <div style={s.resultStat}>
                <span style={s.resultLabel}>Route profile</span>
                <span style={simulatorResult.route_profile === "full_tunnel" ? s.modeFull : s.modeStandard}>{simulatorResult.route_profile}</span>
              </div>
              <div style={s.resultBlock}>
                <div style={s.resultLabel}>Allowed IPs</div>
                <div style={s.destinationList}>
                  {simulatorResult.allowed_ips.length === 0 ? (
                    <span style={s.muted}>none</span>
                  ) : (
                    simulatorResult.allowed_ips.map((item) => (
                      <span key={item} style={s.destinationChip}>{item}</span>
                    ))
                  )}
                </div>
              </div>
              <div style={s.resultBlock}>
                <div style={s.resultLabel}>Destinations</div>
                <div style={s.destinationList}>
                  {simulatorResult.destinations.length === 0 ? (
                    <span style={s.muted}>none</span>
                  ) : (
                    simulatorResult.destinations.map((item) => (
                      <span key={item} style={s.destinationChip}>{item}</span>
                    ))
                  )}
                </div>
              </div>
            </div>
          )}
        </section>

        <section style={s.card}>
          <h3 style={s.subheading}>Recent policy changes</h3>
          {assignmentEvents.length === 0 ? (
            <p style={s.muted}>No recent policy assignment events.</p>
          ) : (
            <div style={s.assignmentFeed}>
              {assignmentEvents.map((event) => {
                const policy = policies.find((item) => item.id === event.resource_id);
                const agentID = typeof event.metadata?.agent_id === "string" ? event.metadata.agent_id : "";
                const agent = agents.find((item) => item.id === agentID);
                const verb = event.action === "access_policy.unassign" ? "removed from" : "assigned to";
                return (
                    <div key={event.id} style={s.assignmentEvent}>
                      <div style={s.assignmentEventTitle}>
                        {policy?.name ?? event.resource_id ?? "unknown policy"} {verb} {(agent?.hostname ?? agentID) || "unknown agent"}
                      </div>
                    <div style={s.assignmentEventMeta}>
                      {formatDate(event.created_at)} · actor {event.actor_user_id ?? "system"} · {agent?.platform ?? "unknown platform"}
                    </div>
                  </div>
                );
              })}
            </div>
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
  logo: { fontSize: "1.2rem", fontWeight: 700, letterSpacing: 1 },
  nav: { display: "flex", gap: "1rem" },
  navLink: { color: uiTheme.headerLink, textDecoration: "none", fontSize: "0.9rem" },
  headerRight: { display: "flex", alignItems: "center", gap: "1rem" },
  userInfo: { fontSize: "0.9rem", color: uiTheme.headerLink },
  roleBadge: { background: uiTheme.headerChipBg, padding: "2px 8px", borderRadius: 4, fontSize: "0.8rem", color: uiTheme.headerChipText },
  logoutBtn: { background: "transparent", border: `1px solid ${uiTheme.border}`, color: uiTheme.headerText, padding: "5px 14px", borderRadius: 5, cursor: "pointer", fontSize: "0.875rem" },
  page: { padding: "2rem", maxWidth: 1150, margin: "0 auto" },
  headingRow: { marginBottom: "1.5rem" },
  heading: { margin: 0, fontSize: "1.5rem", color: uiTheme.text },
  subtitle: { margin: "0.5rem 0 0", color: uiTheme.textMuted },
  card: { background: uiTheme.surface, borderRadius: 12, padding: "1.25rem", boxShadow: uiTheme.shadow, marginBottom: "1.25rem" },
  subheading: { marginTop: 0, color: uiTheme.text },
  field: { display: "grid", gap: "0.35rem" },
  fieldLabel: { fontSize: "0.82rem", color: uiTheme.textMuted },
  form: { display: "grid", gap: "0.9rem" },
  input: { padding: "0.75rem 0.9rem", borderRadius: 8, border: `1px solid ${uiTheme.border}`, fontSize: "0.95rem", background: uiTheme.inputBg, color: uiTheme.inputText },
  textarea: { padding: "0.75rem 0.9rem", borderRadius: 8, border: `1px solid ${uiTheme.border}`, fontSize: "0.95rem", resize: "vertical", background: uiTheme.inputBg, color: uiTheme.inputText },
  formActions: { display: "flex", gap: "0.75rem", flexWrap: "wrap" },
  simulatorHeader: { display: "flex", justifyContent: "space-between", alignItems: "flex-start", gap: "1rem", flexWrap: "wrap", marginBottom: "1rem" },
  simulatorGrid: { display: "grid", gridTemplateColumns: "repeat(auto-fit, minmax(240px, 1fr))", gap: "0.85rem", marginBottom: "1rem" },
  simulatorPolicies: { display: "grid", gridTemplateColumns: "repeat(auto-fit, minmax(210px, 1fr))", gap: "0.7rem", marginBottom: "1rem" },
  policyToggle: { display: "flex", gap: "0.7rem", alignItems: "flex-start", border: `1px solid ${uiTheme.border}`, borderRadius: 10, padding: "0.75rem 0.85rem", background: uiTheme.surfaceAlt, cursor: "pointer" },
  policyToggleActive: { borderColor: "#60a5fa", background: "#eff6ff" },
  policyToggleText: { display: "grid", gap: "0.2rem", color: uiTheme.text, fontSize: "0.88rem" },
  simulatorResult: { display: "grid", gap: "0.9rem", border: `1px solid ${uiTheme.borderSubtle}`, borderRadius: 12, padding: "1rem", background: uiTheme.surfaceAlt },
  resultStat: { display: "flex", alignItems: "center", gap: "0.6rem", flexWrap: "wrap" },
  resultLabel: { fontSize: "0.82rem", color: uiTheme.textMuted, fontWeight: 600 },
  resultBlock: { display: "grid", gap: "0.45rem" },
  primaryBtn: { background: "#1f6feb", color: "#fff", border: "none", borderRadius: 8, padding: "0.7rem 1rem", cursor: "pointer", fontWeight: 600 },
  secondaryBtn: { background: uiTheme.surface, color: uiTheme.textSoft, border: `1px solid ${uiTheme.border}`, borderRadius: 8, padding: "0.7rem 1rem", cursor: "pointer" },
  inlineBtn: { marginTop: "0.4rem", background: "transparent", color: uiTheme.textMuted, border: `1px solid ${uiTheme.border}`, borderRadius: 6, padding: "0.35rem 0.55rem", cursor: "pointer", fontSize: "0.8rem" },
  tableWrap: { overflowX: "auto" },
  loadMoreRow: { display: "flex", justifyContent: "center", marginTop: "1rem" },
  table: { width: "100%", borderCollapse: "collapse" },
  th: { textAlign: "left", padding: "0.75rem", fontSize: "0.8rem", color: uiTheme.textMuted, borderBottom: `1px solid ${uiTheme.borderTableStrong}` },
  td: { padding: "0.9rem 0.75rem", verticalAlign: "top", borderBottom: `1px solid ${uiTheme.borderTable}` },
  primaryCell: { fontWeight: 600, color: uiTheme.text },
  secondaryCell: { fontSize: "0.84rem", color: uiTheme.textMuted, marginTop: "0.2rem" },
  metaBlock: { fontSize: "0.86rem", color: uiTheme.textMuted, marginTop: "0.45rem" },
  destinationList: { display: "flex", flexWrap: "wrap", gap: "0.4rem" },
  destinationChip: { background: "#edf2ff", color: "#1d4ed8", borderRadius: 999, padding: "0.25rem 0.6rem", fontSize: "0.82rem", fontWeight: 600 },
  modeStandard: { display: "inline-block", background: "#e0f2fe", color: "#075985", borderRadius: 999, padding: "0.25rem 0.6rem", fontSize: "0.82rem", fontWeight: 600 },
  modeFull: { display: "inline-block", background: "#fff7ed", color: "#9a3412", borderRadius: 999, padding: "0.25rem 0.6rem", fontSize: "0.82rem", fontWeight: 600 },
  assignBox: { display: "grid", gap: "0.55rem", minWidth: 220 },
  assignmentNote: { fontSize: "0.82rem", color: "#175cd3" },
  assignmentFeed: { display: "grid", gap: "0.75rem" },
  assignmentEvent: { border: `1px solid ${uiTheme.borderSubtle}`, borderRadius: 10, padding: "0.85rem 0.95rem", background: uiTheme.surfaceAlt },
  assignmentEventTitle: { fontWeight: 600, color: uiTheme.text },
  assignmentEventMeta: { marginTop: "0.3rem", fontSize: "0.84rem", color: uiTheme.textMuted },
  muted: { color: uiTheme.textMuted },
  error: { color: "#b42318", margin: 0 },
  infoText: { color: "#175cd3", margin: 0 },
};
