import { FormEvent, useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { createUser, deleteUser, listUsers, logout, resetUserPassword, Role, updateUser, UserRecord } from "../api";
import { useAuth } from "../AuthContext";
import { HeaderNavMenu } from "../HeaderNavMenu";
import { HeaderBrand } from "../Brand";
import { ThemeToggleButton } from "../ThemeToggleButton";
import { uiTheme } from "../uiTheme";

export function UsersPage() {
  const { session, setSession } = useAuth();
  const navigate = useNavigate();

  const [users, setUsers] = useState<UserRecord[]>([]);
  const [nextCursor, setNextCursor] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [loadingMore, setLoadingMore] = useState(false);
  const [error, setError] = useState("");

  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [role, setRole] = useState<Role>("operator");
  const [creating, setCreating] = useState(false);
  const [createError, setCreateError] = useState("");

  const [editingUserID, setEditingUserID] = useState<string | null>(null);
  const [editEmail, setEditEmail] = useState("");
  const [editRole, setEditRole] = useState<Role>("operator");
  const [editError, setEditError] = useState("");

  const [resetUserID, setResetUserID] = useState<string | null>(null);
  const [resetPassword, setResetPassword] = useState("");
  const [resetConfirm, setResetConfirm] = useState("");
  const [resetError, setResetError] = useState("");

  const [actingUserID, setActingUserID] = useState<string | null>(null);
  const [notice, setNotice] = useState("");

  function load() {
    setLoading(true);
    setError("");
    listUsers()
      .then((page) => {
        setUsers(page.users);
        setNextCursor(page.next_cursor ?? null);
      })
      .catch((e) => setError(e instanceof Error ? e.message : "failed to load users"))
      .finally(() => setLoading(false));
  }

  useEffect(load, []);

  async function handleCreate(e: FormEvent) {
    e.preventDefault();
    setCreateError("");
    setNotice("");
    setCreating(true);
    try {
      await createUser({ email, password, role });
      setEmail("");
      setPassword("");
      setRole("operator");
      setNotice("User created");
      load();
    } catch (e: unknown) {
      setCreateError(e instanceof Error ? e.message : "failed to create user");
    } finally {
      setCreating(false);
    }
  }

  function startEdit(user: UserRecord) {
    setEditingUserID(user.id);
    setEditEmail(user.email);
    setEditRole(user.role);
    setEditError("");
    setResetUserID(null);
    setNotice("");
  }

  function cancelEdit() {
    setEditingUserID(null);
    setEditEmail("");
    setEditRole("operator");
    setEditError("");
  }

  async function saveEdit(userID: string) {
    setEditError("");
    setNotice("");
    setActingUserID(userID);
    try {
      const updated = await updateUser(userID, { email: editEmail, role: editRole });
      setUsers((current) => current.map((u) => (u.id === userID ? updated : u)));
      setEditingUserID(null);
      setNotice("User updated");
    } catch (e: unknown) {
      setEditError(e instanceof Error ? e.message : "failed to update user");
    } finally {
      setActingUserID(null);
    }
  }

  function startReset(user: UserRecord) {
    setResetUserID(user.id);
    setResetPassword("");
    setResetConfirm("");
    setResetError("");
    setEditingUserID(null);
    setNotice("");
  }

  function cancelReset() {
    setResetUserID(null);
    setResetPassword("");
    setResetConfirm("");
    setResetError("");
  }

  async function submitReset(userID: string) {
    setResetError("");
    setNotice("");
    if (!resetPassword || !resetConfirm) {
      setResetError("new password and confirmation are required");
      return;
    }
    if (resetPassword !== resetConfirm) {
      setResetError("new password and confirmation must match");
      return;
    }

    setActingUserID(userID);
    try {
      await resetUserPassword(userID, resetPassword);
      setResetUserID(null);
      setResetPassword("");
      setResetConfirm("");
      setNotice("Password reset completed; user sessions revoked");
    } catch (e: unknown) {
      setResetError(e instanceof Error ? e.message : "failed to reset password");
    } finally {
      setActingUserID(null);
    }
  }

  async function handleDelete(id: string, userEmail: string) {
    if (!confirm(`Delete user ${userEmail}?`)) return;
    setNotice("");
    setActingUserID(id);
    try {
      await deleteUser(id);
      setUsers((current) => current.filter((u) => u.id !== id));
      setNotice("User deleted");
    } catch (e: unknown) {
      alert(e instanceof Error ? e.message : "failed to delete user");
    } finally {
      setActingUserID(null);
    }
  }

  async function handleLogout() {
    await logout();
    setSession(null);
    navigate("/login", { replace: true });
  }

  if (session?.role !== "admin") {
    return <div style={s.page}><p style={{ color: "#e74c3c" }}>Access denied - admin only.</p></div>;
  }

  async function handleLoadMore() {
    if (!nextCursor) {
      return;
    }
    setLoadingMore(true);
    setError("");
    try {
      const page = await listUsers(nextCursor, 50);
      setUsers((current) => {
        const seen = new Set(current.map((user) => user.id));
        const appended = page.users.filter((user) => !seen.has(user.id));
        return [...current, ...appended];
      });
      setNextCursor(page.next_cursor ?? null);
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : "failed to load more users");
    } finally {
      setLoadingMore(false);
    }
  }

  return (
    <div style={s.shell}>
      <header style={s.header}>
        <div style={s.headerLeft}>
          <HeaderBrand />
          <HeaderNavMenu current="users" role={session?.role} />
        </div>
        <div style={s.headerRight}>
          <ThemeToggleButton />
          <span style={s.userInfo}>
            {session?.email} <span style={s.roleBadge}>{session?.role}</span>
          </span>
          <button onClick={handleLogout} style={s.logoutBtn}>Logout</button>
        </div>
      </header>

      <div style={s.page}>
        <h2 style={s.heading}>Users</h2>
        {notice && <p style={s.notice}>{notice}</p>}

        <div style={s.card}>
          <h3 style={s.subheading}>Create user</h3>
          <form onSubmit={handleCreate} style={s.form}>
            <input
              type="email"
              placeholder="Email"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              required
              style={s.input}
            />
            <input
              type="password"
              placeholder="Password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              autoComplete="new-password"
              name="create-user-password"
              required
              style={s.passwordInput}
            />
            <select value={role} onChange={(e) => setRole(e.target.value as Role)} style={s.input}>
              <option value="admin">admin</option>
              <option value="operator">operator</option>
              <option value="readonly">readonly</option>
            </select>
            {createError && <p style={s.err}>{createError}</p>}
            <button type="submit" disabled={creating} style={s.btn}>
              {creating ? "Creating..." : "Create"}
            </button>
          </form>
        </div>

        <div style={s.card}>
          {nextCursor && !loading && !error && <p style={s.notice}>Showing the newest 50 users first. Load older admin history on demand.</p>}
          {loading ? (
            <p style={{ color: uiTheme.textMuted }}>Loading...</p>
          ) : error ? (
            <p style={s.err}>{error}</p>
          ) : (
            <div style={s.tableWrap}>
              <table style={s.table}>
                <thead>
                  <tr>
                    <th style={s.th}>Email</th>
                    <th style={s.th}>Role</th>
                    <th style={s.th}>Created</th>
                    <th style={s.th}>Actions</th>
                  </tr>
                </thead>
                <tbody>
                  {users.map((u) => {
                    const isEditing = editingUserID === u.id;
                    const isResetting = resetUserID === u.id;
                    const isSelf = u.id === session?.user_id;
                    const busy = actingUserID === u.id;

                    return (
                      <tr key={u.id} style={s.tr}>
                        <td style={s.td}>
                          {isEditing ? (
                            <input value={editEmail} onChange={(e) => setEditEmail(e.target.value)} style={s.inlineInput} />
                          ) : (
                            u.email
                          )}
                        </td>
                        <td style={s.td}>
                          {isEditing ? (
                            <select
                              value={editRole}
                              onChange={(e) => setEditRole(e.target.value as Role)}
                              style={s.inlineInput}
                              disabled={isSelf}
                            >
                              <option value="admin">admin</option>
                              <option value="operator">operator</option>
                              <option value="readonly">readonly</option>
                            </select>
                          ) : (
                            <span style={{ ...s.badge, background: roleColor(u.role) }}>{u.role}</span>
                          )}
                        </td>
                        <td style={s.td}>{u.created_at}</td>
                        <td style={s.td}>
                          <div style={s.actions}>
                            {isEditing ? (
                              <>
                                <button onClick={() => saveEdit(u.id)} disabled={busy} style={s.actionBtn}>Save</button>
                                <button onClick={cancelEdit} disabled={busy} style={s.actionBtn}>Cancel</button>
                              </>
                            ) : (
                              <button onClick={() => startEdit(u)} disabled={busy} style={s.actionBtn}>Edit</button>
                            )}

                            {!isSelf && (
                              <button onClick={() => startReset(u)} disabled={busy} style={s.actionBtn}>Reset password</button>
                            )}

                            {!isSelf && (
                              <button onClick={() => handleDelete(u.id, u.email)} disabled={busy} style={s.deleteBtn}>Delete</button>
                            )}
                          </div>

                          {isEditing && editError && <p style={s.errInline}>{editError}</p>}

                          {isResetting && (
                            <div style={s.resetBox}>
                              <input
                                type="password"
                                placeholder="New password"
                                value={resetPassword}
                                onChange={(e) => setResetPassword(e.target.value)}
                                autoComplete="new-password"
                                name={`reset-password-${u.id}`}
                                spellCheck={false}
                                style={s.inlinePasswordInput}
                                data-lpignore="true"
                                data-1p-ignore="true"
                              />
                              <input
                                type="password"
                                placeholder="Confirm password"
                                value={resetConfirm}
                                onChange={(e) => setResetConfirm(e.target.value)}
                                autoComplete="new-password"
                                name={`reset-password-confirm-${u.id}`}
                                spellCheck={false}
                                style={s.inlinePasswordInput}
                                data-lpignore="true"
                                data-1p-ignore="true"
                              />
                              <div style={s.actions}>
                                <button onClick={() => submitReset(u.id)} disabled={busy} style={s.actionBtn}>Apply reset</button>
                                <button onClick={cancelReset} disabled={busy} style={s.actionBtn}>Cancel</button>
                              </div>
                              {resetError && <p style={s.errInline}>{resetError}</p>}
                            </div>
                          )}
                        </td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            </div>
          )}
          {nextCursor && !loading && !error && (
            <div style={s.loadMoreRow}>
              <button type="button" onClick={handleLoadMore} disabled={loadingMore} style={s.actionBtn}>
                {loadingMore ? "Loading..." : "Load older users"}
              </button>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}

function roleColor(role: string): string {
  if (role === "admin") return "#e74c3c";
  if (role === "operator") return "#2980b9";
  return "#7f8c8d";
}

const s: Record<string, React.CSSProperties> = {
  shell: { minHeight: "100vh", background: uiTheme.pageBg, fontFamily: "system-ui, sans-serif" },
  header: {
    background: uiTheme.headerBg,
    color: uiTheme.headerText,
    padding: "0.55rem clamp(0.75rem, 3.5vw, 2rem)",
    minHeight: 56,
    display: "flex",
    alignItems: "center",
    justifyContent: "space-between",
    gap: "0.55rem 1rem",
    flexWrap: "wrap",
  },
  headerLeft: { display: "flex", alignItems: "center", gap: "0.75rem", flexWrap: "wrap" },
  logo: { fontSize: "1.2rem", fontWeight: 700, letterSpacing: 1 },
  nav: { display: "flex", gap: "1rem" },
  navLink: { color: uiTheme.headerLink, textDecoration: "none", fontSize: "0.9rem" },
  headerRight: { display: "flex", alignItems: "center", gap: "0.6rem", flexWrap: "wrap", justifyContent: "flex-end", marginLeft: "auto" },
  userInfo: { fontSize: "0.9rem", color: uiTheme.headerLink, maxWidth: "min(62vw, 340px)", overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" },
  roleBadge: { background: uiTheme.headerChipBg, padding: "2px 8px", borderRadius: 4, fontSize: "0.8rem", color: uiTheme.headerChipText },
  logoutBtn: { background: "transparent", border: `1px solid ${uiTheme.border}`, color: uiTheme.headerText, padding: "5px 14px", borderRadius: 5, cursor: "pointer", fontSize: "0.875rem" },
  page: { padding: "clamp(0.85rem, 3.5vw, 2rem)", maxWidth: 980, margin: "0 auto" },
  heading: { fontSize: "1.4rem", fontWeight: 600, color: uiTheme.text, marginBottom: "1rem" },
  subheading: { fontSize: "1rem", fontWeight: 600, marginBottom: "1rem", color: uiTheme.text },
  card: { background: uiTheme.surface, borderRadius: 8, padding: "1.5rem", boxShadow: uiTheme.shadow, marginBottom: "1.25rem" },
  form: { display: "grid", gridTemplateColumns: "repeat(auto-fit, minmax(180px, 1fr))", gap: "0.75rem", alignItems: "flex-start" },
  input: { padding: "0.5rem 0.75rem", border: `1px solid ${uiTheme.border}`, borderRadius: 5, fontSize: "0.95rem", minWidth: 0, width: "100%", background: uiTheme.inputBg, color: uiTheme.inputText },
  passwordInput: { padding: "0.5rem 0.75rem", border: `1px solid ${uiTheme.border}`, borderRadius: 5, fontSize: "0.95rem", minWidth: 0, width: "100%", background: uiTheme.inputBg, color: uiTheme.inputText },
  inlineInput: { padding: "0.42rem 0.6rem", border: `1px solid ${uiTheme.border}`, borderRadius: 5, fontSize: "0.88rem", minWidth: 150, background: uiTheme.inputBg, color: uiTheme.inputText },
  inlinePasswordInput: { padding: "0.42rem 0.6rem", border: `1px solid ${uiTheme.border}`, borderRadius: 5, fontSize: "0.88rem", minWidth: 150, background: uiTheme.inputBg, color: uiTheme.inputText },
  btn: { padding: "0.5rem 1.25rem", background: uiTheme.headerBg, color: uiTheme.headerText, border: "none", borderRadius: 5, cursor: "pointer", fontSize: "0.95rem", width: "100%" },
  tableWrap: { width: "100%", overflowX: "auto", WebkitOverflowScrolling: "touch" as const },
  loadMoreRow: { display: "flex", justifyContent: "center", marginTop: "1rem" },
  table: { width: "100%", minWidth: 720, borderCollapse: "collapse" },
  th: { textAlign: "left", fontSize: "0.8rem", color: uiTheme.textMuted, textTransform: "uppercase", padding: "0.5rem 0.75rem", borderBottom: `1px solid ${uiTheme.borderTableStrong}` },
  tr: { borderBottom: `1px solid ${uiTheme.borderTable}` },
  td: { padding: "0.75rem", fontSize: "0.9rem", color: uiTheme.text, verticalAlign: "top", wordBreak: "break-word" as const },
  badge: { color: "#fff", padding: "2px 8px", borderRadius: 4, fontSize: "0.8rem" },
  actions: { display: "flex", gap: "0.4rem", flexWrap: "wrap" },
  actionBtn: { background: "transparent", border: `1px solid ${uiTheme.border}`, color: uiTheme.text, padding: "3px 10px", borderRadius: 4, cursor: "pointer", fontSize: "0.8rem" },
  deleteBtn: { background: "transparent", border: "1px solid #e74c3c", color: "#e74c3c", padding: "3px 10px", borderRadius: 4, cursor: "pointer", fontSize: "0.8rem" },
  resetBox: { marginTop: "0.55rem", display: "grid", gap: "0.45rem", maxWidth: 360 },
  err: { color: "#c0392b", fontSize: "0.875rem", margin: 0 },
  errInline: { color: "#c0392b", fontSize: "0.8rem", marginTop: "0.4rem", marginBottom: 0 },
  notice: { color: "#1e8449", fontSize: "0.9rem", marginTop: 0, marginBottom: "1rem" },
};
