import { FormEvent, useState } from "react";
import { useLocation, useNavigate } from "react-router-dom";
import { changeOwnPassword, logout } from "../api";
import { useAuth } from "../AuthContext";
import { HeaderNavMenu } from "../HeaderNavMenu";
import { HeaderBrand } from "../Brand";
import { ThemeToggleButton } from "../ThemeToggleButton";
import { uiTheme } from "../uiTheme";

export function AccountPage() {
  const { session, setSession } = useAuth();
  const navigate = useNavigate();
  const location = useLocation();
  const passwordChangeRequired = session?.must_change_password === true;
  const notice = typeof (location.state as { notice?: unknown } | null)?.notice === "string"
    ? ((location.state as { notice?: string }).notice ?? "")
    : "";

  const [currentPassword, setCurrentPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState("");

  async function handleLogout() {
    await logout();
    setSession(null);
    navigate("/login", { replace: true });
  }

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    setError("");

    if (!currentPassword || !newPassword || !confirmPassword) {
      setError("all password fields are required");
      return;
    }
    if (newPassword !== confirmPassword) {
      setError("new password and confirmation must match");
      return;
    }

    setSubmitting(true);
    try {
      await changeOwnPassword(currentPassword, newPassword);
      setSession(null);
      navigate("/login", {
        replace: true,
        state: { notice: "Password changed. Sign in again." },
      });
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : "failed to change password");
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div style={s.shell}>
      <header style={s.header}>
        <div style={s.headerLeft}>
          <HeaderBrand />
          <HeaderNavMenu current="account" role={session?.role} restricted={passwordChangeRequired} />
        </div>
        <div style={s.headerRight}>
          {!passwordChangeRequired && <ThemeToggleButton />}
          <span style={s.userInfo}>
            {session?.email} <span style={s.roleBadge}>{session?.role}</span>
          </span>
          <button onClick={handleLogout} style={s.logoutBtn}>Logout</button>
        </div>
      </header>

      <main style={s.page}>
        <h2 style={s.heading}>Account</h2>
        {notice && <p style={s.notice}>{notice}</p>}
        {passwordChangeRequired ? (
          <p style={s.warning}>Password change required. Other sections are locked until you update your password.</p>
        ) : (
          <p style={s.subtitle}>Change your own password. After success, all your active sessions will be revoked.</p>
        )}

        <section style={s.card}>
          <form onSubmit={handleSubmit} style={s.form}>
            <label style={s.field}>
              <span style={s.fieldLabel}>Current password</span>
              <input
                type="password"
                value={currentPassword}
                onChange={(e) => setCurrentPassword(e.target.value)}
                autoComplete="current-password"
                name="current-password"
                style={s.passwordInput}
                required
              />
            </label>
            <label style={s.field}>
              <span style={s.fieldLabel}>New password</span>
              <input
                type="password"
                value={newPassword}
                onChange={(e) => setNewPassword(e.target.value)}
                autoComplete="new-password"
                name="new-password"
                style={s.passwordInput}
                required
              />
            </label>
            <label style={s.field}>
              <span style={s.fieldLabel}>Confirm new password</span>
              <input
                type="password"
                value={confirmPassword}
                onChange={(e) => setConfirmPassword(e.target.value)}
                autoComplete="new-password"
                name="confirm-new-password"
                style={s.passwordInput}
                required
              />
            </label>

            {error && <p style={s.error}>{error}</p>}

            <button type="submit" disabled={submitting} style={s.submitBtn}>
              {submitting ? "Updating..." : "Change password"}
            </button>
          </form>
        </section>
      </main>
    </div>
  );
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
  page: { padding: "2rem", maxWidth: 900, margin: "0 auto" },
  heading: { margin: 0, color: uiTheme.text, fontSize: "1.4rem" },
  subtitle: { marginTop: "0.5rem", color: uiTheme.textMuted, fontSize: "0.95rem" },
  notice: { marginTop: "0.5rem", marginBottom: 0, color: "#1e8449", fontSize: "0.9rem" },
  warning: { marginTop: "0.5rem", marginBottom: 0, color: "#b9770e", fontSize: "0.95rem", fontWeight: 600 },
  card: { marginTop: "1.5rem", background: uiTheme.surface, borderRadius: 8, padding: "1.5rem", boxShadow: uiTheme.shadow },
  form: { display: "grid", gap: "1rem", maxWidth: 480 },
  field: { display: "grid", gap: "0.4rem" },
  fieldLabel: { fontSize: "0.9rem", color: uiTheme.text },
  input: { padding: "0.55rem 0.75rem", border: `1px solid ${uiTheme.border}`, borderRadius: 5, fontSize: "0.95rem", background: uiTheme.inputBg, color: uiTheme.inputText },
  passwordInput: { padding: "0.55rem 0.75rem", border: `1px solid ${uiTheme.border}`, borderRadius: 5, fontSize: "0.95rem", background: uiTheme.inputBg, color: uiTheme.inputText },
  submitBtn: { marginTop: "0.3rem", width: "fit-content", padding: "0.6rem 1rem", border: "none", borderRadius: 5, background: uiTheme.headerBg, color: uiTheme.headerText, cursor: "pointer" },
  error: { margin: 0, color: "#c0392b", fontSize: "0.9rem" },
};
