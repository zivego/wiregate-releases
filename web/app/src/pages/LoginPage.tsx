import { FormEvent, useEffect, useState } from "react";
import { useLocation, useNavigate } from "react-router-dom";
import { listAuthProviders, login } from "../api";
import { useAuth } from "../AuthContext";
import { LoginBrand } from "../Brand";
import { uiTheme } from "../uiTheme";

export function LoginPage() {
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [ssoEnabled, setSsoEnabled] = useState(false);
  const { setSession } = useAuth();
  const navigate = useNavigate();
  const location = useLocation();
  const notice = typeof (location.state as { notice?: unknown } | null)?.notice === "string"
    ? ((location.state as { notice?: string }).notice ?? "")
    : "";

  useEffect(() => {
    let active = true;
    listAuthProviders()
      .then((response) => {
        if (!active) {
          return;
        }
        const oidcProvider = response.providers.find((provider) => provider.id === "oidc");
        setSsoEnabled(Boolean(oidcProvider?.enabled));
      })
      .catch(() => {
        if (!active) {
          return;
        }
        setSsoEnabled(false);
      });
    return () => {
      active = false;
    };
  }, []);

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    setError("");
    setSubmitting(true);
    try {
      const session = await login(email, password);
      setSession(session);
      if (session.must_change_password) {
        navigate("/account", {
          replace: true,
          state: { notice: "First login requires password change." },
        });
      } else {
        navigate("/dashboard", { replace: true });
      }
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : "Login failed");
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div style={styles.page}>
      <div style={styles.card}>
        <LoginBrand />
        <p style={styles.subtitle}>Admin Login</p>
        {notice && <p style={styles.notice}>{notice}</p>}
        <form onSubmit={handleSubmit} style={styles.form}>
          <label style={styles.label}>
            Email
            <input
              type="email"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              required
              autoFocus
              style={styles.input}
              placeholder="admin@example.com"
            />
          </label>
          <label style={styles.label}>
            Password
            <input
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              autoComplete="current-password"
              name="password"
              required
              style={styles.passwordInput}
            />
          </label>
          {error && <p style={styles.error}>{error}</p>}
          <button type="submit" disabled={submitting} style={styles.button}>
            {submitting ? "Signing in..." : "Sign in"}
          </button>
          {ssoEnabled && (
            <button
              type="button"
              onClick={() => {
                window.location.assign("/api/v1/auth/oidc/start");
              }}
              style={styles.secondaryButton}
            >
              Sign in with SSO
            </button>
          )}
        </form>
      </div>
    </div>
  );
}

const styles: Record<string, React.CSSProperties> = {
  page: {
    minHeight: "100vh",
    display: "flex",
    alignItems: "center",
    justifyContent: "center",
    background: uiTheme.pageBg,
    fontFamily: "system-ui, sans-serif",
    padding: "1rem",
  },
  card: {
    background: uiTheme.surface,
    borderRadius: 8,
    padding: "2.2rem 2rem",
    width: "100%",
    maxWidth: 430,
    boxShadow: uiTheme.shadow,
  },
  subtitle: {
    margin: "0.2rem 0 1.5rem",
    textAlign: "center",
    color: uiTheme.textMuted,
    fontSize: "0.95rem",
  },
  form: {
    display: "flex",
    flexDirection: "column",
    gap: "1rem",
  },
  label: {
    display: "flex",
    flexDirection: "column",
    gap: "0.35rem",
    fontSize: "0.9rem",
    fontWeight: 500,
    color: uiTheme.text,
  },
  input: {
    padding: "0.55rem 0.75rem",
    border: `1px solid ${uiTheme.border}`,
    borderRadius: 5,
    fontSize: "1rem",
    outline: "none",
    background: uiTheme.inputBg,
    color: uiTheme.inputText,
  },
  passwordInput: {
    padding: "0.55rem 0.75rem",
    border: `1px solid ${uiTheme.border}`,
    borderRadius: 5,
    fontSize: "1rem",
    outline: "none",
    background: uiTheme.inputBg,
    color: uiTheme.inputText,
  },
  button: {
    marginTop: "0.5rem",
    padding: "0.65rem",
    background: uiTheme.headerBg,
    color: uiTheme.headerText,
    border: "none",
    borderRadius: 5,
    fontSize: "1rem",
    fontWeight: 600,
    cursor: "pointer",
  },
  secondaryButton: {
    marginTop: "0.25rem",
    padding: "0.65rem",
    background: "transparent",
    color: uiTheme.text,
    border: `1px solid ${uiTheme.border}`,
    borderRadius: 5,
    fontSize: "0.95rem",
    fontWeight: 600,
    cursor: "pointer",
  },
  error: {
    margin: 0,
    color: "#c0392b",
    fontSize: "0.875rem",
    background: "#fdecea",
    border: "1px solid #f5c6cb",
    borderRadius: 4,
    padding: "0.5rem 0.75rem",
  },
  notice: {
    margin: "0 0 0.75rem",
    color: "#1e8449",
    fontSize: "0.875rem",
    background: "#eafaf1",
    border: "1px solid #cdeed9",
    borderRadius: 4,
    padding: "0.5rem 0.75rem",
  },
};
